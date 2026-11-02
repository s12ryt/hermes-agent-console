package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceFilesConfined(t *testing.T) {
	app := testServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("workspace content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.store.SaveSettings(Settings{Python: "python", Source: root, Workspace: root}); err != nil {
		t.Fatal(err)
	}
	cookie := login(t, app)
	for _, path := range []string{"/api/files?path=../escape", "/api/files?path=C:%5CWindows", "/api/files?path=%2Fetc%2Fpasswd"} {
		if w := request(app, "GET", path, "", cookie, ""); w.Code != 400 {
			t.Fatalf("escape %s: %d", path, w.Code)
		}
	}
	if w := request(app, "GET", "/api/files", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(app, "GET", "/api/files", "", cookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "hello.txt") {
		t.Fatalf("list %d %s", w.Code, w.Body)
	}
	if w := request(app, "GET", "/api/files?path=hello.txt", "", cookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "workspace content") {
		t.Fatalf("read %d %s", w.Code, w.Body)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		if w := request(app, "GET", "/api/files?path=link", "", cookie, ""); w.Code != 400 {
			t.Fatal("symlink escaped")
		}
	}
}
