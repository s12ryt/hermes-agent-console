package console

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func pythonExe(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("環境沒有 Python，跳過安裝器測試")
	return ""
}

// fakeTarball 建立假 Hermes tar.gz（頂層 hermes-agent-main/）。
func fakeTarball(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: "hermes-agent-main/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeRepo 啟動一個提供假 Hermes tarball 的 HTTP 伺服器。
func fakeRepo(t *testing.T, entries map[string]string) *httptest.Server {
	t.Helper()
	payload := fakeTarball(t, entries)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
}

func venvPython(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(dir, ".venv", "bin", "python")
}

func waitDone(t *testing.T, in *Installer, timeout time.Duration) InstallStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := in.Status()
		if !st.Running {
			return st
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("安裝未在期限內結束")
	return InstallStatus{}
}

func TestInstallerSuccessFromFakeRepo(t *testing.T) {
	py := pythonExe(t)
	srv := fakeRepo(t, map[string]string{
		"pyproject.toml":          "[project]\nname='fake-hermes'\nversion='0'\n",
		"tui_gateway/__init__.py": "",
	})
	defer srv.Close()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := NewInstaller(store)
	in.base = srv.URL
	target := filepath.Join(dir, "hermes")
	if err := in.Start(InstallOptions{Python: py, TargetDir: target, Ref: "main"}); err != nil {
		t.Fatal(err)
	}
	st := waitDone(t, in, 180*time.Second)
	if st.Error != "" {
		t.Fatalf("安裝失敗: %s\nlog: %v", st.Error, st.Log)
	}
	if _, err := os.Stat(filepath.Join(target, "tui_gateway", "__init__.py")); err != nil {
		t.Fatal("tarball 內容未解壓到目標目錄")
	}
	got, err := store.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != target {
		t.Fatalf("settings.source = %q, want %q", got.Source, target)
	}
	if got.Python != venvPython(target) {
		t.Fatalf("settings.python = %q, want %q", got.Python, venvPython(target))
	}
	steps := strings.Join(st.Steps, "\n")
	for _, want := range []string{"下載", "venv", "pip", "驗證"} {
		if !strings.Contains(steps, want) {
			t.Fatalf("安裝步驟缺少關鍵字 %q：\n%s", want, steps)
		}
	}
}

func TestInstallerRejectsNonEmptyTarget(t *testing.T) {
	py := pythonExe(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "hermes")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dir, "console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := NewInstaller(store)
	if err := in.Start(InstallOptions{Python: py, TargetDir: target}); err == nil {
		t.Fatal("非空目錄應被拒絕")
	}
	if in.Status().Running {
		t.Fatal("被拒絕後不應有安裝在執行")
	}
}

func TestInstallerDownloadFailure(t *testing.T) {
	py := pythonExe(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := NewInstaller(store)
	in.base = srv.URL
	if err := in.Start(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "hermes")}); err != nil {
		t.Fatal(err)
	}
	st := waitDone(t, in, 30*time.Second)
	if st.Error == "" || !strings.Contains(st.Error, "下載") {
		t.Fatalf("應出現下載錯誤，got %q", st.Error)
	}
	if got, _ := store.Settings(); got.Source == filepath.Join(dir, "hermes") {
		t.Fatal("失敗時不應更新設定")
	}
}

func TestInstallerBlocksTarPathTraversal(t *testing.T) {
	py := pythonExe(t)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../../evil.txt", Mode: 0o644, Size: 1}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	store, _ := OpenStore(filepath.Join(dir, "console.db"))
	defer store.Close()
	in := NewInstaller(store)
	in.base = srv.URL
	if err := in.Start(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "hermes")}); err != nil {
		t.Fatal(err)
	}
	st := waitDone(t, in, 30*time.Second)
	if st.Error == "" {
		t.Fatal("含路徑穿越的 tarball 應被拒絕")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("evil.txt 不應被解壓到目標之外")
	}
}

func TestInstallerBusyRejected(t *testing.T) {
	py := pythonExe(t)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)
	dir := t.TempDir()
	store, _ := OpenStore(filepath.Join(dir, "console.db"))
	defer store.Close()
	in := NewInstaller(store)
	in.base = srv.URL
	if err := in.Start(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "hermes")}); err != nil {
		t.Fatal(err)
	}
	if err := in.Start(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "hermes2")}); err == nil {
		t.Fatal("安裝進行中應拒絕第二次啟動")
	}
}

func TestInstallerRefValidated(t *testing.T) {
	py := pythonExe(t)
	dir := t.TempDir()
	store, _ := OpenStore(filepath.Join(dir, "console.db"))
	defer store.Close()
	in := NewInstaller(store)
	if err := in.Start(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "h"), Ref: "../../evil"}); err == nil {
		t.Fatal("非法 ref 應被拒絕")
	}
	if err := in.Start(InstallOptions{Python: py, TargetDir: "relative/path"}); err == nil {
		t.Fatal("相對目標目錄應被拒絕")
	}
}

// Bug A 回歸：run() 內非預期 panic 時，狀態必須收斂為 done 且不卡 Running，
// 否則面板的安裝功能會永久鎖死（之後每次 Start 都被當成忙碌）。
func TestInstallerRecoversFromPanic(t *testing.T) {
	py := pythonExe(t)
	srv := fakeRepo(t, map[string]string{"tui_gateway/__init__.py": ""})
	defer srv.Close()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := NewInstaller(store)
	in.base = srv.URL
	in.testHook = func() { panic("boom") }
	// 模擬 Start 已標記 running 後直接同步執行 run，觸發 panic 路徑
	in.snapshot(func(st *InstallStatus) { st.Running = true })
	in.run(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "hermes"), Ref: "main"})
	st := in.Status()
	if st.Running {
		t.Fatal("panic 後 Running 必須為 false，否則安裝器會永久忙碌")
	}
	if !st.Done {
		t.Fatal("panic 後 Done 必須為 true")
	}
	if !strings.Contains(st.Error, "內部錯誤") {
		t.Fatalf("Error 應包含「內部錯誤」，got %q", st.Error)
	}
	// 崩潰後不能永久鎖死：下一次 Start 必須能再次進入
	if err := in.Start(InstallOptions{Python: py, TargetDir: filepath.Join(dir, "hermes3")}); err != nil {
		t.Fatalf("panic 後應可再次啟動安裝: %v", err)
	}
}

// Bug B 回歸：ref 填標籤（refs/heads 404）時應 fallback 到 refs/tags。
func TestInstallerFallsBackToTagRef(t *testing.T) {
	py := pythonExe(t)
	payload := fakeTarball(t, map[string]string{
		"pyproject.toml":          "[project]\nname='fake-hermes'\nversion='0'\n",
		"tui_gateway/__init__.py": "",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/refs/tags/v1.5":
			w.Write(payload)
		default: // 含 /refs/heads/v1.5
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := NewInstaller(store)
	in.base = srv.URL
	target := filepath.Join(dir, "hermes")
	if err := in.Start(InstallOptions{Python: py, TargetDir: target, Ref: "v1.5"}); err != nil {
		t.Fatal(err)
	}
	st := waitDone(t, in, 180*time.Second)
	if st.Error != "" {
		t.Fatalf("標籤 ref 安裝失敗: %s\nlog: %v", st.Error, st.Log)
	}
	got, err := store.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != target {
		t.Fatalf("settings.source = %q, want %q", got.Source, target)
	}
}
