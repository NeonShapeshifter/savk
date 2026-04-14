# Contributing

Gracias por interesarte en contribuir a SAVK.

## Scope

SAVK mantiene estas reglas:

- core `stdlib-only`
- target soportado en `v0.1`: `linux-systemd`
- el contrato y el reporte están definidos por:
  - [docs/spec/contract-v1.md](docs/spec/contract-v1.md)
  - [docs/spec/report-v1.md](docs/spec/report-v1.md)

Antes de proponer cambios grandes, conviene abrir un issue o describir el
alcance con claridad.

## Development

Requisitos:

- Go en `PATH`

Comandos útiles:

```bash
make test
make build
```

Si tu entorno no puede escribir en el cache default de Go:

```bash
GOCACHE=/tmp/savk-go-build make test
GOCACHE=/tmp/savk-go-build make build
```

## Contribution guidelines

- mantener el core sin dependencias externas
- no ampliar el subset YAML sin actualizar la spec
- no cambiar el shape del JSON sin actualizar la spec y los goldens
- preferir cambios pequeños, probados y con semántica clara
- si agregas checks o collectors, deben respetar `context.Context`

## Pull requests

Un PR sano para SAVK debería incluir:

- motivación clara
- tests o fixtures cuando aplique
- cambios de docs si el comportamiento observable cambia

## Out of scope for `v0.1`

- remediación
- remote execution
- SARIF
- targets no `linux-systemd`
