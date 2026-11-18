package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// stateFile is the incremental ledger filename under the output root.
const stateFile = ".state.json"

// State records downloaded items, keyed by "platform:id".
type State struct {
	Items map[string]StateItem `json:"items"`
}

type StateItem struct {
	Path         string `json:"path"`
	Title        string `json:"title"`
	DownloadedAt string `json:"downloaded_at"`
}

func stateKey(platform, id string) string { return platform + ":" + id }

func loadState(path string) *State {
	s := &State{Items: map[string]StateItem{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, s); err != nil || s.Items == nil {
		return &State{Items: map[string]StateItem{}}
	}
	return s
}

func (s *State) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// has reports whether an item has already been downloaded. Legacy ledgers
// stored bare aweme_ids without a platform prefix; those are treated as
// Douyin entries to avoid re-downloading after migration.
func (s *State) has(platform, id string) bool {
	if _, ok := s.Items[stateKey(platform, id)]; ok {
		return true
	}
	if platform == platformDouyin {
		_, ok := s.Items[id]
		return ok
	}
	return false
}

func (s *State) mark(platform string, it Item, path string) {
	s.Items[stateKey(platform, it.AwemeID)] = StateItem{
		Path:         path,
		Title:        strings.TrimSpace(it.Desc),
		DownloadedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
}
