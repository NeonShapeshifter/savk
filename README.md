# SAVK

Security Appliance Verification Kit.

SAVK verifica un host Linux contra un contrato estricto definido por el
operador. No audita “best practices” genéricas; compara contrato contra
realidad y emite evidencia.

Estado actual:

- `stdlib-only` en el core
- parser zero-deps para un subset estricto de YAML
- `savk validate`
- `savk check`
- reporter `json` estable y reporter `table`
- dominios `paths`, `identity`, `sockets`, `services`
- hardening activo de publish-readiness; la superficie más madura hoy es
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

Artifacts oficiales de release:

```bash
make dist VERSION=0.1.0 COMMIT=abc1234
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

Usa primero el slice más validado:

```bash
./bin/savk validate --contract examples/paths-only.yaml
./bin/savk check --contract examples/paths-only.yaml --format json
```

## Examples

- [paths-only.yaml](examples/paths-only.yaml)
- [sockets-only.yaml](examples/sockets-only.yaml)
- [services-only.yaml](examples/services-only.yaml)
- [identity-runtime.yaml](examples/identity-runtime.yaml)
- [full-sensor-agent.yaml](examples/full-sensor-agent.yaml) - ejemplo mixto, no quickstart recomendado

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

- `paths`: slice principal hoy. End-to-end, real filesystem checks, honesto para existencia, tipo, modo, owner y group.
- `sockets`: filesystem-backed, implementado con `lstat` y tests reales de Unix socket cuando el entorno lo permite.
- `services`: implementado para `linux-systemd`, cubierto por unit tests y una ruta de integración real que valida toda la superficie pública service-backed.
- `identity`: runtime process identity via `service -> (MainPID + ControlGroup) -> /proc/<pid>/{status,cgroup}`, con unit tests y cobertura de integración real para `uid`, `gid` y capability sets.

Superficie actual por dominio:

- `paths`: `exists`, `type`, `mode`, `owner`, `group`
- `sockets`: `exists`, `owner`, `group`, `mode`
- `services`: `state`, `restart`, `run_as.user`, `run_as.group`, `capabilities`
- `identity`: runtime `uid`, `gid`, capability sets `effective`, `permitted`, `inheritable`, `bounding`, `ambient`

## Operational notes

- `services` asume target `linux-systemd`
- `paths` y `sockets` observan el nodo con `lstat`; no siguen symlinks
- sin `--host-root`, `paths`, `sockets` y `services` hacen preflight de namespace
  sobre `/proc/1/comm`
- si PID 1 no es `systemd`, el preflight reporta `NAMESPACE_ISOLATION` y los
  checks dependientes quedan bloqueados con `PREREQUISITE_FAILED`
- `--host-root` remapea solo `paths` y `sockets` a un root explícito del host
- `host` en el reporte identifica al observador; si se usa `--host-root`, el
  reporte añade `hostRoot` para dejar explícito el root observado
- en `v0.1`, `services.capabilities` compara `AmbientCapabilities`
- `evidence.raw` se redacta y trunca por defecto en el reporte
- `--include-raw` expone el raw completo del collector bajo opt-in explícito
- shell-out a `systemctl` fuerza `LANG=C` y `LC_ALL=C` y usa una ruta absoluta allowlisted tras resolución
- los checks por nombre de `owner`/`group` en `paths` y `sockets` resuelven
  contra `/etc/passwd` y `/etc/group` del sistema observado; si no hay mapping
  confiable, degradan a `INSUFFICIENT_DATA`
- los checks `services.run_as.user` y `services.run_as.group` solo resuelven
  IDs numéricos y grupos primarios vía `/etc/passwd` y `/etc/group` locales; si
  esa evidencia no alcanza, degradan a `INSUFFICIENT_DATA`
- `json` es el contrato público estable; `table` es salida humana
- exit codes:

```text
0 -> solo PASS / NOT_APPLICABLE
1 -> al menos un FAIL, sin ERROR / INSUFFICIENT_DATA
2 -> al menos un ERROR o INSUFFICIENT_DATA
3 -> error de CLI o contrato antes del engine
```

## Known limits in the current slice

- solo soporta `linux-systemd`
- el parser soporta un subset explícito de YAML, no YAML completo
- `--host-root` hoy solo aplica a `paths` y `sockets`
- la resolución por nombre depende de `/etc/passwd` y `/etc/group` visibles para
  SAVK; cuentas solo-NSS fuera de esos archivos pueden degradar a
  `INSUFFICIENT_DATA`
- para `services` e `identity`, algunas clasificaciones de fallo todavía dependen
  de `systemctl` y no de una API nativa de systemd
- la integración real incluida en el repo requiere opt-in explícito y un host
  `linux-systemd` real; fuera de eso, la confianza sigue viniendo de unit tests
- no hay remediación, remote execution, snapshots ni SARIF
- el empaquetado recomendado es binario o tarball; `npm` y `pnpm` no forman parte del release
