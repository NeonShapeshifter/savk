package engine

import "savk/internal/evidence"

func propagatePrerequisiteStatus(status evidence.EvalStatus) evidence.EvalStatus {
	switch status {
	case evidence.StatusFail, evidence.StatusNotApplicable:
		return evidence.StatusNotApplicable
	case evidence.StatusError, evidence.StatusInsufficientData:
		return evidence.StatusInsufficientData
	default:
		return evidence.StatusInsufficientData
	}
}
