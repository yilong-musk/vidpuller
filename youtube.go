package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const platformYouTube = "youtube"

var (
	youTubeURLRe = regexp.MustCompile(`(?i)(?:youtube\.com/(?:watch\?(?:[^#]*&)?v=|shorts/|embed/|live/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
	youTubeIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

// extractYouTubeID returns the 11-character video ID from a YouTube URL or a
// bare ID. It returns "" when the input is not a YouTube video reference.
func extractYouTubeID(s string) string {
	s = strings.TrimSpace(s)
	if m := youTubeURLRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if youTubeIDRe.MatchString(s) {
		return s
	}
	return ""
}

// isYouTubeURL reports whether s points at YouTube.
func isYouTubeURL(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "youtube.com/") || strings.Contains(lower, "youtu.be/")
}

// youtubeCookieFile is the --cookies path forwarded to yt-dlp.
var youtubeCookieFile string

// ytdlpBinary locates the yt-dlp executable. YT_DLP_BIN takes precedence,
// then PATH, then ~/.local/bin (where the service user installs it).
func ytdlpBinary() (string, error) {
	if v := strings.TrimSpace(os.Getenv("YT_DLP_BIN")); v != "" {
		if isExecutable(v) {
			return v, nil
		}
		return "", fmt.Errorf("YT_DLP_BIN is not executable: %s", v)
	}
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		return p, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "bin", "yt-dlp")
		if isExecutable(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("yt-dlp not found: install it, put it on PATH, or set YT_DLP_BIN")
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// youtubeArgs builds the yt-dlp command. The output template is resolved by
// yt-dlp; the Go side moves the resulting mp4 into the final directory.
func youtubeArgs(url, outDir, cookieFile string) []string {
	format := strings.TrimSpace(os.Getenv("YT_DLP_FORMAT"))
	if format == "" {
		format = "bv*+ba/b"
	}
	args := []string{
		"--no-playlist",
		"--newline",
		"--no-warnings",
		"--progress-template", "PROGRESS %(progress._percent_str)s",
		"-f", format,
		"--merge-output-format", "mp4",
		"--write-info-json",
		"-o", filepath.Join(outDir, "%(id)s.%(ext)s"),
	}
	if cookieFile != "" {
		args = append(args, "--cookies", cookieFile)
	}
	return append(args, url)
}

// parseYouTubeProgress parses a yt-dlp progress line such as
// "PROGRESS  42.3%". Values are fractions in [0,1].
func parseYouTubeProgress(line string) (float64, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "PROGRESS ") {
		return 0, false
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, "PROGRESS "))
	value = strings.TrimSuffix(value, "%")
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return f / 100, true
}

// lineSink collects command output line by line. It keeps the last few
// non-progress lines for error messages and forwards progress lines.
type lineSink struct {
	mu       sync.Mutex
	buf      strings.Builder
	onLine   func(string)
	tail     []string
	maxLines int
}

func (w *lineSink) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		text := w.buf.String()
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSuffix(text[:i], "\r")
		w.buf.Reset()
		w.buf.WriteString(text[i+1:])
		if w.onLine != nil {
			w.onLine(line)
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "PROGRESS ") {
			w.tail = append(w.tail, line)
			if w.maxLines > 0 && len(w.tail) > w.maxLines {
				w.tail = w.tail[len(w.tail)-w.maxLines:]
			}
		}
	}
	return len(p), nil
}

func (w *lineSink) lastLines() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.tail, "\n")
}

type youtubeInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Uploader  string `json:"uploader"`
	Channel   string `json:"channel"`
	Thumbnail string `json:"thumbnail"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	VCodec    string `json:"vcodec"`
	Format    string `json:"format"`
	Webpage   string `json:"webpage_url"`
}

func loadYouTubeInfo(dir string) youtubeInfo {
	var info youtubeInfo
	paths, _ := filepath.Glob(filepath.Join(dir, "*.info.json"))
	if len(paths) == 0 {
		return info
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		return info
	}
	_ = json.Unmarshal(data, &info)
	return info
}

func youtubeQuality(info youtubeInfo) string {
	if info.Height <= 0 {
		return strings.TrimSpace(info.Format)
	}
	quality := fmt.Sprintf("%dp", info.Height)
	if codec := strings.TrimSpace(strings.Split(info.VCodec, ".")[0]); codec != "" && codec != "none" {
		quality += " " + codec
	}
	return quality
}

// downloadYouTubeItem downloads one YouTube video through yt-dlp. YouTube
// extraction needs a full JavaScript-capable extractor, so the Go binary
// delegates it to the external yt-dlp tool while keeping the rest of the
// pipeline identical.
func downloadYouTubeItem(it Item, outRoot, cookieFile string) (downloadResult, error) {
	var res downloadResult
	id := extractYouTubeID(it.VideoID)
	if id == "" {
		id = extractYouTubeID(it.URL)
	}
	if id == "" {
		return res, fmt.Errorf("missing YouTube video ID")
	}
	videoURL := it.URL
	if !isYouTubeURL(videoURL) {
		videoURL = "https://www.youtube.com/watch?v=" + id
	}
	bin, err := ytdlpBinary()
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		return res, err
	}
	toolDir, err := os.MkdirTemp(outRoot, ".yt-dlp-")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(toolDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, youtubeArgs(videoURL, toolDir, cookieFile)...)
	sink := &lineSink{maxLines: 20, onLine: func(line string) {
		if f, ok := parseYouTubeProgress(line); ok {
			emitProgress(f)
		}
	}}
	cmd.Stdout = sink
	cmd.Stderr = sink
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(sink.lastLines())
		if detail == "" {
			return res, fmt.Errorf("yt-dlp: %w", err)
		}
		return res, fmt.Errorf("yt-dlp: %w: %s", err, truncate(detail, 500))
	}

	videos, _ := filepath.Glob(filepath.Join(toolDir, "*.mp4"))
	if len(videos) == 0 {
		return res, fmt.Errorf("yt-dlp produced no mp4 file")
	}
	video := videos[0]
	var videoSize int64
	for _, candidate := range videos {
		if st, err := os.Stat(candidate); err == nil && st.Size() > videoSize {
			video, videoSize = candidate, st.Size()
		}
	}
	info := loadYouTubeInfo(toolDir)
	if it.VideoID == "" {
		it.VideoID = id
	}
	if it.AwemeID == "" {
		it.AwemeID = id
	}
	if it.Desc == "" {
		it.Desc = info.Title
	}
	if it.Author == "" {
		it.Author = info.Uploader
	}
	if it.Author == "" {
		it.Author = info.Channel
	}
	if it.CoverURL == "" {
		it.CoverURL = info.Thumbnail
	}
	res.Quality = youtubeQuality(info)

	author := it.Author
	if author == "" {
		author = "unknown"
	}
	dir := filepath.Join(outRoot, platformYouTube, safeName(author))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	base := it.AwemeID + "_" + safeName(it.Desc)
	res.Video = filepath.Join(dir, base+".mp4")
	res.Cover = filepath.Join(dir, base+".jpg")
	res.Meta = filepath.Join(dir, base+".json")
	if err := os.Rename(video, res.Video); err != nil {
		return res, err
	}
	st, err := os.Stat(res.Video)
	if err != nil {
		return res, err
	}
	res.Bytes = st.Size()

	if it.CoverURL != "" {
		if _, err := downloadFile(it.CoverURL, res.Cover, "", nil); err != nil {
			fmt.Printf("  warning: cover download failed: %v\n", err)
			res.Cover = ""
		}
	}
	meta := struct {
		Platform  string `json:"platform"`
		Item      Item   `json:"item"`
		Quality   string `json:"quality"`
		FetchedAt string `json:"downloaded_at"`
	}{platformYouTube, it, res.Quality, time.Now().Format(time.RFC3339)}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(res.Meta, data, 0o644); err != nil {
		return res, fmt.Errorf("metadata: %w", err)
	}
	return res, nil
}
