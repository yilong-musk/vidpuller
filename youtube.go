package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	platformYouTube  = "youtube"
	youtubeMaxHeight = 1080
	youtubeUA        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_7_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15"
)

var (
	youTubeURLRe = regexp.MustCompile(`(?i)(?:youtube\.com/(?:watch\?(?:[^#]*&)?v=|shorts/|embed/|live/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
	youTubeIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

	reYouTubeAPIKey  = regexp.MustCompile(`"INNERTUBE_API_KEY"\s*:\s*"([^"]+)"`)
	reYouTubeVisitor = regexp.MustCompile(`"VISITOR_DATA"\s*:\s*"([^"]+)"`)
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

type ytThumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type ytFormat struct {
	Itag            int    `json:"itag"`
	URL             string `json:"url"`
	SignatureCipher string `json:"signatureCipher"`
	MimeType        string `json:"mimeType"`
	QualityLabel    string `json:"qualityLabel"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Bitrate         int64  `json:"bitrate"`
	ContentLength   string `json:"contentLength"`
}

type ytPlayer struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"playabilityStatus"`
	StreamingData struct {
		AdaptiveFormats []ytFormat `json:"adaptiveFormats"`
		Formats         []ytFormat `json:"formats"`
	} `json:"streamingData"`
	VideoDetails struct {
		VideoID       string `json:"videoId"`
		Title         string `json:"title"`
		Author        string `json:"author"`
		ChannelID     string `json:"channelId"`
		LengthSeconds string `json:"lengthSeconds"`
		Thumbnail     struct {
			Thumbnails []ytThumbnail `json:"thumbnails"`
		} `json:"thumbnail"`
	} `json:"videoDetails"`
	Microformat struct {
		PlayerMicroformatRenderer struct {
			PublishDate      string `json:"publishDate"`
			OwnerChannelName string `json:"ownerChannelName"`
		} `json:"playerMicroformatRenderer"`
	} `json:"microformat"`
}

// youtubeCredentials caches the public InnerTube key and visitor ID parsed
// from a watch page; both are stable for the process lifetime.
var youtubeCredentials struct {
	sync.Mutex
	key     string
	visitor string
}

func youtubeHTTPClient(cookies []*http.Cookie) *http.Client {
	jar, _ := cookiejar.New(nil)
	if len(cookies) > 0 {
		base, _ := url.Parse("https://www.youtube.com")
		jar.SetCookies(base, cookies)
	}
	return &http.Client{Jar: jar, Timeout: 30 * time.Second}
}

func fetchYouTubePageCreds(client *http.Client, id string) (string, string, error) {
	youtubeCredentials.Lock()
	defer youtubeCredentials.Unlock()
	if youtubeCredentials.key != "" {
		return youtubeCredentials.key, youtubeCredentials.visitor, nil
	}
	watchURL := "https://www.youtube.com/watch?v=" + url.QueryEscape(id) + "&hl=en&gl=US"
	req, err := http.NewRequest(http.MethodGet, watchURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", youtubeUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch YouTube page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("YouTube page returned HTTP %d", resp.StatusCode)
	}
	key := ""
	if m := reYouTubeAPIKey.FindSubmatch(body); m != nil {
		key = string(m[1])
	}
	if key == "" {
		return "", "", fmt.Errorf("YouTube page did not expose INNERTUBE_API_KEY (bot check or consent page?)")
	}
	visitor := ""
	if m := reYouTubeVisitor.FindSubmatch(body); m != nil {
		visitor = string(m[1])
	}
	youtubeCredentials.key, youtubeCredentials.visitor = key, visitor
	return key, visitor, nil
}

// fetchYouTubePlayer asks the InnerTube player endpoint using the VISIONOS
// client. That client exposes direct adaptive stream URLs without signature
// ciphers or n-sig for the formats used here.
func fetchYouTubePlayer(id string, cookies []*http.Cookie) (ytPlayer, error) {
	var player ytPlayer
	client := youtubeHTTPClient(cookies)
	key, visitor, err := fetchYouTubePageCreds(client, id)
	if err != nil {
		return player, err
	}
	clientContext := map[string]any{
		"clientName":    "VISIONOS",
		"clientVersion": "1.02",
		"deviceMake":    "Apple",
		"deviceModel":   "RealityDevice17,1",
		"userAgent":     youtubeUA,
		"osName":        "visionOS",
		"osVersion":     "26.5.23O471",
		"hl":            "en",
		"gl":            "US",
	}
	if visitor != "" {
		clientContext["visitorData"] = visitor
	}
	payload, err := json.Marshal(map[string]any{
		"context":        map[string]any{"client": clientContext},
		"videoId":        id,
		"contentCheckOk": true,
		"racyCheckOk":    true,
	})
	if err != nil {
		return player, err
	}
	apiURL := "https://www.youtube.com/youtubei/v1/player?key=" + url.QueryEscape(key) + "&prettyPrint=false"
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return player, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", youtubeUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("Referer", "https://www.youtube.com/watch?v="+url.QueryEscape(id))
	req.Header.Set("X-Youtube-Client-Name", "101")
	req.Header.Set("X-Youtube-Client-Version", "1.02")
	if visitor != "" {
		req.Header.Set("X-Goog-Visitor-Id", visitor)
	}
	resp, err := client.Do(req)
	if err != nil {
		return player, fmt.Errorf("YouTube player request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return player, err
	}
	if resp.StatusCode != http.StatusOK {
		return player, fmt.Errorf("YouTube player returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, &player); err != nil {
		return player, fmt.Errorf("parse YouTube player response: %w", err)
	}
	if player.PlayabilityStatus.Status != "OK" {
		reason := strings.TrimSpace(player.PlayabilityStatus.Reason)
		if reason == "" {
			reason = player.PlayabilityStatus.Status
		}
		return player, fmt.Errorf("YouTube unavailable (%s): %s", player.PlayabilityStatus.Status, reason)
	}
	return player, nil
}

func youtubeCodec(mime string) string {
	const marker = `codecs="`
	i := strings.Index(mime, marker)
	if i < 0 {
		return ""
	}
	codec := mime[i+len(marker):]
	if j := strings.IndexAny(codec, `",`); j >= 0 {
		codec = codec[:j]
	}
	return strings.Split(codec, ".")[0]
}

func youtubeVideoRank(f ytFormat) int {
	mime := strings.ToLower(f.MimeType)
	if strings.HasPrefix(mime, "video/mp4") {
		if strings.Contains(mime, "avc1") {
			return 3
		}
		return 2
	}
	if strings.HasPrefix(mime, "video/webm") {
		return 1
	}
	return 0
}

func youtubeAudioRank(f ytFormat) int {
	mime := strings.ToLower(f.MimeType)
	if strings.HasPrefix(mime, "audio/mp4") {
		return 2
	}
	if strings.HasPrefix(mime, "audio/webm") {
		return 1
	}
	return 0
}

// selectYouTubeStreams picks the best video up to youtubeMaxHeight and the
// best audio. A progressive format is returned as combined when no adaptive
// pair exists.
func selectYouTubeStreams(player ytPlayer) (video, audio ytFormat, combined bool, err error) {
	all := append(append([]ytFormat{}, player.StreamingData.AdaptiveFormats...), player.StreamingData.Formats...)
	var bestVideo, bestAudio *ytFormat
	for i := range all {
		f := &all[i]
		if f.URL == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(f.MimeType), "video/") {
			if f.Height <= 0 || f.Height > youtubeMaxHeight {
				continue
			}
			if bestVideo == nil ||
				f.Height > bestVideo.Height ||
				(f.Height == bestVideo.Height && youtubeVideoRank(*f) > youtubeVideoRank(*bestVideo)) ||
				(f.Height == bestVideo.Height && youtubeVideoRank(*f) == youtubeVideoRank(*bestVideo) && f.Bitrate > bestVideo.Bitrate) {
				bestVideo = f
			}
		}
		if strings.HasPrefix(strings.ToLower(f.MimeType), "audio/") {
			if bestAudio == nil ||
				youtubeAudioRank(*f) > youtubeAudioRank(*bestAudio) ||
				(youtubeAudioRank(*f) == youtubeAudioRank(*bestAudio) && f.Bitrate > bestAudio.Bitrate) {
				bestAudio = f
			}
		}
	}
	if bestVideo != nil && bestAudio != nil {
		return *bestVideo, *bestAudio, false, nil
	}
	// Progressive fallback: one URL that carries both video and audio.
	for i := range player.StreamingData.Formats {
		f := player.StreamingData.Formats[i]
		mime := strings.ToLower(f.MimeType)
		if f.URL == "" || !strings.HasPrefix(mime, "video/mp4") || !strings.Contains(mime, "mp4a") {
			continue
		}
		if f.Height <= 0 || f.Height > youtubeMaxHeight {
			continue
		}
		if video.URL == "" || f.Height > video.Height || (f.Height == video.Height && f.Bitrate > video.Bitrate) {
			video = f
		}
	}
	if video.URL != "" {
		return video, ytFormat{}, true, nil
	}
	if bestVideo == nil {
		return video, audio, false, fmt.Errorf("no YouTube video stream up to %dp", youtubeMaxHeight)
	}
	if bestAudio == nil {
		return video, audio, false, fmt.Errorf("no YouTube audio stream available")
	}
	return video, audio, false, fmt.Errorf("no playable YouTube stream up to %dp", youtubeMaxHeight)
}

func youtubeThumbnail(player ytPlayer) string {
	best := ""
	bestWidth := -1
	for _, t := range player.VideoDetails.Thumbnail.Thumbnails {
		if t.URL != "" && t.Width > bestWidth {
			best, bestWidth = t.URL, t.Width
		}
	}
	return best
}

// downloadYouTubeItem downloads one YouTube video using the native InnerTube
// extractor and merges adaptive streams with ffmpeg.
func downloadYouTubeItem(it Item, outRoot string, cookies []*http.Cookie) (downloadResult, error) {
	var res downloadResult
	id := extractYouTubeID(it.VideoID)
	if id == "" {
		id = extractYouTubeID(it.URL)
	}
	if id == "" {
		return res, fmt.Errorf("missing YouTube video ID")
	}
	player, err := fetchYouTubePlayer(id, cookies)
	if err != nil {
		return res, err
	}
	video, audio, combined, err := selectYouTubeStreams(player)
	if err != nil {
		return res, err
	}

	if it.VideoID == "" {
		it.VideoID = id
	}
	if it.AwemeID == "" {
		it.AwemeID = id
	}
	if it.Desc == "" {
		it.Desc = player.VideoDetails.Title
	}
	if it.Author == "" {
		it.Author = player.VideoDetails.Author
	}
	if it.Author == "" {
		it.Author = player.VideoDetails.ChannelID
	}
	if it.CoverURL == "" {
		it.CoverURL = youtubeThumbnail(player)
	}
	if it.DurationMS == 0 {
		if seconds, err := strconv.Atoi(player.VideoDetails.LengthSeconds); err == nil {
			it.DurationMS = seconds * 1000
		}
	}
	if it.CreateTime == 0 {
		if ts, err := time.Parse(time.RFC3339, player.Microformat.PlayerMicroformatRenderer.PublishDate); err == nil {
			it.CreateTime = ts.Unix()
		}
	}
	res.Quality = fmt.Sprintf("%dp", video.Height)
	if codec := youtubeCodec(video.MimeType); codec != "" {
		res.Quality += " " + codec
	}

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

	client := youtubeHTTPClient(cookies)
	dlClient := &http.Client{Jar: client.Jar}
	referer := "https://www.youtube.com/"
	if combined {
		if _, err := downloadFile(video.URL, res.Video, referer, dlClient, progressRange{0, 0.97}); err != nil {
			return res, fmt.Errorf("YouTube video: %w", err)
		}
	} else {
		videoTmp := res.Video + ".video"
		audioTmp := res.Video + ".audio"
		defer os.Remove(videoTmp)
		defer os.Remove(audioTmp)
		if _, err := downloadFile(video.URL, videoTmp, referer, dlClient, progressRange{0, 0.85}); err != nil {
			return res, fmt.Errorf("YouTube video: %w", err)
		}
		if _, err := downloadFile(audio.URL, audioTmp, referer, dlClient, progressRange{0.85, 0.10}); err != nil {
			return res, fmt.Errorf("YouTube audio: %w", err)
		}
		emitProgress(0.97)
		if err := mergeAV(videoTmp, audioTmp, res.Video); err != nil {
			return res, fmt.Errorf("ffmpeg merge: %w", err)
		}
	}
	emitProgress(1.0)
	st, err := os.Stat(res.Video)
	if err != nil {
		return res, err
	}
	res.Bytes = st.Size()

	if it.CoverURL != "" {
		if _, err := downloadFile(it.CoverURL, res.Cover, "", client); err != nil {
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
