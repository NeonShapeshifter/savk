package evidence

import "time"

type EvalStatus string

const (
	StatusPass             EvalStatus = "PASS"
	StatusFail             EvalStatus = "FAIL"
	StatusNotApplicable    EvalStatus = "NOT_APPLICABLE"
	StatusInsufficientData EvalStatus = "INSUFFICIENT_DATA"
	StatusError            EvalStatus = "ERROR"
)

type ReasonCode string

const (
	ReasonNone               ReasonCode = ""
	ReasonTimeout            ReasonCode = "TIMEOUT"
	ReasonPermissionDenied   ReasonCode = "PERMISSION_DENIED"
	ReasonNotFound           ReasonCode = "NOT_FOUND"
	ReasonParseError         ReasonCode = "PARSE_ERROR"
	ReasonNamespaceIsolation ReasonCode = "NAMESPACE_ISOLATION"
	ReasonInternalError      ReasonCode = "INTERNAL_ERROR"
	ReasonPrerequisiteFailed ReasonCode = "PREREQUISITE_FAILED"
)

type Evidence struct {
	Source      string    `json:"source"`
	Collector   string    `json:"collector"`
	CollectedAt time.Time `json:"collectedAt"`
	Command     []string  `json:"command,omitempty"`
	ExitCode    *int      `json:"exitCode,omitempty"`
	Raw         string    `json:"raw,omitempty"`
	Redacted    bool      `json:"redacted"`
	Truncated   bool      `json:"truncated"`
}

type CheckResult struct {
	CheckID    string     `json:"checkID"`
	Domain     string     `json:"domain"`
	Status     EvalStatus `json:"status"`
	ReasonCode ReasonCode `json:"reasonCode,omitempty"`
	Expected   any        `json:"expected,omitempty"`
	Observed   any        `json:"observed,omitempty"`
	Evidence   Evidence   `json:"evidence"`
	DurationMs int64      `json:"durationMs"`
	Message    string     `json:"message"`
}
