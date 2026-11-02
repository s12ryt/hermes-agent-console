# Hermes Agent Console

[![CI](https://github.com/s12ryt/hermes-agent-console/actions/workflows/ci.yml/badge.svg)](https://github.com/s12ryt/hermes-agent-console/actions/workflows/ci.yml)
[![Release](https://github.com/s12ryt/hermes-agent-console/actions/workflows/release.yml/badge.svg)](https://github.com/s12ryt/hermes-agent-console/actions/workflows/release.yml)

以 Go 提供 API 與程序管理、React + TypeScript 提供前端的個人 Hermes 管理控制台。正式建置將前端嵌入單一執行檔，部署時不需要 Node.js，也不需要另外架設前端服務。

目前支援 NousResearch 的 [hermes-agent](https://github.com/NousResearch/hermes-agent)：新增與恢復對話、Markdown 串流回覆、工具事件、單次核准／拒絕、澄清問題、中止、模型切換、偏好設定、工作目錄與操作紀錄。介面支援桌面與手機。

## 對話、安全與維運工具

| 工具 | 行為與範圍 |
| --- | --- |
| 對話管理 | 重新命名、關閉執行階段、歸檔、確認後永久刪除；執行中的對話需先關閉才能刪除。依安裝的 Hermes RPC 版本提供能力，舊版不支援時會顯示錯誤。 |
| 分支與壓縮 | 複製整段對話，或在可見使用者／助手訊息按「從這裡分支」複製到該訊息；另提供上下文壓縮。 |
| 用量與診斷 | 執行紀錄頁呈現各對話 token 快照與去重累計、最近 64 KiB gateway stderr。無供應商帳單或費用估算。 |
| 自動核准 | 設定完整命令精確比對；不接受 shell 運算子、換行或命令替換。規則命中時只允許該次請求。只加入自己信任的命令與參數。 |
| API Key | 支援 OpenRouter、OpenAI、Anthropic、Gemini；以 PBKDF2-SHA256（600,000 次、每筆隨機 salt）與 AES-GCM 加密，列表不回傳明文，重新連線後透過對應環境變數交給 Hermes。變更管理密碼後需重新設定金鑰。 |
| 更新檢查 | 手動查詢本面板 GitHub 最新正式 Release，提供下載頁連結；不自動替換執行檔或更新 Hermes。 |
| 設定備份 | 下載與還原面板設定、核准規則及加密金鑰；還原要求同一管理密碼，並需停止 Hermes 與安裝工作。備份不含 Hermes 對話、workspace、操作紀錄與用量，完整災難復原仍需另備份 data 與 Hermes Home。 |
| 工作目錄瀏覽 | 唯讀列目錄及預覽 UTF-8 文字；以 Go Root 限制存取工作目錄內，每目錄最多 500 項、檔案最大 256 KiB。 |
| 桌面通知 | 按「啟用桌面通知」取得瀏覽器授權；面板仍開啟且分頁在背景時，提示回覆完成、待核准或待澄清。不含訊息內容；關閉頁面後不提供 Web Push。 |
| 匯出 | 下載當前已載入訊息的 Markdown 或 JSON；不會額外讀取未載入歷史。 |
| 內容搜尋 | 面板自己的 SQLite FTS5 trigram 索引；讀取歷史及回合完成時更新，支援短中文詞，最多回傳 50 個結果。尚未載入的舊對話不會自動全庫建立索引；索引含對話文字，須與 data 一併保護。 |

新增 API 均沿用登入、同源與 JSON 請求檢查。診斷 stderr、匯出檔與設定備份可能包含私人資料，請妥善保管。

## 啟動

Linux amd64 產物位於 `bin/hermes-console-linux-amd64`。在伺服器上：

```sh
chmod +x ./hermes-console-linux-amd64
read -rsp '管理密碼（至少 12 字元）: ' CONSOLE_PASSWORD; echo
export CONSOLE_PASSWORD
./hermes-console-linux-amd64 -addr 127.0.0.1:8080 -data ./data
```

瀏覽 `http://127.0.0.1:8080`，以管理密碼登入。遠端自用可透過 SSH 通道：`ssh -L 8080:127.0.0.1:8080 user@server`。第一次開啟時先前往「連線設定」。

**Hermes 本身與 Python 執行環境仍須安裝在伺服器上。** 請先依 Hermes 官方說明安裝依賴、完成模型供應商登入或 API key 設定，確認該環境可以執行 `python -m tui_gateway.entry`。本專案不內含 Hermes、模型或 API key。

**還沒安裝 Hermes？可以先裝面板，之後在面板內安裝 agent。** 到「連線設定」頁的「還沒有安裝 Hermes？」區塊，填入伺服器上可用的 Python（例如 `python3`）、新的安裝目錄（例如 `/opt/hermes-agent`，必須是空的絕對路徑）與分支（預設 `main`），按「下載並安裝」。面板會從官方儲存庫下載原始碼、建立 `.venv` 虛擬環境、`pip` 安裝依賴並驗證 `tui_gateway` 模組，完成後自動把 venv Python 與原始碼目錄填入連線設定，之後按「啟動／重新連線」即可。安裝日誌即時顯示在頁面；若依賴需要額外系統套件导致失敗，可依日誌在伺服器補齊後換新目錄重試。模型供應商登入與 API key 仍依 Hermes 官方流程設定。

填入以下設定並儲存，再按「啟動／重新連線」：

| 欄位 | 範例 | 用途 |
| --- | --- | --- |
| Python 執行檔 | `/opt/hermes-agent/.venv/bin/python` | 安裝 Hermes 依賴的虛擬環境 |
| Hermes 原始碼目錄 | `/opt/hermes-agent` | 必須含 `tui_gateway` 模組 |
| 預設工作目錄 | `/srv/workspace` | 新對話的 cwd；留空使用 Hermes 預設 |
| Hermes Home | `/srv/hermes-home` | 選填，需為既有目錄且已完成 Hermes 設定 |

目錄必須是伺服器的絕對路徑。Go 以指定原始碼目錄啟動 `python -u -m tui_gateway.entry`，繼承服務的環境變數。Home 留空時繼承 `HERMES_HOME` 與 Hermes 預設行為。同一 Home 請避免同時啟動其他 gateway，Hermes 會檢查程序佔用。重新連線會停止目前 Hermes 程序及執行工作。設定儲存於控制台 SQLite；對話由 Hermes 自己管理，不直接修改 Hermes 的資料庫。重啟控制台後會自動嘗試連線。

模型選單可查詢官方回傳的選項，也可輸入模型 ID 與 Provider。涉及額外費用確認的模型切換，需先在 Hermes CLI 處理。不支援的互動請求（例如終端畫面擷取、sudo 密碼）會明確回傳不支援；工具核准僅提供單次允許或拒絕。

## Docker

映像檔發佈於 `ghcr.io/s12ryt/hermes-agent-console`（linux/amd64、linux/arm64），tag `main` 為主分支最新、`vX.Y.Z` 為版本發佈：

```sh
docker run --rm -p 8080:8080 \
  -e CONSOLE_PASSWORD='至少12字元的管理密碼' \
  -v hermes-console-data:/data \
  -v /opt/hermes-agent:/hermes \
  -v /srv/hermes-home:/hermes-home \
  ghcr.io/s12ryt/hermes-agent-console:latest
```

映像檔內含控制台執行檔與 Python 直譯器；Hermes 原始碼與虛擬環境由掛載提供，在連線設定中將 Python 執行檔設為 `/hermes/.venv/bin/python`、原始碼目錄設為 `/hermes`。容器監聽 `0.0.0.0:8080`，務必以埠映射或反向代理控制存取範圍，對外服務時加上 TLS。`/data` 保存控制台設定與操作紀錄。

## HTTPS 反向代理

對外服務時使用 HTTPS，啟動加上 `-secure-cookie`，讓 cookie 只經 HTTPS 傳送。反向代理需保留原始 Host 並關閉 SSE buffering，例如 Nginx：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $http_host;
    proxy_buffering off;
    proxy_read_timeout 1h;
}
```

此設定置於已有 TLS 憑證的 HTTPS server 區塊。不要在一般 HTTP 連線啟用 Secure cookie。登入使用 HttpOnly、SameSite=Strict cookie，12 小時到期，登出撤銷；session token 只以 SHA-256 雜湊儲存於記憶體，程序重啟即全部失效。API 寫入有同源檢查，且帶 body 的寫入請求必須宣告 `Content-Type: application/json`（HTML 表單無法偽造，作為 CSRF 縱深防禦）；每個來源 IP 密碼錯誤 10 次後限制 15 分鐘。SSE 事件連線上限 64 條，超過會回 503。`-secure-cookie` 模式會一併送出 HSTS 標頭；所有回應附 COOP／CORP `same-origin` 隔離與不含 inline script／style 的 CSP。靜態檔一律 `Cache-Control: no-cache`，部署新版本後瀏覽器不會沿用舊介面。代理後端看到的是代理 IP，因此限制會由同一代理的使用者共用。請使用專用低權限帳號執行，限制其可存取的工作目錄；Agent 的工具權限等同該帳號。

可使用 systemd 啟動並在環境檔設定 `CONSOLE_PASSWORD` 與供應商必要變數：

```ini
[Unit]
Description=Hermes Agent Console
After=network-online.target

[Service]
User=hermes
WorkingDirectory=/opt/hermes-console
EnvironmentFile=/etc/hermes-console.env
ExecStart=/opt/hermes-console/hermes-console-linux-amd64 -data /var/lib/hermes-console -secure-cookie
Restart=on-failure
UMask=0077

[Install]
WantedBy=multi-user.target
```

環境檔設為 `0600`；資料目錄需讓服務帳號可寫。不要將密碼或 API key 放入原始碼。控制台保留最近 1,000 筆操作紀錄，不將 prompt 或 key 寫入操作紀錄；對話與工具輸出仍可能含敏感資訊。備份請同時考慮控制台 data 與 Hermes Home。

## 開發與建置

需要 Go 1.26.10+（`GOTOOLCHAIN` 預設 auto 會自動下載）、Node.js 22+ 與 npm。前後端獨立開發：

```sh
cd web
npm ci
npm run build
cd ..
go test ./...
export CONSOLE_PASSWORD='自行設定至少12字元的密碼'
go run ./cmd/console
```

另一個終端執行 `cd web && npm run dev`，Vite 將 `/api` 代理至 `127.0.0.1:8080`。Go 的 embed 需要先有 `web/dist`，所以首次執行 Go 測試／開發前先建置前端。

正式 Linux 建置：

```sh
sh scripts/build.sh
# 可選 GOOS / GOARCH，例如 GOARCH=arm64 sh scripts/build.sh
```

Windows PowerShell：`./scripts/build.ps1 -TargetOS linux -Arch amd64`。建置輸出到 `bin/`，使用 `CGO_ENABLED=0` 與純 Go SQLite；不同平台需各自建置，並非同一個檔案跨平台執行。

```sh
cd web && npm test && npm run build
cd ..
go test ./... -count=1 -cover
go vet ./...
# 有支援的 C 編譯器時才可執行 race detector
CGO_ENABLED=1 go test -race ./internal/console
```

## 結構與協定

`cmd/console` 為啟動入口；`internal/console` 包含登入、SQLite、HTTP/SSE 與 Hermes stdio 適配；`web/src` 為前端；`web/embed.go` 將建置資產嵌入 Go。

瀏覽器 → Go HTTP RPC / SSE → Hermes JSON-RPC stdio。Go 等待 `gateway.ready`，協商 `client.capabilities`，以 request ID 對應並行 RPC、轉送工具事件與互動請求。SSE 斷線會重新連線並重新讀取目前對話與待核准請求。慢速讀取者會斷線重連；事件不是永久訊息佇列。控制台登入為單管理員模式，程序重啟需重新登入。

接入依據為調研時的 Hermes 主線 [programmatic integration](https://hermes-agent.nousresearch.com/docs/developer-guide/programmatic-integration) 及 `tui_gateway` 原始碼。此介面仍隨上游演進，升級 Hermes 後應再次驗證契約。模型選項以原始 JSON 顯示，澄清答案採問題 ID 對應答案的 JSON 物件。

## 驗證範圍

自動測試以真正子程序 fixture 驗證 stdio 握手、RPC、事件、核准、不支援請求、逾時與程序退出；前端測試驗證登入、恢復、送訊、串流、核准、儲存及重試。`scripts/fixture` 為瀏覽器整合用的 Python 協定 fixture，可將原始碼目錄設為該目錄、Python 設為本機 Python 後啟動。**它不是真正 Hermes，也不呼叫模型、不執行系統命令，僅供測試。**

目前環境未安裝真正 Hermes 或提供模型憑證，因此真實 Hermes／LLM 端到端呼叫尚未驗證。Linux 產物已交叉編譯，尚未在 Linux 主機實跑。Windows 環境無 gcc，race detector 未執行成功。CI 會在每次推送執行完整測試、govulncheck 與 npm audit；Docker 映像檔由 CI 建置。
