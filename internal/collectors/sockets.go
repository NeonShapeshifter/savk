package collectors

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"sort"
	"syscall"
	"time"

	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

func BuildSocketChecks(sockets map[string]contract.SocketSpec, checker PathChecker) []engine.Check {
	if checker == nil {
		checker = OSPathChecker{}
	}

	keys := make([]string, 0, len(sockets))
	for path := range sockets {
		keys = append(keys, path)
	}
	sort.Strings(keys)

	checks := make([]engine.Check, 0, len(keys)*4)
	for _, path := range keys {
		spec := sockets[path]
		checks = append(checks, socketCheck{
			id:      fmt.Sprintf("socket.%s.exists", path),
			path:    path,
			spec:    spec,
			checker: checker,
			kind:    "exists",
		})
		if spec.Owner != "" {
			checks = append(checks, socketCheck{
				id:      fmt.Sprintf("socket.%s.owner", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "owner",
			})
		}
		if spec.Group != "" {
			checks = append(checks, socketCheck{
				id:      fmt.Sprintf("socket.%s.group", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "group",
			})
		}
		if spec.Mode != "" {
			checks = append(checks, socketCheck{
				id:      fmt.Sprintf("socket.%s.mode", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "mode",
			})
		}
	}

	return checks
}

type socketCheck struct {
	id      string
	path    string
	spec    contract.SocketSpec
	checker PathChecker
	kind    string
}

func (c socketCheck) ID() string {
	return c.id
}

func (c socketCheck) Domain() string {
	return "sockets"
}

func (c socketCheck) Prerequisites() []string {
	if c.kind == "exists" {
		return nil
	}

	return []string{fmt.Sprintf("socket.%s.exists", c.path)}
}

func (c socketCheck) Run(ctx context.Context) evidence.CheckResult {
	if err := ctx.Err(); err != nil {
		return errorResult("sockets", c.path, evidence.ReasonTimeout, "collector context cancelled before check started", err)
	}

	switch c.kind {
	case "exists":
		return c.runExists()
	case "owner":
		return c.runOwner()
	case "group":
		return c.runGroup()
	case "mode":
		return c.runMode()
	default:
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonInternalError,
			Evidence: evidence.Evidence{
				Source:      "sockets",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
			},
			Message: fmt.Sprintf("unsupported socket check kind %q", c.kind),
		}
	}
}

func (c socketCheck) runExists() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	resolved := resolvedPath(c.checker, c.path)
	if err == nil {
		isSocket := info.Mode()&os.ModeSocket != 0
		status := evidence.StatusPass
		message := "socket exists"
		if !isSocket {
			status = evidence.StatusFail
			message = fmt.Sprintf("expected unix socket at %s, observed %s", c.path, info.Mode().String())
		}
		return evidence.CheckResult{
			Status:   status,
			Expected: true,
			Observed: isSocket,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s mode=%s", resolved, info.Mode().String()),
			},
			Message: message,
		}
	}
	if os.IsNotExist(err) {
		return evidence.CheckResult{
			Status:     evidence.StatusFail,
			ReasonCode: evidence.ReasonNotFound,
			Expected:   true,
			Observed:   false,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s error=%s", resolved, err),
			},
			Message: fmt.Sprintf("expected socket %s to exist", c.path),
		}
	}

	return errorResult("sockets", resolved, mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
}

func (c socketCheck) runOwner() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("sockets", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonParseError,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
			},
			Message: fmt.Sprintf("unable to read ownership metadata for %s", c.path),
		}
	}

	account, err := user.LookupId(fmt.Sprintf("%d", stat.Uid))
	if err != nil {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonParseError,
			Evidence: evidence.Evidence{
				Source:      "os/user.LookupId",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s error=%s", resolvedPath(c.checker, c.path), err),
			},
			Message: fmt.Sprintf("unable to resolve owner UID %d for %s", stat.Uid, c.path),
		}
	}

	expected := c.spec.Owner
	observed := account.Username
	status := evidence.StatusPass
	message := fmt.Sprintf("socket owner matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected owner %s, observed %s", expected, observed)
	}

	return evidence.CheckResult{
		Status:   status,
		Expected: expected,
		Observed: observed,
		Evidence: evidence.Evidence{
			Source:      "fs.lstat",
			Collector:   "sockets",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s uid=%d username=%s", resolvedPath(c.checker, c.path), stat.Uid, observed),
		},
		Message: message,
	}
}

func (c socketCheck) runGroup() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("sockets", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonParseError,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
			},
			Message: fmt.Sprintf("unable to read group metadata for %s", c.path),
		}
	}

	group, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid))
	if err != nil {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonParseError,
			Evidence: evidence.Evidence{
				Source:      "os/user.LookupGroupId",
				Collector:   "sockets",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s error=%s", resolvedPath(c.checker, c.path), err),
			},
			Message: fmt.Sprintf("unable to resolve group GID %d for %s", stat.Gid, c.path),
		}
	}

	expected := c.spec.Group
	observed := group.Name
	status := evidence.StatusPass
	message := fmt.Sprintf("socket group matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected group %s, observed %s", expected, observed)
	}

	return evidence.CheckResult{
		Status:   status,
		Expected: expected,
		Observed: observed,
		Evidence: evidence.Evidence{
			Source:      "fs.lstat",
			Collector:   "sockets",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s gid=%d group=%s", resolvedPath(c.checker, c.path), stat.Gid, observed),
		},
		Message: message,
	}
}

func (c socketCheck) runMode() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("sockets", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	observed := formatFileMode(info.Mode())
	expected := c.spec.Mode
	status := evidence.StatusPass
	message := fmt.Sprintf("socket mode matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected mode %s, observed %s", expected, observed)
	}

	return evidence.CheckResult{
		Status:   status,
		Expected: expected,
		Observed: observed,
		Evidence: evidence.Evidence{
			Source:      "fs.lstat",
			Collector:   "sockets",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s mode=%s", resolvedPath(c.checker, c.path), observed),
		},
		Message: message,
	}
}
