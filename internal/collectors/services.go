package collectors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type OSCommandRunner struct{}

var lookPathExecutable = exec.LookPath

var allowedSystemctlPaths = map[string]struct{}{
	"/usr/bin/systemctl": {},
	"/bin/systemctl":     {},
}

func ResolveObserverLocalSystemctlPath() (string, error) {
	return resolveAllowedSystemctlPath()
}

func (OSCommandRunner) Run(ctx context.Context, argv []string) (CommandResult, error) {
	if len(argv) == 0 {
		return CommandResult{}, fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = stableLocaleEnv(os.Environ())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	return result, err
}

func BuildServiceChecks(services map[string]contract.ServiceSpec, runner CommandRunner) []engine.Check {
	return buildServiceChecksWithResolver(services, runner, nil, false)
}

func BuildServiceStateChecks(services map[string]contract.ServiceSpec, runner CommandRunner) []engine.Check {
	return buildServiceChecksWithResolver(services, runner, nil, true)
}

func buildServiceChecksWithResolver(services map[string]contract.ServiceSpec, runner CommandRunner, resolver AccountResolver, stateOnly bool) []engine.Check {
	if runner == nil {
		runner = OSCommandRunner{}
	}
	if resolver == nil {
		resolver = NewAccountResolver("")
	}
	systemctlPath, commandErr := resolveSystemctlPathForRunner(runner)

	reader := &serviceUnitReader{
		runner:        runner,
		systemctlPath: systemctlPath,
		commandErr:    commandErr,
		cache:         make(map[string]serviceUnitResult, len(services)),
	}

	keys := make([]string, 0, len(services))
	for name := range services {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	checks := make([]engine.Check, 0, len(keys)*5)
	for _, name := range keys {
		spec := services[name]
		checks = append(checks, serviceCheck{
			id:       fmt.Sprintf("service.%s.state", name),
			name:     name,
			spec:     spec,
			kind:     "state",
			reader:   reader,
			resolver: resolver,
		})
		if stateOnly {
			continue
		}
		if spec.Restart != nil {
			checks = append(checks, serviceCheck{
				id:       fmt.Sprintf("service.%s.restart", name),
				name:     name,
				spec:     spec,
				kind:     "restart",
				reader:   reader,
				resolver: resolver,
			})
		}
		if spec.RunAs != nil {
			checks = append(checks, serviceCheck{
				id:       fmt.Sprintf("service.%s.run_as.user", name),
				name:     name,
				spec:     spec,
				kind:     "run_as.user",
				reader:   reader,
				resolver: resolver,
			})
			if spec.RunAs.Group != "" {
				checks = append(checks, serviceCheck{
					id:       fmt.Sprintf("service.%s.run_as.group", name),
					name:     name,
					spec:     spec,
					kind:     "run_as.group",
					reader:   reader,
					resolver: resolver,
				})
			}
		}
		if spec.Capabilities != nil {
			checks = append(checks, serviceCheck{
				id:       fmt.Sprintf("service.%s.capabilities", name),
				name:     name,
				spec:     spec,
				kind:     "capabilities",
				reader:   reader,
				resolver: resolver,
			})
		}
	}

	return checks
}

type serviceCheck struct {
	id       string
	name     string
	spec     contract.ServiceSpec
	kind     string
	reader   *serviceUnitReader
	resolver AccountResolver
}

func (c serviceCheck) ID() string {
	return c.id
}

func (c serviceCheck) Domain() string {
	return "services"
}

func (c serviceCheck) Prerequisites() []string {
	if c.kind == "state" {
		return nil
	}

	return []string{fmt.Sprintf("service.%s.state", c.name)}
}

func (c serviceCheck) Run(ctx context.Context) evidence.CheckResult {
	if err := ctx.Err(); err != nil {
		return serviceError(evidence.ReasonTimeout, "collector context cancelled before service check started", nil, nil, err.Error())
	}

	unit, err := c.reader.Read(ctx, c.name)
	if err != nil {
		command := c.reader.command(c.name)
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return serviceError(evidence.ReasonTimeout, fmt.Sprintf("collector timed out while reading service %s", c.name), command, unit.exitCodePtr(), unit.raw)
		case errors.Is(err, errServiceNotFound):
			return evidence.CheckResult{
				Status:     evidence.StatusFail,
				ReasonCode: evidence.ReasonNotFound,
				Evidence: evidence.Evidence{
					Source:      "systemctl show",
					Collector:   "services",
					CollectedAt: time.Now().UTC(),
					Command:     c.reader.command(c.name),
					ExitCode:    intPtr(unit.exitCode),
					Raw:         unit.raw,
				},
				Message: fmt.Sprintf("expected service %s to exist", c.name),
			}
		case errors.Is(err, errNamespaceIsolation):
			return serviceError(evidence.ReasonNamespaceIsolation, fmt.Sprintf("service-backed checks are observer-local in v0.1.x; observer-local systemd is not reachable for service %s", c.name), command, unit.exitCodePtr(), unit.raw)
		case errors.Is(err, errPermissionDenied):
			return serviceError(evidence.ReasonPermissionDenied, fmt.Sprintf("permission denied while reading observer-local service %s", c.name), command, unit.exitCodePtr(), unit.raw)
		case errors.Is(err, errCommandUnavailable), errors.Is(err, errUnexpectedCommand):
			return serviceUnsupportedEnvironment(
				fmt.Sprintf("unsupported observer-local environment for service %s: %s", c.name, describeSystemctlEnvironmentError(err)),
				unit.raw,
			)
		case errors.Is(err, errServiceParse):
			return serviceError(evidence.ReasonParseError, fmt.Sprintf("unable to parse systemctl output for service %s", c.name), command, unit.exitCodePtr(), unit.raw)
		default:
			return serviceError(evidence.ReasonParseError, fmt.Sprintf("failed to inspect service %s", c.name), command, unit.exitCodePtr(), err.Error())
		}
	}

	switch c.kind {
	case "state":
		return c.runState(unit)
	case "restart":
		return c.runRestart(unit)
	case "run_as.user":
		return c.runUser(unit)
	case "run_as.group":
		return c.runGroup(unit)
	case "capabilities":
		return c.runCapabilities(unit)
	default:
		return serviceError(evidence.ReasonInternalError, fmt.Sprintf("unsupported service check kind %q", c.kind), nil, nil, "")
	}
}

func (c serviceCheck) runState(unit serviceUnitResult) evidence.CheckResult {
	expected := string(c.spec.State)
	observed := unit.properties["ActiveState"]
	status := evidence.StatusPass
	message := fmt.Sprintf("service state matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected state %s, observed %s", expected, observed)
	}

	return c.result(status, evidence.ReasonNone, expected, observed, message, unit)
}

func (c serviceCheck) runRestart(unit serviceUnitResult) evidence.CheckResult {
	expected := string(*c.spec.Restart)
	observed := unit.properties["Restart"]
	status := evidence.StatusPass
	message := fmt.Sprintf("service restart policy matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected restart policy %s, observed %s", expected, observed)
	}

	return c.result(status, evidence.ReasonNone, expected, observed, message, unit)
}

func (c serviceCheck) runUser(unit serviceUnitResult) evidence.CheckResult {
	expected := c.spec.RunAs.User
	observed, err := observedServiceUser(unit, c.resolver)
	if err != nil {
		return serviceInsufficientData(fmt.Sprintf("unable to resolve effective service user for %s", c.name), c.reader.command(c.name), unit.exitCodePtr(), unit.raw)
	}
	status := evidence.StatusPass
	message := fmt.Sprintf("service user matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected service user %s, observed %s", expected, observed)
	}

	return c.result(status, evidence.ReasonNone, expected, observed, message, unit)
}

func (c serviceCheck) runGroup(unit serviceUnitResult) evidence.CheckResult {
	expected := c.spec.RunAs.Group
	observed, err := observedServiceGroup(unit, c.resolver)
	if err != nil {
		return serviceInsufficientData(fmt.Sprintf("unable to resolve effective service group for %s", c.name), c.reader.command(c.name), unit.exitCodePtr(), unit.raw)
	}
	status := evidence.StatusPass
	message := fmt.Sprintf("service group matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected service group %s, observed %s", expected, observed)
	}

	return c.result(status, evidence.ReasonNone, expected, observed, message, unit)
}

func (c serviceCheck) runCapabilities(unit serviceUnitResult) evidence.CheckResult {
	expected := capabilities.SortCanonical(c.spec.Capabilities)
	observed := capabilities.NormalizeObserved(strings.Fields(unit.properties["AmbientCapabilities"]))
	status := evidence.StatusPass
	message := "service ambient capabilities match"
	if !equalStrings(expected, observed) {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected ambient capabilities %v, observed %v", expected, observed)
	}

	return c.result(status, evidence.ReasonNone, expected, observed, message, unit)
}

func (c serviceCheck) result(status evidence.EvalStatus, reason evidence.ReasonCode, expected, observed any, message string, unit serviceUnitResult) evidence.CheckResult {
	return evidence.CheckResult{
		Status:     status,
		ReasonCode: reason,
		Expected:   expected,
		Observed:   observed,
		Evidence: evidence.Evidence{
			Source:      "systemctl show",
			Collector:   "services",
			CollectedAt: time.Now().UTC(),
			Command:     c.reader.command(c.name),
			ExitCode:    intPtr(unit.exitCode),
			Raw:         unit.raw,
		},
		Message: message,
	}
}

type serviceUnitReader struct {
	runner        CommandRunner
	systemctlPath string
	commandErr    error
	mu            sync.Mutex
	cache         map[string]serviceUnitResult
}

type serviceUnitResult struct {
	properties map[string]string
	exitCode   int
	raw        string
	err        error
}

func (r serviceUnitResult) exitCodePtr() *int {
	if r.exitCode == 0 {
		return nil
	}

	return intPtr(r.exitCode)
}

var (
	errServiceNotFound    = errors.New("service not found")
	errNamespaceIsolation = errors.New("systemd unavailable")
	errPermissionDenied   = errors.New("permission denied")
	errServiceParse       = errors.New("service parse error")
	errCommandUnavailable = errors.New("command unavailable")
	errUnexpectedCommand  = errors.New("unexpected command path")
)

func (r *serviceUnitReader) Read(ctx context.Context, name string) (serviceUnitResult, error) {
	r.mu.Lock()
	if cached, ok := r.cache[name]; ok {
		r.mu.Unlock()
		return cached, cached.err
	}
	r.mu.Unlock()

	result := serviceUnitResult{}
	if r.commandErr != nil {
		result.raw = r.commandErr.Error()
		result.err = r.commandErr
		r.store(name, result)
		return result, result.err
	}
	command := r.command(name)
	runResult, err := r.runner.Run(ctx, command)
	result.exitCode = runResult.ExitCode
	result.raw = strings.TrimSpace(runResult.Stdout)
	if strings.TrimSpace(runResult.Stderr) != "" {
		if result.raw != "" {
			result.raw += "\n"
		}
		result.raw += strings.TrimSpace(runResult.Stderr)
	}
	if err != nil {
		result.err = err
		r.store(name, result)
		return result, result.err
	}
	if runResult.ExitCode != 0 {
		result.err = mapSystemctlFailure(runResult)
		r.store(name, result)
		return result, result.err
	}

	properties, parseErr := parseSystemctlShow(runResult.Stdout)
	result.properties = properties
	result.err = parseErr
	r.store(name, result)
	return result, result.err
}

func (r *serviceUnitReader) store(name string, result serviceUnitResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[name] = result
}

func (r *serviceUnitReader) command(name string) []string {
	systemctlPath := r.systemctlPath
	if systemctlPath == "" {
		systemctlPath = "systemctl"
	}
	return []string{
		systemctlPath,
		"show",
		name,
		"--property=LoadState",
		"--property=ActiveState",
		"--property=Restart",
		"--property=User",
		"--property=Group",
		"--property=AmbientCapabilities",
	}
}

func parseSystemctlShow(stdout string) (map[string]string, error) {
	properties, err := parseSystemctlProperties(stdout, []string{"LoadState", "ActiveState", "Restart", "User", "Group", "AmbientCapabilities"})
	if err != nil {
		return nil, err
	}
	if properties["LoadState"] == "not-found" {
		return nil, errServiceNotFound
	}

	return properties, nil
}

func observedServiceUser(unit serviceUnitResult, resolver AccountResolver) (string, error) {
	value := strings.TrimSpace(unit.properties["User"])
	if value == "" {
		return "root", nil
	}

	return resolver.NormalizeUserValue(value)
}

func observedServiceGroup(unit serviceUnitResult, resolver AccountResolver) (string, error) {
	value := strings.TrimSpace(unit.properties["Group"])
	if value != "" {
		return resolver.NormalizeGroupValue(value)
	}

	userName, err := observedServiceUser(unit, resolver)
	if err != nil {
		return "", err
	}

	return resolver.PrimaryGroupNameByUser(userName)
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}

	_, err := strconv.Atoi(value)
	return err == nil
}

func mapSystemctlFailure(result CommandResult) error {
	if strings.TrimSpace(result.Stdout) != "" {
		if properties, err := parseSystemctlProperties(result.Stdout, nil); err == nil {
			if properties["LoadState"] == "not-found" {
				return errServiceNotFound
			}
		}
	}

	combined := strings.ToLower(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	switch {
	case strings.Contains(combined, "could not be found"),
		strings.Contains(combined, "not-found"),
		strings.Contains(combined, "unit ") && strings.Contains(combined, " not found"):
		return errServiceNotFound
	case strings.Contains(combined, "permission denied"):
		return errPermissionDenied
	case strings.Contains(combined, "system has not been booted with systemd"),
		strings.Contains(combined, "failed to connect to bus"),
		strings.Contains(combined, "host is down"):
		return errNamespaceIsolation
	default:
		return errServiceParse
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func serviceError(reason evidence.ReasonCode, message string, command []string, exitCode *int, raw string) evidence.CheckResult {
	return evidence.CheckResult{
		Status:     evidence.StatusError,
		ReasonCode: reason,
		Evidence: evidence.Evidence{
			Source:      "systemctl show",
			Collector:   "services",
			CollectedAt: time.Now().UTC(),
			Command:     command,
			ExitCode:    exitCode,
			Raw:         raw,
		},
		Message: message,
	}
}

func serviceInsufficientData(message string, command []string, exitCode *int, raw string) evidence.CheckResult {
	return evidence.CheckResult{
		Status:     evidence.StatusInsufficientData,
		ReasonCode: evidence.ReasonParseError,
		Evidence: evidence.Evidence{
			Source:      "systemctl show+accountdb",
			Collector:   "services",
			CollectedAt: time.Now().UTC(),
			Command:     command,
			ExitCode:    exitCode,
			Raw:         raw,
		},
		Message: message,
	}
}

func serviceUnsupportedEnvironment(message, raw string) evidence.CheckResult {
	return evidence.CheckResult{
		Status: evidence.StatusError,
		Evidence: evidence.Evidence{
			Source:      "systemctl lookup",
			Collector:   "services",
			CollectedAt: time.Now().UTC(),
			Raw:         raw,
		},
		Message: message,
	}
}

func describeSystemctlEnvironmentError(err error) string {
	switch {
	case errors.Is(err, errCommandUnavailable):
		return "systemctl is not available in PATH; service-backed checks currently require an allowlisted absolute observer-local systemctl path"
	case errors.Is(err, errUnexpectedCommand):
		return fmt.Sprintf("resolved systemctl path is outside the current allowlist; service-backed checks currently require /usr/bin/systemctl or /bin/systemctl (%v)", err)
	default:
		return err.Error()
	}
}

func intPtr(value int) *int {
	return &value
}

func stableLocaleEnv(base []string) []string {
	filtered := make([]string, 0, len(base)+2)
	for _, entry := range base {
		switch {
		case strings.HasPrefix(entry, "LANG="):
			continue
		case strings.HasPrefix(entry, "LC_ALL="):
			continue
		default:
			filtered = append(filtered, entry)
		}
	}

	filtered = append(filtered, "LANG=C", "LC_ALL=C")
	return filtered
}

func resolveSystemctlPathForRunner(runner CommandRunner) (string, error) {
	switch runner.(type) {
	case OSCommandRunner, *OSCommandRunner:
		return resolveAllowedSystemctlPath()
	default:
		return "systemctl", nil
	}
}

func resolveAllowedSystemctlPath() (string, error) {
	path, err := lookPathExecutable("systemctl")
	if err != nil {
		return "", fmt.Errorf("%w: %v", errCommandUnavailable, err)
	}
	path = filepath.Clean(path)
	if _, ok := allowedSystemctlPaths[path]; !ok {
		return "", fmt.Errorf("%w: %s", errUnexpectedCommand, path)
	}

	return path, nil
}

func parseSystemctlProperties(stdout string, required []string) (map[string]string, error) {
	properties := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, errServiceParse
		}
		properties[key] = value
	}

	for _, key := range required {
		if _, ok := properties[key]; !ok {
			return nil, errServiceParse
		}
	}

	return properties, nil
}
