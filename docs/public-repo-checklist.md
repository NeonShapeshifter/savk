# Public Repo Checklist

SAVK puede hacerse público cuando el repositorio no sobrepromete más de lo
que hoy está verificado.

## Ya resuelto

- README y docs base
- specs del contrato y del reporte
- examples/
- changelog
- release notes
- release checklist
- `.gitignore`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `LICENSE`

## Revisión final antes de abrirlo

1. confirmar que `README` no haga claims más fuertes que los tests
2. revisar que no quieras publicar `bin/` ni `dist/` en el repo
3. confirmar que no hay secretos ni datos locales en la historia
4. correr `GOCACHE=/tmp/savk-go-build make test GO=/usr/local/go/bin/go`
5. correr la ruta real de [docs/integration.md](integration.md) en un host `linux-systemd`
6. si vas a publicar artefactos, adjuntarlos en el release y no como contenido del repo
