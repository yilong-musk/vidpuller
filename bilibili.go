package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Bilibili download flow: view gives the cid, playurl gives DASH streams,
// then both streams are downloaded and merged with ffmpeg.
//
// Measured in 2026-11: the plain playurl endpoint needs no WBI signature;
// anonymous access tops out at 480p, while SESSDATA (--cookies) unlocks
// 1080p and higher depending on account permissions.
const (
	platformBilibili = "bilibili"
	biliReferer      = "https://www.bilibili.com/"
	biliAPIBase      = "https://api.bilibili.com"
)

var (
	bvidRe  = regexp.MustCompile(`(BV[0-9A-Za-z]{10})`)
	biliPRe = regexp.MustCompile(`[?&]p=(\d+)`)
	b23Re   = regexp.MustCompile(`(https?://b23\.tv/[0-9A-Za-z]+)`)
)

type biliView struct {
	BVID     string `json:"bvid"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Pic      string `json:"pic"`
	Duration int    `json:"duration"`
	Pubdate  int64  `json:"pubdate"`
	Owner    struct {
		MID  int64  `json:"mid"`
		Name string `json:"name"`
	} `json:"owner"`
	Pages []struct {
		CID      int64  `json:"cid"`
		Page     int    `json:"page"`
		Part     string `json:"part"`
		Duration int64  `json:"duration"`
	} `json:"pages"`
}

type biliStream struct {
	ID        int      `json:"id"`
	CodeCID   int      `json:"codecid"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	Bandwidth int      `json:"bandwidth"`
	BaseURL   string   `json:"baseUrl"`
	BackupURL []string `json:"backupUrl"`
	MimeType  string   `json:"mimeType"`
}

type biliDash struct {
	Video []biliStream `json:"video"`
	Audio []biliStream `json:"audio"`
	Dolby struct {
		Audio []biliStream `json:"audio"`
	} `json:"dolby"`
	Flac struct {
		Audio *biliStream `json:"audio"`
	} `json:"flac"`
}

type biliPlayData struct {
	Quality       int      `json:"quality"`
	AcceptQuality []int    `json:"accept_quality"`
	Dash          biliDash `json:"dash"`
}

func biliClient(cookies []*http.Cookie) *http.Client {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 60 * time.Second}
	if os.Getenv("VIDPULLER_BILI_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "DEBUG biliClient in=%d\n", len(cookies))
	}
	if len(cookies) > 0 {
		base, _ := url.Parse("https://www.bilibili.com")
		jar.SetCookies(base, cookies)
	}
	return client
}

// biliEnsureBuvid refreshes the anonymous fingerprint cookies (buvid3/buvid4)
// to significantly reduce 412 responses. It fetches fresh values on every
// call, avoiding expired fingerprints from a cookie file.
func biliEnsureBuvid(client *http.Client) {
	base, _ := url.Parse("https://www.bilibili.com")
	// Clear old fingerprints first.
	if old := client.Jar.Cookies(base); len(old) > 0 {
		reset := make([]*http.Cookie, 0, 2)
		for _, c := range old {
			if c.Name == "buvid3" || c.Name == "buvid4" {
				reset = append(reset, &http.Cookie{Name: c.Name, Value: "", MaxAge: -1, Path: "/"})
			}
		}
		if len(reset) > 0 {
			client.Jar.SetCookies(base, reset)
		}
	}
	req, err := http.NewRequest(http.MethodGet, biliAPIBase+"/x/frontend/finger/spi", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.Data.B3 == "" {
		return
	}
	cks := []*http.Cookie{{Name: "buvid3", Value: out.Data.B3, Path: "/", Domain: ".bilibili.com"}}
	if out.Data.B4 != "" {
		cks = append(cks, &http.Cookie{Name: "buvid4", Value: out.Data.B4, Path: "/", Domain: ".bilibili.com"})
	}
	client.Jar.SetCookies(base, cks)
}

func biliGetJSON(client *http.Client, target, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en,zh-CN;q=0.9,zh;q=0.8")
	req.Header.Set("Referer", referer)
	req.Header.Set("Origin", "https://www.bilibili.com")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusPreconditionFailed {
		return nil, fmt.Errorf("Bilibili risk control blocked the request (HTTP 412); retry later or pass --cookies")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 120))
	}
	return body, nil
}

// biliAPIResult is Bilibili's common response envelope (a non-zero code puts
// the error in message).
type biliAPIResult struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func biliAPI(client *http.Client, target, referer string) (json.RawMessage, error) {
	biliEnsureBuvid(client)
	var body []byte
	var err error
	for attempt := 1; attempt <= 4; attempt++ {
		body, err = biliGetJSON(client, target, referer)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "412") {
			return nil, err
		}
		time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	var res biliAPIResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("Bilibili API error %d: %s", res.Code, res.Message)
	}
	return res.Data, nil
}

// extractBVID extracts a BVID from a video URL or short link and parses the
// ?p= page number.
func extractBVID(target string) (bvid string, part int) {
	part = 1
	target = strings.TrimSpace(target)
	// b23.tv short links must be expanded first.
	if m := b23Re.FindString(target); m != "" {
		if loc, err := resolveRedirect(m); err == nil {
			target = loc
		}
	}
	if m := bvidRe.FindString(target); m != "" {
		bvid = m
	}
	if m := biliPRe.FindStringSubmatch(target); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			part = n
		}
	}
	return bvid, part
}

// resolveRedirect follows a short link and returns the final URL.
func resolveRedirect(target string) (string, error) {
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" {
		return target, nil
	}
	return loc, nil
}

// fetchBilibili resolves one Bilibili video and returns the best video/audio
// URLs plus metadata.
func fetchBilibili(bvid string, part int, cookies []*http.Cookie) (videoDetail, error) {
	var det videoDetail
	if bvid == "" {
		return det, fmt.Errorf("not a valid Bilibili video URL")
	}
	client := biliClient(cookies)
	referer := biliReferer + "video/" + bvid

	// 1) view: cid / title / cover / pages
	raw, err := biliAPI(client, biliAPIBase+"/x/web-interface/view?bvid="+url.QueryEscape(bvid), referer)
	if err != nil {
		return det, err
	}
	var view biliView
	if err := json.Unmarshal(raw, &view); err != nil {
		return det, fmt.Errorf("parse view: %w", err)
	}
	if len(view.Pages) == 0 {
		return det, fmt.Errorf("video has no playable pages")
	}
	page := view.Pages[0]
	for _, p := range view.Pages {
		if p.Page == part {
			page = p
			break
		}
	}
	title := view.Title
	if len(view.Pages) > 1 {
		title = fmt.Sprintf("%s - P%d %s", view.Title, page.Page, page.Part)
	}

	// 2) playurl: qn=127 asks for the highest quality, fnval=4048 asks for DASH.
	q := url.Values{}
	q.Set("bvid", bvid)
	q.Set("cid", strconv.FormatInt(page.CID, 10))
	q.Set("qn", "127")
	q.Set("fnval", "4048")
	q.Set("fnver", "0")
	q.Set("fourk", "1")
	q.Set("platform", "pc")
	q.Set("high_quality", "1")
	raw, err = biliAPI(client, biliAPIBase+"/x/player/playurl?"+q.Encode(), referer)
	if err != nil {
		return det, err
	}
	var play biliPlayData
	if err := json.Unmarshal(raw, &play); err != nil {
		return det, fmt.Errorf("parse playurl: %w", err)
	}
	if len(play.Dash.Video) == 0 {
		return det, fmt.Errorf("no DASH video stream available (login may be required)")
	}

	if os.Getenv("VIDPULLER_BILI_DEBUG") != "" {
		base, _ := url.Parse("https://api.bilibili.com")
		names := []string{}
		for _, c := range client.Jar.Cookies(base) {
			names = append(names, c.Name)
		}
		fmt.Fprintf(os.Stderr, "DEBUG cookies=%v quality=%d accept=%v streams=%d\n", names, play.Quality, play.AcceptQuality, len(play.Dash.Video))
	}
	vs := pickBiliVideo(play.Dash.Video)
	as := pickBiliAudio(play.Dash)
	if vs == nil {
		return det, fmt.Errorf("no video stream available")
	}
	if as == nil {
		return det, fmt.Errorf("no audio stream available")
	}
	det = videoDetail{
		PlayURL:  firstURL(*vs),
		AudioURL: firstURL(*as),
		CoverURL: view.Pic,
		Quality:  fmt.Sprintf("%dx%d %s %dkbps", vs.Width, vs.Height, codecName(vs.CodeCID), vs.Bandwidth/1000),
		Desc:     title,
		secUID:   view.Owner.Name, // reused field: Bilibili author name
		id:       bvid,
	}
	return det, nil
}

// pickBiliVideo prefers the highest quality ID, then AVC at the same ID, then
// bitrate.
func pickBiliVideo(streams []biliStream) *biliStream {
	var best *biliStream
	for i := range streams {
		s := &streams[i]
		if firstURL(*s) == "" {
			continue
		}
		if best == nil || betterBiliVideo(*s, *best) {
			best = s
		}
	}
	return best
}

func betterBiliVideo(a, b biliStream) bool {
	if a.ID != b.ID {
		return a.ID > b.ID
	}
	// AVC (7) has the best compatibility; HEVC (12)/AV1 (13) are smaller at
	// the same quality.
	ap, bp := a.CodeCID == 7, b.CodeCID == 7
	if ap != bp {
		return ap
	}
	return a.Bandwidth > b.Bandwidth
}

// pickBiliAudio prefers FLAC/Dolby, then bitrate.
func pickBiliAudio(d biliDash) *biliStream {
	if d.Flac.Audio != nil && firstURL(*d.Flac.Audio) != "" {
		return d.Flac.Audio
	}
	if len(d.Dolby.Audio) > 0 && firstURL(d.Dolby.Audio[0]) != "" {
		return &d.Dolby.Audio[0]
	}
	var best *biliStream
	for i := range d.Audio {
		s := &d.Audio[i]
		if firstURL(*s) == "" {
			continue
		}
		if best == nil || s.Bandwidth > best.Bandwidth {
			best = s
		}
	}
	return best
}

func firstURL(s biliStream) string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	if len(s.BackupURL) > 0 {
		return s.BackupURL[0]
	}
	return ""
}

func codecName(codecid int) string {
	switch codecid {
	case 7:
		return "avc"
	case 12:
		return "hevc"
	case 13:
		return "av1"
	default:
		return "codec" + strconv.Itoa(codecid)
	}
}

// downloadBilibiliItem downloads the audio and video streams and merges them
// into an mp4.
func downloadBilibiliItem(it Item, outRoot string, cookies []*http.Cookie) (downloadResult, error) {
	var res downloadResult
	det, err := fetchBilibili(it.AwemeID, 1, cookies)
	if err != nil {
		return res, err
	}
	if it.Desc == "" {
		it.Desc = det.Desc
	}
	it.Quality = det.Quality
	if it.CoverURL == "" {
		it.CoverURL = det.CoverURL
	}

	author := it.Author
	if author == "" {
		author = det.secUID
	}
	if author == "" {
		author = "unknown"
	}
	dir := filepath.Join(outRoot, platformBilibili, safeName(author))
	base := it.AwemeID + "_" + safeName(it.Desc)
	res.Video = filepath.Join(dir, base+".mp4")
	res.Cover = filepath.Join(dir, base+".jpg")
	res.Meta = filepath.Join(dir, base+".json")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}

	videoTmp := res.Video + ".video.m4s"
	audioTmp := res.Video + ".audio.m4s"
	defer os.Remove(videoTmp)
	defer os.Remove(audioTmp)

	client := biliClient(cookies)
	// Large downloads are not bound by the API's 60s timeout: use a client
	// without Timeout; downloadFile has its own 30-minute context.
	dlClient := &http.Client{Jar: client.Jar}
	if _, err := downloadFile(biliFastURL(det.PlayURL), videoTmp, biliReferer, dlClient, progressRange{0, 0.90}); err != nil {
		// Fall back to the original CDN when the mirror is unavailable.
		if _, err2 := downloadFile(det.PlayURL, videoTmp+".retry", biliReferer, dlClient, progressRange{0, 0.90}); err2 == nil {
			os.Rename(videoTmp+".retry", videoTmp)
		} else {
			return res, fmt.Errorf("video stream: %w (fallback also failed: %v)", err, err2)
		}
	}
	if _, err := downloadFile(det.AudioURL, audioTmp, biliReferer, dlClient, progressRange{0.90, 0.07}); err != nil {
		return res, fmt.Errorf("audio stream: %w", err)
	}
	emitProgress(0.97)
	if err := mergeAV(videoTmp, audioTmp, res.Video); err != nil {
		return res, fmt.Errorf("ffmpeg merge: %w", err)
	}
	res.Quality = det.Quality
	st, err := os.Stat(res.Video)
	if err != nil {
		return res, err
	}
	res.Bytes = st.Size()
	emitProgress(1.0)

	if it.CoverURL != "" {
		if _, err := downloadFile(it.CoverURL, res.Cover, biliReferer, dlClient); err != nil {
			fmt.Printf("  warning: cover download failed: %v\n", err)
			res.Cover = ""
		}
	}
	meta := struct {
		Platform  string `json:"platform"`
		Item      Item   `json:"item"`
		Quality   string `json:"quality"`
		FetchedAt string `json:"downloaded_at"`
	}{platformBilibili, it, det.Quality, time.Now().Format(time.RFC3339)}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(res.Meta, data, 0o644); err != nil {
		return res, fmt.Errorf("metadata: %w", err)
	}
	return res, nil
}

// biliFastURL rewrites an akamai URL to the Huawei Cloud mirror (measured an
// order of magnitude faster); other CDNs are left alone and the caller falls
// back to the original URL on failure.
func biliFastURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return u
	}
	if strings.Contains(parsed.Host, "akamaized.net") {
		parsed.Host = "upos-sz-mirrorhw.bilivideo.com"
	}
	return parsed.String()
}

// mergeAV losslessly remuxes audio and video with ffmpeg (-c copy).
func mergeAV(videoPath, audioPath, outPath string) error {
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-i", videoPath, "-i", audioPath,
		"-c", "copy", "-movflags", "+faststart", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, truncate(string(out), 200))
	}
	return nil
}
