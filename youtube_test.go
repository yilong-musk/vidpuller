package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractYouTubeID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/watch?feature=share&v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://example.com/watch?v=dQw4w9WgXcQ", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractYouTubeID(tc.in); got != tc.want {
			t.Fatalf("extractYouTubeID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestYouTubePlatformDetection(t *testing.T) {
	if p := platformFromSource("https://www.youtube.com/watch?v=dQw4w9WgXcQ"); p != platformYouTube {
		t.Fatalf("source = %q", p)
	}
	if p := platformFromItem(Item{URL: "https://youtu.be/dQw4w9WgXcQ"}, "timeline.json"); p != platformYouTube {
		t.Fatalf("item = %q", p)
	}
	tl := &Timeline{Items: []Item{{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}}}
	if p := platformForItem(tl, tl.Items[0], "timeline.json", ""); p != platformYouTube {
		t.Fatalf("platformForItem = %q", p)
	}
}

func TestYouTubeTimelineNormalization(t *testing.T) {
	raw := `{"items":[{"url":"https://youtu.be/dQw4w9WgXcQ","title":"YT","platform":"youtube"}]}`
	var tl Timeline
	if err := json.Unmarshal([]byte(raw), &tl); err != nil {
		t.Fatal(err)
	}
	normalizeItems(&tl)
	it := tl.Items[0]
	if it.AwemeID != "dQw4w9WgXcQ" || it.VideoID != "dQw4w9WgXcQ" || it.Desc != "YT" {
		t.Fatalf("normalized = %+v", it)
	}
}

func ytTestFormat(itag int, mime string, height int, bitrate int64, url string) ytFormat {
	return ytFormat{Itag: itag, MimeType: mime, Height: height, Bitrate: bitrate, URL: url}
}

func TestSelectYouTubeStreams(t *testing.T) {
	player := ytPlayer{}
	player.StreamingData.AdaptiveFormats = []ytFormat{
		ytTestFormat(137, `video/mp4; codecs="avc1.640028"`, 1080, 4_000_000, "v1080avc"),
		ytTestFormat(399, `video/mp4; codecs="av01.0.08M.08"`, 1080, 1_500_000, "v1080av1"),
		ytTestFormat(401, `video/mp4; codecs="av01.0.12M.08"`, 2160, 9_000_000, "v2160"),
		ytTestFormat(140, `audio/mp4; codecs="mp4a.40.2"`, 0, 130_000, "aac"),
		ytTestFormat(251, `audio/webm; codecs="opus"`, 0, 160_000, "opus"),
	}
	video, audio, combined, err := selectYouTubeStreams(player)
	if err != nil {
		t.Fatal(err)
	}
	if combined || video.URL != "v1080avc" || audio.URL != "aac" {
		t.Fatalf("selected video=%+v audio=%+v combined=%v", video, audio, combined)
	}
}

func TestSelectYouTubeStreamsAllows4K(t *testing.T) {
	old := youtubeMaxHeight
	youtubeMaxHeight = 2160
	defer func() { youtubeMaxHeight = old }()
	player := ytPlayer{}
	player.StreamingData.AdaptiveFormats = []ytFormat{
		ytTestFormat(137, `video/mp4; codecs="avc1.640028"`, 1080, 4_000_000, "v1080"),
		ytTestFormat(401, `video/mp4; codecs="av01.0.12M.08"`, 2160, 9_000_000, "v2160"),
		ytTestFormat(140, `audio/mp4; codecs="mp4a.40.2"`, 0, 130_000, "aac"),
	}
	video, audio, combined, err := selectYouTubeStreams(player)
	if err != nil {
		t.Fatal(err)
	}
	if combined || video.URL != "v2160" || audio.URL != "aac" {
		t.Fatalf("selected video=%+v audio=%+v combined=%v", video, audio, combined)
	}
}

func TestSelectYouTubeStreamsFallsBackTo720(t *testing.T) {
	player := ytPlayer{}
	player.StreamingData.AdaptiveFormats = []ytFormat{
		ytTestFormat(136, `video/mp4; codecs="avc1.4d401f"`, 720, 1_000_000, "v720"),
		ytTestFormat(140, `audio/mp4; codecs="mp4a.40.2"`, 0, 130_000, "aac"),
	}
	video, audio, combined, err := selectYouTubeStreams(player)
	if err != nil {
		t.Fatal(err)
	}
	if combined || video.URL != "v720" || audio.URL != "aac" {
		t.Fatalf("selected video=%+v audio=%+v combined=%v", video, audio, combined)
	}
}

func TestSelectYouTubeStreamsProgressiveFallback(t *testing.T) {
	player := ytPlayer{}
	player.StreamingData.Formats = []ytFormat{
		ytTestFormat(22, `video/mp4; codecs="avc1.64001F, mp4a.40.2"`, 720, 1_000_000, "progressive"),
	}
	video, audio, combined, err := selectYouTubeStreams(player)
	if err != nil {
		t.Fatal(err)
	}
	if !combined || video.URL != "progressive" || audio.URL != "" {
		t.Fatalf("selected video=%+v audio=%+v combined=%v", video, audio, combined)
	}
}

func TestSelectYouTubeStreamsRejectsCipherOnly(t *testing.T) {
	player := ytPlayer{}
	player.StreamingData.AdaptiveFormats = []ytFormat{
		{Itag: 137, MimeType: `video/mp4; codecs="avc1.640028"`, Height: 1080, SignatureCipher: "cipher"},
		ytTestFormat(140, `audio/mp4; codecs="mp4a.40.2"`, 0, 130_000, "aac"),
	}
	if _, _, _, err := selectYouTubeStreams(player); err == nil || !strings.Contains(err.Error(), "no YouTube video stream") {
		t.Fatalf("err = %v", err)
	}
}

func TestYouTubeCodec(t *testing.T) {
	cases := map[string]string{
		`video/mp4; codecs="avc1.640028"`:   "avc1",
		`video/mp4; codecs="av01.0.08M.08"`: "av01",
		`audio/mp4; codecs="mp4a.40.2"`:     "mp4a",
		`video/webm; codecs="vp9"`:          "vp9",
		"video/mp4":                         "",
	}
	for mime, want := range cases {
		if got := youtubeCodec(mime); got != want {
			t.Fatalf("youtubeCodec(%q) = %q, want %q", mime, got, want)
		}
	}
}
