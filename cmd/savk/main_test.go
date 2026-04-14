package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"savk/internal/collectors"
)

func TestRunValidateExitCodes(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "valid contract",
			args:       []string{"validate", "--contract", fixturePath("valid", "minimal-paths.yaml")},
			wantCode:   0,
			wantStdout: "contract valid\n",
		},
		{
			name:       "invalid contract",
			args:       []string{"validate", "--contract", fixturePath("invalid", "unknown-field.yaml")},
			wantCode:   3,
			wantStderr: `unknown field "onwer" at paths./etc/savk/config.yaml`,
		},
		{
			name:       "missing flag",
			args:       []string{"validate"},
			wantCode:   3,
			wantStderr: "missing required flag --contract",
		},
		{
			name:       "unknown command",
			args:       []string{"wat"},
			wantCode:   3,
			wantStderr: `unknown command "wat"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			gotCode := run(tc.args, &stdout, &stderr)
			if gotCode != tc.wantCode {
				t.Fatalf("run() code = %d, want %d", gotCode, tc.wantCode)
			}
			if stdout.String() != tc.wantStdout {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

func TestRunCheckJSON(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}

	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json",
		"  target: linux-systemd",
		"paths:",
		"  " + target + ":",
		`    mode: "0640"`,
		"    type: file",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "systemd"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(check) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schemaVersion": "savk-report/v1"`) {
		t.Fatalf("stdout missing schemaVersion: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"status": "PASS"`) {
		t.Fatalf("stdout missing PASS result: %s", stdout.String())
	}
}

func TestRunCheckRejectsZeroCollectorTimeout(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: zero-timeout",
		"  target: linux-systemd",
		"paths:",
		"  /etc/hosts:",
		"    type: file",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--collector-timeout", "0"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("run(check zero timeout) code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "--collector-timeout must be > 0") {
		t.Fatalf("stderr = %q, want timeout validation message", stderr.String())
	}
}

func TestRunCheckRejectsRelativeHostRoot(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: relative-host-root",
		"  target: linux-systemd",
		"paths:",
		"  /etc/hosts:",
		"    type: file",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--host-root", "relative-root"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("run(check relative host-root) code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "--host-root must be an absolute path") {
		t.Fatalf("stderr = %q, want host-root validation message", stderr.String())
	}
}

func TestRunCheckRejectsHostRootForServices(t *testing.T) {
	dir := t.TempDir()
	hostRoot := filepath.Join(dir, "host")
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(hostRoot) error = %v", err)
	}

	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: host-root-services",
		"  target: linux-systemd",
		"services:",
		"  sensor-agent.service:",
		"    state: active",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--host-root", hostRoot}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("run(check host-root services) code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "--host-root is only supported for paths and sockets in v0.1") {
		t.Fatalf("stderr = %q, want service-backed host-root validation message", stderr.String())
	}
}

func TestRunCheckJSONIdentity(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json-identity",
		"  target: linux-systemd",
		"identity:",
		"  sensor_runtime:",
		"    service: sensor-agent.service",
		"    uid: 1001",
		"    gid: 1001",
		"    capabilities:",
		"      effective: []",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	previousRunner := newCommandRunner
	newCommandRunner = func() collectors.CommandRunner {
		return fakeMainCommandRunner{
			results: map[string]collectors.CommandResult{
				"sensor-agent.service": {
					Stdout: "LoadState=loaded\nActiveState=active\nRestart=no\nUser=\nGroup=\nAmbientCapabilities=\nMainPID=123\nControlGroup=/system.slice/sensor-agent.service\n",
				},
			},
		}
	}
	t.Cleanup(func() {
		newCommandRunner = previousRunner
	})
	previousProcessReader := newProcessReader
	newProcessReader = func() collectors.ProcessReader {
		return fakeMainProcessReader{
			results: map[int]collectors.ProcessStatus{
				123: {
					UID:         1001,
					GID:         1001,
					Effective:   []string{},
					Permitted:   []string{},
					Inheritable: []string{},
					Bounding:    []string{},
					Ambient:     []string{},
					Cgroups:     []string{"/system.slice/sensor-agent.service"},
					Raw:         "Uid:\t1001\t1001\t1001\t1001",
				},
			},
		}
	}
	t.Cleanup(func() {
		newProcessReader = previousProcessReader
	})
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "systemd"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "identity"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(check identity) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"domain": "identity"`) {
		t.Fatalf("stdout missing identity domain: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"checkID": "service.__preflight__.namespace"`) {
		t.Fatalf("stdout missing services namespace preflight: %s", stdout.String())
	}
}

func TestRunCheckJSONServices(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json-services",
		"  target: linux-systemd",
		"services:",
		"  sensor-agent.service:",
		"    state: active",
		"    run_as:",
		"      user: sensor",
		"      group: sensor",
		"    restart: on-failure",
		"    capabilities: []",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	previous := newCommandRunner
	newCommandRunner = func() collectors.CommandRunner {
		return fakeMainCommandRunner{
			results: map[string]collectors.CommandResult{
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
	}
	t.Cleanup(func() {
		newCommandRunner = previous
	})
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "systemd"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "services"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(check services) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"domain": "services"`) {
		t.Fatalf("stdout missing services domain: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"status": "PASS"`) {
		t.Fatalf("stdout missing PASS result: %s", stdout.String())
	}
}

func TestRunCheckJSONIncludeRawPreservesCollectorRaw(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json-include-raw",
		"  target: linux-systemd",
		"services:",
		"  sensor-agent.service:",
		"    state: active",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	previousRunner := newCommandRunner
	newCommandRunner = func() collectors.CommandRunner {
		return fakeMainCommandRunner{
			results: map[string]collectors.CommandResult{
				"sensor-agent.service": {
					Stdout: "LoadState=loaded\nActiveState=active\nRestart=no\nUser=\nGroup=\nAmbientCapabilities=\n",
					Stderr: "Authorization: Bearer super-secret-token",
				},
			},
		}
	}
	t.Cleanup(func() {
		newCommandRunner = previousRunner
	})
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "systemd"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "services", "--include-raw"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(check include-raw) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "super-secret-token") {
		t.Fatalf("stdout missing full collector raw under --include-raw: %s", stdout.String())
	}
}

func TestRunCheckTable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-table",
		"  target: linux-systemd",
		"paths:",
		"  " + target + ":",
		`    mode: "0640"`,
		"    type: file",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "systemd"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "table"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(check table) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "STATUS  DOMAIN  CHECK ID") {
		t.Fatalf("stdout missing table header: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Summary:") {
		t.Fatalf("stdout missing summary: %s", stdout.String())
	}
}

func TestRunCheckJSONCollectorTimeout(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-timeout",
		"  target: linux-systemd",
		"services:",
		"  sensor-agent.service:",
		"    state: active",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	previousRunner := newCommandRunner
	newCommandRunner = func() collectors.CommandRunner {
		return fakeMainCommandRunner{
			delay: 50 * time.Millisecond,
			results: map[string]collectors.CommandResult{
				"sensor-agent.service": {
					Stdout: "LoadState=loaded\nActiveState=active\nRestart=no\nUser=\nGroup=\nAmbientCapabilities=\n",
				},
			},
		}
	}
	t.Cleanup(func() {
		newCommandRunner = previousRunner
	})
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "systemd"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "services", "--collector-timeout", "10ms"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(check timeout) code = %d, want 2; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"reasonCode": "TIMEOUT"`) {
		t.Fatalf("stdout missing TIMEOUT reason: %s", stdout.String())
	}
}

func TestRunCheckJSONServicesNamespaceIsolation(t *testing.T) {
	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json-services-namespace",
		"  target: linux-systemd",
		"services:",
		"  sensor-agent.service:",
		"    state: active",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	callCount := 0
	previousRunner := newCommandRunner
	newCommandRunner = func() collectors.CommandRunner {
		return fakeMainCommandRunner{calls: &callCount}
	}
	t.Cleanup(func() {
		newCommandRunner = previousRunner
	})
	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "init"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "services"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(check namespace isolation) code = %d, want 2; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"checkID": "service.__preflight__.namespace"`) {
		t.Fatalf("stdout missing service preflight check: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"reasonCode": "NAMESPACE_ISOLATION"`) {
		t.Fatalf("stdout missing namespace isolation reason: %s", stdout.String())
	}
	if callCount != 0 {
		t.Fatalf("command runner callCount = %d, want 0 when preflight blocks services", callCount)
	}
}

func TestRunCheckJSONPathsNamespaceIsolation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json-paths-namespace",
		"  target: linux-systemd",
		"paths:",
		"  " + target + ":",
		"    type: file",
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "init"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "paths"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(check paths namespace isolation) code = %d, want 2; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"checkID": "path.__preflight__.namespace"`) {
		t.Fatalf("stdout missing path preflight check: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"reasonCode": "NAMESPACE_ISOLATION"`) {
		t.Fatalf("stdout missing namespace isolation reason: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"status": "PASS"`) {
		t.Fatalf("stdout unexpectedly reports PASS under invalid namespace: %s", stdout.String())
	}
}

func TestRunCheckJSONPathsHostRootBypassesNamespaceHeuristic(t *testing.T) {
	dir := t.TempDir()
	hostRoot := filepath.Join(dir, "host")
	target := filepath.Join(hostRoot, "etc", "savk", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(target dir) error = %v", err)
	}
	if err := os.WriteFile(target, []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("os.WriteFile(target) error = %v", err)
	}

	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: check-json-paths-host-root",
		"  target: linux-systemd",
		"paths:",
		"  /etc/savk/config.yaml:",
		"    type: file",
		`    mode: "0640"`,
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	previousProbe := newServiceNamespaceProbe
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe {
		return fakeMainNamespaceProbe{value: "init"}
	}
	t.Cleanup(func() {
		newServiceNamespaceProbe = previousProbe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "paths", "--host-root", hostRoot}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(check paths host-root) code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), `"checkID": "path.__preflight__.namespace"`) {
		t.Fatalf("stdout unexpectedly contains namespace preflight under host-root: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"status": "PASS"`) {
		t.Fatalf("stdout missing PASS result under host-root: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"hostRoot": "`+hostRoot+`"`) {
		t.Fatalf("stdout missing hostRoot report context: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), hostRoot+`/etc/savk/config.yaml`) {
		t.Fatalf("stdout missing resolved host-root path in evidence: %s", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	previousVersion := version
	previousCommit := commit
	previousBuildDate := buildDate
	version = "0.1.0-test"
	commit = "abc1234"
	buildDate = "2026-04-13T12:00:00Z"
	t.Cleanup(func() {
		version = previousVersion
		commit = previousCommit
		buildDate = previousBuildDate
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(version) code = %d, want 0", code)
	}
	want := fmt.Sprintf(
		"savk %s\ncommit: %s\nbuildDate: %s\nplatform: %s/%s\ncontractVersion: savk/v1\nreportSchema: savk-report/v1\n",
		version,
		commit,
		buildDate,
		runtime.GOOS,
		runtime.GOARCH,
	)
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func fixturePath(kind, name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", kind, name)
}

type fakeMainCommandRunner struct {
	results map[string]collectors.CommandResult
	delay   time.Duration
	calls   *int
}

func (f fakeMainCommandRunner) Run(ctx context.Context, argv []string) (collectors.CommandResult, error) {
	_ = ctx
	if f.calls != nil {
		*f.calls = *f.calls + 1
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	name := argv[2]
	if result, ok := f.results[name]; ok {
		return result, nil
	}
	return collectors.CommandResult{ExitCode: 1, Stderr: "Unit not found"}, nil
}

type fakeMainNamespaceProbe struct {
	value string
	err   error
}

func (f fakeMainNamespaceProbe) PID1Comm(ctx context.Context) (string, error) {
	_ = ctx
	return f.value, f.err
}

type fakeMainProcessReader struct {
	results map[int]collectors.ProcessStatus
	errs    map[int]error
}

func (f fakeMainProcessReader) ReadStatus(ctx context.Context, pid int) (collectors.ProcessStatus, error) {
	_ = ctx
	if err, ok := f.errs[pid]; ok {
		return collectors.ProcessStatus{}, err
	}
	if result, ok := f.results[pid]; ok {
		return result, nil
	}
	return collectors.ProcessStatus{}, os.ErrNotExist
}
