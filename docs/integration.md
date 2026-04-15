# SAVK Integration Path

This path exists to validate the service-backed domains on a real
`linux-systemd` host without trimming features from the public repo.

## What It Covers Today

- namespace preflight
- `services.state`
- `services.restart`
- `services.run_as.user`
- `services.run_as.group`
- `services.capabilities`
- `identity.uid`
- `identity.gid`
- the capability sets of `identity`
- real assertions over `PASS` checks on an observer-local path, not just domain
  presence

Honest boundary:

- this integration proves one real observer-local path, not a general guarantee
  over mixed-namespace behavior or a distro matrix
- the default subject tries to choose an active unit with richer properties; if
  none exists, it falls back to the safe fallback
- even then, real coverage still depends on the unit available on that host

## Requirements

- Linux host with `systemd` as PID 1
- `go` available in `PATH`

## Quick Run

By default it tries to choose an active unit with more informative `User`,
`Group`, or `AmbientCapabilities` values and derives expectations from the
observer-local host at test time. If it does not find a better candidate, it
uses the available fallback.

```bash
SAVK_RUN_SYSTEMD_INTEGRATION=1 \
GOCACHE=/tmp/savk-go-build \
make integration GO=/usr/local/go/bin/go
```

## Overrides

If you want to test a different unit on your host:

```bash
SAVK_RUN_SYSTEMD_INTEGRATION=1 \
SAVK_SYSTEMD_INTEGRATION_SERVICE=dbus.service \
GOCACHE=/tmp/savk-go-build \
make integration GO=/usr/local/go/bin/go
```

## Note

`make integration` now fails unless `SAVK_RUN_SYSTEMD_INTEGRATION=1` is enabled
explicitly; a skip no longer counts as release signal.

This does not replace a distro matrix. It is the minimum reproducible path for
validating one observer-local service-backed path on a real system.
