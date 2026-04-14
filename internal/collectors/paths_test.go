package collectors

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

func TestBuildPathChecksPassesForExistingFile(t *testing.T) {
	t.Parallel()

	account := currentAccount(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}

	checks := BuildPathChecks(map[string]contract.PathSpec{
		target: {
			Owner: account.User,
			Group: account.Group,
			Mode:  "0640",
			Type:  contract.PathTypeFile,
		},
	}, OSPathChecker{})

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, result := range results {
		if result.Status != evidence.StatusPass {
			t.Fatalf("result %s status = %s, want %s", result.CheckID, result.Status, evidence.StatusPass)
		}
	}
}

func TestBuildPathChecksHandlesMissingPath(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "missing.yaml")
	checks := BuildPathChecks(map[string]contract.PathSpec{
		target: {
			Mode: "0640",
			Type: contract.PathTypeFile,
		},
	}, OSPathChecker{})

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	exists := results[0]
	if exists.Status != evidence.StatusFail {
		t.Fatalf("exists.Status = %s, want %s", exists.Status, evidence.StatusFail)
	}
	if exists.ReasonCode != evidence.ReasonNotFound {
		t.Fatalf("exists.ReasonCode = %s, want %s", exists.ReasonCode, evidence.ReasonNotFound)
	}

	for _, result := range results[1:] {
		if result.Status != evidence.StatusNotApplicable {
			t.Fatalf("dependent status = %s, want %s", result.Status, evidence.StatusNotApplicable)
		}
		if result.ReasonCode != evidence.ReasonPrerequisiteFailed {
			t.Fatalf("dependent reason = %s, want %s", result.ReasonCode, evidence.ReasonPrerequisiteFailed)
		}
	}
}

func TestBuildPathChecksDoesNotFollowSymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	link := filepath.Join(dir, "config-link.yaml")
	if err := os.WriteFile(target, []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("os.WriteFile(target) error = %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	checks := BuildPathChecks(map[string]contract.PathSpec{
		link: {
			Mode: "0640",
			Type: contract.PathTypeFile,
		},
	}, OSPathChecker{})

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
	if byID["path."+link+".exists"].Status != evidence.StatusPass {
		t.Fatalf("exists result = %#v, want PASS for existing symlink node", byID["path."+link+".exists"])
	}
	if byID["path."+link+".mode"].Status != evidence.StatusFail {
		t.Fatalf("mode result = %#v, want FAIL", byID["path."+link+".mode"])
	}
	if byID["path."+link+".type"].Status != evidence.StatusFail {
		t.Fatalf("type result = %#v, want FAIL", byID["path."+link+".type"])
	}
}

func TestBuildPathChecksUsesRootedAccountDatabase(t *testing.T) {
	t.Parallel()

	hostRoot := t.TempDir()
	writeTestAccountFiles(t, hostRoot,
		[]string{"sensor:x:1234:1234::/nonexistent:/usr/sbin/nologin"},
		[]string{"sensor:x:1234:"},
	)

	target := "/etc/sensor-agent/config.yaml"
	resolved := filepath.Join(hostRoot, "etc", "sensor-agent", "config.yaml")
	checks := BuildPathChecks(map[string]contract.PathSpec{
		target: {
			Owner: "sensor",
			Group: "sensor",
		},
	}, NewRootedPathChecker(hostRoot, fakePathChecker{
		entries: map[string]fakeFileInfo{
			resolved: {
				mode: 0o640,
				sys:  &syscall.Stat_t{Uid: 1234, Gid: 1234},
			},
		},
	}))

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, result := range results {
		if result.Status != evidence.StatusPass {
			t.Fatalf("result %s status = %s, want %s", result.CheckID, result.Status, evidence.StatusPass)
		}
	}
}

func TestBuildPathChecksReturnInsufficientDataWhenRootedOwnerCannotBeResolved(t *testing.T) {
	t.Parallel()

	hostRoot := t.TempDir()
	writeTestAccountFiles(t, hostRoot, []string{}, []string{})

	target := "/etc/sensor-agent/config.yaml"
	resolved := filepath.Join(hostRoot, "etc", "sensor-agent", "config.yaml")
	checks := BuildPathChecks(map[string]contract.PathSpec{
		target: {
			Owner: "sensor",
		},
	}, NewRootedPathChecker(hostRoot, fakePathChecker{
		entries: map[string]fakeFileInfo{
			resolved: {
				mode: 0o640,
				sys:  &syscall.Stat_t{Uid: 1234, Gid: 1234},
			},
		},
	}))

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	byID := make(map[string]evidence.CheckResult, len(results))
	for _, result := range results {
		byID[result.CheckID] = result
	}

	owner := byID["path."+target+".owner"]
	if owner.Status != evidence.StatusInsufficientData {
		t.Fatalf("owner.Status = %s, want %s", owner.Status, evidence.StatusInsufficientData)
	}
	if owner.ReasonCode != evidence.ReasonParseError {
		t.Fatalf("owner.ReasonCode = %s, want %s", owner.ReasonCode, evidence.ReasonParseError)
	}
}

type testAccount struct {
	User  string
	Group string
}

func currentAccount(t *testing.T) testAccount {
	t.Helper()

	resolver := NewAccountResolver("")
	currentUser, err := resolver.UserNameByUID(uint32(os.Getuid()))
	if err != nil {
		t.Skipf("current uid is not resolvable via %s: %v", resolver.PasswdPath(), err)
	}
	currentGroup, err := resolver.GroupNameByGID(uint32(os.Getgid()))
	if err != nil {
		t.Skipf("current gid is not resolvable via %s: %v", resolver.GroupPath(), err)
	}

	return testAccount{
		User:  currentUser,
		Group: currentGroup,
	}
}
