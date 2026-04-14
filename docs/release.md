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
- si quieres afirmar validación real para service-backed domains, correr también
  la ruta de [integration.md](integration.md)

Alcance honesto de esa integración:

- cubre namespace preflight
- cubre `services.state`
- cubre `identity.uid`
- no demuestra toda la superficie de `services` ni de `identity`

Build local:

```bash
make build
./bin/savk version
```

`make build` y `make dist` fuerzan `CGO_ENABLED=0`, así que el binario
oficial de SAVK sale alineado con la postura de single-binary release.

Tradeoff explícito:

- el binario oficial usa el comportamiento pure-Go de `os/user`
- por tanto, los checks por nombre de `owner`/`group` dependen de la
  resolución local de cuentas visible para ese binario, no de libc/NSS

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
