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
	fmt.Fprintf(os.Stderr, `vidpuller %s — multi-platform video downloader (Douyin / TikTok / Bilibili / YouTube)

Usage:
  vidpuller download <timeline-url|file>   download items from a timeline
  vidpuller get <video-url>                download one video (TikTok / Bilibili / YouTube)
  vidpuller list <timeline-url|file>       list items without downloading
  vidpuller version

Input is a platform-neutral JSON timeline (local file or any HTTP URL):
  {
    "platform": "douyin",                // optional; item.platform also works
    "items": [
      {"video_id": "…", "title": "…", "play_url": "…"}
    ]
  }
  Video IDs accept video_id / aweme_id / id (Bilibili BV ID).
  Set item.platform per item (it overrides the top-level source); mixed
  timelines are supported. If the platform is omitted, it is inferred from
  the BV ID, video page URL, timeline URL, or --platform.

download options:
  -o, --out DIR        output root (default ./downloads)
  -n, --limit N        download at most N items (skipped items do not count; 0 = unlimited)
      --since DATE     only items published on or after DATE (YYYY-MM-DD)
      --ids A,B        only the given video IDs (video_id / aweme_id)
      --dry-run        print what would be downloaded
      --force          re-download items that are already recorded
      --interval DUR   delay between items (default 3s)
      --platform NAME  override platform detection (douyin|tiktok|bilibili|youtube)
      --cookies FILE   login cookies (Netscape / JSON / "k=v; k=v"); raises
                       TikTok quality and unlocks Bilibili 1080p+
      --progress       write progress lines to stderr (PROGRESS <0-100>)

get options:
  -o, --out DIR        output root (default ./downloads)
      --force          re-download an item that is already recorded

list options:
  -n, --limit N        list at most N items (0 = all)
      --since DATE     only items published on or after DATE

Douyin single-video URLs are not supported: the detail endpoint requires a
browser signature and pure HTTP cannot obtain a watermark-free stream. Use
download + --ids, or put the item in a timeline JSON.

YouTube downloads use the built-in InnerTube extractor (up to 1080p).

Output: <out>/<platform>/<author>/<video_id>_<title>.mp4 + .jpg + .json
State:  <out>/.state.json (dedup key: platform:id)
`, version)
}

// options holds the parsed flags shared by all subcommands.
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

// parseOptions is a hand-written parser: the standard flag package stops at
// the first positional argument, while this tool accepts both
// `vidpuller download URL -n 5` and `-n 5 URL`.
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
				return o, nil, fmt.Errorf("invalid --limit: %q", v)
			}
			o.limit, i = n, next
		case "--since":
			v, next, err := flagValue(inline, hasInline, args, i)
			if err != nil {
				return o, nil, err
			}
			t, err := time.ParseInLocation("2006-01-02", v, time.Local)
			if err != nil {
				return o, nil, fmt.Errorf("invalid --since: %q (want YYYY-MM-DD)", v)
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
				return o, nil, fmt.Errorf("invalid --interval: %q", v)
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
			if v != platformDouyin && v != platformTikTok && v != platformBilibili && v != platformYouTube {
				return o, nil, fmt.Errorf("invalid --platform: %q (douyin|tiktok|bilibili|youtube)", v)
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
				return o, nil, fmt.Errorf("unknown flag: %s", arg)
			}
			pos = append(pos, arg)
		}
	}
	return o, pos, nil
}

// flagValue returns the value of --flag value or --flag=value, plus the next
// argument index to read.
func flagValue(inline string, hasInline bool, args []string, i int) (string, int, error) {
	if hasInline {
		if inline == "" {
			return "", i, fmt.Errorf("flag %s needs a value", args[i])
		}
		return inline, i, nil
	}
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("flag %s needs a value", args[i])
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
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}
