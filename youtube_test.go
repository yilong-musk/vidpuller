package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestParseYouTubeProgress(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"PROGRESS  42.3%", 0.423, true},
		{"PROGRESS 100.0%", 1.0, true},
		{"[download] 10%", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseYouTubeProgress(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("parseYouTubeProgress(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestYouTubeArgs(t *testing.T) {
	args := strings.Join(youtubeArgs("https://youtu.be/x", "/tmp/out", "/tmp/cookies.txt"), " ")
	for _, want := range []string{
		"--no-playlist",
		"--write-info-json",
		"--merge-output-format mp4",
		"-o /tmp/out/%(id)s.%(ext)s",
		"--cookies /tmp/cookies.txt",
		"https://youtu.be/x",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q: %s", want, args)
		}
	}
}

func TestDownloadYouTubeItemWithFakeTool(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "yt-dlp")
	script := `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
dir=$(dirname "$out")
printf '%s' '{"id":"abc12345678","title":"Fake video","uploader":"Fake author","height":720,"vcodec":"avc1.4d401f"}' > "$dir/abc12345678.info.json"
printf 'fake-mp4-data' > "$dir/abc12345678.mp4"
printf 'PROGRESS  50.0%%\n'
printf 'PROGRESS 100.0%%\n'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YT_DLP_BIN", fake)

	out := t.TempDir()
	res, err := downloadYouTubeItem(Item{
		VideoID: "abc12345678",
		URL:     "https://www.youtube.com/watch?v=abc12345678",
	}, out, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Video, filepath.Join("youtube", "Fake author")) {
		t.Fatalf("video path = %s", res.Video)
	}
	if res.Bytes != int64(len("fake-mp4-data")) {
		t.Fatalf("bytes = %d", res.Bytes)
	}
	if res.Quality != "720p avc1" {
		t.Fatalf("quality = %q", res.Quality)
	}
	data, err := os.ReadFile(res.Meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"platform": "youtube"`) || !strings.Contains(string(data), "Fake video") {
		t.Fatalf("metadata = %s", data)
	}
}
