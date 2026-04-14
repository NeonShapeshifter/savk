package reporters

import (
	"encoding/json"
	"sort"
	"time"

	"savk/internal/evidence"
)

const SchemaVersionV1 = "savk-report/v1"

type JSONReportInput struct {
	ToolVersion     string
	ContractVersion string
	ContractHash    string
	RunID           string
	Target          string
	Host            string
	HostRoot        string
	StartedAt       time.Time
	DurationMs      int64
	IncludeRaw      bool
	Results         []evidence.CheckResult
}

type JSONReport struct {
	SchemaVersion   string                 `json:"schemaVersion"`
	ToolVersion     string                 `json:"toolVersion"`
	ContractVersion string                 `json:"contractVersion"`
	ContractHash    string                 `json:"contractHash"`
	RunID           string                 `json:"runID"`
	Target          string                 `json:"target"`
	Host            string                 `json:"host"`
	HostRoot        string                 `json:"hostRoot,omitempty"`
	StartedAt       time.Time              `json:"startedAt"`
	DurationMs      int64                  `json:"durationMs"`
	ExitCode        int                    `json:"exitCode"`
	Summary         JSONReportSummary      `json:"summary"`
	Results         []evidence.CheckResult `json:"results"`
}

type JSONReportSummary struct {
	Pass             int `json:"pass"`
	Fail             int `json:"fail"`
	NotApplicable    int `json:"notApplicable"`
	InsufficientData int `json:"insufficientData"`
	Error            int `json:"error"`
}

func RenderJSONReport(input JSONReportInput) ([]byte, error) {
	report := BuildJSONReport(input)
	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}

	output = append(output, '\n')
	return output, nil
}

func BuildJSONReport(input JSONReportInput) JSONReport {
	results := normalizeResults(input.Results, input.IncludeRaw)

	return JSONReport{
		SchemaVersion:   SchemaVersionV1,
		ToolVersion:     input.ToolVersion,
		ContractVersion: input.ContractVersion,
		ContractHash:    input.ContractHash,
		RunID:           input.RunID,
		Target:          input.Target,
		Host:            input.Host,
		HostRoot:        input.HostRoot,
		StartedAt:       input.StartedAt.UTC(),
		DurationMs:      input.DurationMs,
		ExitCode:        ExitCodeForResults(results),
		Summary:         SummarizeResults(results),
		Results:         results,
	}
}

func SummarizeResults(results []evidence.CheckResult) JSONReportSummary {
	var summary JSONReportSummary

	for _, result := range results {
		switch result.Status {
		case evidence.StatusPass:
			summary.Pass++
		case evidence.StatusFail:
			summary.Fail++
		case evidence.StatusNotApplicable:
			summary.NotApplicable++
		case evidence.StatusInsufficientData:
			summary.InsufficientData++
		case evidence.StatusError:
			summary.Error++
		}
	}

	return summary
}

func ExitCodeForResults(results []evidence.CheckResult) int {
	summary := SummarizeResults(results)
	if summary.Error > 0 || summary.InsufficientData > 0 {
		return 2
	}
	if summary.Fail > 0 {
		return 1
	}

	return 0
}

func normalizeResults(results []evidence.CheckResult, includeRaw bool) []evidence.CheckResult {
	cloned := make([]evidence.CheckResult, len(results))
	copy(cloned, results)

	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].CheckID < cloned[j].CheckID
	})

	for index := range cloned {
		cloned[index].Evidence = sanitizeEvidence(cloned[index].Evidence, includeRaw)
	}

	return cloned
}
