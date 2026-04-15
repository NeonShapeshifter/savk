package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"savk/internal/capabilities"
)

var integrationProperties = []string{
	"LoadState",
	"ActiveState",
	"Restart",
	"User",
	"Group",
	"AmbientCapabilities",
	"MainPID",
	"ControlGroup",
}

func TestSystemdIntegration(t *testing.T) {
	if os.Getenv("SAVK_RUN_SYSTEMD_INTEGRATION") != "1" {
		t.Skip("set SAVK_RUN_SYSTEMD_INTEGRATION=1 to run against a real linux-systemd host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service, properties := integrationSelectService(t, ctx)
	if properties["LoadState"] != "loaded" {
		t.Fatalf("LoadState = %q, want %q", properties["LoadState"], "loaded")
	}
	if properties["ActiveState"] != "active" {
		t.Fatalf("ActiveState = %q, want %q", properties["ActiveState"], "active")
	}
	t.Logf(
		"observer-local integration subject: %s (user=%q group=%q ambient=%q)",
		service,
		properties["User"],
		properties["Group"],
		properties["AmbientCapabilities"],
	)

	pid, err := strconv.Atoi(strings.TrimSpace(properties["MainPID"]))
	if err != nil || pid <= 0 {
		t.Fatalf("MainPID = %q, want positive integer", properties["MainPID"])
	}

	processStatus := integrationReadProcStatus(t, pid)
	expectedUser, err := integrationExpectedServiceUser(properties["User"])
	if err != nil {
		t.Fatalf("integrationExpectedServiceUser() error = %v", err)
	}
	expectedGroup, err := integrationExpectedServiceGroup(properties["User"], properties["Group"])
	if err != nil {
		t.Fatalf("integrationExpectedServiceGroup() error = %v", err)
	}
	serviceCaps := capabilities.NormalizeObserved(strings.Fields(properties["AmbientCapabilities"]))

	dir := t.TempDir()
	contractPath := filepath.Join(dir, "contract.yaml")
	lines := []string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: systemd-integration",
		"  target: linux-systemd",
		"services:",
		"  " + service + ":",
		"    state: active",
		"    run_as:",
		"      user: " + expectedUser,
		"      group: " + expectedGroup,
		"    restart: " + properties["Restart"],
	}
	lines = append(lines, integrationYAMLKeyList("    ", "capabilities", serviceCaps)...)
	lines = append(lines,
		"identity:",
		"  runtime_subject:",
		"    service: "+service,
		"    uid: "+strconv.Itoa(processStatus.uid),
		"    gid: "+strconv.Itoa(processStatus.gid),
		"    capabilities:",
	)
	lines = append(lines, integrationYAMLKeyList("      ", "effective", processStatus.effective)...)
	lines = append(lines, integrationYAMLKeyList("      ", "permitted", processStatus.permitted)...)
	lines = append(lines, integrationYAMLKeyList("      ", "inheritable", processStatus.inheritable)...)
	lines = append(lines, integrationYAMLKeyList("      ", "bounding", processStatus.bounding)...)
	lines = append(lines, integrationYAMLKeyList("      ", "ambient", processStatus.ambient)...)
	contractBody := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(contractPath, []byte(contractBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile(contract) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"check", "--contract", contractPath, "--format", "json", "--domain", "services,identity"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(systemd integration) code = %d, want 0; stderr = %q\nstdout=%s", code, stderr.String(), stdout.String())
	}

	type reportResult struct {
		CheckID string `json:"checkID"`
		Status  string `json:"status"`
	}
	type reportSummary struct {
		Pass int `json:"pass"`
	}
	type report struct {
		Summary reportSummary  `json:"summary"`
		Results []reportResult `json:"results"`
	}

	var got report
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}

	statusByID := make(map[string]string, len(got.Results))
	for _, result := range got.Results {
		statusByID[result.CheckID] = result.Status
	}

	expectedChecks := []string{
		"service.__preflight__.namespace",
		"service." + service + ".state",
		"service." + service + ".restart",
		"service." + service + ".run_as.user",
		"service." + service + ".run_as.group",
		"service." + service + ".capabilities",
		"identity.runtime_subject.uid",
		"identity.runtime_subject.gid",
		"identity.runtime_subject.capabilities.effective",
		"identity.runtime_subject.capabilities.permitted",
		"identity.runtime_subject.capabilities.inheritable",
		"identity.runtime_subject.capabilities.bounding",
		"identity.runtime_subject.capabilities.ambient",
	}
	for _, checkID := range expectedChecks {
		if statusByID[checkID] != "PASS" {
			t.Fatalf("statusByID[%q] = %q, want PASS\nstdout=%s", checkID, statusByID[checkID], stdout.String())
		}
	}
	if got.Summary.Pass != len(expectedChecks) {
		t.Fatalf("summary.pass = %d, want %d\nstdout=%s", got.Summary.Pass, len(expectedChecks), stdout.String())
	}
}

type integrationProcStatus struct {
	uid         int
	gid         int
	effective   []string
	permitted   []string
	inheritable []string
	bounding    []string
	ambient     []string
}

func integrationSelectService(t *testing.T, ctx context.Context) (string, map[string]string) {
	t.Helper()

	if explicit := strings.TrimSpace(os.Getenv("SAVK_SYSTEMD_INTEGRATION_SERVICE")); explicit != "" {
		return explicit, integrationSystemctlShow(t, ctx, explicit, integrationProperties)
	}

	candidates := []string{
		"dbus.service",
		"systemd-resolved.service",
		"systemd-networkd.service",
		"systemd-journald.service",
	}

	bestService := ""
	bestScore := -1
	var bestProperties map[string]string
	for _, service := range candidates {
		properties, err := integrationTrySystemctlShow(ctx, service, integrationProperties)
		if err != nil {
			continue
		}
		if properties["LoadState"] != "loaded" || properties["ActiveState"] != "active" {
			continue
		}

		score := 0
		if strings.TrimSpace(properties["User"]) != "" {
			score++
		}
		if strings.TrimSpace(properties["Group"]) != "" {
			score++
		}
		if strings.TrimSpace(properties["AmbientCapabilities"]) != "" {
			score++
		}
		if score > bestScore {
			bestService = service
			bestScore = score
			bestProperties = properties
		}
	}

	if bestService == "" {
		t.Fatalf("no active observer-local integration subject found in candidates %v", candidates)
	}

	return bestService, bestProperties
}

func integrationSystemctlShow(t *testing.T, ctx context.Context, service string, properties []string) map[string]string {
	t.Helper()

	result, err := integrationTrySystemctlShow(ctx, service, properties)
	if err != nil {
		t.Fatalf("systemctl show %s error = %v", service, err)
	}

	return result
}

func integrationTrySystemctlShow(ctx context.Context, service string, properties []string) (map[string]string, error) {
	args := []string{"show", service}
	for _, property := range properties {
		args = append(args, "--property="+property)
	}
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(properties))
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("unexpected systemctl output line %q", line)
		}
		result[key] = value
	}

	for _, property := range properties {
		if _, ok := result[property]; !ok {
			return nil, fmt.Errorf("systemctl output missing property %q", property)
		}
	}

	if result["LoadState"] == "not-found" {
		return nil, errors.New("unit not found")
	}

	return result, nil
}

func integrationReadProcStatus(t *testing.T, pid int) integrationProcStatus {
	t.Helper()

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		t.Fatalf("os.ReadFile(/proc/%d/status) error = %v", pid, err)
	}

	fields := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("unexpected /proc status line %q", line)
		}
		fields[key] = strings.TrimSpace(value)
	}

	uid := integrationParseEffectiveID(t, fields["Uid"])
	gid := integrationParseEffectiveID(t, fields["Gid"])

	return integrationProcStatus{
		uid:         uid,
		gid:         gid,
		effective:   integrationParseCapabilityMask(t, fields["CapEff"]),
		permitted:   integrationParseCapabilityMask(t, fields["CapPrm"]),
		inheritable: integrationParseCapabilityMask(t, fields["CapInh"]),
		bounding:    integrationParseCapabilityMask(t, fields["CapBnd"]),
		ambient:     integrationParseCapabilityMask(t, fields["CapAmb"]),
	}
}

func integrationParseEffectiveID(t *testing.T, value string) int {
	t.Helper()

	parts := strings.Fields(value)
	if len(parts) < 2 {
		t.Fatalf("effective id field = %q, want at least two columns", value)
	}
	result, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("strconv.Atoi(%q) error = %v", parts[1], err)
	}
	return result
}

func integrationParseCapabilityMask(t *testing.T, value string) []string {
	t.Helper()

	mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
	if err != nil {
		t.Fatalf("strconv.ParseUint(%q) error = %v", value, err)
	}

	names := make([]string, 0)
	for bit := 0; bit < 64; bit++ {
		if mask&(1<<bit) == 0 {
			continue
		}
		name := capabilities.LinuxCapabilityName(bit)
		if name == "" {
			name = fmt.Sprintf("CAP_UNKNOWN_%d", bit)
		}
		names = append(names, name)
	}
	return names
}

func integrationExpectedServiceUser(rawUser string) (string, error) {
	rawUser = strings.TrimSpace(rawUser)
	if rawUser == "" {
		return "root", nil
	}
	resolver := accountResolverForIntegration()
	return resolver.NormalizeUserValue(rawUser)
}

func integrationExpectedServiceGroup(rawUser, rawGroup string) (string, error) {
	resolver := accountResolverForIntegration()
	rawGroup = strings.TrimSpace(rawGroup)
	if rawGroup != "" {
		return resolver.NormalizeGroupValue(rawGroup)
	}

	userName, err := integrationExpectedServiceUser(rawUser)
	if err != nil {
		return "", err
	}
	return resolver.PrimaryGroupNameByUser(userName)
}

func integrationYAMLList(indent string, values []string) []string {
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, indent+"- "+strconv.Quote(value))
	}
	return lines
}

func integrationYAMLKeyList(indent, key string, values []string) []string {
	if len(values) == 0 {
		return []string{indent + key + ": []"}
	}

	lines := []string{indent + key + ":"}
	lines = append(lines, integrationYAMLList(indent+"  ", values)...)
	return lines
}

func accountResolverForIntegration() interface {
	NormalizeUserValue(value string) (string, error)
	NormalizeGroupValue(value string) (string, error)
	PrimaryGroupNameByUser(user string) (string, error)
} {
	return integrationAccountResolver
}

var integrationAccountResolver = newIntegrationAccountResolver()

func newIntegrationAccountResolver() interface {
	NormalizeUserValue(value string) (string, error)
	NormalizeGroupValue(value string) (string, error)
	PrimaryGroupNameByUser(user string) (string, error)
} {
	type accountResolver interface {
		NormalizeUserValue(value string) (string, error)
		NormalizeGroupValue(value string) (string, error)
		PrimaryGroupNameByUser(user string) (string, error)
	}

	return accountResolver(integrationLocalAccountResolver{})
}

type integrationLocalAccountResolver struct{}

func (integrationLocalAccountResolver) NormalizeUserValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !integrationIsNumeric(value) {
		return value, nil
	}
	return integrationLookupNameByID("/etc/passwd", value, 2)
}

func (integrationLocalAccountResolver) NormalizeGroupValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !integrationIsNumeric(value) {
		return value, nil
	}
	return integrationLookupNameByID("/etc/group", value, 2)
}

func (integrationLocalAccountResolver) PrimaryGroupNameByUser(user string) (string, error) {
	passwdData, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(passwdData)), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 4 || parts[0] != user {
			continue
		}
		return integrationLookupNameByID("/etc/group", parts[3], 2)
	}
	return "", fmt.Errorf("user %q not found in /etc/passwd", user)
}

func integrationLookupNameByID(path, rawID string, idIndex int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) <= idIndex || parts[idIndex] != rawID {
			continue
		}
		if strings.TrimSpace(parts[0]) == "" {
			break
		}
		return parts[0], nil
	}
	return "", fmt.Errorf("id %s not found in %s", rawID, path)
}

func integrationIsNumeric(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.Atoi(value)
	return err == nil
}
