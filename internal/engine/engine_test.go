package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"savk/internal/evidence"
)

type stubCheck struct {
	id            string
	domain        string
	prerequisites []string
	run           func(context.Context) evidence.CheckResult
}

func (c stubCheck) ID() string {
	return c.id
}

func (c stubCheck) Domain() string {
	return c.domain
}

func (c stubCheck) Prerequisites() []string {
	return c.prerequisites
}

func (c stubCheck) Run(ctx context.Context) evidence.CheckResult {
	if c.run == nil {
		return evidence.CheckResult{
			Status:  evidence.StatusPass,
			Message: "ok",
		}
	}

	return c.run(ctx)
}

func TestEngineRunsChecksInStableOrder(t *testing.T) {
	t.Parallel()

	var executed []string
	engine := Engine{
		now: func() time.Time {
			return time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC)
		},
	}

	results, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:     "check.z",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				executed = append(executed, "check.z")
				return evidence.CheckResult{Status: evidence.StatusPass, Message: "z"}
			},
		},
		stubCheck{
			id:     "check.a",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				executed = append(executed, "check.a")
				return evidence.CheckResult{Status: evidence.StatusPass, Message: "a"}
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !reflect.DeepEqual(executed, []string{"check.a", "check.z"}) {
		t.Fatalf("executed = %v, want %v", executed, []string{"check.a", "check.z"})
	}
	if results[0].CheckID != "check.a" || results[1].CheckID != "check.z" {
		t.Fatalf("results order = [%s %s], want [check.a check.z]", results[0].CheckID, results[1].CheckID)
	}
}

func TestEngineRunsPrerequisitesBeforeDependentsAcrossDomains(t *testing.T) {
	t.Parallel()

	var executed []string
	engine := Engine{
		now: func() time.Time {
			return time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC)
		},
	}

	results, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:            "identity.sensor_runtime.uid",
			domain:        "identity",
			prerequisites: []string{"service.sensor-agent.service.state"},
			run: func(context.Context) evidence.CheckResult {
				executed = append(executed, "identity.sensor_runtime.uid")
				return evidence.CheckResult{Status: evidence.StatusPass, Message: "identity"}
			},
		},
		stubCheck{
			id:     "service.sensor-agent.service.state",
			domain: "services",
			run: func(context.Context) evidence.CheckResult {
				executed = append(executed, "service.sensor-agent.service.state")
				return evidence.CheckResult{Status: evidence.StatusPass, Message: "service"}
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !reflect.DeepEqual(executed, []string{"service.sensor-agent.service.state", "identity.sensor_runtime.uid"}) {
		t.Fatalf("executed = %v, want %v", executed, []string{"service.sensor-agent.service.state", "identity.sensor_runtime.uid"})
	}
	if results[0].CheckID != "service.sensor-agent.service.state" || results[1].CheckID != "identity.sensor_runtime.uid" {
		t.Fatalf("results order = [%s %s], want [service.sensor-agent.service.state identity.sensor_runtime.uid]", results[0].CheckID, results[1].CheckID)
	}
}

func TestEnginePropagatesPrerequisiteFailure(t *testing.T) {
	t.Parallel()

	engine := Engine{
		now: func() time.Time {
			return time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC)
		},
	}

	dependentRan := false
	results, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:     "check.base",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				return evidence.CheckResult{
					Status:  evidence.StatusFail,
					Message: "base failed",
				}
			},
		},
		stubCheck{
			id:            "check.dependent",
			domain:        "paths",
			prerequisites: []string{"check.base"},
			run: func(context.Context) evidence.CheckResult {
				dependentRan = true
				return evidence.CheckResult{
					Status:  evidence.StatusPass,
					Message: "should not run",
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if dependentRan {
		t.Fatal("dependent check ran, want blocked by prerequisite")
	}

	dependent := results[1]
	if dependent.Status != evidence.StatusNotApplicable {
		t.Fatalf("dependent.Status = %s, want %s", dependent.Status, evidence.StatusNotApplicable)
	}
	if dependent.ReasonCode != evidence.ReasonPrerequisiteFailed {
		t.Fatalf("dependent.ReasonCode = %s, want %s", dependent.ReasonCode, evidence.ReasonPrerequisiteFailed)
	}
	if !strings.Contains(dependent.Message, "check.base") {
		t.Fatalf("dependent.Message = %q, want blocker name", dependent.Message)
	}
}

func TestEnginePropagatesPrerequisiteErrorAsInsufficientData(t *testing.T) {
	t.Parallel()

	engine := Engine{
		now: func() time.Time {
			return time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC)
		},
	}

	results, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:     "check.base",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				return evidence.CheckResult{
					Status:     evidence.StatusError,
					ReasonCode: evidence.ReasonTimeout,
					Message:    "timeout",
				}
			},
		},
		stubCheck{
			id:            "check.dependent",
			domain:        "paths",
			prerequisites: []string{"check.base"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	dependent := results[1]
	if dependent.Status != evidence.StatusInsufficientData {
		t.Fatalf("dependent.Status = %s, want %s", dependent.Status, evidence.StatusInsufficientData)
	}
	if dependent.ReasonCode != evidence.ReasonPrerequisiteFailed {
		t.Fatalf("dependent.ReasonCode = %s, want %s", dependent.ReasonCode, evidence.ReasonPrerequisiteFailed)
	}
}

func TestEngineRecoversFromPanicAndContinues(t *testing.T) {
	t.Parallel()

	engine := Engine{
		now: func() time.Time {
			return time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC)
		},
	}

	var executed []string
	results, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:     "check.a",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				executed = append(executed, "check.a")
				panic("boom")
			},
		},
		stubCheck{
			id:     "check.b",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				executed = append(executed, "check.b")
				return evidence.CheckResult{
					Status:  evidence.StatusPass,
					Message: "ok",
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !reflect.DeepEqual(executed, []string{"check.a", "check.b"}) {
		t.Fatalf("executed = %v, want %v", executed, []string{"check.a", "check.b"})
	}

	first := results[0]
	if first.Status != evidence.StatusError {
		t.Fatalf("first.Status = %s, want %s", first.Status, evidence.StatusError)
	}
	if first.ReasonCode != evidence.ReasonInternalError {
		t.Fatalf("first.ReasonCode = %s, want %s", first.ReasonCode, evidence.ReasonInternalError)
	}

	second := results[1]
	if second.Status != evidence.StatusPass {
		t.Fatalf("second.Status = %s, want %s", second.Status, evidence.StatusPass)
	}
}

func TestEngineTimesOutSlowChecks(t *testing.T) {
	t.Parallel()

	engine := New().WithCollectorTimeout(10 * time.Millisecond)

	results, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:     "check.slow",
			domain: "services",
			run: func(context.Context) evidence.CheckResult {
				time.Sleep(50 * time.Millisecond)
				return evidence.CheckResult{
					Status:  evidence.StatusPass,
					Message: "too late",
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	result := results[0]
	if result.Status != evidence.StatusError {
		t.Fatalf("result.Status = %s, want %s", result.Status, evidence.StatusError)
	}
	if result.ReasonCode != evidence.ReasonTimeout {
		t.Fatalf("result.ReasonCode = %s, want %s", result.ReasonCode, evidence.ReasonTimeout)
	}
	if result.Evidence.Source != "engine.timeout" {
		t.Fatalf("result.Evidence.Source = %q, want %q", result.Evidence.Source, "engine.timeout")
	}
	if !strings.Contains(result.Message, "collector timed out after 10ms") {
		t.Fatalf("result.Message = %q, want timeout detail", result.Message)
	}
}

func TestEngineReportsParentCancellationWithoutTimeoutReason(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := New().Run(ctx, []Check{
		stubCheck{
			id:     "check.cancelled",
			domain: "paths",
			run: func(context.Context) evidence.CheckResult {
				return evidence.CheckResult{
					Status:  evidence.StatusPass,
					Message: "should not run",
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	result := results[0]
	if result.Status != evidence.StatusError {
		t.Fatalf("result.Status = %s, want %s", result.Status, evidence.StatusError)
	}
	if result.ReasonCode != evidence.ReasonNone {
		t.Fatalf("result.ReasonCode = %q, want empty reason", result.ReasonCode)
	}
	if result.Evidence.Source != "engine.context" {
		t.Fatalf("result.Evidence.Source = %q, want %q", result.Evidence.Source, "engine.context")
	}
	if result.Message != "collector context cancelled" {
		t.Fatalf("result.Message = %q, want %q", result.Message, "collector context cancelled")
	}
}

func TestEngineRejectsCycles(t *testing.T) {
	t.Parallel()

	engine := Engine{
		now: func() time.Time {
			return time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC)
		},
	}

	_, err := engine.Run(context.Background(), []Check{
		stubCheck{
			id:            "check.a",
			domain:        "paths",
			prerequisites: []string{"check.b"},
		},
		stubCheck{
			id:            "check.b",
			domain:        "paths",
			prerequisites: []string{"check.a"},
		},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want cycle error")
	}
	if !strings.Contains(err.Error(), "prerequisite cycle detected") {
		t.Fatalf("Run() error = %q, want cycle error", err.Error())
	}
}
