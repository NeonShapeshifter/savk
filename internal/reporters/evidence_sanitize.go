package reporters

import (
	"regexp"
	"unicode/utf8"

	"savk/internal/evidence"
)

const (
	defaultEvidenceRawLimit = 4096
	truncateSuffix          = "\n...[truncated]"
)

var evidenceRedactionPatterns = []redactionPattern{
	{
		regex:       regexp.MustCompile(`(?im)\b(authorization)(\s*[:=]\s*)([^\r\n]+)`),
		replacement: `${1}${2}[REDACTED]`,
	},
	{
		regex:       regexp.MustCompile(`(?im)\b(password|passwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s\r\n]+)`),
		replacement: `${1}${2}[REDACTED]`,
	},
	{
		regex:       regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/-]+=*`),
		replacement: `Bearer [REDACTED]`,
	},
	{
		regex:       regexp.MustCompile(`(?i)\bBasic\s+[A-Za-z0-9+/=]+`),
		replacement: `Basic [REDACTED]`,
	},
}

type redactionPattern struct {
	regex       *regexp.Regexp
	replacement string
}

func sanitizeEvidence(ev evidence.Evidence, includeRaw bool) evidence.Evidence {
	ev.CollectedAt = ev.CollectedAt.UTC()
	if ev.Raw == "" || includeRaw {
		return ev
	}

	raw, redacted := redactEvidenceRaw(ev.Raw)
	raw, truncated := truncateEvidenceRaw(raw, defaultEvidenceRawLimit)
	ev.Raw = raw
	ev.Redacted = ev.Redacted || redacted
	ev.Truncated = ev.Truncated || truncated
	return ev
}

func redactEvidenceRaw(raw string) (string, bool) {
	redacted := false
	sanitized := raw

	for _, pattern := range evidenceRedactionPatterns {
		replaced := pattern.regex.ReplaceAllString(sanitized, pattern.replacement)
		if replaced != sanitized {
			redacted = true
			sanitized = replaced
		}
	}

	return sanitized, redacted
}

func truncateEvidenceRaw(raw string, limit int) (string, bool) {
	if limit <= 0 || len(raw) <= limit {
		return raw, false
	}
	if len(truncateSuffix) >= limit {
		return truncateUTF8(raw, limit), true
	}

	visibleLimit := limit - len(truncateSuffix)
	return truncateUTF8(raw, visibleLimit) + truncateSuffix, true
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || value == "" {
		return ""
	}
	if len(value) <= limit {
		return value
	}

	cut := 0
	for index, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		if index+size > limit {
			break
		}
		cut = index + size
	}

	return value[:cut]
}
