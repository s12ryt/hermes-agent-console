package console

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type attempt struct {
	Count int
	Until time.Time
}

// maxEventStreams 限制單一程序可同時存在的 SSE 連線數，防止已登入用戶耗盡資源。
const maxEventStreams = 64

type Server struct {
	store     *Store
	gateway   *Gateway
	installer *Installer
	password  [32]byte
	secure    bool
	assets    fs.FS
	mu        sync.Mutex
	tokens    map[string]time.Time
	attempts  map[string]attempt
	streams   int
}

func NewServer(store *Store, password string, secure bool, assets fs.FS) *Server {
	return &Server{store: store, gateway: NewGateway(), installer: NewInstaller(store), password: sha256.Sum256([]byte(password)), secure: secure, assets: assets, tokens: map[string]time.Time{}, attempts: map[string]attempt{}}
}
func (s *Server) Close() { s.gateway.Stop() }
func (s *Server) AutoStart(ctx context.Context) error {
	v, err := s.store.Settings()
	if err != nil {
		return err
	}
	if v.Source == "" {
		return nil
	}
	return s.gateway.Start(ctx, v)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err any) {
	writeJSON(w, status, map[string]any{"error": fmt.Sprint(err)})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "無效的 JSON 請求")
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		fail(w, 400, "請求只能包含一個 JSON 物件")
		return false
	}
	return true
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (s *Server) authenticated(r *http.Request) bool {
	c, err := r.Cookie("console_session")
	if err != nil {
		return false
	}
	key := hashToken(c.Value)
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.tokens[key]
	if !ok || time.Now().After(until) {
		delete(s.tokens, key)
		return false
	}
	return true
}
func (s *Server) cookie(w http.ResponseWriter, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "console_session", Value: value, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if s.secure {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		s.static(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" && r.Method != "HEAD" {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			scheme := "http"
			if r.TLS != nil || s.secure {
				scheme = "https"
			}
			if err != nil || u.Host != r.Host || u.Scheme != scheme || u.Path != "" {
				fail(w, 403, "拒絕跨來源請求")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "拒絕跨網站請求")
			return
		}
		// 有 body 的寫入請求必須宣告 application/json，表單無法偽造此 Content-Type，作為 CSRF 縱深防禦。
		if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				fail(w, 415, "Content-Type 必須是 application/json")
				return
			}
		}
	}
	if r.URL.Path == "/api/login" && r.Method == "POST" {
		s.login(w, r)
		return
	}
	if !s.authenticated(r) {
		fail(w, 401, "請先登入")
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/me":
		writeJSON(w, 200, map[string]bool{"authenticated": true})
	case "POST /api/logout":
		c, _ := r.Cookie("console_session")
		s.mu.Lock()
		delete(s.tokens, hashToken(c.Value))
		s.mu.Unlock()
		s.cookie(w, "", -1)
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "GET /api/status":
		writeJSON(w, 200, s.gateway.Status())
	case "GET /api/settings":
		v, err := s.store.Settings()
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case "PUT /api/settings":
		var v Settings
		if !decode(w, r, &v) {
			return
		}
		if err := s.store.SaveSettings(v); err != nil {
			fail(w, 400, err)
			return
		}
		s.store.AddLog("settings", "更新 Hermes 連線設定")
		writeJSON(w, 200, v)
	case "POST /api/restart":
		v, err := s.store.Settings()
		if err == nil {
			err = s.gateway.Start(r.Context(), v)
		}
		if err != nil {
			fail(w, 503, err)
			return
		}
		s.store.AddLog("gateway", "Hermes 已連線")
		writeJSON(w, 200, s.gateway.Status())
	case "POST /api/stop":
		s.gateway.Stop()
		s.store.AddLog("gateway", "Hermes 已停止")
		writeJSON(w, 200, s.gateway.Status())
	case "GET /api/logs":
		v, err := s.store.Logs()
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case "GET /api/install/status":
		writeJSON(w, 200, s.installer.Status())
	case "POST /api/install/start":
		var opt InstallOptions
		if !decode(w, r, &opt) {
			return
		}
		if err := s.installer.Start(opt); err != nil {
			fail(w, 400, err)
			return
		}
		s.store.AddLog("install", "開始安裝 Hermes 到 "+opt.TargetDir)
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "POST /api/rpc":
		s.rpc(w, r)
	case "POST /api/respond":
		var v struct {
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if !decode(w, r, &v) {
			return
		}
		if err := s.gateway.Respond(v.ID, v.Result); err != nil {
			fail(w, 409, err)
			return
		}
		s.store.AddLog("response", "已回應互動請求")
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "GET /api/events":
		s.events(w, r)
	default:
		fail(w, 404, "API 不存在或方法不符")
	}
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	s.mu.Lock()
	for k, a := range s.attempts {
		if now.After(a.Until) {
			delete(s.attempts, k)
		}
	}
	a := s.attempts[host]
	if a.Count >= 10 && now.Before(a.Until) {
		s.mu.Unlock()
		fail(w, 429, "登入嘗試過多，請於 15 分鐘後重試")
		return
	}
	s.mu.Unlock()
	var v struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	hash := sha256.Sum256([]byte(v.Password))
	if subtle.ConstantTimeCompare(hash[:], s.password[:]) != 1 {
		s.mu.Lock()
		if len(s.attempts) >= 4096 {
			s.mu.Unlock()
			fail(w, 429, "登入嘗試過多")
			return
		}
		a = s.attempts[host]
		a.Count++
		a.Until = now.Add(15 * time.Minute)
		s.attempts[host] = a
		s.mu.Unlock()
		fail(w, 401, "密碼不正確")
		return
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		fail(w, 500, "無法建立登入階段")
		return
	}
	token := hex.EncodeToString(bytes[:])
	s.mu.Lock()
	delete(s.attempts, host)
	for k, until := range s.tokens {
		if now.After(until) {
			delete(s.tokens, k)
		}
	}
	if len(s.tokens) >= 128 {
		for k := range s.tokens {
			delete(s.tokens, k)
			break
		}
	}
	// 只儲存 token 的雜湊，記憶體傾印也無法直接重放工作階段。
	s.tokens[hashToken(token)] = now.Add(12 * time.Hour)
	s.mu.Unlock()
	s.cookie(w, token, 43200)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

var allowedMethods = map[string]bool{"session.create": true, "session.list": true, "session.resume": true, "session.activate": true, "session.history": true, "session.interrupt": true, "session.title": true, "session.usage": true, "session.status": true, "session.active_list": true, "session.events.since": true, "model.options": true, "config.get": true, "config.set": true, "command.dispatch": true, "prompt.submit": true}

func (s *Server) rpc(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if !decode(w, r, &v) {
		return
	}
	if !allowedMethods[v.Method] {
		fail(w, 400, "不支援的 RPC 方法")
		return
	}
	if v.Params == nil {
		v.Params = map[string]any{}
	}
	if v.Method == "command.dispatch" {
		cmd, _ := v.Params["command"].(string)
		if !strings.HasPrefix(cmd, "/model ") {
			fail(w, 400, "僅允許 /model 指令")
			return
		}
	}
	if v.Method == "session.create" {
		if _, ok := v.Params["cwd"]; !ok {
			cfg, err := s.store.Settings()
			if err != nil {
				fail(w, 500, err)
				return
			}
			if cfg.Workspace != "" {
				v.Params["cwd"] = cfg.Workspace
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	result, err := s.gateway.Call(ctx, v.Method, v.Params)
	if err != nil {
		code := 502
		if !s.gateway.Status().Connected {
			code = 503
		}
		fail(w, code, err)
		return
	}
	s.store.AddLog("rpc", v.Method)
	writeJSON(w, 200, json.RawMessage(result))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.streams >= maxEventStreams {
		s.mu.Unlock()
		fail(w, 503, "串流連線過多，請關閉其他分頁後重試")
		return
	}
	s.streams++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.streams--
		s.mu.Unlock()
	}()
	f, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "串流不可用")
		return
	}
	ch, snapshot, unsubscribe := s.gateway.Subscribe()
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(kind string, data any) bool {
		raw, _ := json.Marshal(data)
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, raw)
		f.Flush()
		return err == nil
	}
	if !send("snapshot", snapshot) {
		return
	}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			if !s.authenticated(r) {
				send("expired", map[string]bool{"expired": true})
				return
			}
			if !send("frame", event) {
				return
			}
		case <-tick.C:
			if !s.authenticated(r) {
				send("expired", map[string]bool{"expired": true})
				return
			}
			if !send("heartbeat", nil) {
				return
			}
		}
	}
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	// 永遠重新驗證：部署新版本後，瀏覽器不會繼續使用舊的 SPA 搭配新 API。
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if _, err := fs.Stat(s.assets, name); err != nil {
		if strings.Contains(name, ".") {
			http.NotFound(w, r)
			return
		}
		r = r.Clone(r.Context())
		r.URL.Path = "/"
	}
	http.FileServer(http.FS(s.assets)).ServeHTTP(w, r)
}
