package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// refererFor 返回平台 CDN 要求的 Referer；缺了会被 403。
func refererFor(platform string) string {
	if platform == platformTikTok {
		return "https://www.tiktok.com/"
	}
	return "https://www.douyin.com/"
}

// runDownload 执行 download 子命令。
func runDownload(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "参数错误:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "用法: vidpuller download <timeline-url|file> [参数]")
		return 2
	}
	if o.progress {
		reportProgress = func(f float64) { fmt.Fprintf(os.Stderr, "PROGRESS %.1f\n", f*100) }
	}
	if o.cookies != "" {
		cks, err := loadCookieFile(o.cookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 cookie 失败:", err)
			return 2
		}
		sessionCookies = cks
	}
	tl, err := loadTimeline(pos[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取时间线失败:", err)
		return 1
	}
	platform := detectPlatform(tl, pos[0], o.platform)
	if platform == "" {
		fmt.Fprintln(os.Stderr, "无法判断平台：请在时间线顶层提供 source 或 platform（douyin|tiktok|bilibili），或在条目里逐条提供 platform，或用 --platform 指定")
		return 2
	}
	items := selectItems(tl, o)
	if len(items) == 0 {
		fmt.Println("没有符合条件的条目")
		return 0
	}
	label := platformLabel(tl, items, pos[0], o.platform)

	st := loadState(filepath.Join(o.out, stateFile))
	if o.dryRun {
		fmt.Printf("[%s] %s — 共 %d 条待检查\n", label, displayAuthor(tl), len(items))
		for _, it := range items {
			p := platformForItem(tl, it, pos[0], o.platform)
			status := "将下载"
			if p == "" {
				status = "平台未知"
			} else if !o.force && st.has(p, it.AwemeID) {
				status = "已下载，跳过"
			}
			fmt.Printf("  %s  %s  %s\n", it.AwemeID, formatDate(it.CreateTime), status)
		}
		return 0
	}

	sessions := map[string]*http.Client{}
	var okCount, skipCount, failCount int
	for i, it := range items {
		p := platformForItem(tl, it, pos[0], o.platform)
		if p == "" {
			fmt.Printf("  [跳过] %s 无法判断平台（请在条目里提供 platform 或用 --platform 指定）\n", it.AwemeID)
			failCount++
			continue
		}
		if !o.force && st.has(p, it.AwemeID) {
			skipCount++
			continue
		}
		if it.PlayURL == "" && p != platformBilibili {
			fmt.Printf("  [跳过] %s 没有视频直链（图文帖？）\n", it.AwemeID)
			failCount++
			continue
		}
		fmt.Printf("[%d/%d] %s  %s\n", i+1, len(items), it.AwemeID, truncate(it.Desc, 50))
		var client *http.Client
		if p == platformTikTok {
			// 用当前会话重新签发直链：上游的 URL 绑定它自己的 cookie。
			det, c, rerr := resolveTikTokPlayURL(sessions[it.Author], it.Author, it.AwemeID)
			if rerr != nil {
				fmt.Printf("  警告: 重新解析直链失败（%v），改用时间线里的地址\n", rerr)
				if sessions[it.Author] == nil {
					c, _ = tiktokSessionFor(it.Author)
				}
			} else {
				if det.PlayURL != "" {
					it.PlayURL = det.PlayURL
				}
				if det.CoverURL != "" {
					it.CoverURL = det.CoverURL
				}
				if it.Desc == "" {
					it.Desc = det.Desc
				}
				it.Quality = det.Quality
			}
			if c != nil {
				sessions[it.Author] = c
				client = c
			} else {
				client = sessions[it.Author]
			}
		}
		res, err := downloadItem(it, p, o.out, client)
		quality := ""
		if res.Quality != "" {
			quality = "，" + res.Quality
		}
		if err != nil {
			fmt.Printf("  失败: %v\n", err)
			failCount++
			continue
		}
		st.mark(p, it, res.Video)
		if err := st.save(filepath.Join(o.out, stateFile)); err != nil {
			fmt.Printf("  警告: 写入增量账本失败: %v\n", err)
		}
		fmt.Printf("  完成 %s（%s%s）\n", res.Video, humanBytes(res.Bytes), quality)
		okCount++
		if o.interval > 0 && i < len(items)-1 {
			time.Sleep(o.interval)
		}
	}

	fmt.Printf("\n完成 %d 条，跳过 %d 条，失败 %d 条\n", okCount, skipCount, failCount)
	if failCount > 0 {
		return 1
	}
	return 0
}

// platformLabel 给列表/摘要用的平台标签：单一平台返回名字，混合返回 mixed。
func platformLabel(tl *Timeline, items []Item, src, override string) string {
	seen := map[string]bool{}
	for _, it := range items {
		if p := platformForItem(tl, it, src, override); p != "" {
			seen[p] = true
		}
	}
	if len(seen) == 0 {
		return "unknown"
	}
	if len(seen) == 1 {
		for p := range seen {
			return p
		}
	}
	return "mixed"
}

// runList 执行 list 子命令：只打印，不下载。
func runList(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "参数错误:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "用法: vidpuller list <timeline-url|file> [参数]")
		return 2
	}
	tl, err := loadTimeline(pos[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取时间线失败:", err)
		return 1
	}
	items := selectItems(tl, o)
	label := platformLabel(tl, items, pos[0], o.platform)
	fmt.Printf("[%s] %s — %d 条\n", label, displayAuthor(tl), len(items))
	mixed := label == "mixed"
	for _, it := range items {
		if mixed {
			p := platformForItem(tl, it, pos[0], o.platform)
			if p == "" {
				p = "?"
			}
			fmt.Printf("  [%s] %s  %s  %5s  %s\n",
				p, it.AwemeID, formatDate(it.CreateTime), formatDuration(it.DurationMS), truncate(it.Desc, 60))
			continue
		}
		fmt.Printf("  %s  %s  %5s  %s\n",
			it.AwemeID, formatDate(it.CreateTime), formatDuration(it.DurationMS), truncate(it.Desc, 60))
	}
	return 0
}

// downloadResult 是一次下载落盘的文件。
type downloadResult struct {
	Video   string
	Cover   string
	Meta    string
	Bytes   int64
	Quality string
}

// downloadItem 下载视频、封面和元数据到 <out>/<platform>/<author>/。
func downloadItem(it Item, platform, outRoot string, client *http.Client) (downloadResult, error) {
	if platform == platformBilibili {
		return downloadBilibiliItem(it, outRoot, sessionCookies)
	}
	var res downloadResult
	res.Quality = it.Quality
	author := it.Author
	if author == "" {
		author = "unknown"
	}
	dir := filepath.Join(outRoot, platform, safeName(author))
	base := it.AwemeID + "_" + safeName(it.Desc)
	res.Video = filepath.Join(dir, base+".mp4")
	res.Cover = filepath.Join(dir, base+".jpg")
	res.Meta = filepath.Join(dir, base+".json")

	n, err := downloadFile(it.PlayURL, res.Video, refererFor(platform), client)
	if err != nil {
		return res, fmt.Errorf("视频: %w", err)
	}
	res.Bytes = n

	if it.CoverURL != "" {
		if _, err := downloadFile(it.CoverURL, res.Cover, refererFor(platform), client); err != nil {
			fmt.Printf("  警告: 封面下载失败: %v\n", err)
			res.Cover = ""
		}
	}

	meta := struct {
		Platform  string `json:"platform"`
		Item      Item   `json:"item"`
		FetchedAt string `json:"downloaded_at"`
	}{
		Platform:  platform,
		Item:      it,
		FetchedAt: time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(res.Meta, data, 0o644); err != nil {
		return res, fmt.Errorf("元数据: %w", err)
	}
	return res, nil
}

// reportProgress 是全局进度回调（--progress 时设置），值域 0..1。
// 进度行走 stderr，格式固定为 "PROGRESS <0-100>"，供调用方解析。
var reportProgress func(float64)

var lastProgressAt time.Time
var lastProgressPct = -1.0

// emitProgress 节流后回调：至少 1% 或 200ms 才输出一次。
func emitProgress(f float64) {
	if reportProgress == nil {
		return
	}
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	pct := math.Floor(f*100*10) / 10
	now := time.Now()
	if pct == lastProgressPct || (now.Sub(lastProgressAt) < 200*time.Millisecond && pct < 100) {
		return
	}
	lastProgressAt, lastProgressPct = now, pct
	reportProgress(f)
}

// progressRange 把单个文件的 0..1 进度映射到总进度的一段（B 站视频/音频分段）。
type progressRange struct{ base, span float64 }

type progressWriter struct {
	w     io.Writer
	total int64
	done  int64
	base  float64
	span  float64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.total > 0 {
		emitProgress(p.base + float64(p.done)/float64(p.total)*p.span)
	}
	return n, err
}

// downloadFile 下载直链到 dest（先写 .part 再改名，避免半成品）。
func downloadFile(url, dest, referer string, client *http.Client, rng ...progressRange) (int64, error) {
	if url == "" {
		return 0, fmt.Errorf("空下载地址")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	tmp := dest + ".part"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	r := progressRange{base: 0, span: 1}
	if len(rng) > 0 {
		r = rng[0]
	}
	pw := &progressWriter{w: f, total: resp.ContentLength, base: r.base, span: r.span}
	n, copyErr := io.Copy(pw, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return n, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return n, closeErr
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		_ = os.Remove(tmp)
		return n, fmt.Errorf("长度不符: 收到 %d / 期望 %d", n, resp.ContentLength)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return n, err
	}
	return n, nil
}

// safeName 把标题变成安全的文件名片段。
func safeName(s string) string {
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "：", "*", "_", "?", "？",
		"\"", "'", "<", "《", ">", "》", "|", "_", "\n", " ", "\r", " ",
		"\t", " ", "\x00", "",
	)
	s = repl.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 60 {
		s = string(r[:60])
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

func displayAuthor(tl *Timeline) string {
	if tl.Author != "" {
		return tl.Author
	}
	if tl.User != "" {
		return tl.User
	}
	if tl.SecUID != "" {
		return tl.SecUID
	}
	return "unknown"
}

func formatDate(unix int64) string {
	if unix <= 0 {
		return "----------"
	}
	return time.Unix(unix, 0).Format("2006-01-02")
}

func formatDuration(ms int) string {
	if ms <= 0 {
		return "-"
	}
	sec := ms / 1000
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func humanBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
