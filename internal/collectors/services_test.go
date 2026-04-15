package collectors

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

type fakeCommandRunner struct {
	results map[string]CommandResult
	errs    map[string]error
	calls   map[string]int
}

func (f *fakeCommandRunner) Run(ctx context.Context, argv []string) (CommandResult, error) {
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
	return CommandResult{ExitCode: 1, Stderr: fmt.Sprintf("Unit %s could not be found.", name)}, nil
}

func TestBuildServiceChecksPassesForStateRestartRunAsAndCapabilities(t *testing.T) {
	t.Parallel()

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"sensor-agent.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=on-failure",
					"User=sensor",
					"Group=sensor",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	restart := contract.RestartPolicyOnFailure
	checks := BuildServiceChecks(map[string]contract.ServiceSpec{
		"sensor-agent.service": {
			State:   contract.ServiceStateActive,
			Restart: &restart,
			RunAs: &contract.RunAsSpec{
				User:  "sensor",
				Group: "sensor",
			},
			Capabilities: []string{},
		},
	}, runner)

	results, err := engine.New().Run(context.Background(), checks)
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
}

func TestBuildServiceChecksHandlesMissingUnit(t *testing.T) {
	t.Parallel()

	restart := contract.RestartPolicyAlways
	checks := BuildServiceChecks(map[string]contract.ServiceSpec{
		"missing.service": {
			State:   contract.ServiceStateActive,
			Restart: &restart,
		},
	}, &fakeCommandRunner{})

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	if results[0].Status != evidence.StatusFail || results[0].ReasonCode != evidence.ReasonNotFound {
		t.Fatalf("state result = (%s, %s), want (%s, %s)", results[0].Status, results[0].ReasonCode, evidence.StatusFail, evidence.ReasonNotFound)
	}
	if results[1].Status != evidence.StatusNotApplicable || results[1].ReasonCode != evidence.ReasonPrerequisiteFailed {
		t.Fatalf("restart result = (%s, %s), want (%s, %s)", results[1].Status, results[1].ReasonCode, evidence.StatusNotApplicable, evidence.ReasonPrerequisiteFailed)
	}
}

func TestBuildServiceChecksMapsSystemdUnavailableToNamespaceIsolation(t *testing.T) {
	t.Parallel()

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"sensor-agent.service": {
				ExitCode: 1,
				Stderr:   "System has not been booted with systemd as init system (PID 1). Can't operate.",
			},
		},
	}

	checks := BuildServiceChecks(map[string]contract.ServiceSpec{
		"sensor-agent.service": {
			State: contract.ServiceStateActive,
		},
	}, runner)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != evidence.StatusError || results[0].ReasonCode != evidence.ReasonNamespaceIsolation {
		t.Fatalf("result = (%s, %s), want (%s, %s)", results[0].Status, results[0].ReasonCode, evidence.StatusError, evidence.ReasonNamespaceIsolation)
	}
	if len(results[0].Evidence.Command) == 0 {
		t.Fatalf("Evidence.Command = %v, want populated command", results[0].Evidence.Command)
	}
}

func TestBuildServiceChecksUsesSystemdDefaultRootUser(t *testing.T) {
	t.Parallel()

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"default-root.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=",
					"Group=",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := BuildServiceChecks(map[string]contract.ServiceSpec{
		"default-root.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User: "root",
			},
		},
	}, runner)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[1].Status != evidence.StatusPass {
		t.Fatalf("user result status = %s, want %s", results[1].Status, evidence.StatusPass)
	}
}

func TestBuildServiceChecksUsesPrimaryGroupWhenGroupPropertyIsBlank(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot,
		[]string{"sensor:x:1001:1002::/nonexistent:/usr/sbin/nologin"},
		[]string{"sensor:x:1002:"},
	)

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"default-group.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=1001",
					"Group=",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"default-group.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User:  "sensor",
				Group: "sensor",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[1].Status != evidence.StatusPass {
		t.Fatalf("user result status = %s, want %s", results[1].Status, evidence.StatusPass)
	}
	if results[2].Status != evidence.StatusPass {
		t.Fatalf("group result status = %s, want %s", results[2].Status, evidence.StatusPass)
	}
}

func TestBuildServiceChecksReturnsInsufficientDataWhenNumericUserCannotBeResolved(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot, []string{}, []string{"sensor:x:1002:"})

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"numeric-user.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=1001",
					"Group=",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"numeric-user.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User: "sensor",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[1].Status != evidence.StatusInsufficientData {
		t.Fatalf("user result status = %s, want %s", results[1].Status, evidence.StatusInsufficientData)
	}
	if results[1].ReasonCode != evidence.ReasonParseError {
		t.Fatalf("user result reason = %s, want %s", results[1].ReasonCode, evidence.ReasonParseError)
	}
}

func TestBuildServiceChecksTreatsLiteralNumericUserNameAsName(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot,
		[]string{
			"1001:x:2000:3000::/nonexistent:/usr/sbin/nologin",
			"shadow:x:1001:1002::/nonexistent:/usr/sbin/nologin",
		},
		[]string{"grp3000:x:3000:"},
	)

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"literal-numeric-user.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=1001",
					"Group=",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"literal-numeric-user.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User: "1001",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[1].Status != evidence.StatusPass {
		t.Fatalf("user result status = %s, want %s", results[1].Status, evidence.StatusPass)
	}
}

func TestBuildServiceChecksTreatsLiteralNumericGroupNameAsName(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot,
		[]string{"root:x:0:0::/root:/bin/sh"},
		[]string{
			"1002:x:4000:",
			"shadowgrp:x:1002:",
		},
	)

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"literal-numeric-group.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=root",
					"Group=1002",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"literal-numeric-group.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User:  "root",
				Group: "1002",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	byID := make(map[string]evidence.CheckResult, len(results))
	for _, result := range results {
		byID[result.CheckID] = result
	}
	group := byID["service.literal-numeric-group.service.run_as.group"]
	if group.Status != evidence.StatusPass {
		t.Fatalf("group result status = %s, want %s", group.Status, evidence.StatusPass)
	}
}

func TestBuildServiceChecksReturnInsufficientDataWhenNumericRunAsValuesAreAmbiguous(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot,
		[]string{
			"alpha:x:1001:1002::/nonexistent:/usr/sbin/nologin",
			"beta:x:1001:1002::/nonexistent:/usr/sbin/nologin",
		},
		[]string{
			"alpha:x:1002:",
			"beta:x:1002:",
		},
	)

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"ambiguous-user.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=1001",
					"Group=1002",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"ambiguous-user.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User:  "alpha",
				Group: "alpha",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	byID := make(map[string]evidence.CheckResult, len(results))
	for _, result := range results {
		byID[result.CheckID] = result
	}
	user := byID["service.ambiguous-user.service.run_as.user"]
	if user.Status != evidence.StatusInsufficientData {
		t.Fatalf("user result status = %s, want %s", user.Status, evidence.StatusInsufficientData)
	}
	if user.ReasonCode != evidence.ReasonParseError {
		t.Fatalf("user result reason = %s, want %s", user.ReasonCode, evidence.ReasonParseError)
	}
	group := byID["service.ambiguous-user.service.run_as.group"]
	if group.Status != evidence.StatusInsufficientData {
		t.Fatalf("group result status = %s, want %s", group.Status, evidence.StatusInsufficientData)
	}
	if group.ReasonCode != evidence.ReasonParseError {
		t.Fatalf("group result reason = %s, want %s", group.ReasonCode, evidence.ReasonParseError)
	}
}

func TestBuildServiceChecksReturnInsufficientDataWhenLiteralNumericRunAsNamesAreAmbiguous(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot,
		[]string{
			"1001:x:2000:3000::/nonexistent:/usr/sbin/nologin",
			"1001:x:2001:3001::/nonexistent:/usr/sbin/nologin",
		},
		[]string{
			"1002:x:4000:",
			"1002:x:4001:",
		},
	)

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"ambiguous-literal-numeric.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=1001",
					"Group=1002",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"ambiguous-literal-numeric.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User:  "1001",
				Group: "1002",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[1].Status != evidence.StatusInsufficientData {
		t.Fatalf("user result status = %s, want %s", results[1].Status, evidence.StatusInsufficientData)
	}
	if results[1].ReasonCode != evidence.ReasonParseError {
		t.Fatalf("user result reason = %s, want %s", results[1].ReasonCode, evidence.ReasonParseError)
	}
	if results[2].Status != evidence.StatusInsufficientData {
		t.Fatalf("group result status = %s, want %s", results[2].Status, evidence.StatusInsufficientData)
	}
	if results[2].ReasonCode != evidence.ReasonParseError {
		t.Fatalf("group result reason = %s, want %s", results[2].ReasonCode, evidence.ReasonParseError)
	}
}

func TestBuildServiceChecksReturnsInsufficientDataWhenPrimaryGroupLookupIsAmbiguous(t *testing.T) {
	t.Parallel()

	accountRoot := t.TempDir()
	writeTestAccountFiles(t, accountRoot,
		[]string{
			"sensor:x:1001:1002::/nonexistent:/usr/sbin/nologin",
			"sensor:x:1003:1004::/nonexistent:/usr/sbin/nologin",
		},
		[]string{
			"sensor-a:x:1002:",
			"sensor-b:x:1004:",
		},
	)

	runner := &fakeCommandRunner{
		results: map[string]CommandResult{
			"ambiguous-primary-group.service": {
				Stdout: strings.Join([]string{
					"LoadState=loaded",
					"ActiveState=active",
					"Restart=no",
					"User=sensor",
					"Group=",
					"AmbientCapabilities=",
				}, "\n"),
			},
		},
	}

	checks := buildServiceChecksWithResolver(map[string]contract.ServiceSpec{
		"ambiguous-primary-group.service": {
			State: contract.ServiceStateActive,
			RunAs: &contract.RunAsSpec{
				User:  "sensor",
				Group: "sensor-a",
			},
		},
	}, runner, NewAccountResolver(accountRoot), false)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	byID := make(map[string]evidence.CheckResult, len(results))
	for _, result := range results {
		byID[result.CheckID] = result
	}

	user := byID["service.ambiguous-primary-group.service.run_as.user"]
	if user.Status != evidence.StatusPass {
		t.Fatalf("user result status = %s, want %s", user.Status, evidence.StatusPass)
	}

	group := byID["service.ambiguous-primary-group.service.run_as.group"]
	if group.Status != evidence.StatusInsufficientData {
		t.Fatalf("group result status = %s, want %s", group.Status, evidence.StatusInsufficientData)
	}
	if group.ReasonCode != evidence.ReasonParseError {
		t.Fatalf("group result reason = %s, want %s", group.ReasonCode, evidence.ReasonParseError)
	}
}

func TestBuildServiceChecksMapsContextDeadlineToTimeout(t *testing.T) {
	t.Parallel()

	runner := &fakeCommandRunner{
		errs: map[string]error{
			"slow.service": context.DeadlineExceeded,
		},
	}

	checks := BuildServiceChecks(map[string]contract.ServiceSpec{
		"slow.service": {
			State: contract.ServiceStateActive,
		},
	}, runner)

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != evidence.StatusError || results[0].ReasonCode != evidence.ReasonTimeout {
		t.Fatalf("result = (%s, %s), want (%s, %s)", results[0].Status, results[0].ReasonCode, evidence.StatusError, evidence.ReasonTimeout)
	}
}

func TestResolveSystemctlPathForOSRunnerPinsAllowlistedPath(t *testing.T) {
	previous := lookPathExecutable
	lookPathExecutable = func(name string) (string, error) {
		if name != "systemctl" {
			t.Fatalf("lookPathExecutable() name = %q, want %q", name, "systemctl")
		}
		return "/usr/bin/systemctl", nil
	}
	t.Cleanup(func() {
		lookPathExecutable = previous
	})

	path, err := resolveSystemctlPathForRunner(OSCommandRunner{})
	if err != nil {
		t.Fatalf("resolveSystemctlPathForRunner() error = %v", err)
	}
	if path != "/usr/bin/systemctl" {
		t.Fatalf("path = %q, want %q", path, "/usr/bin/systemctl")
	}
}

func TestResolveSystemctlPathForOSRunnerRejectsUnexpectedPath(t *testing.T) {
	previous := lookPathExecutable
	lookPathExecutable = func(string) (string, error) {
		return "/tmp/fake/systemctl", nil
	}
	t.Cleanup(func() {
		lookPathExecutable = previous
	})

	_, err := resolveSystemctlPathForRunner(OSCommandRunner{})
	if err == nil {
		t.Fatal("resolveSystemctlPathForRunner() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "/tmp/fake/systemctl") {
		t.Fatalf("resolveSystemctlPathForRunner() error = %q, want path detail", err.Error())
	}
}

func TestServiceNamespaceCheckPassesWhenPID1IsSystemd(t *testing.T) {
	t.Parallel()

	result := NewServiceNamespaceCheck(contract.TargetLinuxSystemd, fakeServiceNamespaceProbe{value: "systemd"}).Run(context.Background())
	if result.Status != evidence.StatusPass {
		t.Fatalf("result.Status = %s, want %s", result.Status, evidence.StatusPass)
	}
	if result.Observed != "systemd" {
		t.Fatalf("result.Observed = %v, want systemd", result.Observed)
	}
}

func TestServiceNamespaceCheckDetectsNamespaceIsolation(t *testing.T) {
	t.Parallel()

	result := NewServiceNamespaceCheck(contract.TargetLinuxSystemd, fakeServiceNamespaceProbe{value: "init"}).Run(context.Background())
	if result.Status != evidence.StatusError {
		t.Fatalf("result.Status = %s, want %s", result.Status, evidence.StatusError)
	}
	if result.ReasonCode != evidence.ReasonNamespaceIsolation {
		t.Fatalf("result.ReasonCode = %s, want %s", result.ReasonCode, evidence.ReasonNamespaceIsolation)
	}
}

func TestPathNamespaceCheckUsesPathsDomain(t *testing.T) {
	t.Parallel()

	check := NewPathNamespaceCheck(contract.TargetLinuxSystemd, fakeServiceNamespaceProbe{value: "init"})
	if check.Domain() != "paths" {
		t.Fatalf("check.Domain() = %q, want %q", check.Domain(), "paths")
	}
	result := check.Run(context.Background())
	if result.Evidence.Collector != "paths" {
		t.Fatalf("result.Evidence.Collector = %q, want %q", result.Evidence.Collector, "paths")
	}
	if result.ReasonCode != evidence.ReasonNamespaceIsolation {
		t.Fatalf("result.ReasonCode = %s, want %s", result.ReasonCode, evidence.ReasonNamespaceIsolation)
	}
}

func TestSocketNamespaceCheckUsesSocketsDomain(t *testing.T) {
	t.Parallel()

	check := NewSocketNamespaceCheck(contract.TargetLinuxSystemd, fakeServiceNamespaceProbe{value: "init"})
	if check.Domain() != "sockets" {
		t.Fatalf("check.Domain() = %q, want %q", check.Domain(), "sockets")
	}
	result := check.Run(context.Background())
	if result.Evidence.Collector != "sockets" {
		t.Fatalf("result.Evidence.Collector = %q, want %q", result.Evidence.Collector, "sockets")
	}
	if result.ReasonCode != evidence.ReasonNamespaceIsolation {
		t.Fatalf("result.ReasonCode = %s, want %s", result.ReasonCode, evidence.ReasonNamespaceIsolation)
	}
}

type fakeServiceNamespaceProbe struct {
	value string
	err   error
}

func (f fakeServiceNamespaceProbe) PID1Comm(ctx context.Context) (string, error) {
	_ = ctx
	return f.value, f.err
}

func writeTestAccountFiles(t *testing.T, root string, passwdLines, groupLines []string) {
	t.Helper()

	etcDir := filepath.Join(root, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v", etcDir, err)
	}

	passwdPath := filepath.Join(etcDir, "passwd")
	groupPath := filepath.Join(etcDir, "group")
	if err := os.WriteFile(passwdPath, []byte(strings.Join(passwdLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", passwdPath, err)
	}
	if err := os.WriteFile(groupPath, []byte(strings.Join(groupLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", groupPath, err)
	}
}
