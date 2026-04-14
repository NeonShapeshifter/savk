package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSystemdIntegration(t *testing.T) {
	if os.Getenv("SAVK_RUN_SYSTEMD_INTEGRATION") != "1" {
		t.Skip("set SAVK_RUN_SYSTEMD_INTEGRATION=1 to run against a real linux-systemd host")
	}

	service := os.Getenv("SAVK_SYSTEMD_INTEGRATION_SERVICE")
	if service == "" {
		service = "systemd-journald.service"
	}
	uid := os.Getenv("SAVK_SYSTEMD_INTEGRATION_UID")
	if uid == "" {
		uid = "0"
	}
	if _, err := strconv.Atoi(uid); err != nil {
		t.Fatalf("SAVK_SYSTEMD_INTEGRATION_UID = %q, want integer", uid)
	}

	dir := t.TempDir()
	contractBody := strings.Join([]string{
		"apiVersion: savk/v1",
		"kind: ApplianceContract",
		"metadata:",
		"  name: systemd-integration",
		"  target: linux-systemd",
		"services:",
		"  " + service + ":",
		"    state: active",
		"identity:",
		"  runtime_subject:",
		"    service: " + service,
		"    uid: " + uid,
	}, "\n") + "\n"

	contractPath := filepath.Join(dir, "contract.yaml")
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
		Domain  string `json:"domain"`
		Status  string `json:"status"`
	}
	type report struct {
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

	expectedChecks := map[string]string{
		"service.__preflight__.namespace": "PASS",
		"service." + service + ".state":   "PASS",
		"identity.runtime_subject.uid":    "PASS",
	}
	for checkID, wantStatus := range expectedChecks {
		if statusByID[checkID] != wantStatus {
			t.Fatalf("statusByID[%q] = %q, want %q\nstdout=%s", checkID, statusByID[checkID], wantStatus, stdout.String())
		}
	}
}
