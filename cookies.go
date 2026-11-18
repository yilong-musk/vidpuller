package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// loadCookieFile reads browser-exported cookies in three formats:
//   - Netscape cookies.txt (seven tab-separated columns)
//   - a JSON array exported by DevTools or an extension
//     [{"name":"…","value":"…"}, …]
//   - a request header string "name=value; name=value"
//
// TikTok login state relies mainly on sessionid / sid_tt / ttwid; with these
// the server usually serves higher quality renditions (1080p/4K depending on
// the video).
func loadCookieFile(path string) ([]*http.Cookie, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cookie file: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("cookie file is empty")
	}

	// JSON array
	if strings.HasPrefix(text, "[") {
		var entries []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(text), &entries); err != nil {
			return nil, fmt.Errorf("parse JSON cookies: %w", err)
		}
		cookies := make([]*http.Cookie, 0, len(entries))
		for _, e := range entries {
			if e.Name != "" {
				cookies = append(cookies, &http.Cookie{Name: e.Name, Value: e.Value, Path: "/"})
			}
		}
		if len(cookies) == 0 {
			return nil, fmt.Errorf("JSON cookies contain no entries")
		}
		return cookies, nil
	}

	// Netscape cookies.txt
	if strings.Contains(text, "	") {
		var cookies []*http.Cookie
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			parts := strings.Split(line, "	")
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
			return nil, fmt.Errorf("Netscape cookie file contains no entries")
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
		return nil, fmt.Errorf("no cookies parsed")
	}
	return cookies, nil
}
