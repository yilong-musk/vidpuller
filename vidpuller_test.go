package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testChallenge builds a WAF challenge page with a known answer.
func testChallenge(answer int) string {
	seed := []byte("vidpuller-seed")
	digest := sha256.Sum256(append(seed, []byte(fmt.Sprint(answer))...))
	blob := fmt.Sprintf(`{"v":{"a":%q,"c":%q},"t":"test"}`,
		base64.StdEncoding.EncodeToString(seed),
		base64.StdEncoding.EncodeToString(digest[:]))
	return `<html><body>Please wait...<p id="cs" class="` +
		base64.StdEncoding.EncodeToString([]byte(blob)) +
		`"></p><p id="wci" class="_wafchallengeid"></p></body></html>`
}

func TestSolveWAFChallenge(t *testing.T) {
	name, val, err := solveWAFChallenge(testChallenge(11))
	if err != nil {
		t.Fatal(err)
	}
	if name != "_wafchallengeid" {
		t.Fatalf("cookie name = %q", name)
	}
	raw, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	d, _ := decoded["d"].(string)
	plain, _ := base64.StdEncoding.DecodeString(d)
	if string(plain) != "11" {
		t.Fatalf("d = %q, want answer 11", plain)
	}
	if !strings.HasPrefix(string(raw), `{"v":`) {
		t.Fatalf("field order changed: %s", raw)
	}
}

func TestSolveWAFChallengeRejectsGarbage(t *testing.T) {
	if _, _, err := solveWAFChallenge("<html>nothing</html>"); err == nil {
		t.Fatal("expected an error")
	}
}

func videoPageJSON(id, playURL string) string {
	page := map[string]any{
		"__DEFAULT_SCOPE__": map[string]any{
			"webapp.video-detail": map[string]any{
				"statusCode": 0,
				"itemInfo": map[string]any{
					"itemStruct": map[string]any{
						"id":         id,
						"desc":       "test caption",
						"createTime": 1794753620,
						"author":     map[string]any{"uniqueId": "nasa", "nickname": "NASA"},
						"video": map[string]any{
							"duration":    79,
							"width":       720,
							"height":      1280,
							"playAddr":    "https://cdn/default.mp4",
							"originCover": "https://cdn/origin.jpg",
							"bitrateInfo": []any{
								map[string]any{"Bitrate": 344361, "PlayAddr": map[string]any{"UrlList": []string{"https://cdn/540p.mp4"}}},
								map[string]any{"Bitrate": 1037801, "PlayAddr": map[string]any{"UrlList": []string{"https://cdn/1080p.mp4"}}},
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(page)
	return `<html><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">` + string(b) + `</script></html>`
}

func TestParseTikTokVideoPage(t *testing.T) {
	det, err := parseTikTokVideoPage(videoPageJSON("7000000000000000001", ""), "https://www.tiktok.com/@nasa/video/7000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if det.ID() != "7000000000000000001" || det.Author != "nasa" || det.Desc != "test caption" {
		t.Fatalf("detail = %+v", det)
	}
	if det.PlayURL != "https://cdn/1080p.mp4" {
		t.Fatalf("playURL = %q, want the highest bitrate", det.PlayURL)
	}
	if det.CoverURL != "https://cdn/origin.jpg" || det.DurationMS != 79000 {
		t.Fatalf("media fields = %+v", det)
	}
	if det.CreateTime != 1794753620 {
		t.Fatalf("createTime = %d", det.CreateTime)
	}
}

func TestParseTikTokVideoPageError(t *testing.T) {
	bad := `{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"statusCode":10204}}}`
	page := `<html><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">` + bad + `</script></html>`
	if _, err := parseTikTokVideoPage(page, "https://www.tiktok.com/@x/video/1"); err == nil {
		t.Fatal("non-zero statusCode should return an error")
	}
}

func TestLoadTimelineFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tl.json")
	raw := `{"source":"tiktok","author":"NASA","user":"nasa","items":[
	  {"aweme_id":"2","desc":"older","create_time":100,"play_url":"https://x/2.mp4"},
	  {"aweme_id":"3","desc":"newer","create_time":200,"play_url":"https://x/3.mp4"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	tl, err := loadTimeline(path)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Source != "tiktok" || len(tl.Items) != 2 {
		t.Fatalf("timeline = %+v", tl)
	}
	if tl.Items[0].AwemeID != "3" {
		t.Fatalf("not sorted newest-first: %+v", tl.Items)
	}
}

func TestLoadTimelineBadInput(t *testing.T) {
	if _, err := loadTimeline(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("a missing file should return an error")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte(`{"nope":1}`), 0o644)
	if _, err := loadTimeline(path); err == nil {
		t.Fatal("a timeline without items should return an error")
	}
}

func TestDetectPlatform(t *testing.T) {
	cases := []struct {
		source   string
		src      string
		override string
		want     string
	}{
		{"tiktok", "", "", platformTikTok},
		{"douyin", "", "", platformDouyin},
		{"", "https://example.com/api/v1/tiktok/timeline?user=demo", "", platformTikTok},
		{"", "https://example.com/api/v1/douyin/timeline?user=demo", "", platformDouyin},
		{"", "timeline.json", "", ""},
		{"tiktok", "", "douyin", platformDouyin},
	}
	for _, tc := range cases {
		tl := &Timeline{Source: tc.source}
		if got := detectPlatform(tl, tc.src, tc.override); got != tc.want {
			t.Fatalf("%+v -> %q, want %q", tc, got, tc.want)
		}
	}
}

func TestSelectItems(t *testing.T) {
	tl := &Timeline{Items: []Item{
		{AwemeID: "1", CreateTime: 100, PlayURL: "u1"},
		{AwemeID: "2", CreateTime: 200, PlayURL: "u2"},
		{AwemeID: "3", CreateTime: 0, PlayURL: "u3"},
	}}
	got := selectItems(tl, options{limit: 2})
	if len(got) != 2 || got[0].AwemeID != "1" {
		t.Fatalf("limit filter = %+v", got)
	}
	got = selectItems(tl, options{ids: []string{"2"}})
	if len(got) != 1 || got[0].AwemeID != "2" {
		t.Fatalf("ids filter = %+v", got)
	}
	since := time.Unix(150, 0)
	got = selectItems(tl, options{since: since})
	if len(got) != 2 || got[0].AwemeID != "2" {
		t.Fatalf("since filter = %+v", got)
	}
}

func TestStateLegacyDouyinKeys(t *testing.T) {
	st := &State{Items: map[string]StateItem{
		"123": {Path: "old.mp4"},
	}}
	if !st.has(platformDouyin, "123") {
		t.Fatal("legacy bare IDs should match as Douyin")
	}
	if st.has(platformTikTok, "123") {
		t.Fatal("TikTok must not match a legacy Douyin key")
	}
	st.mark(platformDouyin, Item{AwemeID: "456", Desc: "t"}, "/tmp/456.mp4")
	if !st.has(platformDouyin, "456") {
		t.Fatal("mark should make the item match")
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"a/b:c*d?e":         "a_b_c_d_e",
		"  many   spaces  ": "many spaces",
		"":                  "untitled",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Fatalf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadFileWritesAtomically(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.tiktok.com/" {
			t.Errorf("Referer = %q", r.Header.Get("Referer"))
		}
		w.Write([]byte("video-bytes"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.mp4")
	n, err := downloadFile(srv.URL, dest, refererFor(platformTikTok), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 11 {
		t.Fatalf("bytes = %d", n)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "video-bytes" {
		t.Fatalf("content = %q, err = %v", data, err)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part should have been renamed")
	}
}

func TestDownloadFileHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := downloadFile(srv.URL, filepath.Join(t.TempDir(), "b.mp4"), "", nil); err == nil {
		t.Fatal("403 should return an error")
	}
}

func TestDownloadItemWritesThreeFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v.mp4":
			w.Write([]byte("mp4"))
		case "/c.jpg":
			w.Write([]byte("jpg"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	out := t.TempDir()
	it := Item{AwemeID: "42", Desc: "title/with slash", Author: "nasa", PlayURL: srv.URL + "/v.mp4", CoverURL: srv.URL + "/c.jpg", CreateTime: 100}
	res, err := downloadItem(it, platformTikTok, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{res.Video, res.Cover, res.Meta} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing file %s: %v", p, err)
		}
	}
	if !strings.Contains(res.Video, filepath.Join("tiktok", "nasa")) {
		t.Fatalf("path = %s", res.Video)
	}
	var meta struct {
		Platform string `json:"platform"`
		Item     Item   `json:"item"`
	}
	data, _ := os.ReadFile(res.Meta)
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Platform != "tiktok" || meta.Item.AwemeID != "42" {
		t.Fatalf("metadata = %+v", meta)
	}
}

// Verify at compile time that there are no external module dependencies
// (the context import in this file is only a test placeholder).
var _ = context.Background

func TestParseURLKey(t *testing.T) {
	cases := []struct {
		key    string
		codec  string
		height int
		ok     bool
	}{
		{"v12044gd0000dbtm9jnog65o8drkgk5g_bytevc1_1080p_1037801", "bytevc1", 1080, true},
		{"v12044gd0000dbtm9jnog65o8drkgk5g_h264_720p_490821", "h264", 720, true},
		{"v12044gd0000dbtm9jnog65o8drkgk5g_bytevc1_540p_219682", "bytevc1", 540, true},
		{"", "", 0, false},
		{"no-key", "", 0, false},
	}
	for _, tc := range cases {
		codec, height, ok := parseURLKey(tc.key)
		if codec != tc.codec || height != tc.height || ok != tc.ok {
			t.Fatalf("parseURLKey(%q) = %q,%d,%v want %q,%d,%v", tc.key, codec, height, ok, tc.codec, tc.height, tc.ok)
		}
	}
}

func TestPickBestRenditionPrefersResolutionThenBitrate(t *testing.T) {
	rends := []rendition{
		{Gear: "adapt_540_1", Codec: "bytevc1", Height: 540, Bitrate: 219682, URL: "https://cdn/540.mp4"},
		{Gear: "normal_720_0", Codec: "h264", Height: 720, Bitrate: 490821, URL: "https://cdn/720a.mp4"},
		{Gear: "adapt_lower_720_1", Codec: "bytevc1", Height: 720, Bitrate: 275802, URL: "https://cdn/720b.mp4"},
		{Gear: "adapt_lowest_1080_1", Codec: "bytevc1", Height: 1080, Bitrate: 1037801, URL: "https://cdn/1080.mp4"},
	}
	url, quality := pickBestRendition(rends, "https://cdn/default.mp4")
	if url != "https://cdn/1080.mp4" {
		t.Fatalf("picked %q, want the 1080p rendition", url)
	}
	if quality != "1080p bytevc1 1037kbps" {
		t.Fatalf("quality = %q", quality)
	}
	// At the same resolution, pick the higher bitrate.
	only720 := []rendition{
		{Codec: "bytevc1", Height: 720, Bitrate: 275802, URL: "https://cdn/low.mp4"},
		{Codec: "h264", Height: 720, Bitrate: 490821, URL: "https://cdn/high.mp4"},
	}
	if url, _ := pickBestRendition(only720, ""); url != "https://cdn/high.mp4" {
		t.Fatalf("same resolution tie-break picked %q", url)
	}
	if url, q := pickBestRendition(nil, "https://cdn/default.mp4"); url != "https://cdn/default.mp4" || q != "" {
		t.Fatalf("fallback = %q / %q", url, q)
	}
}

func TestLoadCookieFileFormats(t *testing.T) {
	dir := t.TempDir()
	netscape := filepath.Join(dir, "cookies.txt")
	os.WriteFile(netscape, []byte("# Netscape HTTP Cookie File\n.tiktok.com\tTRUE\t/\tTRUE\t0\tsessionid\tabc123\n.tiktok.com\tTRUE\t/\tTRUE\t0\tttwid\txyz789\n"), 0o644)
	cks, err := loadCookieFile(netscape)
	if err != nil || len(cks) != 2 {
		t.Fatalf("netscape: %v %d", err, len(cks))
	}
	if cks[0].Name != "sessionid" || cks[0].Value != "abc123" {
		t.Fatalf("netscape cookie = %+v", cks[0])
	}

	jsonPath := filepath.Join(dir, "cookies.json")
	os.WriteFile(jsonPath, []byte(`[{"name":"sid_tt","value":"v1"},{"name":"sessionid","value":"v2"}]`), 0o644)
	cks, err = loadCookieFile(jsonPath)
	if err != nil || len(cks) != 2 || cks[0].Name != "sid_tt" {
		t.Fatalf("json: %v %+v", err, cks)
	}

	header := filepath.Join(dir, "header.txt")
	os.WriteFile(header, []byte("sessionid=a; ttwid=b; sid_tt=c"), 0o644)
	cks, err = loadCookieFile(header)
	if err != nil || len(cks) != 3 || cks[2].Name != "sid_tt" || cks[2].Value != "c" {
		t.Fatalf("header: %v %+v", err, cks)
	}

	if _, err := loadCookieFile(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("missing file should error")
	}
}

func TestExtractBVID(t *testing.T) {
	cases := []struct {
		in   string
		bvid string
		part int
	}{
		{"https://www.bilibili.com/video/BV1xxxxxxxxx", "BV1xxxxxxxxx", 1},
		{"https://www.bilibili.com/video/BV1xxxxxxxxx?p=3&spm=1", "BV1xxxxxxxxx", 3},
		{"BV1xxxxxxxxx", "BV1xxxxxxxxx", 1},
		{"https://www.bilibili.com/video/BV1xxxxxxxxx/?p=2", "BV1xxxxxxxxx", 2},
		{"https://example.com/watch?v=1", "", 1},
		{"https://www.douyin.com/video/123", "", 1},
	}
	for _, tc := range cases {
		bvid, part := extractBVID(tc.in)
		if bvid != tc.bvid || part != tc.part {
			t.Fatalf("extractBVID(%q) = %q,%d want %q,%d", tc.in, bvid, part, tc.bvid, tc.part)
		}
	}
}

func TestPickBiliVideoPrefersQualityThenAVC(t *testing.T) {
	streams := []biliStream{
		{ID: 32, CodeCID: 7, Height: 480, Bandwidth: 526754, BaseURL: "https://cdn/480avc.m4s"},
		{ID: 80, CodeCID: 12, Height: 1080, Bandwidth: 3123018, BaseURL: "https://cdn/1080hevc.m4s"},
		{ID: 112, CodeCID: 7, Height: 1080, Bandwidth: 5085196, BaseURL: "https://cdn/1080avc.m4s"},
		{ID: 112, CodeCID: 12, Height: 1080, Bandwidth: 3261002, BaseURL: "https://cdn/1080hevc2.m4s"},
		{ID: 120, CodeCID: 7, Height: 2160, Bandwidth: 0, BaseURL: ""}, // no URL, skip
		{ID: 80, CodeCID: 7, Height: 1080, Bandwidth: 999, BackupURL: []string{"https://cdn/backup.m4s"}},
	}
	best := pickBiliVideo(streams)
	if best == nil || best.BaseURL != "https://cdn/1080avc.m4s" {
		t.Fatalf("pickBiliVideo = %+v", best)
	}
	// At the same quality: AVC wins over HEVC even when HEVC has a higher bitrate.
	only := []biliStream{
		{ID: 80, CodeCID: 12, Height: 1080, Bandwidth: 9000, BaseURL: "https://cdn/hevc.m4s"},
		{ID: 80, CodeCID: 7, Height: 1080, Bandwidth: 1000, BaseURL: "https://cdn/avc.m4s"},
	}
	if best := pickBiliVideo(only); best.BaseURL != "https://cdn/avc.m4s" {
		t.Fatalf("codec preference: %+v", best)
	}
	if best := pickBiliVideo(nil); best != nil {
		t.Fatal("nil streams should pick nil")
	}
}

func TestPickBiliAudio(t *testing.T) {
	d := biliDash{Audio: []biliStream{
		{ID: 30216, Bandwidth: 65690, BaseURL: "https://cdn/a64.m4s"},
		{ID: 30280, Bandwidth: 79674, BaseURL: "https://cdn/a192.m4s"},
		{ID: 30232, Bandwidth: 70000, BaseURL: "https://cdn/a128.m4s"},
	}}
	if a := pickBiliAudio(d); a == nil || a.ID != 30280 {
		t.Fatalf("pickBiliAudio = %+v", a)
	}
	d.Flac.Audio = &biliStream{ID: 30250, Bandwidth: 500000, BaseURL: "https://cdn/flac.m4s"}
	if a := pickBiliAudio(d); a == nil || a.ID != 30250 {
		t.Fatalf("flac preference = %+v", a)
	}
}

func TestDetectPlatformFromItems(t *testing.T) {
	if p := detectPlatform(&Timeline{Items: []Item{{VideoID: "1", Platform: "tiktok"}}}, "timeline.json", ""); p != platformTikTok {
		t.Fatalf("item.platform = %q", p)
	}
	if p := detectPlatform(&Timeline{Items: []Item{{BVID: "BV1xxxxxxxxx"}}}, "timeline.json", ""); p != platformBilibili {
		t.Fatalf("bvid = %q", p)
	}
	if p := detectPlatform(&Timeline{Items: []Item{{VideoID: "1", PlayURL: "https://v.douyin.com/x"}}}, "timeline.json", ""); p != platformDouyin {
		t.Fatalf("play_url = %q", p)
	}
	if p := detectPlatform(&Timeline{Items: []Item{{VideoID: "1", PlayURL: "https://example.com/x.mp4"}}}, "timeline.json", ""); p != "" {
		t.Fatalf("unknown = %q", p)
	}
}

func TestGenericTimelineNormalize(t *testing.T) {
	raw := `{"platform":"tiktok","items":[
		{"video_id":"42","title":"hello","published_at":"2026-01-02T03:04:05Z","play_url":"https://example.com/42.mp4"}
	]}`
	var tl Timeline
	if err := json.Unmarshal([]byte(raw), &tl); err != nil {
		t.Fatal(err)
	}
	normalizeItems(&tl)
	it := tl.Items[0]
	if it.AwemeID != "42" || it.VideoID != "42" || it.Desc != "hello" {
		t.Fatalf("normalized = %+v", it)
	}
	want, _ := time.Parse(time.RFC3339, "2026-01-02T03:04:05Z")
	if it.CreateTime != want.Unix() {
		t.Fatalf("create_time = %d", it.CreateTime)
	}
	if tl.Source != platformTikTok {
		t.Fatalf("source = %q", tl.Source)
	}
}

func TestPlatformPerItem(t *testing.T) {
	tl := &Timeline{Source: "douyin", Items: []Item{
		{VideoID: "1", Platform: "tiktok", PlayURL: "https://example.com/1.mp4"},
		{VideoID: "BV1xxxxxxxxx", Platform: "bilibili", Title: "bili"},
	}}
	if p := platformForItem(tl, tl.Items[0], "timeline.json", ""); p != platformTikTok {
		t.Fatalf("item platform = %q", p)
	}
	if p := platformForItem(tl, tl.Items[1], "timeline.json", ""); p != platformBilibili {
		t.Fatalf("bili item platform = %q", p)
	}
	if p := platformForItem(tl, tl.Items[0], "timeline.json", "douyin"); p != platformDouyin {
		t.Fatalf("override = %q", p)
	}
	if label := platformLabel(tl, tl.Items, "timeline.json", ""); label != "mixed" {
		t.Fatalf("label = %q", label)
	}
	// With no top-level source and no per-item platform, a BV ID should be
	// detected as Bilibili.
	if p := platformForItem(&Timeline{}, Item{VideoID: "BV1xxxxxxxxx"}, "timeline.json", ""); p != platformBilibili {
		t.Fatalf("bvid inference = %q", p)
	}
}

func TestLoadTimelineShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"array", `[{"video_id":"1","play_url":"https://x/1.mp4"}]`, 1},
		{"data", `{"data":[{"video_id":"1","play_url":"https://x/1.mp4"}]}`, 1},
		{"empty array", `[]`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tl.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o644); err != nil {
				t.Fatal(err)
			}
			tl, err := loadTimeline(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(tl.Items) != tc.want {
				t.Fatalf("items = %d, want %d", len(tl.Items), tc.want)
			}
		})
	}
}

func TestNormalizeBilibiliTimeline(t *testing.T) {
	raw := `{"user":"123456789","title":"UP on Bilibili","items":[
		{"id":"BV1xxxxxxxxx","title":"lunch","url":"https://www.bilibili.com/video/BV1xxxxxxxxx",
		 "author":"test author","images":["https://i2.hdslb.com/cover.jpg"],"created_at":"2026-11-17T02:30:00Z"}
	]}`
	var tl Timeline
	if err := json.Unmarshal([]byte(raw), &tl); err != nil {
		t.Fatal(err)
	}
	normalizeItems(&tl)
	it := tl.Items[0]
	if it.AwemeID != "BV1xxxxxxxxx" || it.VideoID != "BV1xxxxxxxxx" || it.Desc != "lunch" || it.CoverURL != "https://i2.hdslb.com/cover.jpg" {
		t.Fatalf("normalized item = %+v", it)
	}
	if it.CreateTime != 1794882600 {
		t.Fatalf("create_time = %d", it.CreateTime)
	}
	if p := detectPlatform(&tl, "https://example.com/api/v1/bilibili?user=123456789", ""); p != platformBilibili {
		t.Fatalf("detectPlatform = %q", p)
	}
}
