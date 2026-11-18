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

// B 站下载：view 拿 cid → playurl 拿 DASH 流 → 下载音视频 → ffmpeg 合并。
//
// 实测（2026-11）：playurl 的 plain 接口不需要 WBI 签名；未登录最高 480p，
// 带 SESSDATA（--cookies）可拿 1080p/更高，最高档位由账号权限决定。
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
	Pic      string `json:"pic"`
	Duration int    `json:"duration"`
	Pubdate  int64  `json:"pubdate"`
	Owner    struct {
		Name string `json:"name"`
	} `json:"owner"`
	Pages []struct {
		CID  int64  `json:"cid"`
		Page int    `json:"page"`
		Part string `json:"part"`
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

// biliEnsureBuvid 给客户端刷新游客指纹（buvid3/buvid4），显著降低 412 概率。
// 每次调用都重新取，避免沿用 cookie 文件里的过期指纹。
func biliEnsureBuvid(client *http.Client) {
	base, _ := url.Parse("https://www.bilibili.com")
	// 先清掉旧指纹
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
		return nil, fmt.Errorf("B 站风控拦截（HTTP 412），稍后重试或带 --cookies")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 120))
	}
	return body, nil
}

// biliAPIResult 是 B 站通用响应外壳（code != 0 时 error 在 message 里）。
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
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("B 站接口错误 %d: %s", res.Code, res.Message)
	}
	return res.Data, nil
}

// extractBVID 从视频链接/短链里取 BVID，顺带解析 ?p= 分P。
func extractBVID(target string) (bvid string, part int) {
	part = 1
	target = strings.TrimSpace(target)
	// b23.tv 短链需要先展开
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

// resolveRedirect 跟随短链，返回最终地址。
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

// fetchBilibili 解析一条 B 站视频，返回最高档视频/音频地址与元信息。
func fetchBilibili(bvid string, part int, cookies []*http.Cookie) (videoDetail, error) {
	var det videoDetail
	if bvid == "" {
		return det, fmt.Errorf("不是有效的 B 站视频链接")
	}
	client := biliClient(cookies)
	referer := biliReferer + "video/" + bvid

	// 1) view：拿 cid / 标题 / 封面 / 分P
	raw, err := biliAPI(client, biliAPIBase+"/x/web-interface/view?bvid="+url.QueryEscape(bvid), referer)
	if err != nil {
		return det, err
	}
	var view biliView
	if err := json.Unmarshal(raw, &view); err != nil {
		return det, fmt.Errorf("解析 view 失败: %w", err)
	}
	if len(view.Pages) == 0 {
		return det, fmt.Errorf("视频没有可播放的分P")
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

	// 2) playurl：qn=127 请求最高，fnval=4048 要 DASH
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
		return det, fmt.Errorf("解析 playurl 失败: %w", err)
	}
	if len(play.Dash.Video) == 0 {
		return det, fmt.Errorf("没有可用的 DASH 视频流（可能需要登录）")
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
		return det, fmt.Errorf("没有可用的视频流")
	}
	if as == nil {
		return det, fmt.Errorf("没有可用的音频流")
	}
	det = videoDetail{
		PlayURL:  firstURL(*vs),
		AudioURL: firstURL(*as),
		CoverURL: view.Pic,
		Quality:  fmt.Sprintf("%dx%d %s %dkbps", vs.Width, vs.Height, codecName(vs.CodeCID), vs.Bandwidth/1000),
		Desc:     title,
		secUID:   view.Owner.Name, // 复用字段：B 站作者名
		id:       bvid,
	}
	return det, nil
}

// pickBiliVideo：清晰度 id 优先，同 id 下 AVC 优先，再比带宽。
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
	// AVC（7）兼容性最好；HEVC（12）/AV1（13）同清晰度下体积更小
	ap, bp := a.CodeCID == 7, b.CodeCID == 7
	if ap != bp {
		return ap
	}
	return a.Bandwidth > b.Bandwidth
}

// pickBiliAudio：优先 FLAC/杜比，其次按带宽。
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

// downloadBilibiliItem 下载音视频流并合并成 mp4。
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
	// 大文件下载不受 API 的 60s 超时约束（用无 Timeout 的 client + downloadFile 内部 30 分钟 ctx）。
	dlClient := &http.Client{Jar: client.Jar}
	if _, err := downloadFile(biliFastURL(det.PlayURL), videoTmp, biliReferer, dlClient, progressRange{0, 0.90}); err != nil {
		// 镜像不可用时回退原始 CDN
		if _, err2 := downloadFile(det.PlayURL, videoTmp+".retry", biliReferer, dlClient, progressRange{0, 0.90}); err2 == nil {
			os.Rename(videoTmp+".retry", videoTmp)
		} else {
			return res, fmt.Errorf("视频流: %w（回退也失败: %v）", err, err2)
		}
	}
	if _, err := downloadFile(det.AudioURL, audioTmp, biliReferer, dlClient, progressRange{0.90, 0.07}); err != nil {
		return res, fmt.Errorf("音频流: %w", err)
	}
	emitProgress(0.97)
	if err := mergeAV(videoTmp, audioTmp, res.Video); err != nil {
		return res, fmt.Errorf("ffmpeg 合并: %w", err)
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
			fmt.Printf("  警告: 封面下载失败: %v\n", err)
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
		return res, fmt.Errorf("元数据: %w", err)
	}
	return res, nil
}

// biliFastURL 把 akamai 的直链换到华为云镜像（实测快一个数量级）；
// 其它 CDN 不动，下载失败时调用方会回退原地址。
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

// mergeAV 用 ffmpeg 无损封装音视频（-c copy）。
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
