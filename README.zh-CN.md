# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | 简体中文

从抖音、TikTok 和 B 站下载视频，输入是一份平台无关的 JSON 时间线 —— 自包含、
单个静态二进制、只用标准库（`go.mod` 里没有任何 `require`）。

## 支持的平台

| 平台 | 内容 | 单条链接 | 时间线 |
|---|---|---|---|
| 抖音 | 创作者视频 | 不支持（需要浏览器签名） | 支持 |
| TikTok | 视频 | 支持（`get`） | 支持 |
| B 站 | 视频与分 P | 支持（`get`） | 支持（DASH + ffmpeg） |

## 特性

- **增量下载**：`.state.json` 账本以 `platform:id` 为键，已下载的条目自动跳过；
  `--force` 可强制重下。
- **无水印**：直接使用平台提供或当前会话签发的播放地址，不做去水印处理。
- **统一输入**：任何脚本、服务或手工文件都能生成时间线 JSON，不依赖特定上游或 SDK。
- **单二进制**：静态链接（`CGO_ENABLED=0`），无第三方 Go 依赖。
- **机器可读进度**：`--progress` 向 stderr 输出 `PROGRESS <0-100>` 行，便于调用方解析。

## 输入格式

顶层可以是 `{ "items": [...] }`、直接是条目数组 `[...]`，或用 `data` 作为
`items` 的别名。完整例子：

```json
{
  "platform": "douyin",
  "author": "作者名",
  "items": [
    {
      "video_id": "1234567890123456789",
      "title": "示例标题",
      "url": "https://example.com/video/123",
      "play_url": "https://example.com/video.mp4",
      "cover_url": "https://example.com/cover.jpg",
      "published_at": "2026-01-02T03:04:05Z",
      "author": "作者名"
    }
  ]
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `items` / `data` | 是 | 条目数组；顶层也可直接是数组 |
| `platform` / `source` | 否 | `douyin` / `tiktok` / `bilibili`；也可逐条写在 `item.platform` |
| `video_id` | 是 | 通用作品 ID；兼容别名 `aweme_id`（抖音/TikTok）、`id`（B 站 BV 号） |
| `title` / `desc` | 否 | 标题；两者都接受，输出文件名使用其中之一 |
| `url` | 否 | 视频页地址；B 站可只给 BV 号，也可给完整链接 |
| `play_url` | 否 | 直链；抖音必须提供，TikTok 会重新解析，B 站不需要 |
| `cover_url` / `images` | 否 | 封面；没有 `cover_url` 时取 `images[0]` |
| `published_at` / `create_time` | 否 | 发布时间；RFC3339 字符串或 Unix 秒 |
| `author` | 否 | 作者名，用于输出目录和展示 |

平台识别顺序：`--platform` > 条目 `platform` > 顶层 `platform`/`source` >
BV 号或 B 站视频页地址 > 时间线 URL > 媒体直链域名。都匹配不上时命令直接报错，
不会默认成某个平台。条目逐条携带 `platform` 时，支持混合平台时间线。

## 前置

- 构建需要 Go 1.27+。
- B 站下载需要系统里有 `ffmpeg`（DASH 音视频流要合并）。
- 可选 cookie 提升画质：TikTok 未登录可能只有 720p，登录后可到 1080p；
  B 站带 `SESSDATA` 可解锁 1080p+。

## 快速开始

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + 测试 + 语言检查
make install      # 安装到 ~/.local/bin/vidpuller
vidpuller version
```

## 用法

```bash
# 本地时间线
vidpuller download ./timeline.json -n 5

# 远程时间线（从 JSON 或 URL 自动识别平台）
vidpuller download 'https://example.com/timeline.json' -n 5

# 时间线没有平台提示时显式指定
vidpuller download ./timeline.json --platform douyin -n 5

# 先看看会下什么
vidpuller download '<timeline-url>' -n 10 --dry-run

# 只下指定作品
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# 只列条目
vidpuller list '<timeline-url>' -n 20

# 单条 TikTok（支持 vt.tiktok.com 短链）
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# B 站：带 cookie 批量下载
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# 单条 B 站（BV 号或完整链接，?p=N 指定分 P）
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'
```

## 参数

| 参数 | 说明 |
|---|---|
| `-o, --out DIR` | 输出根目录，默认 `./downloads` |
| `-n, --limit N` | 最多下载 N 条（已下载跳过的不计数；0=不限） |
| `--since YYYY-MM-DD` | 只下该日期之后发布的 |
| `--ids A,B` | 只下指定作品 ID（`video_id` / `aweme_id`） |
| `--dry-run` | 只打印，不下载 |
| `--force` | 已下载过也重新下载 |
| `--interval DUR` | 每条间隔，默认 `3s` |
| `--platform douyin\|tiktok\|bilibili` | 覆盖平台自动识别 |
| `--cookies FILE` | 登录 cookie（Netscape / JSON / `k=v; k=v`） |
| `--progress` | 向 stderr 输出 `PROGRESS <0-100>` 行 |

## 产物

```
downloads/
  douyin/<作者>/<video_id>_<标题>.mp4|.jpg|.json
  tiktok/<作者>/<video_id>_<标题>.mp4|.jpg|.json
  bilibili/<作者>/<video_id>_<标题>.mp4|.jpg|.json
  .state.json            增量账本（键 = platform:id）
```

`.json` 里是时间线的原始条目；`play_url` 有时效，仅作记录。

## 实现要点

- **抖音**：`play_url` 直接可下（带 `Referer: https://www.douyin.com/`）。
- **TikTok**：CDN 直链绑定签发它的会话（`ttwid`/`msToken`/`tt_chain_token`），
  批量下载时每条都会用当前会话重新解析视频页的直链；用别的 cookie jar 下载
  上游 URL 会 403。
- TikTok 的 WAF 是 SHA-256 工作量证明，纯标准库解（无需浏览器）。
- **画质选择**：解析每个档位的 `UrlKey`（形如 `…_bytevc1_1080p_1037801`）取真实
  codec 与分辨率；分辨率优先，同分辨率比码率。下载结果会打印实际档位
  （如 `1080p bytevc1 722kbps`）。
- **B 站**：`view` 取 cid，`playurl` 取 DASH 流，`ffmpeg -c copy` 合并；Akamai
  地址优先走镜像，失败时回退原始 CDN。

## 已知限制

- **抖音单条链接不支持**：详情接口需要浏览器签名（`a_bogus`），纯 HTTP 拿不到
  无水印直链；要下某一条，用 `download --ids <video_id>`。
- TikTok 图文帖没有 `play_url`，会被跳过（封面仍可从时间线获取）。
- 私密或地区限制内容由平台报错；时间线里没有的条目，本工具也看不到。
- 时间线的抓取、签名与登录态由调用方负责，本工具只消费 JSON。

## 文档

- **[English](README.md)**：英文原文，唯一权威来源。
- **[AGENTS.md](AGENTS.md)**：仓库约束，包括仅英文规则与合并策略。

## License

[MIT](LICENSE)
