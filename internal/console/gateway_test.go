package console

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Real child process, upstream line-delimited protocol, no model or network.
func TestHermesFixture(t *testing.T) {
	if os.Getenv("CONSOLE_RPC_FIXTURE") != "1" {
		return
	}
	emit := func(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }
	emit(map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "gateway.ready", "payload": map[string]any{}}})
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var v map[string]json.RawMessage
		json.Unmarshal(scanner.Bytes(), &v)
		var method string
		json.Unmarshal(v["method"], &method)
		if method == "" {
			emit(map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "fixture.response", "payload": v}})
			continue
		}
		result := any(map[string]any{"ok": true})
		switch method {
		case "client.capabilities":
			var p map[string]any
			json.Unmarshal(v["params"], &p)
			if p["server_requests"] != true {
				os.Exit(2)
			}
		case "session.list":
			result = map[string]any{"sessions": []any{map[string]any{"id": "stored-1", "title": "Fixture"}}}
		case "usage.ping":
			emit(map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "message.complete", "session_id": "live-1", "payload": map[string]any{"text": "done"}}})
		case "session.usage":
			result = map[string]any{"calls": 1, "input": 120, "output": 80, "total": 200}
		case "prompt.submit":
			emit(map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "message.delta", "session_id": "live-1", "payload": map[string]any{"text": "hello"}}})
			emit(map[string]any{"jsonrpc": "2.0", "id": "srq-1", "method": "approval", "params": map[string]any{"session_id": "live-1", "command": "ls"}})
			emit(map[string]any{"jsonrpc": "2.0", "id": "srq-unsupported", "method": "terminal.read", "params": map[string]any{}})
		case "hang":
			continue
		case "crash":
			os.Exit(3)
		case "error":
			emit(map[string]any{"jsonrpc": "2.0", "id": v["id"], "error": map[string]any{"code": 4002, "message": "bad model"}})
			continue
		}
		emit(map[string]any{"jsonrpc": "2.0", "id": v["id"], "result": result})
	}
	os.Exit(0)
}
func fixtureGateway(t *testing.T) *Gateway {
	t.Helper()
	g := NewGateway()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHermesFixture$")
	cmd.Env = append(os.Environ(), "CONSOLE_RPC_FIXTURE=1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.startCommand(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Stop)
	return g
}
func TestGatewayRPCRequestsAndEvents(t *testing.T) {
	g := fixtureGateway(t)
	if !g.Status().Connected {
		t.Fatal("not connected after ready handshake")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := g.Call(ctx, "session.list", map[string]any{})
	if err != nil || !strings.Contains(string(result), "stored-1") {
		t.Fatalf("list %s %v", result, err)
	}
	ch, _, unsub := g.Subscribe()
	defer unsub()
	if _, err = g.Call(ctx, "prompt.submit", map[string]any{"session_id": "live-1", "text": "hello"}); err != nil {
		t.Fatal(err)
	}
	seenDelta, seenApproval, seenUnsupported := false, false, false
	for !seenDelta || !seenApproval || !seenUnsupported {
		select {
		case e := <-ch:
			b, _ := json.Marshal(e)
			text := string(b)
			seenDelta = seenDelta || strings.Contains(text, "message.delta")
			seenApproval = seenApproval || strings.Contains(text, `"method":"approval"`)
			seenUnsupported = seenUnsupported || (strings.Contains(text, "srq-unsupported") && strings.Contains(text, "-32601"))
		case <-ctx.Done():
			t.Fatalf("missing events delta=%v approval=%v unsupported=%v", seenDelta, seenApproval, seenUnsupported)
		}
	}
	if err = g.Respond("srq-1", json.RawMessage(`{"choice":"always"}`)); err == nil {
		t.Fatal("accepted unsupported approval escalation")
	}
	if err = g.Respond("srq-1", json.RawMessage(`{"choice":"once"}`)); err != nil {
		t.Fatal(err)
	}
	if err = g.Respond("srq-1", json.RawMessage(`{"choice":"once"}`)); err == nil {
		t.Fatal("duplicate response accepted")
	}
	if _, err = g.Call(ctx, "error", nil); err == nil || !strings.Contains(err.Error(), "bad model") {
		t.Fatalf("missing RPC error %v", err)
	}
	g.Stop()
	if g.Status().Connected {
		t.Fatal("connected after stop")
	}
}
func TestGatewayTimeoutAndExit(t *testing.T) {
	g := fixtureGateway(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := g.Call(ctx, "hang", nil); err == nil {
		t.Fatal("missing timeout")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	if _, err := g.Call(ctx2, "crash", nil); err == nil {
		t.Fatal("missing process exit")
	}
	if g.Status().Connected {
		t.Fatal("connected after crash")
	}
}
