# SAVK Release Notes

## Packaging stance

SAVK does not use `npm` or `pnpm`.

Why:

- the runtime is a Go binary
- there is no frontend or Node ecosystem that would justify that packaging
- adding `npm` or `pnpm` would only add another toolchain and support surface

The recommended way to distribute `v0.1` is:

- a compiled binary
- a per-platform `tar.gz` tarball

Native packaging such as `.deb` or `.rpm` can come later if needed, but it is
not required for `v0.1`.

## Release flow

Prerequisites:

- `go` available in `PATH`
- green test suite with `make test`
- also run the path in [integration.md](integration.md) with
  `SAVK_RUN_SYSTEMD_INTEGRATION=1`

Honest scope of that integration:

- it covers namespace preflight
- it covers one narrow observer-local service-backed smoke path
- it is still a minimum path on a real host, not a distro matrix
- it is not a fully independent oracle for every hardened service-backed edge
  case

Local build:

```bash
make build
./bin/savk version
```

`make build` and `make dist` force `CGO_ENABLED=0`, so the official SAVK
binary stays aligned with the single-binary release stance.

Explicit tradeoff:

- name-based `owner` and `group` checks depend on `/etc/passwd` and
  `/etc/group` visible to SAVK or to `--host-root`, not on libc/NSS
- if those files do not provide sufficient evidence to resolve names, SAVK
  degrades to `INSUFFICIENT_DATA`
- `services` and `identity` are observer-local only in `v0.1.x`; SAVK does not
  try to remap or prove a service-backed target distinct from the observer
- service-backed checks currently also assume observer-local `systemctl`
  resolves to `/usr/bin/systemctl` or `/bin/systemctl`; outside that trust
  boundary, SAVK fails closed with an explicit unsupported-environment `ERROR`

Release artifact for the current platform:

```bash
make clean
make dist VERSION=0.1.5 COMMIT=abc1234
```

That produces:

- `dist/savk-<version>-<goos>-<goarch>`
- `dist/savk-<version>-<goos>-<goarch>.tar.gz`
- `dist/SHA256SUMS`

Release hygiene for the local working tree:

- run `make clean` before `make build` / `make dist` so stale ignored artifacts
  from earlier versions do not sit next to the current release candidate
- if local `bin/` or `dist/` contents are not from the current tree and current
  version, treat them as stale and regenerate them before using them as release
  signal

Operational checklist:

- [Release checklist](release-checklist.md)

## Build metadata

`savk version` exposes:

- version
- commit
- build date
- platform
- `contractVersion`
- `reportSchema`

The first three are injected via `ldflags` from the `Makefile`.
