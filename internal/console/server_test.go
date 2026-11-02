package console

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestEventStreamRevokedBeforeNextFrame(t *testing.T) {
	app := testServer(t)
	cookie := login(t, app)
	host := httptest.NewServer(app)
	defer host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", host.URL+"/api/events", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scan := bufio.NewScanner(res.Body)
	for scan.Scan() {
		if scan.Text() == "" {
			break
		}
	}
	request(app, "POST", "/api/logout", `{}`, cookie, "")
	app.gateway.Stop()
	for scan.Scan() {
		if scan.Text() == "event: frame" {
			t.Fatal("revoked cookie received a new frame")
		}
		if scan.Text() == "event: expired" {
			return
		}
	}
	t.Fatal("stream did not signal expiration")
}

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewServer(s, "correct-password-123", false, fstest.MapFS{"index.html": {Data: []byte("<html>console</html>")}})
	t.Cleanup(func() { app.Close(); s.Close() })
	return app
}
func request(app http.Handler, method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://console.test"+path, strings.NewReader(body))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, app http.Handler) *http.Cookie {
	t.Helper()
	w := request(app, "POST", "/api/login", `{"password":"correct-password-123"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body)
	}
	return w.Result().Cookies()[0]
}
func TestAuthAndOrigin(t *testing.T) {
	app := testServer(t)
	for _, path := range []string{"/api/status", "/api/settings", "/api/logs", "/api/events"} {
		if w := request(app, "GET", path, "", nil, ""); w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if w := request(app, "POST", "/api/login", `{"password":"wrong"}`, nil, ""); w.Code != 401 {
		t.Fatalf("wrong password: %d", w.Code)
	}
	cookie := login(t, app)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	if w := request(app, "GET", "/api/status", "", cookie, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(app, "POST", "/api/restart", `{}`, cookie, "https://evil.test"); w.Code != 403 {
		t.Fatalf("cross origin: %d", w.Code)
	}
	if w := request(app, "POST", "/api/logout", `{}`, cookie, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(app, "GET", "/api/status", "", cookie, ""); w.Code != 401 {
		t.Fatal("logout did not revoke token")
	}
}
func TestAPIErrorsAndSettings(t *testing.T) {
	app := testServer(t)
	cookie := login(t, app)
	v := Settings{Python: "python", Source: t.TempDir()}
	raw, _ := json.Marshal(v)
	if w := request(app, "PUT", "/api/settings", string(raw), cookie, ""); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w := request(app, "GET", "/api/settings", "", cookie, "")
	var got Settings
	json.Unmarshal(w.Body.Bytes(), &got)
	if got != v {
		t.Fatalf("settings %+v", got)
	}
	for _, tc := range []struct {
		path, body string
		code       int
	}{{"/api/rpc", `{"method":"shell.exec"}`, 400}, {"/api/rpc", `{"method":"session.list","params":{}}`, 503}, {"/api/respond", `{"id":"missing","result":{}}`, 409}, {"/api/settings", `{`, 400}} {
		method := "POST"
		if tc.path == "/api/settings" {
			method = "PUT"
		}
		if w := request(app, method, tc.path, tc.body, cookie, ""); w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
	if w := request(app, "GET", "/api/missing", "", cookie, ""); w.Code != 404 {
		t.Fatal("API fell back to SPA")
	}
	w = request(app, "GET", "/chat/123", "", nil, "")
	body, _ := io.ReadAll(w.Result().Body)
	if w.Code != 200 || !strings.Contains(string(body), "console") {
		t.Fatal("SPA fallback failed")
	}
}
func TestPromptAPIDeliversToHermes(t *testing.T) {
	app := testServer(t)
	app.gateway = fixtureGateway(t)
	cookie := login(t, app)
	w := request(app, "POST", "/api/rpc", `{"method":"prompt.submit","params":{"session_id":"live-1","text":"hello"}}`, cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("prompt %d %s", w.Code, w.Body)
	}
}

func TestJSONBodyRequiresJSONContentType(t *testing.T) {
	app := testServer(t)
	// httptest.NewRequest 對 strings.Reader 會自動給 text/x-web-string，模擬非 JSON Content-Type
	r := httptest.NewRequest("POST", "http://console.test/api/login", strings.NewReader(`{"password":"correct-password-123"}`))
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatalf("non-JSON content-type accepted: %d %s", w.Code, w.Body)
	}
	// 空 body 的 POST（例如 CLI 呼叫）不受 Content-Type 限制
	r2 := httptest.NewRequest("POST", "http://console.test/api/logout", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, r2)
	if w2.Code == 415 {
		t.Fatal("empty body must not require a content-type")
	}
}

func TestEventStreamConnectionLimit(t *testing.T) {
	app := testServer(t)
	cookie := login(t, app)
	host := httptest.NewServer(app)
	defer host.Close()
	var bodies []io.ReadCloser
	defer func() {
		for _, b := range bodies {
			b.Close()
		}
	}()
	for i := 0; i < maxEventStreams; i++ {
		req, _ := http.NewRequest("GET", host.URL+"/api/events", nil)
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			t.Fatalf("stream %d: %d", i, res.StatusCode)
		}
		bodies = append(bodies, res.Body)
	}
	req, _ := http.NewRequest("GET", host.URL+"/api/events", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 503 {
		t.Fatalf("stream beyond limit: %d, want 503", res.StatusCode)
	}
}

func TestSecureResponseHeaders(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "secure.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewServer(store, "correct-password-123", true, fstest.MapFS{"index.html": {Data: []byte("<html>console</html>")}})
	t.Cleanup(func() { app.Close(); store.Close() })
	w := request(app, "GET", "/", "", nil, "")
	if got := w.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("HSTS missing: %q", got)
	}
	if got := w.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
		t.Fatalf("COOP missing: %q", got)
	}
	if got := w.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
		t.Fatalf("CORP missing: %q", got)
	}
	plain := testServer(t)
	if got := request(plain, "GET", "/", "", nil, "").Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS must require secure mode: %q", got)
	}
}

func TestStaticCacheControl(t *testing.T) {
	app := testServer(t)
	if got := request(app, "GET", "/", "", nil, "").Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index cache-control: %q", got)
	}
}

func TestTokensStoredAsHashes(t *testing.T) {
	app := testServer(t)
	cookie := login(t, app)
	app.mu.Lock()
	defer app.mu.Unlock()
	if _, ok := app.tokens[cookie.Value]; ok {
		t.Fatal("session token stored in plaintext")
	}
	sum := sha256.Sum256([]byte(cookie.Value))
	if _, ok := app.tokens[hex.EncodeToString(sum[:])]; !ok {
		t.Fatal("hashed session token missing")
	}
}

func TestCSPWithoutUnsafeInline(t *testing.T) {
	app := testServer(t)
	if csp := request(app, "GET", "/", "", nil, "").Header().Get("Content-Security-Policy"); strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("CSP still allows unsafe-inline: %s", csp)
	}
}
