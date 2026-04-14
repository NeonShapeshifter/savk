package collectors

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/evidence"
)

const ServiceNamespaceCheckID = "service.__preflight__.namespace"
const PathNamespaceCheckID = "path.__preflight__.namespace"
const SocketNamespaceCheckID = "socket.__preflight__.namespace"

type ServiceNamespaceProbe interface {
	PID1Comm(ctx context.Context) (string, error)
}

type OSServiceNamespaceProbe struct{}

func (OSServiceNamespaceProbe) PID1Comm(ctx context.Context) (string, error) {
	_ = ctx

	data, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func NewServiceNamespaceCheck(target string, probe ServiceNamespaceProbe) engine.Check {
	return newNamespaceCheck("services", ServiceNamespaceCheckID, target, probe)
}

func NewPathNamespaceCheck(target string, probe ServiceNamespaceProbe) engine.Check {
	return newNamespaceCheck("paths", PathNamespaceCheckID, target, probe)
}

func NewSocketNamespaceCheck(target string, probe ServiceNamespaceProbe) engine.Check {
	return newNamespaceCheck("sockets", SocketNamespaceCheckID, target, probe)
}

func newNamespaceCheck(domain, id, target string, probe ServiceNamespaceProbe) engine.Check {
	if probe == nil {
		probe = OSServiceNamespaceProbe{}
	}

	return serviceNamespaceCheck{
		id:     id,
		domain: domain,
		target: target,
		probe:  probe,
	}
}

type serviceNamespaceCheck struct {
	id     string
	domain string
	target string
	probe  ServiceNamespaceProbe
}

func (c serviceNamespaceCheck) ID() string {
	return c.id
}

func (c serviceNamespaceCheck) Domain() string {
	return c.domain
}

func (c serviceNamespaceCheck) Prerequisites() []string {
	return nil
}

func (c serviceNamespaceCheck) Run(ctx context.Context) evidence.CheckResult {
	if err := ctx.Err(); err != nil {
		return serviceNamespaceError(c.domain, evidence.ReasonTimeout, "collector context cancelled before namespace preflight started", err.Error())
	}

	if c.target != contract.TargetLinuxSystemd {
		return evidence.CheckResult{
			Status:     evidence.StatusNotApplicable,
			ReasonCode: evidence.ReasonNone,
			Expected:   contract.TargetLinuxSystemd,
			Observed:   c.target,
			Evidence: evidence.Evidence{
				Source:      "contract.metadata.target",
				Collector:   c.domain,
				CollectedAt: time.Now().UTC(),
				Raw:         c.target,
			},
			Message: fmt.Sprintf("%s namespace preflight does not apply to target %s", c.domain, c.target),
		}
	}

	observed, err := c.probe.PID1Comm(ctx)
	if err != nil {
		reason := evidence.ReasonParseError
		if errors.Is(err, fs.ErrPermission) {
			reason = evidence.ReasonPermissionDenied
		}

		return serviceNamespaceError(c.domain, reason, "failed to inspect /proc/1/comm", err.Error())
	}
	if observed == "" {
		return serviceNamespaceError(c.domain, evidence.ReasonParseError, "empty /proc/1/comm while checking namespace", "")
	}
	if observed != "systemd" {
		return evidence.CheckResult{
			Status:     evidence.StatusError,
			ReasonCode: evidence.ReasonNamespaceIsolation,
			Expected:   "systemd",
			Observed:   observed,
			Evidence: evidence.Evidence{
				Source:      "/proc/1/comm",
				Collector:   c.domain,
				CollectedAt: time.Now().UTC(),
				Raw:         observed,
			},
			Message: fmt.Sprintf("expected PID 1 to be systemd for target %s, observed %s", c.target, observed),
		}
	}

	return evidence.CheckResult{
		Status:     evidence.StatusPass,
		ReasonCode: evidence.ReasonNone,
		Expected:   "systemd",
		Observed:   observed,
		Evidence: evidence.Evidence{
			Source:      "/proc/1/comm",
			Collector:   c.domain,
			CollectedAt: time.Now().UTC(),
			Raw:         observed,
		},
		Message: fmt.Sprintf("%s namespace preflight confirms PID 1 is systemd", c.domain),
	}
}

func serviceNamespaceError(domain string, reason evidence.ReasonCode, message, raw string) evidence.CheckResult {
	return evidence.CheckResult{
		Status:     evidence.StatusError,
		ReasonCode: reason,
		Evidence: evidence.Evidence{
			Source:      "/proc/1/comm",
			Collector:   domain,
			CollectedAt: time.Now().UTC(),
			Raw:         raw,
		},
		Message: message,
	}
}
