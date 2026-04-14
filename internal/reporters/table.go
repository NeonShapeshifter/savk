package reporters

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

func RenderTableReport(input JSONReportInput) ([]byte, error) {
	report := BuildJSONReport(input)

	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "SAVK %s\n", report.ToolVersion)
	fmt.Fprintf(&buffer, "RunID: %s\n", report.RunID)
	fmt.Fprintf(&buffer, "Target: %s\n", report.Target)
	fmt.Fprintf(&buffer, "Host: %s\n", report.Host)
	fmt.Fprintf(&buffer, "Started: %s\n", report.StartedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(&buffer, "Duration: %dms\n", report.DurationMs)
	fmt.Fprintf(
		&buffer,
		"Summary: pass=%d fail=%d notApplicable=%d insufficientData=%d error=%d exit=%d\n\n",
		report.Summary.Pass,
		report.Summary.Fail,
		report.Summary.NotApplicable,
		report.Summary.InsufficientData,
		report.Summary.Error,
		report.ExitCode,
	)

	writer := tabwriter.NewWriter(&buffer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "STATUS\tDOMAIN\tCHECK ID\tREASON\tMESSAGE")
	for _, result := range report.Results {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\n",
			result.Status,
			result.Domain,
			result.CheckID,
			string(result.ReasonCode),
			sanitizeTableText(result.Message),
		)
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func sanitizeTableText(value string) string {
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")
	return replacer.Replace(value)
}
