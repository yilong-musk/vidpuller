package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TikTok 的网页在 Slardar WAF 后面：第一次响应是 ~1.4KB 的挑战页，
// <p id="cs"> 的 class 是 base64 JSON（v.a = seed，v.c = 目标摘要）。
// 找到 i 使 sha256(seed || i) 等于目标，把 d 拼回原 JSON（保持字段顺序），
// 再 base64 成 _wafchallengeid cookie 重试即可。纯标准库，不用浏览器。
var (
	reChallengeCS   = regexp.MustCompile(`(?s)<p[^>]*\bid="cs"[^>]*\bclass="([^"]+)"`)
	reChallengeName = regexp.MustCompile(`(?s)<p[^>]*\bid="wci"[^>]*\bclass="([^"]+)"`)
	reUniversalData = regexp.MustCompile(`(?s)<script[^>]*\bid="__UNIVERSAL_DATA_FOR_REHYDRATION__"[^>]*>(.*?)</script>`)
)

// flexInt64 兼容 TikTok 的两种写法：数字 1794753620 和字符串 "1794753620"。
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt64(n)
	return nil
}

// sessionCookies 是 --cookies 注入的会话 cookie（登录后可拿到更高画质档位）。
var sessionCookies []*http.Cookie

// applySessionCookies 把 --cookies 的条目塞进 client 的 jar。
func applySessionCookies(client *http.Client) {
	if len(sessionCookies) == 0 || client.Jar == nil {
		return
	}
	base, err := url.Parse("https://www.tiktok.com")
	if err != nil {
		return
	}
	client.Jar.SetCookies(base, sessionCookies)
}

// videoDetail 是单条视频解析结果，字段与时间线条目的通用字段对齐。
type videoDetail struct {
	Author     string
	Nickname   string
	Desc       string
	CreateTime int64
	DurationMS int
	Width      int
	Height     int
	IsTop      int
	PlayURL    string
	AudioURL   string // B 站 DASH 音频流（TikTok/抖音不用）
	CoverURL   string
	Quality    string      // 选中档位的人类可读描述，如 "1080p bytevc1 1037kbps"
	Renditions []rendition // 服务端下发的全部档位（由高到低处理）
	secUID     string
	id         string
}

// rendition 是一个可下载的画质档位。分辨率优先从 UrlKey 解析
// （形如 v12044gd…_bytevc1_1080p_1037801），比 PlayAddr 的 Width/Height 可靠。
type rendition struct {
	Gear    string
	URLKey  string
	Codec   string
	Width   int
	Height  int
	Bitrate int64
	URL     string
}

// runGet 执行 get 子命令（目前只支持 TikTok）。
func runGet(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "参数错误:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "用法: vidpuller get <TikTok 视频链接> [参数]")
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
	target := pos[0]
	if bvid, part := extractBVID(target); bvid != "" {
		det, err := fetchBilibili(bvid, part, sessionCookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "解析失败:", err)
			return 1
		}
		it := Item{AwemeID: det.ID(), Desc: det.Desc, Author: det.secUID, PlayURL: det.PlayURL, CoverURL: det.CoverURL}
		if !o.force {
			st := loadState(o.out + "/" + stateFile)
			if st.has(platformBilibili, it.AwemeID) {
				fmt.Println("已下载过，跳过（--force 可强制）")
				return 0
			}
		}
		res, err := downloadBilibiliItem(it, o.out, sessionCookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "下载失败:", err)
			return 1
		}
		st := loadState(o.out + "/" + stateFile)
		st.mark(platformBilibili, it, res.Video)
		if err := st.save(o.out + "/" + stateFile); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 写入 state 失败: %v\n", err)
		}
		quality := ""
		if det.Quality != "" {
			quality = "，" + det.Quality
		}
		fmt.Printf("完成 %s（%s%s）\n", res.Video, humanBytes(res.Bytes), quality)
		return 0
	}
	if strings.Contains(target, "douyin.com") || regexp.MustCompile(`^\d{15,}$`).MatchString(target) {
		fmt.Fprintln(os.Stderr, "抖音单条需要浏览器签名，纯 HTTP 拿不到无水印直链。")
		fmt.Fprintln(os.Stderr, "请改用: vidpuller download <timeline-url> --ids <aweme_id>")
		return 2
	}

	det, client, err := fetchTikTokVideo(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析失败:", err)
		return 1
	}
	it := Item{
		AwemeID:    det.ID(),
		Desc:       det.Desc,
		Author:     det.Author,
		CreateTime: det.CreateTime,
		DurationMS: det.DurationMS,
		Width:      det.Width,
		Height:     det.Height,
		IsTop:      det.IsTop,
		PlayURL:    det.PlayURL,
		CoverURL:   det.CoverURL,
	}
	if it.PlayURL == "" {
		fmt.Fprintln(os.Stderr, "这条没有视频直链（可能是图文帖）")
		return 1
	}
	if !o.force {
		st := loadState(o.out + "/" + stateFile)
		if st.has(platformTikTok, it.AwemeID) {
			fmt.Println("已下载过，跳过（--force 可强制）")
			return 0
		}
	}
	fmt.Printf("[tiktok] %s  %s\n", it.AwemeID, truncate(it.Desc, 50))
	res, err := downloadItem(it, platformTikTok, o.out, client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "下载失败:", err)
		return 1
	}
	st := loadState(o.out + "/" + stateFile)
	st.mark(platformTikTok, it, res.Video)
	if err := st.save(o.out + "/" + stateFile); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 写入增量账本失败: %v\n", err)
	}
	quality := ""
	if det.Quality != "" {
		quality = "，" + det.Quality
	}
	fmt.Printf("完成 %s（%s%s）\n", res.Video, humanBytes(res.Bytes), quality)
	return 0
}

// ID 从页面 URL 或详情里取 aweme_id；详情结构里不带 id 时回退到 URL。
func (d videoDetail) ID() string { return d.id }

// resolveTikTokPlayURL 用已有会话打开视频页，取当前会话签发的直链。
// 上游给的 play_url 是它自己的会话签发的，换一个 cookie jar 下载会 403，
// 所以批量模式要按作者/视频重新解析一次。
func resolveTikTokPlayURL(client *http.Client, author, id string) (videoDetail, *http.Client, error) {
	pageURL := "https://www.tiktok.com/@" + url.PathEscape(author) + "/video/" + id
	if client != nil {
		body, err := getPage(client, pageURL)
		if err == nil && reUniversalData.MatchString(body) {
			det, perr := parseTikTokVideoPage(body, pageURL)
			if perr == nil {
				return det, client, nil
			}
		}
	}
	return fetchTikTokVideo(pageURL)
}

// tiktokSessionFor 用作者主页建立一次带 cookie 的会话。
// TikTok 的 CDN 直链要求 ttwid/msToken/tt_chain_token，缺了就是 403，
// 批量下载时每个作者只需要建一次。
func tiktokSessionFor(author string) (*http.Client, error) {
	author = strings.TrimPrefix(strings.TrimSpace(author), "@")
	if author == "" {
		return nil, fmt.Errorf("缺少作者 handle，无法建立 TikTok 会话")
	}
	_, client, err := fetchTikTokPage("https://www.tiktok.com/@" + url.PathEscape(author))
	if err != nil {
		return nil, err
	}
	return client, nil
}

// fetchTikTokVideo 打开视频页（自动解 WAF），解析 webapp.video-detail。
// 返回的 client 带着这一轮的 cookie（ttwid/msToken/tt_chain_token），
// 下载直链时必须复用，否则 CDN 会回 403。
func fetchTikTokVideo(target string) (videoDetail, *http.Client, error) {
	var det videoDetail
	pageURL, err := resolveTikTokURL(target)
	if err != nil {
		return det, nil, err
	}
	body, client, err := fetchTikTokPage(pageURL)
	if err != nil {
		return det, nil, err
	}
	det, err = parseTikTokVideoPage(body, pageURL)
	return det, client, err
}

// parseTikTokVideoPage 从已取到的 HTML 里解析视频详情。
func parseTikTokVideoPage(body, pageURL string) (videoDetail, error) {
	var det videoDetail
	m := reUniversalData.FindStringSubmatch(body)
	if m == nil {
		return det, fmt.Errorf("页面里没有 __UNIVERSAL_DATA_FOR_REHYDRATION__")
	}
	var data struct {
		Scope map[string]json.RawMessage `json:"__DEFAULT_SCOPE__"`
	}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return det, fmt.Errorf("页面数据解析失败: %w", err)
	}
	raw, ok := data.Scope["webapp.video-detail"]
	if !ok {
		return det, fmt.Errorf("页面里没有视频详情（链接失效或需要登录）")
	}
	var vd struct {
		StatusCode int `json:"statusCode"`
		ItemInfo   struct {
			ItemStruct struct {
				ID         string    `json:"id"`
				Desc       string    `json:"desc"`
				CreateTime flexInt64 `json:"createTime"`
				Author     struct {
					UniqueID string `json:"uniqueId"`
					Nickname string `json:"nickname"`
					SecUID   string `json:"secUid"`
				} `json:"author"`
				Video struct {
					Duration    int    `json:"duration"`
					Width       int    `json:"width"`
					Height      int    `json:"height"`
					PlayAddr    string `json:"playAddr"`
					Cover       string `json:"cover"`
					OriginCover string `json:"originCover"`
					BitrateInfo []struct {
						Bitrate  int64  `json:"Bitrate"`
						GearName string `json:"GearName"`
						PlayAddr struct {
							Width   int      `json:"Width"`
							Height  int      `json:"Height"`
							URLKey  string   `json:"UrlKey"`
							URLList []string `json:"UrlList"`
						} `json:"PlayAddr"`
					} `json:"bitrateInfo"`
				} `json:"video"`
			} `json:"itemStruct"`
		} `json:"itemInfo"`
	}
	if err := json.Unmarshal(raw, &vd); err != nil {
		return det, fmt.Errorf("视频详情解析失败: %w", err)
	}
	if vd.StatusCode != 0 {
		return det, fmt.Errorf("TikTok 拒绝访问（statusCode=%d）：可能是地区限制或私密内容", vd.StatusCode)
	}
	is := vd.ItemInfo.ItemStruct
	rends := make([]rendition, 0, len(is.Video.BitrateInfo))
	for _, b := range is.Video.BitrateInfo {
		r := rendition{
			Gear:    b.GearName,
			URLKey:  b.PlayAddr.URLKey,
			Width:   b.PlayAddr.Width,
			Height:  b.PlayAddr.Height,
			Bitrate: b.Bitrate,
		}
		if len(b.PlayAddr.URLList) > 0 {
			r.URL = b.PlayAddr.URLList[0]
		}
		if codec, height, ok := parseURLKey(b.PlayAddr.URLKey); ok {
			r.Codec, r.Height = codec, height
		}
		rends = append(rends, r)
	}
	best, quality := pickBestRendition(rends, is.Video.PlayAddr)
	det = videoDetail{
		Author:     is.Author.UniqueID,
		Nickname:   is.Author.Nickname,
		Desc:       is.Desc,
		CreateTime: int64(is.CreateTime),
		DurationMS: is.Video.Duration * 1000,
		Width:      is.Video.Width,
		Height:     is.Video.Height,
		PlayURL:    best,
		Quality:    quality,
		Renditions: rends,
		CoverURL:   is.Video.OriginCover,
		secUID:     is.Author.SecUID,
		id:         is.ID,
	}
	if det.CoverURL == "" {
		det.CoverURL = is.Video.Cover
	}
	if det.id == "" {
		det.id = awemeIDFromURL(pageURL)
	}
	if det.id == "" {
		return det, fmt.Errorf("拿不到视频 ID")
	}
	return det, nil
}

// urlKeyRe 匹配 v12044gd…_<codec>_<height>p_<bitrate>。
var urlKeyRe = regexp.MustCompile(`_([a-z0-9]+)_(\d+)p_(\d+)$`)

// parseURLKey 从 UrlKey 里取出 codec 与真实高度。
func parseURLKey(key string) (codec string, height int, ok bool) {
	m := urlKeyRe.FindStringSubmatch(key)
	if m == nil {
		return "", 0, false
	}
	h, err := strconv.Atoi(m[2])
	if err != nil || h <= 0 {
		return "", 0, false
	}
	return m[1], h, true
}

// pickBestRendition 选分辨率最高、同分辨率下码率最高的档位；
// 全部没有 URL 时回退到 video.playAddr。
func pickBestRendition(rends []rendition, fallback string) (string, string) {
	best := rendition{}
	for _, r := range rends {
		if r.URL == "" {
			continue
		}
		if r.Height > best.Height || (r.Height == best.Height && r.Bitrate > best.Bitrate) {
			best = r
		}
	}
	if best.URL == "" {
		return fallback, ""
	}
	quality := fmt.Sprintf("%dp", best.Height)
	if best.Codec != "" {
		quality += " " + best.Codec
	}
	if best.Bitrate > 0 {
		quality += fmt.Sprintf(" %dkbps", best.Bitrate/1000)
	}
	return best.URL, quality
}

// resolveTikTokPlayURL 用已有会话打开视频页，取当前会话签发的直链。
// 上游给的 play_url 是它自己的会话签发的，换一个 cookie jar 下载会 403，
// 所以批量模式要按作者/视频重新解析一次。
var awemeIDRe = regexp.MustCompile(`/video/(\d{15,})`)

func awemeIDFromURL(u string) string {
	if m := awemeIDRe.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// resolveTikTokURL 把 vt.tiktok.com 短链跟随到最终视频页。
func resolveTikTokURL(target string) (string, error) {
	if awemeIDFromURL(target) != "" {
		return target, nil
	}
	if !strings.Contains(target, "tiktok.com") {
		return "", fmt.Errorf("不是 TikTok 链接: %s", target)
	}
	client := &http.Client{Timeout: 30 * time.Second}
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
	final := resp.Request.URL.String()
	if awemeIDFromURL(final) == "" {
		return "", fmt.Errorf("短链没有解析到视频页: %s", final)
	}
	return final, nil
}

// fetchTikTokPage 取页面；遇到 WAF 挑战就解一次再重试。
func fetchTikTokPage(target string) (string, *http.Client, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 40 * time.Second}
	applySessionCookies(client)
	body, err := getPage(client, target)
	if err != nil {
		return "", nil, err
	}
	if reUniversalData.MatchString(body) {
		return body, client, nil
	}
	name, value, err := solveWAFChallenge(body)
	if err != nil {
		return "", nil, err
	}
	base, err := url.Parse(target)
	if err != nil {
		return "", nil, err
	}
	jar.SetCookies(base, []*http.Cookie{{Name: name, Value: value, Path: "/", Domain: ".tiktok.com"}})
	body, err = getPage(client, target)
	if err != nil {
		return "", nil, err
	}
	if !reUniversalData.MatchString(body) {
		return "", nil, fmt.Errorf("挑战已解但页面仍被拦截（%d 字节）", len(body))
	}
	return body, client, nil
}

func getPage(client *http.Client, target string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// solveWAFChallenge 返回 (cookie 名, cookie 值)。
func solveWAFChallenge(page string) (string, string, error) {
	m := reChallengeCS.FindStringSubmatch(page)
	if m == nil {
		return "", "", fmt.Errorf("页面上没有找到 WAF 挑战数据")
	}
	raw := decodeB64(m[1])
	var ch struct {
		V struct {
			A string `json:"a"`
			C string `json:"c"`
		} `json:"v"`
	}
	if err := json.Unmarshal(raw, &ch); err != nil {
		return "", "", fmt.Errorf("挑战数据解析失败: %w", err)
	}
	seed, want := decodeB64(ch.V.A), decodeB64(ch.V.C)
	if len(seed) == 0 || len(want) != sha256.Size {
		return "", "", fmt.Errorf("挑战数据不完整")
	}
	answer := -1
	for i := 0; i <= 1_000_000; i++ {
		h := sha256.New()
		h.Write(seed)
		h.Write([]byte(strconv.Itoa(i)))
		if string(h.Sum(nil)) == string(want) {
			answer = i
			break
		}
	}
	if answer < 0 {
		return "", "", fmt.Errorf("挑战无解")
	}
	// 拼回原 JSON 并保留字段顺序，否则 cookie 值不对。
	text := strings.TrimSuffix(strings.TrimSpace(string(raw)), "}") +
		`,"d":"` + base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(answer))) + `"}`
	nameM := reChallengeName.FindStringSubmatch(page)
	if nameM == nil || nameM[1] == "" {
		return "", "", fmt.Errorf("挑战 cookie 名缺失")
	}
	return nameM[1], base64.StdEncoding.EncodeToString([]byte(text)), nil
}

func randomHex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}

func decodeB64(s string) []byte {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	b, _ := base64.RawStdEncoding.DecodeString(s)
	return b
}
