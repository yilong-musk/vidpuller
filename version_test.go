package main

import "testing"

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		injected string
		module   string
		want     string
	}{
		{"0.2.0", "v0.2.0", "0.2.0"},
		{"v0.2.0", "", "0.2.0"},
		{"dev", "v0.2.1", "0.2.1"},
		{"", "v0.2.1", "0.2.1"},
		{"dev", "(devel)", "dev"},
		{"", "", "dev"},
		{"v0.2.0-1-gabc123", "v0.2.0", "0.2.0-1-gabc123"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.injected, c.module); got != c.want {
			t.Errorf("resolveVersion(%q, %q) = %q, want %q", c.injected, c.module, got, c.want)
		}
	}
}
