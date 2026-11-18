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

// TikTok pages sit behind the Slardar WAF: the first response is a ~1.4 KB
// challenge page, and <p id="cs"> carries base64 JSON whose v.a is the seed
// and v.c is the target digest. Find i such that sha256(seed || i) matches
// the target, append d to the original JSON while preserving field order,
// base64-encode the result as the _wafchallengeid cookie, and retry. This
// needs only the standard library, not a browser.
var (
	reChallengeCS   = regexp.MustCompile(`(?s)<p[^>]*\bid="cs"[^>]*\bclass="([^"]+)"`)
	reChallengeName = regexp.MustCompile(`(?s)<p[^>]*\bid="wci"[^>]*\bclass="([^"]+)"`)
	reUniversalData = regexp.MustCompile(`(?s)<script[^>]*\bid="__UNIVERSAL_DATA_FOR_REHYDRATION__"[^>]*>(.*?)</script>`)
)

// flexInt64 accepts both TikTok encodings: the number 1794753620 and the
// string "1794753620".
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

// sessionCookies holds cookies injected by --cookies; a logged-in session can
// unlock higher quality renditions.
var sessionCookies []*http.Cookie

// applySessionCookies adds the --cookies entries to the client's jar.
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

// videoDetail is the result of resolving one video; its fields align with the
// generic timeline item fields.
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
	AudioURL   string // Bilibili DASH audio stream (unused for TikTok/Douyin)
	CoverURL   string
	Quality    string      // human-readable selected rendition, e.g. "1080p bytevc1 1037kbps"
	Renditions []rendition // all renditions served, processed high to low
	secUID     string
	id         string
}

// rendition is one downloadable quality level. Resolution is parsed from
// UrlKey first (e.g. v12044gd…_bytevc1_1080p_1037801), which is more reliable
// than PlayAddr's Width/Height.
type rendition struct {
	Gear    string
	URLKey  string
	Codec   string
	Width   int
	Height  int
	Bitrate int64
	URL     string
}

// runGet executes the get subcommand (TikTok or Bilibili).
func runGet(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpuller get <video-url> [options]")
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
	target := pos[0]
	if bvid, part := extractBVID(target); bvid != "" {
		det, err := fetchBilibili(bvid, part, sessionCookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve:", err)
			return 1
		}
		it := Item{AwemeID: det.ID(), Desc: det.Desc, Author: det.secUID, PlayURL: det.PlayURL, CoverURL: det.CoverURL}
		if !o.force {
			st := loadState(o.out + "/" + stateFile)
			if st.has(platformBilibili, it.AwemeID) {
				fmt.Println("already downloaded, skip (use --force to download again)")
				return 0
			}
		}
		res, err := downloadBilibiliItem(it, o.out, sessionCookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "download failed:", err)
			return 1
		}
		st := loadState(o.out + "/" + stateFile)
		st.mark(platformBilibili, it, res.Video)
		if err := st.save(o.out + "/" + stateFile); err != nil {
			fmt.Fprintf(os.Stderr, "warning: save state: %v\n", err)
		}
		quality := ""
		if det.Quality != "" {
			quality = ", " + det.Quality
		}
		fmt.Printf("done %s (%s%s)\n", res.Video, humanBytes(res.Bytes), quality)
		return 0
	}
	if strings.Contains(target, "douyin.com") || regexp.MustCompile(`^\d{15,}$`).MatchString(target) {
		fmt.Fprintln(os.Stderr, "Douyin single-video URLs need a browser signature; pure HTTP cannot obtain a watermark-free stream.")
		fmt.Fprintln(os.Stderr, "Use: vidpuller download <timeline-url> --ids <aweme_id>")
		return 2
	}

	if id := extractYouTubeID(target); id != "" {
		it := Item{VideoID: id, AwemeID: id, URL: target}
		if !o.force {
			st := loadState(o.out + "/" + stateFile)
			if st.has(platformYouTube, it.AwemeID) {
				fmt.Println("already downloaded, skip (use --force to download again)")
				return 0
			}
		}
		fmt.Printf("[youtube] %s\n", it.AwemeID)
		res, err := downloadYouTubeItem(it, o.out, youtubeCookieFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "download failed:", err)
			return 1
		}
		st := loadState(o.out + "/" + stateFile)
		st.mark(platformYouTube, it, res.Video)
		if err := st.save(o.out + "/" + stateFile); err != nil {
			fmt.Fprintf(os.Stderr, "warning: save state: %v\n", err)
		}
		quality := ""
		if res.Quality != "" {
			quality = ", " + res.Quality
		}
		fmt.Printf("done %s (%s%s)\n", res.Video, humanBytes(res.Bytes), quality)
		return 0
	}

	det, client, err := fetchTikTokVideo(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve:", err)
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
		fmt.Fprintln(os.Stderr, "no video URL (possibly an image post)")
		return 1
	}
	if !o.force {
		st := loadState(o.out + "/" + stateFile)
		if st.has(platformTikTok, it.AwemeID) {
			fmt.Println("already downloaded, skip (use --force to download again)")
			return 0
		}
	}
	fmt.Printf("[tiktok] %s  %s\n", it.AwemeID, truncate(it.Desc, 50))
	res, err := downloadItem(it, platformTikTok, o.out, client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "download failed:", err)
		return 1
	}
	st := loadState(o.out + "/" + stateFile)
	st.mark(platformTikTok, it, res.Video)
	if err := st.save(o.out + "/" + stateFile); err != nil {
		fmt.Fprintf(os.Stderr, "warning: save state: %v\n", err)
	}
	quality := ""
	if det.Quality != "" {
		quality = ", " + det.Quality
	}
	fmt.Printf("done %s (%s%s)\n", res.Video, humanBytes(res.Bytes), quality)
	return 0
}

// ID returns the aweme_id from the page URL or detail payload; when the
// detail payload has no id, the URL is used.
func (d videoDetail) ID() string { return d.id }

// resolveTikTokPlayURL opens the video page with an existing session and
// returns a play URL signed by that session. The play_url from upstream is
// tied to its own cookie jar and returns 403 when downloaded with another, so
// batch mode re-resolves once per video.
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

// tiktokSessionFor creates one cookie-bearing session from the author's
// profile. TikTok CDN URLs require ttwid/msToken/tt_chain_token and return
// 403 without them; batch mode only needs one session per author.
func tiktokSessionFor(author string) (*http.Client, error) {
	author = strings.TrimPrefix(strings.TrimSpace(author), "@")
	if author == "" {
		return nil, fmt.Errorf("missing author handle; cannot create a TikTok session")
	}
	_, client, err := fetchTikTokPage("https://www.tiktok.com/@" + url.PathEscape(author))
	if err != nil {
		return nil, err
	}
	return client, nil
}

// fetchTikTokVideo opens a video page (solving the WAF when needed) and parses
// webapp.video-detail. The returned client carries this round's cookies
// (ttwid/msToken/tt_chain_token) and must be reused for the download, or the
// CDN returns 403.
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

// parseTikTokVideoPage parses video details from already-fetched HTML.
func parseTikTokVideoPage(body, pageURL string) (videoDetail, error) {
	var det videoDetail
	m := reUniversalData.FindStringSubmatch(body)
	if m == nil {
		return det, fmt.Errorf("page has no __UNIVERSAL_DATA_FOR_REHYDRATION__")
	}
	var data struct {
		Scope map[string]json.RawMessage `json:"__DEFAULT_SCOPE__"`
	}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return det, fmt.Errorf("parse page data: %w", err)
	}
	raw, ok := data.Scope["webapp.video-detail"]
	if !ok {
		return det, fmt.Errorf("page has no video details (expired link or login required)")
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
		return det, fmt.Errorf("parse video details: %w", err)
	}
	if vd.StatusCode != 0 {
		return det, fmt.Errorf("TikTok denied access (statusCode=%d): region-restricted or private content", vd.StatusCode)
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
		return det, fmt.Errorf("could not determine video ID")
	}
	return det, nil
}

// urlKeyRe matches v12044gd…_<codec>_<height>p_<bitrate>.
var urlKeyRe = regexp.MustCompile(`_([a-z0-9]+)_(\d+)p_(\d+)$`)

// parseURLKey extracts the codec and real height from UrlKey.
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

// pickBestRendition chooses the highest resolution, breaking ties by bitrate;
// when no rendition has a URL, it falls back to video.playAddr.
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

var awemeIDRe = regexp.MustCompile(`/video/(\d{15,})`)

func awemeIDFromURL(u string) string {
	if m := awemeIDRe.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// resolveTikTokURL follows a vt.tiktok.com short link to the final video page.
func resolveTikTokURL(target string) (string, error) {
	if awemeIDFromURL(target) != "" {
		return target, nil
	}
	if !strings.Contains(target, "tiktok.com") {
		return "", fmt.Errorf("not a TikTok URL: %s", target)
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
		return "", fmt.Errorf("short link did not resolve to a video page: %s", final)
	}
	return final, nil
}

// fetchTikTokPage fetches a page, solving the WAF challenge once and retrying
// when needed.
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
		return "", nil, fmt.Errorf("challenge solved but page is still blocked (%d bytes)", len(body))
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

// solveWAFChallenge returns (cookie name, cookie value).
func solveWAFChallenge(page string) (string, string, error) {
	m := reChallengeCS.FindStringSubmatch(page)
	if m == nil {
		return "", "", fmt.Errorf("no WAF challenge data on the page")
	}
	raw := decodeB64(m[1])
	var ch struct {
		V struct {
			A string `json:"a"`
			C string `json:"c"`
		} `json:"v"`
	}
	if err := json.Unmarshal(raw, &ch); err != nil {
		return "", "", fmt.Errorf("parse challenge data: %w", err)
	}
	seed, want := decodeB64(ch.V.A), decodeB64(ch.V.C)
	if len(seed) == 0 || len(want) != sha256.Size {
		return "", "", fmt.Errorf("challenge data is incomplete")
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
		return "", "", fmt.Errorf("challenge has no solution")
	}
	// Reassemble the original JSON while preserving field order; otherwise
	// the cookie value is wrong.
	text := strings.TrimSuffix(strings.TrimSpace(string(raw)), "}") +
		`,"d":"` + base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(answer))) + `"}`
	nameM := reChallengeName.FindStringSubmatch(page)
	if nameM == nil || nameM[1] == "" {
		return "", "", fmt.Errorf("challenge cookie name is missing")
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
