package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// loadCookieFile 读取浏览器导出的 cookie，支持三种格式：
//   - Netscape cookies.txt（7 列制表符分隔）
//   - DevTools / 扩展导出的 JSON 数组 [{"name":"…","value":"…"}, …]
//   - 请求头里的 "name=value; name=value"
//
// TikTok 的登录态主要靠 sessionid / sid_tt / ttwid；带了这些之后
// 服务端通常会下发比游客更高的画质档位（1080p/4K 视视频而定）。
func loadCookieFile(path string) ([]*http.Cookie, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 cookie 文件失败: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("cookie 文件为空")
	}

	// JSON 数组
	if strings.HasPrefix(text, "[") {
		var entries []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(text), &entries); err != nil {
			return nil, fmt.Errorf("解析 JSON cookie 失败: %w", err)
		}
		cookies := make([]*http.Cookie, 0, len(entries))
		for _, e := range entries {
			if e.Name != "" {
				cookies = append(cookies, &http.Cookie{Name: e.Name, Value: e.Value, Path: "/"})
			}
		}
		if len(cookies) == 0 {
			return nil, fmt.Errorf("JSON cookie 里没有条目")
		}
		return cookies, nil
	}

	// Netscape cookies.txt
	if strings.Contains(text, "\t") {
		var cookies []*http.Cookie
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 7 {
				continue
			}
			domain := strings.TrimPrefix(strings.TrimSpace(parts[0]), "#HttpOnly_")
			name, value := strings.TrimSpace(parts[5]), parts[6]
			if name == "" {
				continue
			}
			cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/", Domain: domain})
		}
		if len(cookies) == 0 {
			return nil, fmt.Errorf("Netscape cookie 文件里没有条目")
		}
		return cookies, nil
	}

	// name=value; name=value
	var cookies []*http.Cookie
	for _, pair := range strings.Split(text, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: strings.TrimSpace(name), Value: value, Path: "/"})
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("没有解析出任何 cookie")
	}
	return cookies, nil
}
