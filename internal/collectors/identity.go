package collectors

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"savk/internal/capabilities"
	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

var (
	errMainPIDParse      = errors.New("main pid parse error")
	errProcessParse      = errors.New("process status parse error")
	errProcessProvenance = errors.New("process provenance mismatch")
)

type OSProcessReader struct{}

func (OSProcessReader) ReadStatus(ctx context.Context, pid int) (ProcessStatus, error) {
	if err := ctx.Err(); err != nil {
		return ProcessStatus{}, err
	}

	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return ProcessStatus{}, err
	}
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	cgroupData, err := os.ReadFile(cgroupPath)
	if err != nil {
		return ProcessStatus{}, err
	}

	status, err := parseProcessStatus(data)
	if err != nil {
		return ProcessStatus{}, err
	}
	cgroups, err := parseProcessCgroups(cgroupData)
	if err != nil {
		return ProcessStatus{}, err
	}
	status.Cgroups = cgroups
	if trimmed := strings.TrimSpace(string(cgroupData)); trimmed != "" {
		status.Raw += "\n---\n" + trimmed
	}

	return status, nil
}

func BuildIdentityChecks(identities map[string]contract.IdentitySpec, runner CommandRunner, processReader ProcessReader) ([]engine.Check, error) {
	if runner == nil {
		runner = OSCommandRunner{}
	}
	if processReader == nil {
		processReader = OSProcessReader{}
	}
	systemctlPath, commandErr := resolveSystemctlPathForRunner(runner)

	pidReader := &serviceMainPIDReader{
		runner:        runner,
		systemctlPath: systemctlPath,
		commandErr:    commandErr,
		cache:         make(map[string]serviceMainPIDResult, len(identities)),
	}
	observer := &runtimeIdentityObserver{
		pidReader:     pidReader,
		processReader: processReader,
		cache:         make(map[string]runtimeIdentityObservation, len(identities)),
	}

	labels := make([]string, 0, len(identities))
	for label := range identities {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	checks := make([]engine.Check, 0, len(labels)*7)
	for _, label := range labels {
		spec := identities[label]
		if spec.UID != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.uid", label),
				label:    label,
				spec:     spec,
				kind:     "uid",
				observer: observer,
			})
		}
		if spec.GID != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.gid", label),
				label:    label,
				spec:     spec,
				kind:     "gid",
				observer: observer,
			})
		}
		if spec.Capabilities == nil {
			continue
		}
		if spec.Capabilities.Effective != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.capabilities.effective", label),
				label:    label,
				spec:     spec,
				kind:     "capabilities.effective",
				observer: observer,
			})
		}
		if spec.Capabilities.Permitted != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.capabilities.permitted", label),
				label:    label,
				spec:     spec,
				kind:     "capabilities.permitted",
				observer: observer,
			})
		}
		if spec.Capabilities.Inheritable != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.capabilities.inheritable", label),
				label:    label,
				spec:     spec,
				kind:     "capabilities.inheritable",
				observer: observer,
			})
		}
		if spec.Capabilities.Bounding != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.capabilities.bounding", label),
				label:    label,
				spec:     spec,
				kind:     "capabilities.bounding",
				observer: observer,
			})
		}
		if spec.Capabilities.Ambient != nil {
			checks = append(checks, identityCheck{
				id:       fmt.Sprintf("identity.%s.capabilities.ambient", label),
				label:    label,
				spec:     spec,
				kind:     "capabilities.ambient",
				observer: observer,
			})
		}
	}

	return checks, nil
}

type identityCheck struct {
	id       string
	label    string
	spec     contract.IdentitySpec
	kind     string
	observer *runtimeIdentityObserver
}

func (c identityCheck) ID() string {
	return c.id
}

func (c identityCheck) Domain() string {
	return "identity"
}

func (c identityCheck) Prerequisites() []string {
	return []string{fmt.Sprintf("service.%s.state", c.spec.Service)}
}

func (c identityCheck) Run(ctx context.Context) evidence.CheckResult {
	if err := ctx.Err(); err != nil {
		return c.errorResult(evidence.StatusError, evidence.ReasonTimeout, "collector context cancelled before identity check started", runtimeIdentityObservation{err: err})
	}

	observed, err := c.observer.Read(ctx, c.spec.Service)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return c.errorResult(evidence.StatusError, evidence.ReasonTimeout, fmt.Sprintf("collector timed out while reading runtime identity for service %s", c.spec.Service), observed)
		case errors.Is(err, errNamespaceIsolation):
			return c.errorResult(evidence.StatusError, evidence.ReasonNamespaceIsolation, fmt.Sprintf("service-backed checks are observer-local in v0.1.x; observer-local systemd is not reachable for service %s", c.spec.Service), observed)
		case errors.Is(err, errPermissionDenied), errors.Is(err, fs.ErrPermission):
			return c.errorResult(evidence.StatusError, evidence.ReasonPermissionDenied, fmt.Sprintf("permission denied while reading observer-local runtime identity for service %s", c.spec.Service), observed)
		case errors.Is(err, errCommandUnavailable), errors.Is(err, errUnexpectedCommand):
			return c.errorResult(evidence.StatusError, evidence.ReasonParseError, fmt.Sprintf("systemctl executable is not available or not allowlisted for service %s", c.spec.Service), observed)
		case errors.Is(err, errMainPIDParse):
			return c.errorResult(evidence.StatusInsufficientData, evidence.ReasonParseError, fmt.Sprintf("service %s did not expose a usable MainPID", c.spec.Service), observed)
		case errors.Is(err, errProcessParse):
			return c.errorResult(evidence.StatusInsufficientData, evidence.ReasonParseError, fmt.Sprintf("unable to parse /proc status for service %s", c.spec.Service), observed)
		case errors.Is(err, errProcessProvenance):
			return c.errorResult(evidence.StatusInsufficientData, evidence.ReasonNone, fmt.Sprintf("unable to prove that MainPID still belongs to service %s", c.spec.Service), observed)
		case errors.Is(err, os.ErrNotExist):
			return c.errorResult(evidence.StatusInsufficientData, evidence.ReasonNone, fmt.Sprintf("process for service %s disappeared before runtime identity could be read", c.spec.Service), observed)
		default:
			return c.errorResult(evidence.StatusError, evidence.ReasonParseError, fmt.Sprintf("failed to inspect runtime identity for service %s", c.spec.Service), observed)
		}
	}

	switch c.kind {
	case "uid":
		return c.runUID(observed)
	case "gid":
		return c.runGID(observed)
	case "capabilities.effective":
		return c.runCapabilities(observed, "effective", c.spec.Capabilities.Effective, observed.status.Effective)
	case "capabilities.permitted":
		return c.runCapabilities(observed, "permitted", c.spec.Capabilities.Permitted, observed.status.Permitted)
	case "capabilities.inheritable":
		return c.runCapabilities(observed, "inheritable", c.spec.Capabilities.Inheritable, observed.status.Inheritable)
	case "capabilities.bounding":
		return c.runCapabilities(observed, "bounding", c.spec.Capabilities.Bounding, observed.status.Bounding)
	case "capabilities.ambient":
		return c.runCapabilities(observed, "ambient", c.spec.Capabilities.Ambient, observed.status.Ambient)
	default:
		return c.errorResult(evidence.StatusError, evidence.ReasonInternalError, fmt.Sprintf("unsupported identity check kind %q", c.kind), observed)
	}
}

func (c identityCheck) runUID(observed runtimeIdentityObservation) evidence.CheckResult {
	expected := *c.spec.UID
	actual := observed.status.UID
	status := evidence.StatusPass
	message := fmt.Sprintf("runtime uid matches %d", expected)
	if actual != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected runtime uid %d, observed %d", expected, actual)
	}

	return c.result(status, evidence.ReasonNone, expected, actual, message, observed)
}

func (c identityCheck) runGID(observed runtimeIdentityObservation) evidence.CheckResult {
	expected := *c.spec.GID
	actual := observed.status.GID
	status := evidence.StatusPass
	message := fmt.Sprintf("runtime gid matches %d", expected)
	if actual != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected runtime gid %d, observed %d", expected, actual)
	}

	return c.result(status, evidence.ReasonNone, expected, actual, message, observed)
}

func (c identityCheck) runCapabilities(observed runtimeIdentityObservation, setName string, expected, actual []string) evidence.CheckResult {
	expected = capabilities.SortCanonical(expected)
	status := evidence.StatusPass
	message := fmt.Sprintf("runtime %s capabilities match", setName)
	if !equalStrings(expected, actual) {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected %s capabilities %v, observed %v", setName, expected, actual)
	}

	return c.result(status, evidence.ReasonNone, expected, actual, message, observed)
}

func (c identityCheck) result(status evidence.EvalStatus, reason evidence.ReasonCode, expected, actual any, message string, observed runtimeIdentityObservation) evidence.CheckResult {
	return evidence.CheckResult{
		Status:     status,
		ReasonCode: reason,
		Expected:   expected,
		Observed:   actual,
		Evidence: evidence.Evidence{
			Source:      observed.source(),
			Collector:   "identity",
			CollectedAt: time.Now().UTC(),
			Command:     observed.command,
			ExitCode:    observed.exitCodePtr(),
			Raw:         observed.raw,
		},
		Message: message,
	}
}

func (c identityCheck) errorResult(status evidence.EvalStatus, reason evidence.ReasonCode, message string, observed runtimeIdentityObservation) evidence.CheckResult {
	return evidence.CheckResult{
		Status:     status,
		ReasonCode: reason,
		Evidence: evidence.Evidence{
			Source:      observed.source(),
			Collector:   "identity",
			CollectedAt: time.Now().UTC(),
			Command:     observed.command,
			ExitCode:    observed.exitCodePtr(),
			Raw:         observed.raw,
		},
		Message: message,
	}
}

type runtimeIdentityObserver struct {
	pidReader     *serviceMainPIDReader
	processReader ProcessReader
	mu            sync.Mutex
	cache         map[string]runtimeIdentityObservation
}

type runtimeIdentityObservation struct {
	pid          int
	controlGroup string
	status       ProcessStatus
	command      []string
	exitCode     int
	raw          string
	err          error
}

func (o *runtimeIdentityObserver) Read(ctx context.Context, service string) (runtimeIdentityObservation, error) {
	o.mu.Lock()
	if cached, ok := o.cache[service]; ok {
		o.mu.Unlock()
		return cached, cached.err
	}
	o.mu.Unlock()

	observed := runtimeIdentityObservation{}

	pidResult, err := o.pidReader.Read(ctx, service)
	observed.command = pidResult.command
	observed.exitCode = pidResult.exitCode
	observed.raw = pidResult.raw
	observed.controlGroup = pidResult.controlGroup
	if err != nil {
		observed.err = err
		o.store(service, observed)
		return observed, observed.err
	}

	status, err := o.processReader.ReadStatus(ctx, pidResult.pid)
	observed.pid = pidResult.pid
	observed.status = status
	if trimmed := strings.TrimSpace(status.Raw); trimmed != "" {
		if observed.raw != "" {
			observed.raw += "\n---\n"
		}
		observed.raw += trimmed
	}
	if err != nil {
		observed.err = err
		o.store(service, observed)
		return observed, observed.err
	}
	if !cgroupsContain(status.Cgroups, pidResult.controlGroup) {
		observed.err = errProcessProvenance
		o.store(service, observed)
		return observed, observed.err
	}
	observed.err = nil
	o.store(service, observed)
	return observed, observed.err
}

func (o *runtimeIdentityObserver) store(service string, observed runtimeIdentityObservation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cache[service] = observed
}

func (o runtimeIdentityObservation) source() string {
	if o.pid > 0 {
		return fmt.Sprintf("/proc/%d/status", o.pid)
	}
	if len(o.command) > 0 {
		return "systemctl show"
	}
	return "identity.runtime"
}

func (o runtimeIdentityObservation) exitCodePtr() *int {
	if len(o.command) == 0 {
		return nil
	}
	return intPtr(o.exitCode)
}

type serviceMainPIDReader struct {
	runner        CommandRunner
	systemctlPath string
	commandErr    error
	mu            sync.Mutex
	cache         map[string]serviceMainPIDResult
}

type serviceMainPIDResult struct {
	pid          int
	controlGroup string
	command      []string
	exitCode     int
	raw          string
	err          error
}

func (r *serviceMainPIDReader) Read(ctx context.Context, service string) (serviceMainPIDResult, error) {
	r.mu.Lock()
	if cached, ok := r.cache[service]; ok {
		r.mu.Unlock()
		return cached, cached.err
	}
	r.mu.Unlock()

	result := serviceMainPIDResult{
		command: r.command(service),
	}
	if r.commandErr != nil {
		result.raw = r.commandErr.Error()
		result.err = r.commandErr
		r.store(service, result)
		return result, result.err
	}

	runResult, err := r.runner.Run(ctx, result.command)
	result.exitCode = runResult.ExitCode
	result.raw = strings.TrimSpace(runResult.Stdout)
	if trimmed := strings.TrimSpace(runResult.Stderr); trimmed != "" {
		if result.raw != "" {
			result.raw += "\n"
		}
		result.raw += trimmed
	}
	if err != nil {
		result.err = err
		r.store(service, result)
		return result, result.err
	}
	if runResult.ExitCode != 0 {
		result.err = mapSystemctlFailure(runResult)
		r.store(service, result)
		return result, result.err
	}

	parsed, parseErr := parseSystemctlMainPID(runResult.Stdout)
	result.pid = parsed.pid
	result.controlGroup = parsed.controlGroup
	result.err = parseErr
	r.store(service, result)
	return result, result.err
}

func (r *serviceMainPIDReader) store(service string, result serviceMainPIDResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[service] = result
}

func (r *serviceMainPIDReader) command(service string) []string {
	systemctlPath := r.systemctlPath
	if systemctlPath == "" {
		systemctlPath = "systemctl"
	}
	return []string{
		systemctlPath,
		"show",
		service,
		"--property=LoadState",
		"--property=MainPID",
		"--property=ControlGroup",
	}
}

type parsedMainPID struct {
	pid          int
	controlGroup string
}

func parseSystemctlMainPID(stdout string) (parsedMainPID, error) {
	properties, err := parseSystemctlProperties(stdout, []string{"LoadState", "MainPID", "ControlGroup"})
	if err != nil {
		return parsedMainPID{}, err
	}
	if properties["LoadState"] == "not-found" {
		return parsedMainPID{}, errServiceNotFound
	}

	pid, err := strconv.Atoi(strings.TrimSpace(properties["MainPID"]))
	if err != nil || pid <= 0 {
		return parsedMainPID{}, errMainPIDParse
	}
	controlGroup := strings.TrimSpace(properties["ControlGroup"])
	if controlGroup == "" {
		return parsedMainPID{}, errMainPIDParse
	}

	return parsedMainPID{
		pid:          pid,
		controlGroup: controlGroup,
	}, nil
}

func parseProcessStatus(data []byte) (ProcessStatus, error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ProcessStatus{}, errProcessParse
	}

	fields := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || key == "" {
			return ProcessStatus{}, errProcessParse
		}
		fields[key] = strings.TrimSpace(value)
	}

	uid, err := parseEffectiveID(fields["Uid"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}
	gid, err := parseEffectiveID(fields["Gid"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}
	effective, err := parseCapabilityMask(fields["CapEff"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}
	permitted, err := parseCapabilityMask(fields["CapPrm"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}
	inheritable, err := parseCapabilityMask(fields["CapInh"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}
	bounding, err := parseCapabilityMask(fields["CapBnd"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}
	ambient, err := parseCapabilityMask(fields["CapAmb"])
	if err != nil {
		return ProcessStatus{}, errProcessParse
	}

	return ProcessStatus{
		UID:         uid,
		GID:         gid,
		Effective:   effective,
		Permitted:   permitted,
		Inheritable: inheritable,
		Bounding:    bounding,
		Ambient:     ambient,
		Raw:         raw,
	}, nil
}

func parseProcessCgroups(data []byte) ([]string, error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return nil, errProcessParse
	}

	cgroups := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			return nil, errProcessParse
		}
		if strings.TrimSpace(parts[2]) == "" {
			return nil, errProcessParse
		}
		cgroups = append(cgroups, strings.TrimSpace(parts[2]))
	}
	if len(cgroups) == 0 {
		return nil, errProcessParse
	}

	return cgroups, nil
}

func cgroupsContain(cgroups []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}

	for _, observed := range cgroups {
		if strings.TrimSpace(observed) == expected {
			return true
		}
	}

	return false
}

func parseEffectiveID(value string) (int, error) {
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return 0, errProcessParse
	}

	return strconv.Atoi(parts[1])
}

func parseCapabilityMask(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errProcessParse
	}

	mask, err := strconv.ParseUint(value, 16, 64)
	if err != nil {
		return nil, errProcessParse
	}

	capabilities := make([]string, 0)
	for bit := 0; bit < 64; bit++ {
		if mask&(1<<bit) == 0 {
			continue
		}
		capabilities = append(capabilities, linuxCapabilityName(bit))
	}
	sort.Strings(capabilities)
	return capabilities, nil
}

func linuxCapabilityName(bit int) string {
	if name := capabilities.LinuxCapabilityName(bit); name != "" {
		return name
	}

	return fmt.Sprintf("CAP_UNKNOWN_%d", bit)
}
