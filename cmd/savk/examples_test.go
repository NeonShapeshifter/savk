package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"savk/internal/contract"
)

func TestExamplesParseAndBuildChecks(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("os.ReadDir(examples) error = %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("examples directory is empty")
	}

	for _, entry := range entries {
		entry := entry
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", "..", "examples", entry.Name())
			cfg, err := contract.ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile(%q) error = %v", path, err)
			}

			domains, err := selectedDomains(cfg, "")
			if err != nil {
				t.Fatalf("selectedDomains(%q) error = %v", path, err)
			}
			slices.Sort(domains)
			checks, err := buildChecksForDomains(cfg, domains, "")
			if err != nil {
				t.Fatalf("buildChecksForDomains(%q) error = %v", path, err)
			}
			if len(checks) == 0 {
				t.Fatalf("buildChecksForDomains(%q) returned 0 checks", path)
			}
		})
	}
}

func TestREADMEQuickstartValidatePasses(t *testing.T) {
	t.Parallel()

	contractPath := filepath.Join("..", "..", "examples", "paths-only.yaml")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"validate", "--contract", contractPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(validate quickstart) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "contract valid\n" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), "contract valid\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
