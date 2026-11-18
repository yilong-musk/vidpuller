package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const version = "0.1.0"

func usage() {
	fmt.Fprintf(os.Stderr, `vidpuller %s — 多平台视频下载（抖音 / TikTok / B 站）

用法:
  vidpuller download <timeline-url|file>   按时间线批量下载
  vidpuller get <视频链接>                 下载单条（TikTok / B 站）
  vidpuller list <timeline-url|file>       只列出条目，不下载
  vidpuller version

输入是一份平台无关的 JSON 时间线（本地文件或任意 HTTP 地址）:
  {
    "platform": "douyin",                // 可选；也可逐条写 item.platform
    "items": [
      {"video_id": "…", "title": "…", "play_url": "…"}
    ]
  }
  作品 ID 兼容 video_id / aweme_id / id（B 站 BV 号）；
  平台可逐条写 item.platform（优先于顶层 source），也支持混合平台时间线；
  缺失时可从 BV 号、视频页地址或 --platform 推断。

download 参数:
  -o, --out DIR        输出根目录（默认 ./downloads）
  -n, --limit N        最多下载 N 条（已下载跳过的不计数；0=不限）
      --since DATE     只下该日期之后（YYYY-MM-DD）
      --ids A,B        只下指定作品 ID（video_id / aweme_id）
      --dry-run        只打印将下载哪些
      --force          已下载过也重新下载
      --interval DUR   每条之间的间隔（默认 3s）
      --platform NAME  覆盖平台识别（douyin|tiktok|bilibili）
      --cookies FILE   登录 cookie（Netscape / JSON / "k=v; k=v"），TikTok 提画质、B 站 1080p+
      --progress       往 stderr 输出进度行（PROGRESS <0-100>，供调用方解析）

get 参数:
  -o, --out DIR        输出根目录（默认 ./downloads）
      --force          已下载过也重新下载

list 参数:
  -n, --limit N        最多列出 N 条（0=全部）
      --since DATE     只列该日期之后

抖音单条暂不支持直接给链接：detail 接口需要浏览器签名，纯 HTTP 拿不到
无水印直链。请改用 download + --ids，或把该条放进时间线 JSON。

产物: <out>/<platform>/<author>/<video_id>_<标题>.mp4 + .jpg + .json
增量: <out>/.state.json（按 platform:id 去重）
`, version)
}

// options 是所有子命令共享的解析结果。
type options struct {
	out      string
	limit    int
	since    time.Time
	ids      []string
	dryRun   bool
	force    bool
	quiet    bool
	interval time.Duration
	platform string
	cookies  string
	progress bool
}

// parseOptions 手写解析：标准库 flag 包遇到第一个位置参数就停止，
// 而这个工具希望 `vidpuller download URL -n 5` 和 `-n 5 URL` 都能用。
func parseOptions(args []string) (options, []string, error) {
	o := options{out: "./downloads", interval: 3 * time.Second}
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		name, inline, hasInline := strings.Cut(arg, "=")
		switch name {
		case "-o", "--out":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			o.out, i = v, next
		case "-n", "--limit":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return o, nil, fmt.Errorf("无效的 --limit: %q", v)
			}
			o.limit, i = n, next
		case "--since":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			t, err := time.ParseInLocation("2006-01-02", v, time.Local)
			if err != nil {
				return o, nil, fmt.Errorf("无效的 --since: %q（需要 YYYY-MM-DD）", v)
			}
			o.since, i = t, next
		case "--ids":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			for _, id := range strings.Split(v, ",") {
				if id = strings.TrimSpace(id); id != "" {
					o.ids = append(o.ids, id)
				}
			}
			i = next
		case "--interval":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			d, err := time.ParseDuration(v)
			if err != nil || d < 0 {
				return o, nil, fmt.Errorf("无效的 --interval: %q", v)
			}
			o.interval, i = d, next
		case "--progress":
			o.progress = true
		case "--cookies":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			o.cookies, i = v, next
		case "--platform":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			if v != platformDouyin && v != platformTikTok && v != platformBilibili {
				return o, nil, fmt.Errorf("无效的 --platform: %q（douyin|tiktok|bilibili）", v)
			}
			o.platform, i = v, next
		case "--dry-run":
			o.dryRun = true
		case "--force":
			o.force = true
		case "-q", "--quiet":
			o.quiet = true
		case "-h", "--help":
			usage()
			os.Exit(0)
		default:
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return o, nil, fmt.Errorf("未知参数: %s", arg)
			}
			pos = append(pos, arg)
		}
	}
	return o, pos, nil
}

// flagValue 取出 --flag value 或 --flag=value 的值，返回下一个待读下标。
func flagValue(inline string, hasInline bool, args []string, i int) (string, int, error) {
	if hasInline {
		if inline == "" {
			return "", i, fmt.Errorf("参数 %s 缺少值", args[i])
		}
		return inline, i, nil
	}
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("参数 %s 缺少值", args[i])
	}
	return args[i+1], i + 1, nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "download":
		os.Exit(runDownload(os.Args[2:]))
	case "get":
		os.Exit(runGet(os.Args[2:]))
	case "list":
		os.Exit(runList(os.Args[2:]))
	case "version", "-v", "--version":
		fmt.Printf("vidpuller %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}
