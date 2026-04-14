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

type OSPathChecker struct{}

func (OSPathChecker) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (OSPathChecker) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func BuildPathChecks(paths map[string]contract.PathSpec, checker PathChecker) []engine.Check {
	if checker == nil {
		checker = OSPathChecker{}
	}

	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)

	checks := make([]engine.Check, 0, len(keys)*5)
	for _, path := range keys {
		spec := paths[path]
		checks = append(checks, pathCheck{
			id:      fmt.Sprintf("path.%s.exists", path),
			path:    path,
			spec:    spec,
			checker: checker,
			kind:    "exists",
		})
		if spec.Type != "" {
			checks = append(checks, pathCheck{
				id:      fmt.Sprintf("path.%s.type", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "type",
			})
		}
		if spec.Mode != "" {
			checks = append(checks, pathCheck{
				id:      fmt.Sprintf("path.%s.mode", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "mode",
			})
		}
		if spec.Owner != "" {
			checks = append(checks, pathCheck{
				id:      fmt.Sprintf("path.%s.owner", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "owner",
			})
		}
		if spec.Group != "" {
			checks = append(checks, pathCheck{
				id:      fmt.Sprintf("path.%s.group", path),
				path:    path,
				spec:    spec,
				checker: checker,
				kind:    "group",
			})
		}
	}

	return checks
}

type pathCheck struct {
	id      string
	path    string
	spec    contract.PathSpec
	checker PathChecker
	kind    string
}

func (c pathCheck) ID() string {
	return c.id
}

func (c pathCheck) Domain() string {
	return "paths"
}

func (c pathCheck) Prerequisites() []string {
	if c.kind == "exists" {
		return nil
	}

	return []string{fmt.Sprintf("path.%s.exists", c.path)}
}

func (c pathCheck) Run(ctx context.Context) evidence.CheckResult {
	if err := ctx.Err(); err != nil {
		return errorResult("paths", c.path, evidence.ReasonTimeout, "collector context cancelled before check started", err)
	}

	switch c.kind {
	case "exists":
		return c.runExists()
	case "type":
		return c.runType()
	case "mode":
		return c.runMode()
	case "owner":
		return c.runOwner()
	case "group":
		return c.runGroup()
	default:
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonInternalError,
			Evidence: evidence.Evidence{
				Source:      "paths",
				Collector:   "paths",
				CollectedAt: time.Now().UTC(),
			},
			Message: fmt.Sprintf("unsupported path check kind %q", c.kind),
		}
	}
}

func (c pathCheck) runExists() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	resolved := resolvedPath(c.checker, c.path)
	if err == nil {
		return evidence.CheckResult{
			Status:   evidence.StatusPass,
			Expected: true,
			Observed: true,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "paths",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s mode=%s", resolved, info.Mode().String()),
			},
			Message: "path exists",
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
				Collector:   "paths",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s error=%s", resolved, err),
			},
			Message: fmt.Sprintf("expected path %s to exist", c.path),
		}
	}

	return errorResult("paths", resolved, mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
}

func (c pathCheck) runType() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("paths", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	observed := observedPathType(info.Mode())
	expected := string(c.spec.Type)
	status := evidence.StatusPass
	message := fmt.Sprintf("path type matches %s", expected)
	if observed != expected {
		status = evidence.StatusFail
		message = fmt.Sprintf("expected type %s, observed %s", expected, observed)
	}

	return evidence.CheckResult{
		Status:   status,
		Expected: expected,
		Observed: observed,
		Evidence: evidence.Evidence{
			Source:      "fs.lstat",
			Collector:   "paths",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s mode=%s", resolvedPath(c.checker, c.path), info.Mode().String()),
		},
		Message: message,
	}
}

func (c pathCheck) runMode() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("paths", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	observed := formatFileMode(info.Mode())
	expected := c.spec.Mode
	status := evidence.StatusPass
	message := fmt.Sprintf("path mode matches %s", expected)
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
			Collector:   "paths",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s mode=%s", resolvedPath(c.checker, c.path), observed),
		},
		Message: message,
	}
}

func (c pathCheck) runOwner() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("paths", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonParseError,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "paths",
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
				Collector:   "paths",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s error=%s", resolvedPath(c.checker, c.path), err),
			},
			Message: fmt.Sprintf("unable to resolve owner UID %d for %s", stat.Uid, c.path),
		}
	}

	expected := c.spec.Owner
	observed := account.Username
	status := evidence.StatusPass
	message := fmt.Sprintf("path owner matches %s", expected)
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
			Collector:   "paths",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s uid=%d username=%s", resolvedPath(c.checker, c.path), stat.Uid, observed),
		},
		Message: message,
	}
}

func (c pathCheck) runGroup() evidence.CheckResult {
	info, err := c.checker.Lstat(c.path)
	if err != nil {
		return errorResult("paths", resolvedPath(c.checker, c.path), mapStatError(err), fmt.Sprintf("failed to lstat %s", c.path), err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonParseError,
			Evidence: evidence.Evidence{
				Source:      "fs.lstat",
				Collector:   "paths",
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
				Collector:   "paths",
				CollectedAt: time.Now().UTC(),
				Raw:         fmt.Sprintf("path=%s error=%s", resolvedPath(c.checker, c.path), err),
			},
			Message: fmt.Sprintf("unable to resolve group GID %d for %s", stat.Gid, c.path),
		}
	}

	expected := c.spec.Group
	observed := group.Name
	status := evidence.StatusPass
	message := fmt.Sprintf("path group matches %s", expected)
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
			Collector:   "paths",
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s gid=%d group=%s", resolvedPath(c.checker, c.path), stat.Gid, observed),
		},
		Message: message,
	}
}

func observedPathType(mode os.FileMode) string {
	if mode.IsRegular() {
		return "file"
	}
	if mode.IsDir() {
		return "directory"
	}

	return "other"
}

func formatFileMode(mode os.FileMode) string {
	value := uint32(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		value |= 0o4000
	}
	if mode&os.ModeSetgid != 0 {
		value |= 0o2000
	}
	if mode&os.ModeSticky != 0 {
		value |= 0o1000
	}
	if value > 0o777 {
		return fmt.Sprintf("%05o", value)
	}

	return fmt.Sprintf("%04o", value)
}

func mapStatError(err error) evidence.ReasonCode {
	if os.IsPermission(err) {
		return evidence.ReasonPermissionDenied
	}

	return evidence.ReasonParseError
}

func errorResult(collector, path string, reason evidence.ReasonCode, message string, err error) evidence.CheckResult {
	if message == "" {
		message = fmt.Sprintf("failed to collect evidence for %s", path)
	}

	return evidence.CheckResult{
		Status:     evidence.StatusError,
		ReasonCode: reason,
		Evidence: evidence.Evidence{
			Source:      "fs.lstat",
			Collector:   collector,
			CollectedAt: time.Now().UTC(),
			Raw:         fmt.Sprintf("path=%s error=%s", path, err),
		},
		Message: message,
	}
}
