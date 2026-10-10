# Ghosts 👻

找出背景裡被遺忘的 dev server 和 Docker 容器，看它們吃了多少 CPU、記憶體和電，並直接關掉。

AI 工具（Claude Code、Codex…）常在背景開 port 測試、起 Docker 服務，結束後卻沒關。Ghosts 讓你一眼看到這些「幽靈」。

## 安裝

**macOS**（13 以上，Apple Silicon 和 Intel 都支援）：

```sh
brew install xamjiang/tap/ghosts
```

**Linux**（amd64 / arm64）：目前先從原始碼安裝，需要 Go：

```sh
go install github.com/xamjiang/ghosts/cmd/ghosts@latest
```

安裝後在終端機輸入：

```sh
ghosts
```

## 參數

| 參數 | 說明 |
|---|---|
| `-interval 2s` | port 掃描間隔（Docker 透過事件串流即時更新，不受影響） |
| `-no-mouse` | 不接管滑鼠，保留終端機原生的文字選取 |
| `-list [-all]` | 只印出一次清單後結束，方便在 script 中使用 |

## 快捷鍵

| 鍵 | 動作 |
|---|---|
| `↑↓` / `j k` | 移動 |
| `⇥` / `1 2` | 切換 Ports / Containers |
| `⏎` | 顯示或隱藏詳細資訊 |
| `x` / `X` | 停止 / 強制停止（程序、容器或整個 compose 專案） |
| `o` | 用瀏覽器開啟 |
| `/` | 過濾（port、名稱、專案、image） |
| `a` | 顯示 App 與系統服務 |
| `s` | 排序：port / 耗電 / 記憶體 / 最新 / 名稱 |
| `?` | 說明 |

## 運作方式

以下是 macOS 的做法，Linux 的差異列在下一節。

- **Port**：不呼叫 `lsof`，而是直接透過 `sysctl` 和 libproc 讀取 kernel 的資料：一次 sysctl 取得整個程序表，再列舉你自己程序的 socket。一次掃描約 2ms，不會 fork 任何外部程式。程序的指令和工作目錄依 PID 快取。
- **資源使用**：用 `proc_pid_rusage` 讀取每個程序累計的 CPU 時間、記憶體（和活動監視器的「記憶體」欄一致）和能耗。能耗計數器只有 Apple Silicon 才有，Intel Mac 上會隱藏 POWER 欄。每個服務的數值包含它的子程序，速率取最近約 6 秒的平均。
- **容器資源**：用 Docker 的 one-shot stats 取得 CPU 和記憶體，每個容器只要幾 ms。容器的耗電無法單獨計算，所以改為顯示整個 Docker VM 的耗電和記憶體。
- **Docker**：直接連 Docker Engine 的 unix socket（支援 Docker Desktop、OrbStack、Colima），訂閱事件串流即時更新。由 Docker 對外開的 port 會對應回實際的容器，停止時執行的是 `docker stop`，不會誤殺 Docker Desktop。
- **停止程序**：先送 SIGTERM，3 秒內沒結束才會詢問是否 SIGKILL；送出前會比對程序啟動時間，避免 PID 被重複使用時誤殺其他程序。

### Linux 上的差異

- **Port**：用 netlink（sock_diag，和 `ss` 相同的機制）只向 kernel 要 LISTEN 狀態的 socket，再從 `/proc/<pid>/fd` 對應回程序。對應結果會快取，只有出現新的 socket 時才重新查，或每 30 秒更新一次。程序表、CPU 和記憶體都來自 `/proc/<pid>/stat`，一次掃描約 4ms，一樣不 fork 任何外部程式。
- **資源使用**：記憶體是 RSS（和 `top` 的 RES 一致）。Linux 沒有每個程序的能耗計數器，也不提供每個程序的 wakeup 次數，所以會隱藏 POWER 欄和 Wakeups。
- **被遺忘的程序**：關掉終端機或 SSH 後留下的 server，會被 PID 1 或使用者的 `systemd --user` 收養。Ghosts 會看它的 cgroup：如果還留在原本 session 的 `*.scope`，就標記為「detached」；由 systemd 正常啟動的服務（`*.service`）則不會被誤判。
- **Docker**：會依照 `docker context` 目前選的 context 決定要連哪個 socket，所以 Docker Engine 和 Docker Desktop 並存時，看到的會和 `docker ps` 一樣。Docker Engine 的容器直接跑在主機上，沒有 VM，這時摘要列會改成所有容器的 CPU 和記憶體加總。
- **開啟瀏覽器**：用 `xdg-open`。透過 SSH 連線時，改用 OSC 52 把網址複製到你本機的剪貼簿（需要終端機支援，例如 iTerm2、Ghostty、kitty）。

## 自身資源佔用

在 M 系列 Mac 上，以預設每 2 秒掃描一次：CPU 約佔一顆核心的 1%、耗電約 3mW、記憶體約 15MB。在 Linux（i7-8700、約 400 個程序）上：CPU 約 0.9%、記憶體約 19MB。終端機視窗失去焦點時，掃描頻率會降到三分之一。想再更省的話，可以用 `-interval 5s`。

## 開發

```sh
go build -o bin/ghosts ./cmd/ghosts && ./bin/ghosts
go test ./...
```

macOS 版用到 libproc，需要 cgo（安裝 Xcode Command Line Tools 即可）。Linux 版不需要 cgo，可以在 Mac 上直接交叉編譯：

```sh
GOOS=linux GOARCH=amd64 go build -o bin/ghosts-linux ./cmd/ghosts
```

平台相關的程式碼用檔名區分：`internal/ports/*_darwin.go`、`*_linux.go`，以及 `internal/ui/platform_*.go`。

## 發佈新版本

推一個 tag 就會自動完成發佈：

```sh
git tag v0.1.0 && git push origin v0.1.0
```

`.github/workflows/release.yml` 會依序做這些事：

1. 跑測試。
2. 編譯 universal binary。
3. 建立 GitHub Release。
4. 用 `packaging/ghosts.rb.tmpl` 產生 formula，推到 [xamjiang/homebrew-tap](https://github.com/xamjiang/homebrew-tap)。

第一次發佈前要先在 ghosts repo 設定 `HOMEBREW_TAP_TOKEN` secret：一個只對 `homebrew-tap` 有 Contents 讀寫權限的 fine-grained token。
