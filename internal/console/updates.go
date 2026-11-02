package console

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"time"
)

const Version = "0.1.0"

func (s *Server) updates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/s12ryt/hermes-agent-console/releases/latest", nil)
	if err != nil {
		fail(w, 500, "無法建立版本查詢")
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "hermes-console/"+Version)
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		fail(w, 502, "無法連線至版本來源")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		fail(w, 502, "版本來源暫時無法提供資訊")
		return
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&release) != nil || !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(release.Tag) {
		fail(w, 502, "版本資訊格式不符")
		return
	}
	writeJSON(w, 200, map[string]any{"current": Version, "latest": release.Tag, "different": release.Tag != "v"+Version, "url": "https://github.com/s12ryt/hermes-agent-console/releases/tag/" + release.Tag})
}
