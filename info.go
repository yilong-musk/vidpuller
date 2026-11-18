package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// videoPart is one page of a multi-part video.
type videoPart struct {
	Page       int    `json:"page"`
	CID        int64  `json:"cid"`
	Title      string `json:"title"`
	DurationMS int    `json:"duration_ms"`
}

// videoMeta is the platform-neutral metadata returned by `vidpuller info`.
type videoMeta struct {
	Platform    string      `json:"platform"`
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Author      string      `json:"author,omitempty"`
	AuthorID    string      `json:"author_id,omitempty"`
	DurationMS  int         `json:"duration_ms,omitempty"`
	Width       int         `json:"width,omitempty"`
	Height      int         `json:"height,omitempty"`
	PublishedAt string      `json:"published_at,omitempty"`
	CoverURL    string      `json:"cover_url,omitempty"`
	WebpageURL  string      `json:"webpage_url,omitempty"`
	Parts       []videoPart `json:"parts,omitempty"`
}

func runInfo(args []string) int {
	o, pos, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpuller info <video-url> [options]")
		return 2
	}
	if o.cookies != "" {
		cks, err := loadCookieFile(o.cookies)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read cookies:", err)
			return 2
		}
		sessionCookies = cks
	}
	meta, err := metadataForURL(pos[0], sessionCookies)
	if err != nil {
		fmt.Fprintln(os.Stderr, "metadata failed:", err)
		return 1
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode metadata:", err)
		return 1
	}
	fmt.Println(string(data))
	return 0
}

func metadataForURL(target string, cookies []*http.Cookie) (videoMeta, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return videoMeta{}, fmt.Errorf("missing video URL")
	}
	if bvid, _ := extractBVID(target); bvid != "" {
		return bilibiliMeta(bvid, cookies)
	}
	if id := extractYouTubeID(target); id != "" {
		return youtubeMeta(id, cookies)
	}
	if strings.Contains(strings.ToLower(target), "douyin.com") {
		return videoMeta{}, fmt.Errorf("Douyin single-video metadata needs a browser signature; read it from a timeline item instead")
	}
	if strings.Contains(strings.ToLower(target), "tiktok.com") {
		return tiktokMeta(target)
	}
	return videoMeta{}, fmt.Errorf("unsupported video URL: %s", target)
}

func bilibiliMeta(bvid string, cookies []*http.Cookie) (videoMeta, error) {
	client := biliClient(cookies)
	referer := biliReferer + "video/" + bvid
	raw, err := biliAPI(client, biliAPIBase+"/x/web-interface/view?bvid="+url.QueryEscape(bvid), referer)
	if err != nil {
		return videoMeta{}, err
	}
	var view biliView
	if err := json.Unmarshal(raw, &view); err != nil {
		return videoMeta{}, fmt.Errorf("parse view: %w", err)
	}
	meta := videoMeta{
		Platform:    platformBilibili,
		ID:          bvid,
		Title:       view.Title,
		Description: view.Desc,
		Author:      view.Owner.Name,
		AuthorID:    strconv.FormatInt(view.Owner.MID, 10),
		DurationMS:  view.Duration * 1000,
		CoverURL:    view.Pic,
		WebpageURL:  "https://www.bilibili.com/video/" + bvid,
		PublishedAt: time.Unix(view.Pubdate, 0).Format(time.RFC3339),
	}
	for _, page := range view.Pages {
		meta.Parts = append(meta.Parts, videoPart{
			Page:       page.Page,
			CID:        page.CID,
			Title:      page.Part,
			DurationMS: int(page.Duration) * 1000,
		})
	}
	return meta, nil
}

func youtubeMeta(id string, cookies []*http.Cookie) (videoMeta, error) {
	player, err := fetchYouTubePlayer(id, cookies)
	if err != nil {
		return videoMeta{}, err
	}
	meta := videoMeta{
		Platform:    platformYouTube,
		ID:          id,
		Title:       player.VideoDetails.Title,
		Description: player.VideoDetails.ShortDescription,
		Author:      player.VideoDetails.Author,
		AuthorID:    player.VideoDetails.ChannelID,
		CoverURL:    youtubeThumbnail(player),
		WebpageURL:  "https://www.youtube.com/watch?v=" + id,
	}
	if seconds, err := strconv.Atoi(player.VideoDetails.LengthSeconds); err == nil {
		meta.DurationMS = seconds * 1000
	}
	if ts, err := time.Parse(time.RFC3339, player.Microformat.PlayerMicroformatRenderer.PublishDate); err == nil {
		meta.PublishedAt = ts.Format(time.RFC3339)
	}
	for _, f := range player.StreamingData.AdaptiveFormats {
		if strings.HasPrefix(strings.ToLower(f.MimeType), "video/") && f.Height > meta.Height {
			meta.Width, meta.Height = f.Width, f.Height
		}
	}
	return meta, nil
}

func tiktokMeta(target string) (videoMeta, error) {
	det, _, err := fetchTikTokVideo(target)
	if err != nil {
		return videoMeta{}, err
	}
	meta := videoMeta{
		Platform:    platformTikTok,
		ID:          det.ID(),
		Title:       det.Desc,
		Description: det.Desc,
		Author:      det.Author,
		AuthorID:    det.secUID,
		DurationMS:  det.DurationMS,
		Width:       det.Width,
		Height:      det.Height,
		CoverURL:    det.CoverURL,
		WebpageURL:  target,
	}
	if det.CreateTime > 0 {
		meta.PublishedAt = time.Unix(det.CreateTime, 0).Format(time.RFC3339)
	}
	return meta, nil
}
