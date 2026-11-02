package console

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// InstallOptions 是「在面板中安裝 Hermes」的請求參數。
type InstallOptions struct {
	Python    string `json:"python"`
	TargetDir string `json:"targetDir"`
	Ref       string `json:"ref"`
}

// InstallStatus 是安裝進度快照，供前端輪詢顯示。
type InstallStatus struct {
	Running bool     `json:"running"`
	Done    bool     `json:"done"`
	Error   string   `json:"error,omitempty"`
	Steps   []string `json:"steps"`
	Log     []string `json:"log"`
	Python  string   `json:"python,omitempty"`
	Source  string   `json:"source,omitempty"`
}

const (
	installMaxDownload = 1 << 29 // 512 MiB
	installMaxLogLines = 500
)

var installRefPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Installer 負責從官方儲存庫下載 Hermes、建立 venv、安裝依賴並驗證。
// 下載來源固定為 NousResearch/hermes-agent，不接受任意 URL，避免 SSRF。
type Installer struct {
	store *Store
	base  string

	mu sync.Mutex
	st InstallStatus
}

// NewInstaller 建立安裝器；正式環境下載來源為 GitHub codeload。
func NewInstaller(store *Store) *Installer {
	return &Installer{store: store, base: "https://codeload.github.com/NousResearch/hermes-agent/tar.gz"}
}

func (in *Installer) snapshot(f func(st *InstallStatus)) {
	in.mu.Lock()
	defer in.mu.Unlock()
	f(&in.st)
}

func (in *Installer) log(line string) {
	in.snapshot(func(st *InstallStatus) {
		st.Log = append(st.Log, line)
		if len(st.Log) > installMaxLogLines {
			st.Log = st.Log[len(st.Log)-installMaxLogLines:]
		}
	})
}

// Start 驗證參數後在背景執行安裝；同一時間僅允許一個安裝。
func (in *Installer) Start(opt InstallOptions) error {
	if strings.TrimSpace(opt.Python) == "" {
		return errors.New("請填入用來建立虛擬環境的 Python")
	}
	if !filepath.IsAbs(opt.TargetDir) {
		return errors.New("安裝目錄必須是絕對路徑")
	}
	if opt.Ref == "" {
		opt.Ref = "main"
	}
	if !installRefPattern.MatchString(opt.Ref) {
		return errors.New("分支或標籤名稱不合法")
	}
	if entries, err := os.ReadDir(opt.TargetDir); err == nil && len(entries) > 0 {
		return errors.New("安裝目錄已存在且非空，請換一個新目錄")
	}
	errBusy := func() bool {
		busy := false
		in.snapshot(func(st *InstallStatus) {
			if st.Running {
				busy = true
				return
			}
			st.Running, st.Done, st.Error = true, false, ""
			st.Steps, st.Log = []string{}, []string{}
			st.Python, st.Source = "", ""
		})
		return busy
	}()
	if errBusy {
		return errors.New("已有安裝正在進行")
	}
	go in.run(opt)
	return nil
}

// Status 回傳目前的安裝進度快照。
func (in *Installer) Status() InstallStatus {
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.st
	st.Steps = append([]string(nil), in.st.Steps...)
	st.Log = append([]string(nil), in.st.Log...)
	return st
}

func (in *Installer) step(name string) {
	in.snapshot(func(st *InstallStatus) { st.Steps = append(st.Steps, name) })
	in.store.AddLog("install", name)
}

func (in *Installer) fail(err error) {
	in.snapshot(func(st *InstallStatus) { st.Error = err.Error() })
	in.store.AddLog("install", "安裝失敗: "+err.Error())
}

func venvPythonPath(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(dir, ".venv", "bin", "python")
}

// runCmd 執行外部指令，將 stdout/stderr 即時附加到安裝日誌。
func (in *Installer) runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(pipe)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		in.log(sc.Text())
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s 失敗: %w", filepath.Base(name), err)
	}
	return nil
}

func (in *Installer) run(opt InstallOptions) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	finish := func() {
		in.snapshot(func(st *InstallStatus) {
			st.Running = false
			st.Done = true
		})
	}
	// 1. 下載
	in.step("下載 Hermes 原始碼（" + opt.Ref + "）")
	url := in.base + "/refs/heads/" + opt.Ref
	tmp, err := in.download(ctx, url)
	if err != nil {
		in.fail(fmt.Errorf("下載失敗: %w", err))
		finish()
		return
	}
	defer os.Remove(tmp)
	in.log("下載完成：" + url)

	// 2. 解壓
	in.step("解壓到 " + opt.TargetDir)
	if err := in.extract(tmp, opt.TargetDir); err != nil {
		in.fail(err)
		finish()
		return
	}

	// 3. venv
	in.step("建立虛擬環境（.venv）")
	if err := in.runCmd(ctx, opt.Python, "-m", "venv", filepath.Join(opt.TargetDir, ".venv")); err != nil {
		in.fail(err)
		finish()
		return
	}

	// 4. pip 安裝依賴
	in.step("pip 安裝 Hermes 依賴")
	venvPy := venvPythonPath(opt.TargetDir)
	if err := in.runCmd(ctx, venvPy, "-m", "pip", "install", "--no-input", "--upgrade", "pip"); err != nil {
		in.fail(err)
		finish()
		return
	}
	if err := in.runCmd(ctx, venvPy, "-m", "pip", "install", "--no-input", opt.TargetDir); err != nil {
		in.fail(err)
		finish()
		return
	}

	// 5. 驗證 tui_gateway 可被載入
	in.step("驗證 tui_gateway 模組")
	if err := in.runCmd(ctx, venvPy, "-c", "import importlib.util,sys; sys.exit(0 if importlib.util.find_spec('tui_gateway') else 1)"); err != nil {
		in.fail(errors.New("找不到 tui_gateway 模組，請檢查安裝日誌"))
		finish()
		return
	}

	// 6. 寫回連線設定
	in.step("更新連線設定")
	cur, err := in.store.Settings()
	if err != nil {
		in.fail(err)
		finish()
		return
	}
	cur.Python, cur.Source = venvPy, opt.TargetDir
	if err := in.store.SaveSettings(cur); err != nil {
		in.fail(err)
		finish()
		return
	}
	in.snapshot(func(st *InstallStatus) {
		st.Python, st.Source = venvPy, opt.TargetDir
	})
	in.store.AddLog("install", "Hermes 安裝完成: "+opt.TargetDir)
	in.log("安裝完成。已將 Python 與原始碼目錄填入連線設定。")
	finish()
}

func (in *Installer) download(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "hermes-console-install-*.tar.gz")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, installMaxDownload)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// extract 解開 tar.gz，剝除頂層目錄並拒絕路徑穿越。
func (in *Installer) extract(archive, target string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Clean(hdr.Name))
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue // 頂層目錄項
		}
		rel := filepath.FromSlash(parts[1])
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("壓縮檔包含不安全路徑: %s", hdr.Name)
		}
		dest := filepath.Join(target, rel)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		default:
			// 忽略 symlink 等其他類型，避免安全風險
		}
	}
}
