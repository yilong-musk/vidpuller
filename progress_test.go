package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// resetProgressState clears the package-level throttle so one assertion cannot
// suppress the next.
func resetProgressState() {
	lastProgressPct = -1
	lastProgressAt = time.Time{}
}

func captureProgress(t *testing.T) *[]float64 {
	t.Helper()
	var got []float64
	prev := reportProgress
	reportProgress = func(f float64) { got = append(got, f) }
	t.Cleanup(func() { reportProgress = prev })
	resetProgressState()
	return &got
}

// A cover download must not move the bar: it runs after the media is already
// at 100%, so any event here would rewind the client.
func TestSilentRangeEmitsNothing(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "262144")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got := captureProgress(t)
	if _, err := downloadFile(srv.URL, filepath.Join(t.TempDir(), "cover.jpg"), "", nil,
		progressRange{silent: true}); err != nil {
		t.Fatalf("download: %v", err)
	}
	if len(*got) != 0 {
		t.Fatalf("silent range emitted %d progress events: %v", len(*got), *got)
	}
}

func TestRangeMapsProgressIntoItsSlice(t *testing.T) {
	got := captureProgress(t)
	w := &progressWriter{w: &bytes.Buffer{}, total: 1000, base: 0.5, span: 0.4}

	resetProgressState()
	if _, err := w.Write(make([]byte, 500)); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("midpoint write emitted %d events, want 1", len(*got))
	}
	if mid := (*got)[0]; mid < 0.69 || mid > 0.71 {
		t.Fatalf("midpoint progress %v, want ~0.7", mid)
	}

	resetProgressState()
	if _, err := w.Write(make([]byte, 500)); err != nil {
		t.Fatal(err)
	}
	if last := (*got)[len(*got)-1]; last < 0.89 || last > 0.91 {
		t.Fatalf("final progress %v, want ~0.9", last)
	}
}

func TestSilentWriterEmitsNothing(t *testing.T) {
	got := captureProgress(t)
	w := &progressWriter{w: &bytes.Buffer{}, total: 1000, base: 0, span: 1, silent: true}
	resetProgressState()
	for i := 0; i < 4; i++ {
		if _, err := w.Write(make([]byte, 250)); err != nil {
			t.Fatal(err)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("silent writer emitted %d events: %v", len(*got), *got)
	}
}
