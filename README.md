# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[German](README.de.md) | English | [Spanish](README.es.md) | [French](README.fr.md) | [Japanese](README.ja.md) | [Korean](README.ko.md) | [Portuguese](README.pt.md) | [Russian](README.ru.md) | [Chinese (Simplified)](README.zh-CN.md) | [Chinese (Traditional)](README.zh-TW.md)

Downloads videos from Douyin, TikTok, Bilibili and YouTube from a
platform-neutral JSON timeline — self-contained, one static binary, standard
library only (there is not a single `require` in `go.mod`). YouTube extraction
is built in; the default ceiling is 1080p and `--max-height` raises it to
1440p or 2160p.

## Supported platforms

| Platform&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Content | Single URL | Timeline |
|---|---|---|---|
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Creator videos | No (needs a browser signature) | Yes |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Videos | Yes (`get`) | Yes |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Videos and pages | Yes (`get`) | Yes (DASH + ffmpeg) |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Videos | Yes (`get`) | Yes (native + ffmpeg) |

## Features

- **Incremental**: a `.state.json` ledger keyed by `platform:id` skips anything
  already downloaded; `--force` re-downloads.
- **Watermark-free**: uses the play URLs supplied by the platform or generated
  by the current session; no watermark removal step.
- **One input format**: any script, service or hand-written file can produce
  the timeline JSON; no provider SDK or specific upstream is required.
- **Metadata without downloading**: `vidpuller info <url>` prints a
  platform-neutral JSON object (title, author, IDs, duration, dimensions,
  publish date, cover, parts) for Bilibili, YouTube and TikTok.
- **Single binary**: statically linked (`CGO_ENABLED=0`), no third-party Go
  dependencies.
- **Machine-readable progress**: `--progress` writes `PROGRESS <0-100>` lines
  to stderr for callers to parse.

## Input format

The top level can be `{ "items": [...] }`, a bare array `[...]`, or an object
using `data` as the alias for `items`. A complete example:

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

| Field | Required | Description |
|---|---|---|
| `items` / `data` | Yes | Item array; the top level may also be a bare array |
| `platform` / `source` | No | `douyin`, `tiktok`, `bilibili` or `youtube`; can also be set per item |
| `video_id` | Yes | Canonical video ID; aliases: `aweme_id` (Douyin/TikTok), `id` (Bilibili BV ID) |
| `title` / `desc` | No | Title; both are accepted and used in the output filename |
| `url` | No | Video page URL; Bilibili accepts a BV ID alone or a full URL |
| `play_url` | No | Direct media URL; required for Douyin, re-resolved for TikTok, unused for Bilibili |
| `cover_url` / `images` | No | Cover image; `images[0]` is used when `cover_url` is absent |
| `published_at` / `create_time` | No | Publish time; RFC 3339 strings or Unix seconds |
| `author` | No | Author name, used for the output directory and display |

Platform detection order: `--platform` > per-item `platform` > top-level
`platform`/`source` > BV ID or Bilibili page URL > YouTube URL > timeline URL >
media URL hosts. If no rule matches, the command fails instead of guessing a
platform. Mixed-platform timelines are supported when each item carries
`platform`.

## Install

Prebuilt static binaries for Linux, macOS and Windows (amd64/arm64) are attached
to every [release](https://github.com/yilong-musk/vidpuller/releases). Download
the archive for your platform, check it against `checksums.txt`, and put
`vidpuller` on your `PATH`.

On macOS or Linux with [Homebrew](https://brew.sh):

```bash
brew install --cask yilong-musk/tap/vidpuller
```

With a Go toolchain (1.27+):

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## Requirements

- Go 1.27+ to build.
- `ffmpeg` on `PATH` for Bilibili and YouTube downloads (DASH audio and video
  are merged).
- Optional cookies for higher quality: TikTok may serve 720p anonymously and
  1080p with a logged-in session; Bilibili unlocks 1080p+ with `SESSDATA`.
  For YouTube, `--cookies` is used by the built-in extractor.

## Quick start

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + tests + language check
make install      # installs to ~/.local/bin/vidpuller
vidpuller version
```

## Usage

```bash
# Local timeline
vidpuller download ./timeline.json -n 5

# Remote timeline (platform detected from the JSON or URL)
vidpuller download 'https://example.com/timeline.json' -n 5

# Explicit platform when the timeline has no hint
vidpuller download ./timeline.json --platform douyin -n 5

# Inspect before downloading
vidpuller download '<timeline-url>' -n 10 --dry-run

# Only specific items
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# List items
vidpuller list '<timeline-url>' -n 20

# Metadata only (JSON, no download)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# One TikTok video (vt.tiktok.com short links are supported)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# Bilibili with cookies
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# One Bilibili video (BV ID or full URL; ?p=N selects a page)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# One YouTube video (native extractor, up to 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## Options

| Option | Description |
|---|---|
| `-o, --out DIR` | Output root, default `./downloads` |
| `-n, --limit N` | Download at most N items (already-downloaded items do not count; 0 = unlimited) |
| `--since YYYY-MM-DD` | Only items published on or after this date |
| `--ids A,B` | Only the given video IDs (`video_id` / `aweme_id`) |
| `--dry-run` | Print what would be downloaded |
| `--force` | Re-download items that are already recorded |
| `--interval DUR` | Delay between items, default `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | Override platform detection |
| `--max-height N` | Maximum YouTube height (default 1080; e.g. 1440, 2160) |
| `--cookies FILE` | Login cookies (Netscape / JSON / `k=v; k=v`) |
| `--progress` | Write `PROGRESS <0-100>` lines to stderr |

## Output

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            incremental ledger (key = platform:id)
```

The `.json` file is the original timeline item; `play_url` may expire and is
only kept for reference.

## Implementation notes

- **Douyin**: `play_url` is downloaded directly with
  `Referer: https://www.douyin.com/`.
- **TikTok**: CDN URLs are bound to the session that signed them
  (`ttwid`/`msToken`/`tt_chain_token`), so batch downloads re-resolve each
  video's URL with the current session; reusing an upstream URL with a
  different cookie jar returns 403.
- TikTok's WAF is a SHA-256 proof of work solved with the standard library; no
  browser is required.
- **Quality selection**: each rendition's `UrlKey` (for example
  `…_bytevc1_1080p_1037801`) provides the real codec and resolution; the
  highest resolution wins, with bitrate as the tie-breaker. The downloaded
  quality is printed (for example `1080p bytevc1 722kbps`).
- **Bilibili**: `view` provides the cid, `playurl` provides DASH streams, and
  `ffmpeg -c copy` merges them. A mirror host is tried first for Akamai URLs,
  with the original CDN as fallback.
- **YouTube**: a native InnerTube extractor requests the player response with
  the VISIONOS client, which exposes direct adaptive stream URLs without a
  signature cipher. The highest video up to `--max-height` is preferred
  (AVC/MP4 first), matched with the best MP4 audio, and merged by
  `ffmpeg -c copy`.

## Limitations

- **Douyin single-video URLs are not supported**: the detail endpoint requires
  a browser signature (`a_bogus`), and pure HTTP cannot obtain a watermark-free
  stream or metadata. Use `download --ids <video_id>`, or read the item from a
  timeline JSON.
- TikTok image posts have no `play_url` and are skipped (the cover can still be
  fetched from the timeline).
- **YouTube** defaults to 1080p; pass `--max-height 1440` or `--max-height
  2160` for higher quality (files can be several hundred MB). Age-restricted,
  DRM-protected or region-blocked videos may need cookies or may not be
  downloadable. Playlists and channel feeds are not expanded; pass single
  video URLs.
- Private or region-restricted content is reported as an error by the platform;
  the tool cannot see what the timeline data does not include.
- Timeline collection, signing and login state are the caller's
  responsibility; this tool only consumes the JSON.

## Documentation

- **Translations** (`README.de.md`, `README.es.md`, `README.fr.md`,
  `README.ja.md`, `README.ko.md`, `README.pt.md`, `README.ru.md`,
  `README.zh-CN.md`, `README.zh-TW.md`): localized copies of this page; the English
  page remains the source of truth.
- **[AGENTS.md](AGENTS.md)**: repository constraints, including the
  English-only rule and the merge policy.

## License

[MIT](LICENSE)
