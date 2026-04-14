package reporters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"savk/internal/contract"
	"savk/internal/evidence"
)

func TestRenderJSONReportGolden(t *testing.T) {
	t.Parallel()

	sourceTZ := time.FixedZone("EST", -5*60*60)
	startedAt := time.Date(2026, 4, 12, 11, 0, 0, 0, sourceTZ)

	report, err := RenderJSONReport(JSONReportInput{
		ToolVersion:     "0.1.0",
		ContractVersion: contract.APIVersionV1,
		ContractHash:    "sha256:4c54e5b4",
		RunID:           "20260412T160000Z-8f2d",
		Target:          contract.TargetLinuxSystemd,
		Host:            "sensor-prod-01",
		StartedAt:       startedAt,
		DurationMs:      14,
		Results: []evidence.CheckResult{
			{
				CheckID:  "path./etc/sensor-agent/config.yaml.mode",
				Domain:   "paths",
				Status:   evidence.StatusFail,
				Expected: "0640",
				Observed: "0666",
				Evidence: evidence.Evidence{
					Source:      "fs.stat",
					Collector:   "paths",
					CollectedAt: startedAt,
					Raw:         "mode=0666",
				},
				DurationMs: 1,
				Message:    "expected mode 0640, observed 0666",
			},
			{
				CheckID:    "path./var/log/sensor-agent.exists",
				Domain:     "paths",
				Status:     evidence.StatusError,
				Expected:   true,
				ReasonCode: evidence.ReasonTimeout,
				Evidence: evidence.Evidence{
					Source:      "fs.stat",
					Collector:   "paths",
					CollectedAt: startedAt.Add(2 * time.Second),
					Command:     []string{"/usr/bin/stat", "/var/log/sensor-agent"},
					ExitCode:    intPtr(124),
					Raw:         "stat timed out while collecting evidence",
					Truncated:   true,
				},
				DurationMs: 2000,
				Message:    "collector timed out while reading /var/log/sensor-agent",
			},
			{
				CheckID:  "path./etc/sensor-agent/config.yaml.exists",
				Domain:   "paths",
				Status:   evidence.StatusPass,
				Expected: true,
				Observed: true,
				Evidence: evidence.Evidence{
					Source:      "fs.stat",
					Collector:   "paths",
					CollectedAt: startedAt,
				},
				DurationMs: 1,
				Message:    "path exists",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderJSONReport() error = %v", err)
	}

	want := readGolden(t, "report", "minimal-report.json")
	got := string(report)
	if got != want {
		t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestRenderJSONReportKeepsZeroExitCodeWhenCommandExists(t *testing.T) {
	t.Parallel()

	report, err := RenderJSONReport(JSONReportInput{
		ToolVersion:     "0.1.0",
		ContractVersion: contract.APIVersionV1,
		ContractHash:    "sha256:abcd",
		RunID:           "run-zero-exit",
		Target:          contract.TargetLinuxSystemd,
		Host:            "host-01",
		StartedAt:       time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		DurationMs:      1,
		Results: []evidence.CheckResult{
			{
				CheckID: "service.agent.state",
				Domain:  "services",
				Status:  evidence.StatusPass,
				Evidence: evidence.Evidence{
					Source:      "systemctl show",
					Collector:   "services",
					CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
					Command:     []string{"systemctl", "show", "agent.service"},
					ExitCode:    intPtr(0),
				},
				Message: "ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderJSONReport() error = %v", err)
	}
	if string(report) == "" {
		t.Fatal("RenderJSONReport() returned empty output")
	}
	if !strings.Contains(string(report), `"exitCode": 0`) {
		t.Fatalf("report missing zero exitCode:\n%s", string(report))
	}
}

func TestRenderJSONReportIncludesHostRootWhenProvided(t *testing.T) {
	t.Parallel()

	report, err := RenderJSONReport(JSONReportInput{
		ToolVersion:     "0.1.0",
		ContractVersion: contract.APIVersionV1,
		ContractHash:    "sha256:abcd",
		RunID:           "run-rooted",
		Target:          contract.TargetLinuxSystemd,
		Host:            "observer-01",
		HostRoot:        "/host",
		StartedAt:       time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		DurationMs:      1,
		Results: []evidence.CheckResult{
			{
				CheckID: "path./etc/hosts.exists",
				Domain:  "paths",
				Status:  evidence.StatusPass,
				Evidence: evidence.Evidence{
					Source:      "fs.lstat",
					Collector:   "paths",
					CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
				},
				Message: "ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderJSONReport() error = %v", err)
	}
	if !strings.Contains(string(report), `"hostRoot": "/host"`) {
		t.Fatalf("report missing hostRoot context:\n%s", string(report))
	}
}

func TestRenderJSONReportRedactsAndTruncatesRawByDefault(t *testing.T) {
	t.Parallel()

	report, err := RenderJSONReport(JSONReportInput{
		ToolVersion:     "0.1.0",
		ContractVersion: contract.APIVersionV1,
		ContractHash:    "sha256:abcd",
		RunID:           "run-sanitized",
		Target:          contract.TargetLinuxSystemd,
		Host:            "host-01",
		StartedAt:       time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		DurationMs:      1,
		Results: []evidence.CheckResult{
			{
				CheckID: "service.agent.state",
				Domain:  "services",
				Status:  evidence.StatusError,
				Evidence: evidence.Evidence{
					Source:      "systemctl show",
					Collector:   "services",
					CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
					Raw: "Authorization: Bearer super-secret-token\n" +
						"password=correct-horse-battery-staple\n" +
						strings.Repeat("x", 5000),
				},
				Message: "bad raw",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderJSONReport() error = %v", err)
	}

	got := string(report)
	if strings.Contains(got, "super-secret-token") || strings.Contains(got, "correct-horse-battery-staple") {
		t.Fatalf("report leaked secret material:\n%s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("report missing redaction marker:\n%s", got)
	}
	if !strings.Contains(got, `"redacted": true`) {
		t.Fatalf("report missing redacted flag:\n%s", got)
	}
	if !strings.Contains(got, `"truncated": true`) {
		t.Fatalf("report missing truncated flag:\n%s", got)
	}
	if !strings.Contains(got, "...[truncated]") {
		t.Fatalf("report missing truncation suffix:\n%s", got)
	}
}

func TestRenderJSONReportIncludeRawPreservesFullRaw(t *testing.T) {
	t.Parallel()

	report, err := RenderJSONReport(JSONReportInput{
		ToolVersion:     "0.1.0",
		ContractVersion: contract.APIVersionV1,
		ContractHash:    "sha256:abcd",
		RunID:           "run-full-raw",
		Target:          contract.TargetLinuxSystemd,
		Host:            "host-01",
		StartedAt:       time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		DurationMs:      1,
		IncludeRaw:      true,
		Results: []evidence.CheckResult{
			{
				CheckID: "service.agent.state",
				Domain:  "services",
				Status:  evidence.StatusError,
				Evidence: evidence.Evidence{
					Source:      "systemctl show",
					Collector:   "services",
					CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
					Raw:         "Authorization: Bearer super-secret-token",
				},
				Message: "bad raw",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderJSONReport() error = %v", err)
	}

	got := string(report)
	if !strings.Contains(got, "super-secret-token") {
		t.Fatalf("report did not preserve raw under IncludeRaw:\n%s", got)
	}
	if !strings.Contains(got, `"redacted": false`) {
		t.Fatalf("report unexpectedly marked raw as redacted:\n%s", got)
	}
	if !strings.Contains(got, `"truncated": false`) {
		t.Fatalf("report unexpectedly marked raw as truncated:\n%s", got)
	}
}

func TestExitCodeForResults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		results []evidence.CheckResult
		want    int
	}{
		{
			name: "only pass and not applicable",
			results: []evidence.CheckResult{
				{Status: evidence.StatusPass},
				{Status: evidence.StatusNotApplicable},
			},
			want: 0,
		},
		{
			name: "fail without error",
			results: []evidence.CheckResult{
				{Status: evidence.StatusPass},
				{Status: evidence.StatusFail},
			},
			want: 1,
		},
		{
			name: "error wins",
			results: []evidence.CheckResult{
				{Status: evidence.StatusFail},
				{Status: evidence.StatusError},
			},
			want: 2,
		},
		{
			name: "insufficient data wins",
			results: []evidence.CheckResult{
				{Status: evidence.StatusPass},
				{Status: evidence.StatusInsufficientData},
			},
			want: 2,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ExitCodeForResults(tc.results)
			if got != tc.want {
				t.Fatalf("ExitCodeForResults() = %d, want %d", got, tc.want)
			}
		})
	}
}

func readGolden(t *testing.T, kind, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "testdata", "golden", kind, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}

	return string(data)
}

func intPtr(value int) *int {
	return &value
}
