package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseMetadataAlignsToV015(t *testing.T) {
	t.Parallel()

	if version != "0.1.5" {
		t.Fatalf("version = %q, want %q", version, "0.1.5")
	}

	cases := []struct {
		path string
		want string
	}{
		{path: filepath.Join("..", "..", "Makefile"), want: "VERSION ?= 0.1.5"},
		{path: filepath.Join("..", "..", "README.md"), want: "make dist VERSION=0.1.5"},
		{path: filepath.Join("..", "..", "docs", "release.md"), want: "make dist VERSION=0.1.5"},
		{path: filepath.Join("..", "..", "docs", "release-checklist.md"), want: "## v0.1.5"},
		{path: filepath.Join("..", "..", "CHANGELOG.md"), want: "## v0.1.5 - "},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error = %v", tc.path, err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("%s missing %q", tc.path, tc.want)
			}
		})
	}
}
