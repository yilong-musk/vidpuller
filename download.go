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

// refererFor returns the Referer required by each platform's CDN; requests
// without it get a 403.
func refererFor(platform string) string {
	if platform == platformTikTok {
		return "https://www.tiktok.com/"
	}
	return "https://www.douyin.com/"
}

// runDownload executes the download subcommand.
func runDownload(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpuller download <timeline-url|file> [options]")
		return 2
	}
	if o.progress {
		reportProgress = func(f float64) { fmt.Fprintf(os.Stderr, "PROGRESS %.1f\n", f*100) }
	}
	if o.cookies != "" {
		cks, err := loadCookieFile(o.cookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read cookies:", err)
			return 2
		}
		sessionCookies = cks
		youtubeCookieFile = o.cookies
	}
	tl, err := loadTimeline(pos[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "load timeline:", err)
		return 1
	}
	platform := detectPlatform(tl, pos[0], o.platform)
	if platform == "" {
		fmt.Fprintln(os.Stderr, "cannot determine platform: set source or platform at the top level (douyin|tiktok|bilibili|youtube), set platform per item, or pass --platform")
		return 2
	}
	items := selectItems(tl, o)
	if len(items) == 0 {
		fmt.Println("no matching items")
		return 0
	}
	label := platformLabel(tl, items, pos[0], o.platform)

	st := loadState(filepath.Join(o.out, stateFile))
	if o.dryRun {
		fmt.Printf("[%s] %s — %d item(s) to check\n", label, displayAuthor(tl), len(items))
		for _, it := range items {
			p := platformForItem(tl, it, pos[0], o.platform)
			status := "would download"
			if p == "" {
				status = "platform unknown"
			} else if !o.force && st.has(p, it.AwemeID) {
				status = "already downloaded, skip"
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
			fmt.Printf("  [skip] %s: cannot determine platform (set item.platform or --platform)\n", it.AwemeID)
			failCount++
			continue
		}
		if !o.force && st.has(p, it.AwemeID) {
			skipCount++
			continue
		}
		if it.PlayURL == "" && p != platformBilibili && p != platformYouTube {
			fmt.Printf("  [skip] %s: no video URL (image post?)\n", it.AwemeID)
			failCount++
			continue
		}
		fmt.Printf("[%d/%d] %s  %s\n", i+1, len(items), it.AwemeID, truncate(it.Desc, 50))
		var client *http.Client
		if p == platformTikTok {
			// Re-resolve the play URL with the current session: the URL from
			// upstream is tied to its own cookies.
			det, c, rerr := resolveTikTokPlayURL(sessions[it.Author], it.Author, it.AwemeID)
			if rerr != nil {
				fmt.Printf("  warning: re-resolving URL failed (%v); using timeline URL\n", rerr)
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
			quality = ", " + res.Quality
		}
		if err != nil {
			fmt.Printf("  failed: %v\n", err)
			failCount++
			continue
		}
		st.mark(p, it, res.Video)
		if err := st.save(filepath.Join(o.out, stateFile)); err != nil {
			fmt.Printf("  warning: save state: %v\n", err)
		}
		fmt.Printf("  done %s (%s%s)\n", res.Video, humanBytes(res.Bytes), quality)
		okCount++
		if o.interval > 0 && i < len(items)-1 {
			time.Sleep(o.interval)
		}
	}

	fmt.Printf("\ndone %d, skipped %d, failed %d\n", okCount, skipCount, failCount)
	if failCount > 0 {
		return 1
	}
	return 0
}

// platformLabel is the platform label for lists and summaries: a single
// platform name, or "mixed".
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

// runList executes the list subcommand: print only, never download.
func runList(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpuller list <timeline-url|file> [options]")
		return 2
	}
	tl, err := loadTimeline(pos[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "load timeline:", err)
		return 1
	}
	items := selectItems(tl, o)
	label := platformLabel(tl, items, pos[0], o.platform)
	fmt.Printf("[%s] %s — %d item(s)\n", label, displayAuthor(tl), len(items))
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

// downloadResult describes the files written by one download.
type downloadResult struct {
	Video   string
	Cover   string
	Meta    string
	Bytes   int64
	Quality string
}

// downloadItem writes the video, cover, and metadata under
// <out>/<platform>/<author>/.
func downloadItem(it Item, platform, outRoot string, client *http.Client) (downloadResult, error) {
	if platform == platformBilibili {
		return downloadBilibiliItem(it, outRoot, sessionCookies)
	}
	if platform == platformYouTube {
		return downloadYouTubeItem(it, outRoot, youtubeCookieFile)
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
		return res, fmt.Errorf("video: %w", err)
	}
	res.Bytes = n

	if it.CoverURL != "" {
		if _, err := downloadFile(it.CoverURL, res.Cover, refererFor(platform), client); err != nil {
			fmt.Printf("  warning: cover download failed: %v\n", err)
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
		return res, fmt.Errorf("metadata: %w", err)
	}
	return res, nil
}

// reportProgress is the global progress callback (set by --progress), with
// values in 0..1. Progress lines go to stderr as "PROGRESS <0-100>" so
// callers can parse them.
var reportProgress func(float64)

var lastProgressAt time.Time
var lastProgressPct = -1.0

// emitProgress throttles callbacks to at least 1% or 200ms apart.
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

// progressRange maps one file's 0..1 progress onto a slice of the total
// progress (Bilibili downloads video and audio separately).
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

// downloadFile downloads a direct URL to dest. It writes a .part file first
// and renames it on success, so a partial file never looks complete.
func downloadFile(url, dest, referer string, client *http.Client, rng ...progressRange) (int64, error) {
	if url == "" {
		return 0, fmt.Errorf("empty download URL")
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
		return n, fmt.Errorf("size mismatch: got %d, want %d", n, resp.ContentLength)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return n, err
	}
	return n, nil
}

// safeName turns a title into a safe filename fragment.
func safeName(s string) string {
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\n", " ", "\r", " ",
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
