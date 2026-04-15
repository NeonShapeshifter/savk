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
- assertions reales sobre checks `PASS` en una ruta observer-local, no solo
  presencia de dominios

Límite honesto:

- esta integración prueba una ruta observer-local real, no una garantía general
  sobre mixed-namespace ni una matrix por distro
- el subject por defecto intenta elegir una unidad activa con propiedades más
  ricas; si no existe, cae al fallback seguro
- aun así, la cobertura real sigue dependiendo de la unidad disponible en ese
  host

## Requisitos

- host Linux con `systemd` como PID 1
- `go` disponible en `PATH`

## Ejecución rápida

Por defecto intenta elegir una unidad activa con `User`, `Group` o
`AmbientCapabilities` más informativas y deriva expectativas desde el host
observer-local en el momento del test. Si no encuentra una mejor candidata,
usa el fallback disponible.

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
validar una ruta service-backed observer-local en un sistema real.
