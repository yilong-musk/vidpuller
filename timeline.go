package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// Platform identifiers are also used as output directory names and state key
// prefixes.
const (
	platformDouyin = "douyin"
	platformTikTok = "tiktok"
)

// Item is the shared item shape for every platform's timeline. VideoID is the
// canonical field; AwemeID and BVID are legacy aliases normalized on load.
type Item struct {
	VideoID     string   `json:"video_id,omitempty"` // canonical video ID
	AwemeID     string   `json:"aweme_id,omitempty"` // Douyin/TikTok alias
	BVID        string   `json:"id,omitempty"`       // Bilibili BV ID alias
	Platform    string   `json:"platform,omitempty"` // optional per-item platform
	Title       string   `json:"title,omitempty"`    // generic title (Bilibili uses this)
	URL         string   `json:"url,omitempty"`      // video page URL (required by Bilibili)
	CreatedAt   string   `json:"created_at,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"` // generic publish-time alias
	Images      []string `json:"images,omitempty"`
	Desc        string   `json:"desc"`
	Author      string   `json:"author"`
	SecUID      string   `json:"sec_uid,omitempty"`
	CreateTime  int64    `json:"create_time"`
	DurationMS  int      `json:"duration_ms"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	IsTop       int      `json:"is_top"`
	PlayURL     string   `json:"play_url"`
	CoverURL    string   `json:"cover_url"`
	Quality     string   `json:"-"` // actual downloaded quality (filled after TikTok re-resolution)
}

// Timeline is a platform-neutral timeline. source and platform are
// equivalent; either can appear at the top level, or per item.
type Timeline struct {
	Source    string `json:"source,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Author    string `json:"author"`
	User      string `json:"user,omitempty"`
	SecUID    string `json:"sec_uid,omitempty"`
	FetchedAt string `json:"fetched_at"`
	Items     []Item `json:"items"`
	Data      []Item `json:"data,omitempty"` // alias for items, for common API wrappers
}

// loadTimeline reads a URL or a local file and returns the parsed timeline.
func loadTimeline(src string) (*Timeline, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("missing timeline URL or file")
	}
	var raw []byte
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		body, err := fetchURL(src)
		if err != nil {
			return nil, err
		}
		raw = []byte(body)
	} else {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read timeline file: %w", err)
		}
		raw = data
	}
	var tl Timeline
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(raw, &tl.Items); err != nil {
			return nil, fmt.Errorf("timeline is not valid JSON: %w", err)
		}
	} else if err := json.Unmarshal(raw, &tl); err != nil {
		return nil, fmt.Errorf("timeline is not valid JSON: %w", err)
	}
	if len(tl.Items) == 0 && len(tl.Data) > 0 {
		tl.Items = tl.Data
	}
	if len(tl.Items) == 0 && !strings.HasPrefix(trimmed, "[") &&
		!strings.Contains(trimmed, `"items"`) && !strings.Contains(trimmed, `"data"`) {
		return nil, fmt.Errorf("timeline has no items field")
	}
	normalizeItems(&tl)
	// Upstreams normally sort newest-first; sort again so a hand-edited local
	// file cannot reorder the queue.
	sort.SliceStable(tl.Items, func(i, j int) bool {
		return tl.Items[i].CreateTime > tl.Items[j].CreateTime
	})
	return &tl, nil
}

// normalizeItems aligns the platform-specific aliases with Item's canonical
// fields.
func normalizeItems(tl *Timeline) {
	if tl.Source == "" && tl.Platform != "" {
		tl.Source = tl.Platform
	}
	tl.Source = normalizePlatform(tl.Source)
	for i := range tl.Items {
		it := &tl.Items[i]
		it.Platform = normalizePlatform(it.Platform)
		if it.AwemeID == "" && it.VideoID != "" {
			it.AwemeID = it.VideoID
		}
		if it.AwemeID == "" && it.BVID != "" {
			it.AwemeID = it.BVID
		}
		if it.AwemeID == "" {
			if id := extractYouTubeID(it.VideoID); id != "" {
				it.AwemeID = id
			} else if id := extractYouTubeID(it.URL); id != "" {
				it.AwemeID = id
			}
		}
		if it.VideoID == "" && it.AwemeID != "" {
			it.VideoID = it.AwemeID
		}
		if it.Desc == "" && it.Title != "" {
			it.Desc = it.Title
		}
		if it.CoverURL == "" && len(it.Images) > 0 {
			it.CoverURL = it.Images[0]
		}
		if it.CreateTime == 0 {
			for _, raw := range []string{it.CreatedAt, it.PublishedAt} {
				if raw == "" {
					continue
				}
				if ts, err := time.Parse(time.RFC3339, raw); err == nil {
					it.CreateTime = ts.Unix()
					break
				}
			}
		}
	}
}

func fetchURL(url string) (string, error) {
	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch timeline: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("timeline returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return string(body), nil
}

// normalizePlatform returns a supported platform name, or "" for an unknown
// value.
func normalizePlatform(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case platformDouyin, platformTikTok, platformBilibili, platformYouTube:
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

// isBVID treats only BV1xxxxxxxxx-shaped IDs as Bilibili BV IDs, so numeric
// IDs in a generic timeline are not misclassified as Bilibili.
func isBVID(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) == 12 && strings.HasPrefix(s, "BV")
}

// platformFromSource infers the platform from the timeline URL only.
func platformFromSource(src string) string {
	lower := strings.ToLower(src)
	switch {
	case strings.Contains(lower, "/douyin/") || strings.Contains(lower, "douyin.com"):
		return platformDouyin
	case strings.Contains(lower, "/tiktok/") || strings.Contains(lower, "tiktok.com"):
		return platformTikTok
	case strings.Contains(lower, "/bilibili/") || strings.Contains(lower, "bilibili.com"):
		return platformBilibili
	case strings.Contains(lower, "youtube.com/") || strings.Contains(lower, "youtu.be/"):
		return platformYouTube
	}
	return ""
}

// platformFromItem infers the platform from one item: BV ID / video page URL,
// then the timeline URL, then media URL hosts.
func platformFromItem(it Item, src string) string {
	if isBVID(it.BVID) || isBVID(it.VideoID) || isBVID(it.AwemeID) ||
		strings.Contains(strings.ToLower(it.URL), "bilibili.com") {
		return platformBilibili
	}
	if isYouTubeURL(it.URL) {
		return platformYouTube
	}
	if p := platformFromSource(src); p != "" {
		return p
	}
	for _, u := range []string{it.PlayURL, it.CoverURL, it.URL} {
		lu := strings.ToLower(u)
		switch {
		case strings.Contains(lu, "douyin") || strings.Contains(lu, "douyinvod"):
			return platformDouyin
		case strings.Contains(lu, "tiktok"):
			return platformTikTok
		case strings.Contains(lu, "bilibili") || strings.Contains(lu, "bilivideo"):
			return platformBilibili
		}
	}
	return ""
}

// platformForItem decides the platform for one item:
// --platform > item.platform > timeline.source/platform > item features >
// timeline URL > media URLs. An empty result means the platform is unknown;
// callers report or skip the item instead of defaulting to a platform.
func platformForItem(tl *Timeline, it Item, src, override string) string {
	if s := normalizePlatform(override); s != "" {
		return s
	}
	if s := normalizePlatform(it.Platform); s != "" {
		return s
	}
	if s := normalizePlatform(tl.Source); s != "" {
		return s
	}
	return platformFromItem(it, src)
}

// detectPlatform decides the display platform for the whole timeline. A
// per-item platform takes precedence over the top-level source, so mixed
// timelines route correctly.
func detectPlatform(tl *Timeline, src, override string) string {
	if s := normalizePlatform(override); s != "" {
		return s
	}
	for _, it := range tl.Items {
		if s := normalizePlatform(it.Platform); s != "" {
			return s
		}
	}
	if s := normalizePlatform(tl.Source); s != "" {
		return s
	}
	for _, it := range tl.Items {
		if p := platformFromItem(it, src); p != "" {
			return p
		}
	}
	return platformFromSource(src)
}

// selectItems filters by --since / --ids / --limit. The limit counts only
// items that will actually be attempted.
func selectItems(tl *Timeline, o options) []Item {
	var out []Item
	for _, it := range tl.Items {
		if it.AwemeID == "" {
			continue
		}
		if !o.since.IsZero() && it.CreateTime > 0 && time.Unix(it.CreateTime, 0).Before(o.since) {
			continue
		}
		if len(o.ids) > 0 && !contains(o.ids, it.AwemeID) {
			continue
		}
		out = append(out, it)
	}
	if o.limit > 0 && len(out) > o.limit {
		out = out[:o.limit]
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
