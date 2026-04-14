# SAVK Release Notes

## Packaging stance

SAVK no usa `npm` ni `pnpm`.

Razones:

- el runtime es un binario Go
- no hay frontend ni ecosistema Node que justificaría ese empaquetado
- meter `npm` o `pnpm` solo agrega otra toolchain y otra superficie de soporte

La forma recomendada de distribuir `v0.1` es:

- binario compilado
- tarball `tar.gz` por plataforma

Empaquetado nativo tipo `.deb` o `.rpm` puede venir después si hace falta,
pero no es requisito para `v0.1`.

## Release flow

Prerequisitos:

- `go` disponible en `PATH`
- suite verde con `make test`
- correr también la ruta de [integration.md](integration.md) con
  `SAVK_RUN_SYSTEMD_INTEGRATION=1`

Alcance honesto de esa integración:

- cubre namespace preflight
- cubre toda la superficie pública service-backed del release
- sigue siendo una ruta mínima sobre un host real, no una matrix por distro

Build local:

```bash
make build
./bin/savk version
```

`make build` y `make dist` fuerzan `CGO_ENABLED=0`, así que el binario
oficial de SAVK sale alineado con la postura de single-binary release.

Tradeoff explícito:

- los checks por nombre de `owner`/`group` dependen de `/etc/passwd` y
  `/etc/group` visibles para SAVK o para `--host-root`, no de libc/NSS
- si esos archivos no ofrecen evidencia suficiente para resolver nombres,
  SAVK degrada a `INSUFFICIENT_DATA`

Artifact de release para la plataforma actual:

```bash
make dist VERSION=0.1.0 COMMIT=abc1234
```

Eso produce:

- `dist/savk-<version>-<goos>-<goarch>`
- `dist/savk-<version>-<goos>-<goarch>.tar.gz`
- `dist/SHA256SUMS`

Checklist operativa:

- [Release checklist](release-checklist.md)

## Metadata de build

`savk version` expone:

- versión
- commit
- fecha de build
- plataforma
- `contractVersion`
- `reportSchema`

Las tres primeras se inyectan por `ldflags` desde el `Makefile`.
