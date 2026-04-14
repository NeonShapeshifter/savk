# SAVK Integration Path

Esta ruta existe para validar los dominios service-backed en un host
`linux-systemd` real sin cortar features del repo público.

## Qué cubre hoy

- preflight de namespace
- `services.state`
- `services.restart`
- `services.run_as.user`
- `services.run_as.group`
- `services.capabilities`
- `identity.uid`
- `identity.gid`
- los capability sets de `identity`
- assertions reales sobre checks `PASS`, no solo presencia de dominios

## Requisitos

- host Linux con `systemd` como PID 1
- `go` disponible en `PATH`

## Ejecución rápida

Por defecto usa `systemd-journald.service` y deriva expectativas desde el host
real en el momento del test.

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
GOCACHE=/tmp/savk-go-build \
make integration GO=/usr/local/go/bin/go
```

## Nota

`make integration` ahora falla si no se habilita explícitamente
`SAVK_RUN_SYSTEMD_INTEGRATION=1`; un skip ya no cuenta como señal de release.

Esto no reemplaza una matrix por distro. Es la ruta mínima reproducible para
validar la superficie pública service-backed en un sistema real.
