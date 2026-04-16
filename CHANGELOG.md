# Changelog

## Unreleased

## v0.1.5 - 2026-04-16

Patch release for the final pre-ship correctness and release-truth gaps.

Includes:

- decode supported quoted-scalar escapes so `validate` and `check` target the
  same literal keys and values
- reject unsupported quoted-string escapes explicitly instead of accepting and
  mis-targeting them later
- align quickstart, changelog, checklist, and local artifact/version guidance
  around `v0.1.5`
- narrow `identity` wording to current observer-local runtime process
  observation with bounded provenance confidence

## v0.1.4 - 2026-04-15

Patch release for the remaining post-hardening correctness issues.

Includes:

- quote-aware parsing for quoted absolute path/socket keys
- explicit parser rejection for unquoted absolute keys containing `:`
- correct handling of literal numeric-looking account names in `services.run_as.*`
- shared production account-resolution semantics in the real integration helper
- release metadata alignment for `v0.1.4`

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
