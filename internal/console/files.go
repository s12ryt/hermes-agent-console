package console

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.Settings()
	if err != nil || settings.Workspace == "" {
		fail(w, 400, "請先設定工作目錄")
		return
	}
	name := r.URL.Query().Get("path")
	if name == "" {
		name = "."
	}
	if filepath.IsAbs(name) || !filepath.IsLocal(name) || strings.ContainsAny(name, "\\:\x00") {
		fail(w, 400, "路徑必須位於工作目錄內")
		return
	}
	root, err := os.OpenRoot(settings.Workspace)
	if err != nil {
		fail(w, 400, "無法開啟工作目錄")
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		fail(w, 400, "路徑不存在或超出工作目錄")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		fail(w, 400, "無法讀取檔案資訊")
		return
	}
	if info.IsDir() {
		entries, err := file.ReadDir(501)
		if err != nil && err != io.EOF {
			fail(w, 400, "無法列出目錄")
			return
		}
		type entry struct {
			Name    string `json:"name"`
			Dir     bool   `json:"dir"`
			Symlink bool   `json:"symlink"`
		}
		out := []entry{}
		truncated := len(entries) > 500
		if truncated {
			entries = entries[:500]
		}
		for _, e := range entries {
			out = append(out, entry{e.Name(), e.IsDir(), e.Type()&os.ModeSymlink != 0})
		}
		writeJSON(w, 200, map[string]any{"path": name, "entries": out, "truncated": truncated})
		return
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<10 {
		fail(w, 400, "只支援 256 KiB 內的一般文字檔")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, (256<<10)+1))
	if err != nil || len(data) > 256<<10 || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		fail(w, 400, "無法預覽此文字檔")
		return
	}
	writeJSON(w, 200, map[string]any{"path": name, "content": string(data)})
}
