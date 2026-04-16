# SAVK

Security Appliance Verification Kit.

SAVK verifies a Linux host against a strict operator-defined contract.
It does not audit generic "best practices"; it compares contract against
reality and emits evidence.

Current status:

- `stdlib-only` in the core
- zero-dependency parser for a strict YAML subset
- `savk validate`
- `savk check`
- stable `json` reporter and `table` reporter
- `paths`, `identity`, `sockets`, and `services` domains
- active publish-readiness hardening; the most mature surface today is
  parser + engine + `paths`

Specs:

- [Contract spec](docs/spec/contract-v1.md)
- [Report spec](docs/spec/report-v1.md)
- [Roadmap](docs/roadmap.md)
- [Release notes](docs/release.md)
- [Release checklist](docs/release-checklist.md)
- [Integration path](docs/integration.md)
- [Blueprint / design notes](savk-blueprint.md)

## Build

```bash
make build
./bin/savk version
```

Official release artifacts:

```bash
make dist VERSION=0.1.5 COMMIT=abc1234
```

## Commands

```bash
./bin/savk validate --contract appliance.yaml
./bin/savk check --contract appliance.yaml --format json
./bin/savk check --contract appliance.yaml --format table
./bin/savk check --contract appliance.yaml --domain services --collector-timeout 5s
./bin/savk check --contract appliance.yaml --domain paths --host-root /host
./bin/savk check --contract appliance.yaml --format json --include-raw
./bin/savk version
```

## Quickstart

Use the most validated slice first:

```bash
./bin/savk validate --contract examples/paths-only.yaml
```

The published quickstart stops at `validate` intentionally. Running `check`
against repo examples is environment-dependent: the referenced files must exist
in the observed root and, without `--host-root`, observer-local namespace
preflight must also succeed.

## Examples

- [paths-only.yaml](examples/paths-only.yaml)
- [sockets-only.yaml](examples/sockets-only.yaml)
- [services-only.yaml](examples/services-only.yaml)
- [identity-runtime.yaml](examples/identity-runtime.yaml)
- [full-sensor-agent.yaml](examples/full-sensor-agent.yaml) - mixed example, not the recommended quickstart

## Minimal contract

```yaml
apiVersion: savk/v1
kind: ApplianceContract
metadata:
  name: sensor-agent
  target: linux-systemd
paths:
  /etc/sensor-agent/config.yaml:
    type: file
    mode: "0640"
```

## Domain maturity

- `paths`: primary slice today. End-to-end, real filesystem checks, honest about existence, type, mode, owner, and group.
- `sockets`: filesystem-backed, implemented with `lstat` and real Unix socket tests when the environment allows it.
- `services`: implemented for observer-local `linux-systemd`, covered by unit tests and a narrow real smoke path on a real host. That smoke path is still narrow today and does not demonstrate every service-backed branch on every host.
- `identity`: observer-local runtime process identity under the current
  `service -> (MainPID + ControlGroup) -> /proc/<pid>/{status,cgroup}`
  observation. It has unit tests and a narrow real smoke path, but it does not
  broadly prove mixed-namespace environments, durable process provenance, or
  every real service combination.

Current surface by domain:

- `paths`: `exists`, `type`, `mode`, `owner`, `group`
- `sockets`: `exists`, `owner`, `group`, `mode`
- `services`: `state`, `restart`, `run_as.user`, `run_as.group`, `capabilities`
- `identity`: runtime `uid`, `gid`, capability sets `effective`, `permitted`, `inheritable`, `bounding`, `ambient`

## Operational notes

- `services` and `identity` are observer-local only in `v0.1.x`
- `services` assumes the `linux-systemd` target
- `paths` and `sockets` observe the node with `lstat`; they do not follow symlinks
- without `--host-root`, `paths`, `sockets`, and the service-backed path run
  observer-local preflight against `/proc/1/comm`
- if observer-local PID 1 is not `systemd`, preflight reports
  `NAMESPACE_ISOLATION` and blocks dependent checks
- `--host-root` remaps only `paths` and `sockets` to an explicit host root
- `host` in the report identifies the observer; when `--host-root` is used, the
  report also includes `hostRoot` to make the observed root explicit
- service-backed checks currently require observer-local `systemctl` to resolve
  to an allowlisted absolute path (`/usr/bin/systemctl` or `/bin/systemctl`);
  otherwise they fail closed with `ERROR` and an explicit unsupported-environment
  message
- `services.run_as.user` and `services.run_as.group` check normalized
  observer-local `systemctl show User=/Group=` unit properties; runtime process
  identity belongs to the `identity` domain
- `identity` PASS means the current observer-local process observation matched
  the contract at collection time under the current `MainPID` +
  `ControlGroup` linkage; it is not a stronger cross-time or cross-namespace
  provenance claim
- in `v0.1`, `services.capabilities` compares `AmbientCapabilities`
- `evidence.raw` is redacted and truncated by default in the report
- `--include-raw` exposes the full collector raw output under explicit opt-in
- shelling out to `systemctl` forces `LANG=C` and `LC_ALL=C` and uses an
  allowlisted absolute path after resolution
- name-based `owner` and `group` checks in `paths` and `sockets` resolve
  against `/etc/passwd` and `/etc/group` from the observed system; if there is
  no trustworthy mapping, they degrade to `INSUFFICIENT_DATA`
- `services.run_as.user` and `services.run_as.group` normalize numeric-looking
  `systemctl show User=/Group=` values through observer-local `/etc/passwd` and
  `/etc/group`; if the literal-name and UID/GID interpretations disagree, or if
  the evidence is otherwise insufficient, they degrade to `INSUFFICIENT_DATA`
- SAVK does not try in `v0.1.x` to prove that `systemctl`, `/proc/<pid>`, and
  the local account databases belong to a target different from the observer
- `json` is the stable public contract; `table` is human output
- exit codes:

```text
0 -> only PASS / NOT_APPLICABLE
1 -> at least one FAIL, with no ERROR / INSUFFICIENT_DATA
2 -> at least one ERROR or INSUFFICIENT_DATA
3 -> CLI or contract error before the engine
```

## Known limits in the current slice

- only `linux-systemd` is supported
- the parser supports an explicit YAML subset, not full YAML
- `--host-root` currently applies only to `paths` and `sockets`
- name resolution depends on `/etc/passwd` and `/etc/group` visible to SAVK;
  NSS-only accounts outside those files can degrade to `INSUFFICIENT_DATA`
- for `services` and `identity`, the current semantics are observer-local; SAVK
  does not support mixed-namespace `v0.1.x` flows with a service-backed target
  separate from the observer
- for `identity`, current linkage to the service is bounded to the
  observer-local `MainPID` + `ControlGroup` observation at collection time; it
  is not a durable provenance model
- for `services` and `identity`, some failure classifications still depend on
  `systemctl` rather than a native systemd API
- the real integration path included in the repo requires explicit opt-in and a
  real `linux-systemd` host; it is a narrow observer-local smoke path, not an
  independent oracle for every service-backed branch, so confidence still comes
  mostly from unit tests
- there is no remediation, remote execution, snapshots, or SARIF
- the recommended packaging is a binary or tarball; `npm` and `pnpm` are not
  part of the release
