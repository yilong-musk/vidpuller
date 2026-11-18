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

// 平台标识：同时也是输出目录名和 state 键前缀。
const (
	platformDouyin = "douyin"
	platformTikTok = "tiktok"
)

// Item 是各平台 timeline 共用的条目结构。VideoID 是通用字段；
// AwemeID 和 BVID 是历史字段名，解析时会自动归一。
type Item struct {
	VideoID     string   `json:"video_id,omitempty"` // 通用作品 ID（推荐）
	AwemeID     string   `json:"aweme_id,omitempty"` // 抖音/TikTok 兼容字段
	BVID        string   `json:"id,omitempty"`       // B 站 BV 号兼容字段
	Platform    string   `json:"platform,omitempty"` // 逐条平台（可选）
	Title       string   `json:"title,omitempty"`    // 通用标题（B 站常用）
	URL         string   `json:"url,omitempty"`      // 视频页地址（B 站需要）
	CreatedAt   string   `json:"created_at,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"` // 通用发布时间别名
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
	Quality     string   `json:"-"` // 实际下载档位（TikTok 重解析后填）
}

// Timeline 是一份平台无关的时间线。source 和 platform 等价，
// 可以放在顶层，也可以逐条放在 Item.Platform 里。
type Timeline struct {
	Source    string `json:"source,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Author    string `json:"author"`
	User      string `json:"user,omitempty"`
	SecUID    string `json:"sec_uid,omitempty"`
	FetchedAt string `json:"fetched_at"`
	Items     []Item `json:"items"`
	Data      []Item `json:"data,omitempty"` // items 的别名，适配常见 API 包装
}

// loadTimeline 读取 URL 或本地文件，返回解析后的时间线。
func loadTimeline(src string) (*Timeline, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("缺少时间线地址或文件")
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
			return nil, fmt.Errorf("读取时间线文件失败: %w", err)
		}
		raw = data
	}
	var tl Timeline
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(raw, &tl.Items); err != nil {
			return nil, fmt.Errorf("时间线不是合法 JSON: %w", err)
		}
	} else if err := json.Unmarshal(raw, &tl); err != nil {
		return nil, fmt.Errorf("时间线不是合法 JSON: %w", err)
	}
	if len(tl.Items) == 0 && len(tl.Data) > 0 {
		tl.Items = tl.Data
	}
	if len(tl.Items) == 0 && !strings.HasPrefix(trimmed, "[") &&
		!strings.Contains(trimmed, `"items"`) && !strings.Contains(trimmed, `"data"`) {
		return nil, fmt.Errorf("时间线里没有 items 字段")
	}
	normalizeItems(&tl)
	// 上游已经按时间倒序，这里再排一次，本地文件被手工改过也不会乱序。
	sort.SliceStable(tl.Items, func(i, j int) bool {
		return tl.Items[i].CreateTime > tl.Items[j].CreateTime
	})
	return &tl, nil
}

// normalizeItems 把各平台视图的差异字段对齐到 Item 的通用字段。
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
		return "", fmt.Errorf("请求时间线失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("时间线返回 HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return string(body), nil
}

// normalizePlatform 返回受支持的平台名；未知值返回空串。
func normalizePlatform(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case platformDouyin, platformTikTok, platformBilibili:
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

// isBVID 只把形如 BV1xxxxxxxxx 的 id 当作 B 站 BV 号，
// 避免通用时间线里的数字 id 被误判成 B 站。
func isBVID(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) == 12 && strings.HasPrefix(s, "BV")
}

// platformFromSource 从时间线 URL 推断平台；纯 URL 规则，不看条目。
func platformFromSource(src string) string {
	lower := strings.ToLower(src)
	switch {
	case strings.Contains(lower, "/douyin/") || strings.Contains(lower, "douyin.com"):
		return platformDouyin
	case strings.Contains(lower, "/tiktok/") || strings.Contains(lower, "tiktok.com"):
		return platformTikTok
	case strings.Contains(lower, "/bilibili/") || strings.Contains(lower, "bilibili.com"):
		return platformBilibili
	}
	return ""
}

// platformFromItem 从单条内容推断平台：BV 号 / 视频页地址 > 时间线 URL > 媒体直链域名。
func platformFromItem(it Item, src string) string {
	if isBVID(it.BVID) || isBVID(it.VideoID) || isBVID(it.AwemeID) ||
		strings.Contains(strings.ToLower(it.URL), "bilibili.com") {
		return platformBilibili
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

// platformForItem 决定单条内容属于哪个平台：
// --platform > item.platform > timeline.source/platform > 条目特征 > 时间线 URL > 媒体直链。
// 返回空串表示无法判断，由调用方报错或跳过，不再默认成某个平台。
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

// detectPlatform 决定整份时间线的展示平台。逐条 platform 优先于顶层 source，
// 这样混合时间线也能按条目正确路由。
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

// selectItems 按 --since / --ids / --limit 过滤；limit 只数真正要下载的条目。
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
