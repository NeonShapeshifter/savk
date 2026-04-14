package collectors

import (
	"context"
	"io/fs"
	"os"
	"slices"
	"testing"

	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

func TestBuildIdentityChecksPassesForRuntimeProcessIdentity(t *testing.T) {
	t.Parallel()

	runner := &fakeIdentityCommandRunner{
		results: map[string]CommandResult{
			"sensor-agent.service": {
				Stdout: "LoadState=loaded\nMainPID=123\nControlGroup=/system.slice/sensor-agent.service\n",
			},
		},
	}
	processReader := &fakeProcessReader{
		results: map[int]ProcessStatus{
			123: {
				UID:         1001,
				GID:         1001,
				Effective:   []string{"CAP_NET_ADMIN"},
				Permitted:   []string{"CAP_NET_ADMIN"},
				Inheritable: []string{},
				Bounding:    []string{"CAP_NET_ADMIN"},
				Ambient:     []string{},
				Cgroups:     []string{"/system.slice/sensor-agent.service"},
				Raw:         "Uid:\t1001\t1001\t1001\t1001\nGid:\t1001\t1001\t1001\t1001\nCapEff:\t0000000000001000\nCapPrm:\t0000000000001000\nCapInh:\t0000000000000000\nCapBnd:\t0000000000001000\nCapAmb:\t0000000000000000",
			},
		},
	}

	checks, err := BuildIdentityChecks(map[string]contract.IdentitySpec{
		"sensor_runtime": {
			Service: "sensor-agent.service",
			UID:     intPtrValue(1001),
			GID:     intPtrValue(1001),
			Capabilities: &contract.CapabilitySetSpec{
				Effective: []string{"CAP_NET_ADMIN"},
				Permitted: []string{"CAP_NET_ADMIN"},
				Ambient:   []string{},
			},
		},
	}, runner, processReader)
	if err != nil {
		t.Fatalf("BuildIdentityChecks() error = %v", err)
	}
	if len(checks) != 5 {
		t.Fatalf("len(checks) = %d, want 5", len(checks))
	}
	if !slices.Equal(checks[0].Prerequisites(), []string{"service.sensor-agent.service.state"}) {
		t.Fatalf("Prerequisites() = %v, want %v", checks[0].Prerequisites(), []string{"service.sensor-agent.service.state"})
	}

	results, err := engine.New().Run(context.Background(), append([]engine.Check{passCheck{id: "service.sensor-agent.service.state", domain: "services"}}, checks...))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, result := range results {
		if result.Status != evidence.StatusPass {
			t.Fatalf("result %s status = %s, want %s", result.CheckID, result.Status, evidence.StatusPass)
		}
	}
	if runner.calls["sensor-agent.service"] != 1 {
		t.Fatalf("runner.calls = %d, want 1 cached call", runner.calls["sensor-agent.service"])
	}
	if processReader.calls[123] != 1 {
		t.Fatalf("processReader.calls = %d, want 1 cached call", processReader.calls[123])
	}
}

func TestBuildIdentityChecksReturnsInsufficientDataWhenMainPIDIsMissing(t *testing.T) {
	t.Parallel()

	runner := &fakeIdentityCommandRunner{
		results: map[string]CommandResult{
			"sensor-agent.service": {
				Stdout: "LoadState=loaded\nMainPID=0\nControlGroup=/system.slice/sensor-agent.service\n",
			},
		},
	}

	checks, err := BuildIdentityChecks(map[string]contract.IdentitySpec{
		"sensor_runtime": {
			Service: "sensor-agent.service",
			UID:     intPtrValue(1001),
		},
	}, runner, &fakeProcessReader{})
	if err != nil {
		t.Fatalf("BuildIdentityChecks() error = %v", err)
	}

	results, err := engine.New().Run(context.Background(), append([]engine.Check{passCheck{id: "service.sensor-agent.service.state", domain: "services"}}, checks...))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	got := results[1]
	if got.Status != evidence.StatusInsufficientData || got.ReasonCode != evidence.ReasonParseError {
		t.Fatalf("result = (%s, %s), want (%s, %s)", got.Status, got.ReasonCode, evidence.StatusInsufficientData, evidence.ReasonParseError)
	}
}

func TestBuildIdentityChecksMapsProcessPermissionDenied(t *testing.T) {
	t.Parallel()

	runner := &fakeIdentityCommandRunner{
		results: map[string]CommandResult{
			"sensor-agent.service": {
				Stdout: "LoadState=loaded\nMainPID=123\nControlGroup=/system.slice/sensor-agent.service\n",
			},
		},
	}
	processReader := &fakeProcessReader{
		errs: map[int]error{
			123: fs.ErrPermission,
		},
	}

	checks, err := BuildIdentityChecks(map[string]contract.IdentitySpec{
		"sensor_runtime": {
			Service: "sensor-agent.service",
			UID:     intPtrValue(1001),
		},
	}, runner, processReader)
	if err != nil {
		t.Fatalf("BuildIdentityChecks() error = %v", err)
	}

	results, err := engine.New().Run(context.Background(), append([]engine.Check{passCheck{id: "service.sensor-agent.service.state", domain: "services"}}, checks...))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := results[1]
	if got.Status != evidence.StatusError || got.ReasonCode != evidence.ReasonPermissionDenied {
		t.Fatalf("result = (%s, %s), want (%s, %s)", got.Status, got.ReasonCode, evidence.StatusError, evidence.ReasonPermissionDenied)
	}
}

func TestBuildIdentityChecksReturnsInsufficientDataOnControlGroupMismatch(t *testing.T) {
	t.Parallel()

	runner := &fakeIdentityCommandRunner{
		results: map[string]CommandResult{
			"sensor-agent.service": {
				Stdout: "LoadState=loaded\nMainPID=123\nControlGroup=/system.slice/sensor-agent.service\n",
			},
		},
	}
	processReader := &fakeProcessReader{
		results: map[int]ProcessStatus{
			123: {
				UID:         1001,
				GID:         1001,
				Effective:   []string{},
				Permitted:   []string{},
				Inheritable: []string{},
				Bounding:    []string{},
				Ambient:     []string{},
				Cgroups:     []string{"/system.slice/other.service"},
				Raw:         "Uid:\t1001\t1001\t1001\t1001",
			},
		},
	}

	checks, err := BuildIdentityChecks(map[string]contract.IdentitySpec{
		"sensor_runtime": {
			Service: "sensor-agent.service",
			UID:     intPtrValue(1001),
		},
	}, runner, processReader)
	if err != nil {
		t.Fatalf("BuildIdentityChecks() error = %v", err)
	}

	results, err := engine.New().Run(context.Background(), append([]engine.Check{passCheck{id: "service.sensor-agent.service.state", domain: "services"}}, checks...))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := results[1]
	if got.Status != evidence.StatusInsufficientData || got.ReasonCode != evidence.ReasonNone {
		t.Fatalf("result = (%s, %s), want (%s, %s)", got.Status, got.ReasonCode, evidence.StatusInsufficientData, evidence.ReasonNone)
	}
}

type fakeIdentityCommandRunner struct {
	results map[string]CommandResult
	errs    map[string]error
	calls   map[string]int
}

func (f *fakeIdentityCommandRunner) Run(ctx context.Context, argv []string) (CommandResult, error) {
	_ = ctx
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	name := argv[2]
	f.calls[name]++
	if err, ok := f.errs[name]; ok {
		return CommandResult{}, err
	}
	if result, ok := f.results[name]; ok {
		return result, nil
	}
	return CommandResult{ExitCode: 1, Stderr: "Unit not found"}, nil
}

type fakeProcessReader struct {
	results map[int]ProcessStatus
	errs    map[int]error
	calls   map[int]int
}

func (f *fakeProcessReader) ReadStatus(ctx context.Context, pid int) (ProcessStatus, error) {
	_ = ctx
	if f.calls == nil {
		f.calls = make(map[int]int)
	}
	f.calls[pid]++
	if err, ok := f.errs[pid]; ok {
		return ProcessStatus{}, err
	}
	if result, ok := f.results[pid]; ok {
		return result, nil
	}
	return ProcessStatus{}, os.ErrNotExist
}

type passCheck struct {
	id     string
	domain string
}

func (c passCheck) ID() string {
	return c.id
}

func (c passCheck) Domain() string {
	return c.domain
}

func (c passCheck) Prerequisites() []string {
	return nil
}

func (c passCheck) Run(ctx context.Context) evidence.CheckResult {
	_ = ctx
	return evidence.CheckResult{
		Status:     evidence.StatusPass,
		ReasonCode: evidence.ReasonNone,
		Message:    "pass",
	}
}

func intPtrValue(value int) *int {
	return &value
}
