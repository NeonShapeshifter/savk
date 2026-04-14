# SAVK Release Checklist

## v0.1.0

Antes de publicar:

- correr `GOCACHE=/tmp/savk-go-build make test GO=/usr/local/go/bin/go`
- correr `SAVK_RUN_SYSTEMD_INTEGRATION=1 GOCACHE=/tmp/savk-go-build make integration GO=/usr/local/go/bin/go`
- validar que `./bin/savk version` muestre version, commit y buildDate correctos
- regenerar artefactos con `GOCACHE=/tmp/savk-go-build make dist GO=/usr/local/go/bin/go VERSION=0.1.0 COMMIT=<git-sha> BUILD_DATE=<utc-rfc3339>`
- regenerar `dist/SHA256SUMS` si cambian los artefactos
- revisar que `examples/` sigan parseando y construyendo checks
- confirmar que el release sigue limitado a `linux-systemd`

Publicacion:

- publicar `dist/savk-<version>-<goos>-<goarch>`
- publicar `dist/savk-<version>-<goos>-<goarch>.tar.gz`
- publicar `dist/SHA256SUMS`
- crear tag git `v0.1.0`
- adjuntar el resumen de [CHANGELOG.md](../CHANGELOG.md)
