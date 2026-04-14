# SAVK Integration Path

Esta ruta existe para validar los dominios service-backed en un host
`linux-systemd` real sin cortar features del repo público.

## Qué cubre hoy

- preflight de namespace
- `services.state`
- `identity.uid` vía
  `service -> (MainPID + ControlGroup) -> /proc/<pid>/{status,cgroup}`
- assertions reales sobre checks `PASS`, no solo presencia de dominios

No cubre todavía:

- `services.restart`
- `services.run_as.group`
- `services.capabilities`
- `identity.gid`
- los capability sets de `identity`

## Requisitos

- host Linux con `systemd` como PID 1
- `go` disponible en `PATH`

## Ejecución rápida

Por defecto usa `systemd-journald.service` y espera `uid=0`.

```bash
SAVK_RUN_SYSTEMD_INTEGRATION=1 \
GOCACHE=/tmp/savk-go-build \
make integration GO=/usr/local/go/bin/go
```

## Overrides

Si en tu host quieres probar otra unidad:

```bash
SAVK_RUN_SYSTEMD_INTEGRATION=1 \
SAVK_SYSTEMD_INTEGRATION_SERVICE=dbus.service \
SAVK_SYSTEMD_INTEGRATION_UID=81 \
GOCACHE=/tmp/savk-go-build \
make integration GO=/usr/local/go/bin/go
```

## Nota

Esto no reemplaza una matrix por distro. Es la ruta mínima reproducible para
validar una parte concreta de la superficie service-backed en un sistema real.
