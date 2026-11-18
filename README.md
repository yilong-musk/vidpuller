# vidpuller

抖音 / TikTok / B 站视频下载器（Go，单二进制，零第三方依赖）。

输入是一份平台无关的 JSON 时间线（本地文件或任意 HTTP 地址）。工具只做三件事：
解析时间线、下载无水印直链、按「平台 + 作品 ID」增量去重。它不依赖任何特定的
抓取服务，也不要求时间线来自某个项目——任何脚本、服务或手工文件，只要按下面的
格式输出 JSON 即可。

## 输入格式

顶层可以是 `{ "items": [...] }`，也可以直接是条目数组 `[...]`；`data` 可作为
`items` 的别名（适配常见 API 包装）。下面是一个完整例子：

```json
{
  "platform": "douyin",
  "author": "作者名",
  "items": [
    {
      "video_id": "作品 ID",
      "title": "标题 / 文案",
      "url": "https://example.com/video/123",
      "play_url": "https://example.com/video.mp4",
      "cover_url": "https://example.com/cover.jpg",
      "published_at": "2026-01-02T03:04:05Z",
      "author": "作者名"
    }
  ]
}
```

字段说明：

| 字段 | 必填 | 说明 |
|---|---|---|
| `items` / `data` | 是 | 条目数组；顶层也可直接是数组，`data` 是 `items` 的别名 |
| `platform` / `source` | 否 | `douyin` / `tiktok` / `bilibili`；也可以逐条写在 `item.platform` |
| `video_id` | 是 | 通用作品 ID；兼容别名 `aweme_id`（抖音/TikTok）、`id`（B 站 BV 号） |
| `title` / `desc` | 否 | 标题；两者都接受，输出文件名用 `title`/`desc` |
| `url` | 否 | 视频页地址；B 站可只给 BV 号，也可以给完整链接 |
| `play_url` | 否 | 直链；抖音必须提供，TikTok 会重新解析，B 站不需要 |
| `cover_url` / `images` | 否 | 封面；`images` 取第一张 |
| `published_at` / `create_time` | 否 | 发布时间；RFC3339 字符串或 Unix 秒都接受 |
| `author` | 否 | 作者名，仅用于输出目录和展示 |

平台识别顺序：`--platform` > 条目 `platform` > 顶层 `platform`/`source` > BV 号或
B 站视频页地址 > 时间线 URL 里的平台名 > 媒体直链域名。混合平台时间线可以逐条指定
`platform`，下载会按条目路由；都匹配不上时会报错，不会默认成某个平台。

## 前置

- B 站下载需要系统里有 `ffmpeg`（DASH 音视频流要合并）。
- 时间线由调用方准备（抓取、签名、登录态都归调用方）；本工具只消费 JSON。
- 抖音单条链接不支持：详情接口需要浏览器签名，纯 HTTP 拿不到无水印直链；
  要下某一条，用 `download --ids <video_id>`。

## 用法

```bash
make build              # 生成 ./vidpuller
make install            # 装到 ~/.local/bin/vidpuller

# 本地时间线
vidpuller download ./timeline.json -n 5

# 远程时间线（平台自动识别）
vidpuller download 'https://example.com/timeline.json' -n 5

# 平台识别不出来时显式指定
vidpuller download ./timeline.json --platform douyin -n 5

# 只看看要下什么
vidpuller download '<timeline-url>' -n 10 --dry-run

# 只下指定作品
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# 只列条目
vidpuller list '<timeline-url>' -n 20

# 单条 TikTok（支持 vt.tiktok.com 短链）
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# B 站：按时间线批量下载（mp4 直接可看）
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
| `--cookies FILE` | 登录 cookie（Netscape / JSON / `k=v; k=v`）：TikTok 提画质、B 站解锁 1080p+ |

## 产物

```
downloads/
  douyin/<作者>/<video_id>_<标题>.mp4|.jpg|.json
  tiktok/<作者>/<video_id>_<标题>.mp4|.jpg|.json
  bilibili/<作者>/<video_id>_<标题>.mp4|.jpg|.json
  .state.json            增量账本（键 = platform:id）
```

`.json` 里是时间线的原始条目（`play_url` 有时效，只作记录）。

## 实现要点

- **抖音**：`play_url` 直接可下（带 `Referer: https://www.douyin.com/`）。
- **TikTok**：CDN 直链绑定签发它的会话 cookie（`ttwid`/`msToken`/`tt_chain_token`），
  所以批量下载时每条都会用当前会话重新解析一次视频页的直链，而不是直接用
  时间线里的 `play_url`——用别的 cookie jar 下载那个 URL 会 403。
- TikTok 的 WAF 是 SHA-256 工作量证明，纯标准库解（无需浏览器）；单条 `get`
  自带这一步。
- **画质选择**：解析每个档位的 `UrlKey`（形如 `…_bytevc1_1080p_1037801`）
  取真实 codec 与分辨率，按「分辨率优先、同分辨率比码率」选最高档，
  下载结果会打印实际档位（如 `1080p bytevc1 722kbps`）。
- **上限由服务端决定**：未登录时 TikTok 对同一条视频可能只下发 720p，
  也可能给 1080p（同一视频不同时段实测会变）。带 `--cookies` 登录后
  通常会解锁更高档位，但本项目不内置账号，需要自己导出 cookie。

## 已知限制

- **抖音单条链接不支持**：`aweme/v1/web/aweme/detail` 需要浏览器签名
  （a_bogus），纯 HTTP 拿不到无水印直链；分享页的 `video_layout` 也是空的。
  要下某一条，用 `download --ids <video_id>`。
- TikTok 的图文帖没有 `play_url`，会被跳过（封面仍可在时间线里取到）。
- 私密 / 地区限制内容由时间线数据决定，本工具看不到被上游省略的条目。
- 时间线的抓取、签名与登录态由调用方负责，本工具只消费 JSON。
