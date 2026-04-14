# Changelog

## v0.1.0 - 2026-04-13

Release candidate notes para `v0.1.0`.

Incluye:

- parser zero-deps para `savk/v1`
- CLI con `validate`, `check` y `version`
- reporter `json` estable y reporter `table`
- dominios `paths`, `sockets`, `services` e `identity`
- `identity` modelado como runtime process identity
- preflight de namespace para `paths`, `sockets` y `services`
- `--host-root` para dominios filesystem-backed
- timeout por collector, propagacion de prerequisitos y panic recovery
- contratos de ejemplo en `examples/`
- metadata de build y flujo de release con `make build` y `make dist`
- ruta de integración reproducible para `linux-systemd`

Notas de alcance:

- target soportado: `linux-systemd`
- distribucion recomendada: binario compilado o `tar.gz`
- `npm` y `pnpm` no forman parte del release
- la cobertura más fuerte hoy está en parser, engine y `paths`
- `services` e `identity` ya tienen ruta de integración real, pero no matrix por distro
