package reporters

import (
	"strings"
	"testing"
	"time"

	"savk/internal/contract"
	"savk/internal/evidence"
)

func TestRenderTableReportIncludesSummaryAndStableOrder(t *testing.T) {
	t.Parallel()

	output, err := RenderTableReport(JSONReportInput{
		ToolVersion:     "0.1.0",
		ContractVersion: contract.APIVersionV1,
		ContractHash:    "sha256:abcd",
		RunID:           "run-01",
		Target:          contract.TargetLinuxSystemd,
		Host:            "sensor-01",
		StartedAt:       time.Date(2026, 4, 12, 11, 0, 0, 0, time.FixedZone("EST", -5*60*60)),
		DurationMs:      25,
		Results: []evidence.CheckResult{
			{
				CheckID: "path./z.exists",
				Domain:  "paths",
				Status:  evidence.StatusPass,
				Message: "z ok",
			},
			{
				CheckID:    "path./a.exists",
				Domain:     "paths",
				Status:     evidence.StatusError,
				ReasonCode: evidence.ReasonTimeout,
				Message:    "collector timed out",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderTableReport() error = %v", err)
	}

	got := string(output)
	if !strings.Contains(got, "SAVK 0.1.0\n") {
		t.Fatalf("table output missing version header: %q", got)
	}
	if !strings.Contains(got, "Summary: pass=1 fail=0 notApplicable=0 insufficientData=0 error=1 exit=2\n") {
		t.Fatalf("table output missing summary: %q", got)
	}
	if !strings.Contains(got, "Started: 2026-04-12T16:00:00Z\n") {
		t.Fatalf("table output missing UTC timestamp: %q", got)
	}

	indexA := strings.Index(got, "path./a.exists")
	indexZ := strings.Index(got, "path./z.exists")
	if indexA == -1 || indexZ == -1 || indexA > indexZ {
		t.Fatalf("table output order is not stable:\n%s", got)
	}
}
