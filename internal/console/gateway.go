package console

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Status struct {
	Connected bool   `json:"connected"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}
type Frame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("Hermes (%d): %s", e.Code, e.Message) }

type reply struct {
	data json.RawMessage
	err  error
}
type process struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	writeMu sync.Mutex
	done    chan struct{}
	ready   chan struct{}
	once    sync.Once
	pending map[string]chan reply
	open    map[string]Frame
	stderr  tailBuffer
}
type tailBuffer struct {
	mu   sync.Mutex
	text string
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text += string(p)
	if len(b.text) > 4096 {
		b.text = b.text[len(b.text)-4096:]
	}
	return len(p), nil
}
func (b *tailBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.text }

type Gateway struct {
	operation   sync.Mutex
	mu          sync.Mutex
	p           *process
	status      Status
	next        uint64
	subscribers map[chan any]bool
}

func NewGateway() *Gateway {
	return &Gateway{status: Status{State: "stopped"}, subscribers: map[chan any]bool{}}
}
func (g *Gateway) publishLocked(v any) {
	for ch := range g.subscribers {
		select {
		case ch <- v:
		default:
			close(ch)
			delete(g.subscribers, ch)
		}
	}
}
func (g *Gateway) setStatusLocked(v Status) {
	g.status = v
	g.publishLocked(map[string]any{"method": "console.status", "params": v})
}
func (g *Gateway) Status() Status { g.mu.Lock(); defer g.mu.Unlock(); return g.status }
func (g *Gateway) Start(ctx context.Context, v Settings) error {
	if v.Source == "" || v.Python == "" {
		return errors.New("請先設定 Hermes Python 與原始碼目錄")
	}
	for _, path := range []string{v.Source, v.Workspace, v.Home} {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("目錄不存在：%s", path)
		}
	}
	cmd := exec.Command(v.Python, "-u", "-m", "tui_gateway.entry")
	cmd.Dir = v.Source
	cmd.Env = os.Environ()
	if v.Home != "" {
		env := []string{}
		for _, e := range cmd.Env {
			if !strings.HasPrefix(e, "HERMES_HOME=") {
				env = append(env, e)
			}
		}
		cmd.Env = append(env, "HERMES_HOME="+v.Home)
	}
	return g.startCommand(ctx, cmd)
}
func (g *Gateway) startCommand(ctx context.Context, cmd *exec.Cmd) error {
	g.operation.Lock()
	defer g.operation.Unlock()
	g.stopLocked()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	p := &process{cmd: cmd, done: make(chan struct{}), ready: make(chan struct{}), pending: map[string]chan reply{}, open: map[string]Frame{}}
	configureProcess(cmd)
	var err error
	p.in, err = cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		p.in.Close()
		return err
	}
	cmd.Stderr = &p.stderr
	g.mu.Lock()
	g.p = p
	g.setStatusLocked(Status{State: "starting"})
	g.mu.Unlock()
	if err = cmd.Start(); err != nil {
		p.in.Close()
		out.Close()
		g.mu.Lock()
		g.p = nil
		g.setStatusLocked(Status{State: "error", Error: err.Error()})
		g.mu.Unlock()
		return err
	}
	go g.read(p, out)
	select {
	case <-p.ready:
	case <-p.done:
		return errors.New(g.Status().Error)
	case <-ctx.Done():
		g.stopLocked()
		return fmt.Errorf("Hermes 啟動逾時：%w", ctx.Err())
	}
	if _, err = g.Call(ctx, "client.capabilities", map[string]bool{"server_requests": true}); err != nil {
		g.stopLocked()
		return err
	}
	g.mu.Lock()
	if g.p != p {
		g.mu.Unlock()
		return errors.New("Hermes 已離線")
	}
	g.setStatusLocked(Status{Connected: true, State: "ready"})
	g.mu.Unlock()
	return nil
}
func (p *process) write(frame Frame) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	frame.JSONRPC = "2.0"
	return json.NewEncoder(p.in).Encode(frame)
}
func (g *Gateway) read(p *process, out io.ReadCloser) {
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 65536), 8<<20)
	for scanner.Scan() {
		var f Frame
		if err := json.Unmarshal(scanner.Bytes(), &f); err != nil {
			continue
		}
		g.handle(p, f)
	}
	scanErr := scanner.Err()
	out.Close()
	if scanErr != nil {
		killProcess(p.cmd)
	}
	err := p.cmd.Wait()
	p.in.Close()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.p == p {
		g.p = nil
		message := "Hermes 已結束"
		if err != nil {
			message = err.Error()
		}
		if scanErr != nil {
			message = scanErr.Error()
		}
		if detail := strings.TrimSpace(p.stderr.String()); detail != "" {
			message += "\n" + detail
		}
		g.setStatusLocked(Status{State: "error", Error: message})
	}
	for key, ch := range p.pending {
		ch <- reply{err: errors.New("Hermes 程序已結束")}
		delete(p.pending, key)
	}
	p.open = map[string]Frame{}
	close(p.done)
}
func (g *Gateway) handle(p *process, f Frame) {
	g.mu.Lock()
	if g.p != p {
		g.mu.Unlock()
		return
	}
	if f.Method == "" {
		if ch, ok := p.pending[string(f.ID)]; ok {
			delete(p.pending, string(f.ID))
			if f.Error != nil {
				ch <- reply{err: f.Error}
			} else {
				ch <- reply{data: f.Result}
			}
		}
		g.mu.Unlock()
		return
	}
	if f.Method == "event" {
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				ID string `json:"id"`
			} `json:"payload"`
		}
		json.Unmarshal(f.Params, &event)
		if event.Type == "gateway.ready" {
			p.once.Do(func() { close(p.ready) })
		}
		if event.Type == "request.cancel" {
			delete(p.open, event.Payload.ID)
		}
		g.publishLocked(f)
		g.mu.Unlock()
		return
	}
	if len(f.ID) > 0 {
		var id string
		if json.Unmarshal(f.ID, &id) != nil {
			g.mu.Unlock()
			p.write(Frame{ID: f.ID, Error: &RPCError{-32600, "request id must be a string"}})
			return
		}
		if f.Method == "approval" || f.Method == "clarify" {
			p.open[id] = f
			g.publishLocked(f)
			g.mu.Unlock()
			return
		}
		g.mu.Unlock()
		p.write(Frame{ID: f.ID, Error: &RPCError{-32601, "此控制台不支援 " + f.Method}})
		return
	}
	g.publishLocked(f)
	g.mu.Unlock()
}
func (g *Gateway) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	p := g.p
	if p == nil {
		g.mu.Unlock()
		return nil, errors.New("Hermes 尚未連線")
	}
	g.next++
	key := strconv.FormatUint(g.next, 10)
	ch := make(chan reply, 1)
	p.pending[key] = ch
	g.mu.Unlock()
	defer func() { g.mu.Lock(); delete(p.pending, key); g.mu.Unlock() }()
	if err = p.write(Frame{ID: json.RawMessage(key), Method: method, Params: raw}); err != nil {
		return nil, err
	}
	select {
	case result := <-ch:
		return result.data, result.err
	case <-ctx.Done():
		return nil, fmt.Errorf("RPC %s：%w", method, ctx.Err())
	case <-p.done:
		return nil, errors.New("Hermes 程序已結束")
	}
}
func (g *Gateway) Respond(id string, result json.RawMessage) error {
	var v map[string]json.RawMessage
	if err := json.Unmarshal(result, &v); err != nil || v == nil {
		return errors.New("回應必須是 JSON 物件")
	}
	g.mu.Lock()
	p := g.p
	if p == nil {
		g.mu.Unlock()
		return errors.New("Hermes 尚未連線")
	}
	f, ok := p.open[id]
	if !ok {
		g.mu.Unlock()
		return errors.New("請求已結束或不存在")
	}
	if f.Method == "approval" {
		var choice string
		json.Unmarshal(v["choice"], &choice)
		if choice != "once" && choice != "deny" {
			g.mu.Unlock()
			return errors.New("僅允許單次核准或拒絕")
		}
	}
	if f.Method == "clarify" {
		if answers, exists := v["answers"]; exists {
			var answer map[string]any
			if json.Unmarshal(answers, &answer) != nil || answer == nil {
				g.mu.Unlock()
				return errors.New("answers 必須是物件")
			}
		}
	}
	delete(p.open, id)
	g.publishLocked(map[string]any{"method": "console.request.resolved", "params": map[string]string{"id": id}})
	g.mu.Unlock()
	return p.write(Frame{ID: f.ID, Result: result})
}
func (g *Gateway) Stop() { g.operation.Lock(); defer g.operation.Unlock(); g.stopLocked() }
func (g *Gateway) stopLocked() {
	g.mu.Lock()
	p := g.p
	g.mu.Unlock()
	if p != nil {
		killProcess(p.cmd)
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
		}
	}
	g.mu.Lock()
	if g.p == p {
		g.p = nil
	}
	g.setStatusLocked(Status{State: "stopped"})
	g.mu.Unlock()
}
func (g *Gateway) Subscribe() (<-chan any, any, func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch := make(chan any, 128)
	g.subscribers[ch] = true
	requests := []Frame{}
	if g.p != nil {
		for _, f := range g.p.open {
			requests = append(requests, f)
		}
	}
	snapshot := map[string]any{"status": g.status, "requests": requests}
	return ch, snapshot, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.subscribers[ch] {
			delete(g.subscribers, ch)
			close(ch)
		}
	}
}
