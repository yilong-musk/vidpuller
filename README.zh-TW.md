# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | 繁體中文 | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português](README.pt.md) | [Русский](README.ru.md)

從抖音、TikTok、B 站和 YouTube 下載影片，輸入是一份平台無關的 JSON 時間線 ——
自包含、單個靜態二進位、只用標準庫（`go.mod` 裡沒有任何 `require`）。YouTube
解析為內建實作，預設上限 1080p，可用 `--max-height` 提到 1440p 或 2160p。

## 支援的平台

<table>
  <thead>
    <tr><th width="136">平台</th><th>內容</th><th>單條連結</th><th>時間線</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="抖音">&nbsp;抖音</td><td>創作者影片</td><td>不支援（需要瀏覽器簽章）</td><td>支援</td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>影片</td><td>支援（<code>get</code>）</td><td>支援</td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="嗶哩嗶哩">&nbsp;B 站</td><td>影片與分 P</td><td>支援（<code>get</code>）</td><td>支援（DASH + ffmpeg）</td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>影片</td><td>支援（<code>get</code>）</td><td>支援（內建 + ffmpeg）</td></tr>
  </tbody>
</table>

## 特性

- **增量下載**：`.state.json` 帳本以 `platform:id` 為鍵，已下載的條目自動跳過；`--force` 可強制重下。
- **無浮水印**：直接使用平台提供或目前工作階段簽發的播放位址，不做去浮水印處理。
- **統一輸入**：任何指令稿、服務或手工檔案都能產生時間線 JSON，不依賴特定上游或 SDK。
- **不下載也能取元資訊**：`vidpuller info <url>` 輸出平台無關的 JSON（標題、作者、ID、時長、解析度、發佈時間、封面、分 P），支援 B 站、YouTube 和 TikTok。
- **單一二進位**：靜態連結（`CGO_ENABLED=0`），無第三方 Go 依賴。
- **機器可讀進度**：`--progress` 向 stderr 輸出 `PROGRESS <0-100>` 行，便於呼叫方解析。

## 輸入格式

頂層可以是 `{ "items": [...] }`、直接是條目陣列 `[...]`，或用 `data` 作為 `items` 的別名。完整例子：

```json
{
  "platform": "douyin",
  "author": "creator",
  "items": [
    {
      "video_id": "1234567890123456789",
      "title": "Example title",
      "url": "https://example.com/video/123",
      "play_url": "https://example.com/video.mp4",
      "cover_url": "https://example.com/cover.jpg",
      "published_at": "2026-01-02T03:04:05Z",
      "author": "creator"
    }
  ]
}
```

| 欄位 | 必填 | 說明 |
|---|---|---|
| `items` / `data` | 是 | 條目陣列；頂層也可直接是陣列 |
| `platform` / `source` | 否 | `douyin`、`tiktok`、`bilibili` 或 `youtube`；也可逐條寫在條目上 |
| `video_id` | 是 | 規範作品 ID；別名：`aweme_id`（抖音/TikTok）、`id`（B 站 BV 號） |
| `title` / `desc` | 否 | 標題；兩者都接受，並用於輸出檔名 |
| `url` | 否 | 影片頁位址；B 站可只給 BV 號，也可給完整連結 |
| `play_url` | 否 | 媒體直連；抖音必須提供，TikTok 會重新解析，B 站不使用 |
| `cover_url` / `images` | 否 | 封面；沒有 `cover_url` 時取 `images[0]` |
| `published_at` / `create_time` | 否 | 發佈時間；RFC 3339 字串或 Unix 秒 |
| `author` | 否 | 作者名，用於輸出目錄和展示 |

平台識別順序：`--platform` > 條目 `platform` > 頂層 `platform`/`source` >
BV 號或 B 站影片頁位址 > YouTube 位址 > 時間線 URL > 媒體直連主機。都匹配不上時
命令直接失敗，不會猜一個平台。條目逐條攜帶 `platform` 時，支援混合平台時間線。

## 安裝

Linux、macOS、Windows（amd64/arm64）的靜態二進位隨每個
[release](https://github.com/yilong-musk/vidpuller/releases) 發布。下載對應平台
的壓縮檔，用 `checksums.txt` 校驗後把 `vidpuller` 放進 `PATH`。

macOS 或 Linux 上裝了 [Homebrew](https://brew.sh) 時：

```bash
brew install --cask yilong-musk/tap/vidpuller
```

已有 Go 工具鏈（1.27+）時：

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## 前置

- 建置需要 Go 1.27+。
- B 站和 YouTube 下載需要 `PATH` 上有 `ffmpeg`（DASH 音訊與視訊要合併）。
- 可選 cookie 提升畫質：TikTok 未登入可能只有 720p，登入後可到 1080p；B 站帶 `SESSDATA` 可解鎖 1080p+。YouTube 的 `--cookies` 由內建解析器使用。

## 快速開始

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + 測試 + 語言檢查
make install      # 安裝到 ~/.local/bin/vidpuller
vidpuller version
```

## 用法

```bash
# 本地時間線
vidpuller download ./timeline.json -n 5

# 遠端時間線（從 JSON 或 URL 自動識別平台）
vidpuller download 'https://example.com/timeline.json' -n 5

# 時間線沒有平台提示時顯式指定
vidpuller download ./timeline.json --platform douyin -n 5

# 下載前先看會下什麼
vidpuller download '<timeline-url>' -n 10 --dry-run

# 只下指定作品
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# 只列條目
vidpuller list '<timeline-url>' -n 20

# 只取元資訊（JSON，不下載）
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# 單條 TikTok（支援 vt.tiktok.com 短鏈）
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# B 站：帶 cookie
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# 單條 B 站（BV 號或完整連結；?p=N 指定分 P）
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# 單條 YouTube（內建解析器，最高 1080p）
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## 參數

| 參數 | 說明 |
|---|---|
| `-o, --out DIR` | 輸出根目錄，預設 `./downloads` |
| `-n, --limit N` | 最多下載 N 條（已下載而跳過的不計數；0 = 不限） |
| `--since YYYY-MM-DD` | 只下該日期當天或之後發佈的 |
| `--ids A,B` | 只下指定作品 ID（`video_id` / `aweme_id`） |
| `--dry-run` | 印出將會下載的內容 |
| `--force` | 已記錄的條目也重新下載 |
| `--interval DUR` | 每條間隔，預設 `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | 覆蓋平台自動識別 |
| `--max-height N` | YouTube 最高高度（預設 1080；例如 1440、2160） |
| `--cookies FILE` | 登入 cookie（Netscape / JSON / `k=v; k=v`） |
| `--progress` | 向 stderr 輸出 `PROGRESS <0-100>` 行 |

## 產物

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            增量帳本（鍵 = platform:id）
```

`.json` 是時間線的原始條目；`play_url` 會過期，只作記錄。

## 實作要點

- **抖音**：`play_url` 直接下載，並帶 `Referer: https://www.douyin.com/`。
- **TikTok**：CDN 位址綁定簽發它的工作階段（`ttwid`/`msToken`/`tt_chain_token`），批次下載時每條都會用目前工作階段重新解析影片位址；用別的 cookie jar 去抓上游 URL 會 403。
- TikTok 的 WAF 是用標準庫解的 SHA-256 工作量證明，不需要瀏覽器。
- **畫質選擇**：每個檔位的 `UrlKey`（例如 `…_bytevc1_1080p_1037801`）提供真實 codec 與解析度；解析度優先，同解析度再比位元率。下載結果會印出實際檔位（例如 `1080p bytevc1 722kbps`）。
- **B 站**：`view` 提供 cid，`playurl` 提供 DASH 串流，`ffmpeg -c copy` 合併。Akamai 位址先試鏡像主機，失敗再回退原始 CDN。
- **YouTube**：內建 InnerTube 解析器，用 VISIONOS 用戶端請求播放器回應，該用戶端給出不帶簽章加密的自適應串流直連。優先選 `--max-height` 以內最高的視訊（AVC/MP4 優先），配上最佳 MP4 音訊，再用 `ffmpeg -c copy` 合併。

## 已知限制

- **抖音單條連結不支援**：詳情介面需要瀏覽器簽章（`a_bogus`），純 HTTP 拿不到無浮水印串流或元資訊。要下某一條，用 `download --ids <video_id>`，或從時間線 JSON 讀取該條目。
- TikTok 圖文帖沒有 `play_url`，會被跳過（封面仍可從時間線取得）。
- **YouTube** 預設 1080p；更高畫質用 `--max-height 1440` 或 `--max-height 2160`（檔案可能有幾百 MB）。年齡限制、DRM 或地區限制的影片可能需要 cookie，也可能無法下載。不展開播放清單或頻道時間線，只接受單條影片位址。
- 私密或地區限制內容由平台報錯；時間線資料裡沒有的，本工具也看不到。
- 時間線的抓取、簽章與登入態由呼叫方負責；本工具只消費 JSON。

## 文件

英文版是唯一權威來源，本頁是它的繁體中文翻譯。

- **[English](README.md)**：英文原文，唯一權威來源。
- **[AGENTS.md](AGENTS.md)**：倉庫約束，包括僅英文規則與合併策略。

## License

[MIT](LICENSE)
