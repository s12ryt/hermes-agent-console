package console

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUpdateCheck(t *testing.T) {
	app := testServer(t)
	cookie := login(t, app)
	original := http.DefaultTransport
	http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/repos/s12ryt/hermes-agent-console/releases/latest" {
			t.Errorf("unexpected destination %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v0.2.0","html_url":"https://evil.invalid"}`)), Header: make(http.Header)}, nil
	})
	defer func() { http.DefaultTransport = original }()
	if w := request(app, "GET", "/api/updates", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := request(app, "GET", "/api/updates", "", cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "v0.2.0") || strings.Contains(w.Body.String(), "evil.invalid") {
		t.Fatalf("update %d %s", w.Code, w.Body)
	}
}
