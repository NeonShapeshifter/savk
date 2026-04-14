package collectors

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

func TestBuildSocketChecksPassesForExistingSocket(t *testing.T) {
	t.Parallel()

	account := currentAccount(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", target)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("unix sockets not permitted in this environment: %v", err)
		}
		t.Fatalf("net.Listen(unix) error = %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	if err := os.Chmod(target, 0o660); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}

	checks := BuildSocketChecks(map[string]contract.SocketSpec{
		target: {
			Owner: account.User,
			Group: account.Group,
			Mode:  "0660",
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

func TestBuildSocketChecksDoesNotFollowSymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "agent.sock")
	link := filepath.Join(dir, "agent-link.sock")
	listener, err := net.Listen("unix", target)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("unix sockets not permitted in this environment: %v", err)
		}
		t.Fatalf("net.Listen(unix) error = %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	checks := BuildSocketChecks(map[string]contract.SocketSpec{
		link: {
			Mode: "0660",
		},
	}, OSPathChecker{})

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	byID := make(map[string]evidence.CheckResult, len(results))
	for _, result := range results {
		byID[result.CheckID] = result
	}
	if byID["socket."+link+".exists"].Status != evidence.StatusFail {
		t.Fatalf("exists result = %#v, want FAIL for symlinked socket path", byID["socket."+link+".exists"])
	}
}

func TestBuildSocketChecksHandlesMissingSocket(t *testing.T) {
	t.Parallel()

	target := "/tmp/missing.sock"
	checks := BuildSocketChecks(map[string]contract.SocketSpec{
		target: {
			Mode: "0660",
		},
	}, fakePathChecker{})

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	exists := results[0]
	if exists.Status != evidence.StatusFail {
		t.Fatalf("exists.Status = %s, want %s", exists.Status, evidence.StatusFail)
	}
	if exists.ReasonCode != evidence.ReasonNotFound {
		t.Fatalf("exists.ReasonCode = %s, want %s", exists.ReasonCode, evidence.ReasonNotFound)
	}

	mode := results[1]
	if mode.Status != evidence.StatusNotApplicable {
		t.Fatalf("mode.Status = %s, want %s", mode.Status, evidence.StatusNotApplicable)
	}
}

func TestBuildSocketChecksPreservesSocketCollectorOnErrors(t *testing.T) {
	t.Parallel()

	target := "/tmp/denied.sock"
	checks := BuildSocketChecks(map[string]contract.SocketSpec{
		target: {
			Mode: "0660",
		},
	}, fakePathChecker{
		errs: map[string]error{
			target: os.ErrPermission,
		},
	})

	results, err := engine.New().Run(context.Background(), checks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	exists := results[0]
	if exists.Status != evidence.StatusError {
		t.Fatalf("exists.Status = %s, want %s", exists.Status, evidence.StatusError)
	}
	if exists.Evidence.Collector != "sockets" {
		t.Fatalf("exists.Evidence.Collector = %q, want %q", exists.Evidence.Collector, "sockets")
	}
}

func TestBuildSocketChecksUsesRootedAccountDatabase(t *testing.T) {
	t.Parallel()

	hostRoot := t.TempDir()
	writeTestAccountFiles(t, hostRoot,
		[]string{"sensor:x:1234:1234::/nonexistent:/usr/sbin/nologin"},
		[]string{"sensor:x:1234:"},
	)

	target := "/run/sensor-agent.sock"
	resolved := filepath.Join(hostRoot, "run", "sensor-agent.sock")
	checks := BuildSocketChecks(map[string]contract.SocketSpec{
		target: {
			Owner: "sensor",
			Group: "sensor",
		},
	}, NewRootedPathChecker(hostRoot, fakePathChecker{
		entries: map[string]fakeFileInfo{
			resolved: {
				mode: os.ModeSocket | 0o660,
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

type fakePathChecker struct {
	entries map[string]fakeFileInfo
	errs    map[string]error
}

func (f fakePathChecker) Stat(name string) (os.FileInfo, error) {
	if err, ok := f.errs[name]; ok {
		return nil, err
	}
	info, ok := f.entries[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return info, nil
}

func (f fakePathChecker) Lstat(name string) (os.FileInfo, error) {
	return f.Stat(name)
}

type fakeFileInfo struct {
	name string
	size int64
	mode os.FileMode
	sys  any
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return f.sys }
