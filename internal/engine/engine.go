package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	"savk/internal/evidence"
)

type Engine struct {
	now              func() time.Time
	collectorTimeout time.Duration
}

func New() Engine {
	return Engine{now: time.Now}
}

func (e Engine) WithCollectorTimeout(timeout time.Duration) Engine {
	e.collectorTimeout = timeout
	return e
}

func (e Engine) Run(ctx context.Context, checks []Check) ([]evidence.CheckResult, error) {
	if e.now == nil {
		e.now = time.Now
	}

	ordered, err := e.validateChecks(checks)
	if err != nil {
		return nil, err
	}

	results := make([]evidence.CheckResult, 0, len(ordered))
	byID := make(map[string]evidence.CheckResult, len(ordered))

	for _, check := range ordered {
		if blockedBy, blockedResult, ok := findBlockingPrerequisite(check, byID); ok {
			result := e.prerequisiteFailure(check, blockedBy, blockedResult.Status)
			results = append(results, result)
			byID[check.ID()] = result
			continue
		}

		result := e.runCheck(ctx, check)
		results = append(results, result)
		byID[check.ID()] = result
	}

	return results, nil
}

func (e Engine) validateChecks(checks []Check) ([]Check, error) {
	byID := make(map[string]Check, len(checks))
	ids := make([]string, 0, len(checks))

	for _, check := range checks {
		id := check.ID()
		if id == "" {
			return nil, fmt.Errorf("check ID must not be empty")
		}
		if _, exists := byID[id]; exists {
			return nil, fmt.Errorf("duplicate check ID %q", id)
		}
		byID[id] = check
		ids = append(ids, id)
	}

	sort.Strings(ids)
	for _, id := range ids {
		for _, prereq := range byID[id].Prerequisites() {
			if _, ok := byID[prereq]; !ok {
				return nil, fmt.Errorf("check %q references missing prerequisite %q", id, prereq)
			}
		}
	}

	if err := detectCycles(byID, ids); err != nil {
		return nil, err
	}

	ordered := make([]Check, 0, len(ids))
	visited := make(map[string]bool, len(ids))

	var visit func(string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		for _, prereq := range byID[id].Prerequisites() {
			visit(prereq)
		}
		ordered = append(ordered, byID[id])
	}
	for _, id := range ids {
		visit(id)
	}

	return ordered, nil
}

func detectCycles(byID map[string]Check, ids []string) error {
	const (
		visitUnknown = iota
		visitActive
		visitDone
	)

	state := make(map[string]int, len(ids))
	stack := make([]string, 0, len(ids))

	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visitDone:
			return nil
		case visitActive:
			cycle := append(stack, id)
			return fmt.Errorf("prerequisite cycle detected: %v", cycle)
		}

		state[id] = visitActive
		stack = append(stack, id)
		for _, prereq := range byID[id].Prerequisites() {
			if err := visit(prereq); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = visitDone

		return nil
	}

	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}

	return nil
}

func findBlockingPrerequisite(check Check, results map[string]evidence.CheckResult) (string, evidence.CheckResult, bool) {
	for _, prereq := range check.Prerequisites() {
		result, ok := results[prereq]
		if !ok {
			continue
		}
		if result.Status != evidence.StatusPass {
			return prereq, result, true
		}
	}

	return "", evidence.CheckResult{}, false
}

func (e Engine) prerequisiteFailure(check Check, blockedBy string, blockedStatus evidence.EvalStatus) evidence.CheckResult {
	now := e.now().UTC()
	return evidence.CheckResult{
		CheckID:    check.ID(),
		Domain:     check.Domain(),
		Status:     propagatePrerequisiteStatus(blockedStatus),
		ReasonCode: evidence.ReasonPrerequisiteFailed,
		Evidence: evidence.Evidence{
			Source:      "engine.prerequisite",
			Collector:   "engine",
			CollectedAt: now,
		},
		Message: fmt.Sprintf("blocked by prerequisite %s with status %s", blockedBy, blockedStatus),
	}
}

func (e Engine) runCheck(ctx context.Context, check Check) (result evidence.CheckResult) {
	startedAt := e.now()
	checkCtx, cancel := e.collectorContext(ctx)
	if cancel != nil {
		defer cancel()
	}

	if err := checkCtx.Err(); err != nil {
		result = e.contextFailure(check, err)
		return e.finalizeResult(check, result, startedAt)
	}
	if !contextHasDeadline(checkCtx) {
		return e.runCheckSync(checkCtx, check, startedAt)
	}

	type checkRun struct {
		result    evidence.CheckResult
		recovered any
	}

	done := make(chan checkRun, 1)
	go func() {
		run := checkRun{}
		defer func() {
			if recovered := recover(); recovered != nil {
				run.recovered = recovered
			}
			done <- run
		}()

		run.result = check.Run(checkCtx)
	}()

	select {
	case <-checkCtx.Done():
		result = e.contextFailure(check, checkCtx.Err())
	case run := <-done:
		if run.recovered != nil {
			result = evidence.CheckResult{
				CheckID:    check.ID(),
				Domain:     check.Domain(),
				Status:     evidence.StatusError,
				ReasonCode: evidence.ReasonInternalError,
				Evidence: evidence.Evidence{
					Source:      "engine.panic",
					Collector:   "engine",
					CollectedAt: e.now().UTC(),
				},
				Message: fmt.Sprintf("panic recovered while running check: %v", run.recovered),
			}
		} else {
			result = run.result
		}
	}

	return e.finalizeResult(check, result, startedAt)
}

func (e Engine) runCheckSync(ctx context.Context, check Check, startedAt time.Time) (result evidence.CheckResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = evidence.CheckResult{
				CheckID:    check.ID(),
				Domain:     check.Domain(),
				Status:     evidence.StatusError,
				ReasonCode: evidence.ReasonInternalError,
				Evidence: evidence.Evidence{
					Source:      "engine.panic",
					Collector:   "engine",
					CollectedAt: e.now().UTC(),
				},
				Message: fmt.Sprintf("panic recovered while running check: %v", recovered),
			}
		}
	}()

	result = check.Run(ctx)
	return e.finalizeResult(check, result, startedAt)
}

func (e Engine) collectorContext(parent context.Context) (context.Context, context.CancelFunc) {
	if e.collectorTimeout > 0 {
		return context.WithTimeout(parent, e.collectorTimeout)
	}

	return parent, nil
}

func (e Engine) contextFailure(check Check, err error) evidence.CheckResult {
	result := evidence.CheckResult{
		CheckID: check.ID(),
		Domain:  check.Domain(),
		Status:  evidence.StatusError,
		Evidence: evidence.Evidence{
			Collector:   "engine",
			CollectedAt: e.now().UTC(),
		},
	}

	if err == context.DeadlineExceeded {
		result.ReasonCode = evidence.ReasonTimeout
		result.Evidence.Source = "engine.timeout"
		result.Message = fmt.Sprintf("collector timed out after %s", e.collectorTimeout)
		return result
	}

	result.Evidence.Source = "engine.context"
	result.Message = "collector context cancelled"
	return result
}

func (e Engine) finalizeResult(check Check, result evidence.CheckResult, startedAt time.Time) evidence.CheckResult {
	result.CheckID = check.ID()
	result.Domain = check.Domain()
	result.DurationMs = e.now().Sub(startedAt).Milliseconds()
	if result.Evidence.CollectedAt.IsZero() {
		result.Evidence.CollectedAt = e.now().UTC()
	}
	return result
}

func contextHasDeadline(ctx context.Context) bool {
	_, ok := ctx.Deadline()
	return ok
}
