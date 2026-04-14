# SAVK Roadmap

Status: execution roadmap for `v0.1`

## Purpose

Este documento define el orden de construcción de SAVK.

No reemplaza:

- [contract-v1.md](spec/contract-v1.md)
- [report-v1.md](spec/report-v1.md)
- [savk-blueprint.md](../savk-blueprint.md)

Las specs siguen siendo la fuente de verdad del contrato y del reporte.
Este roadmap solo decide el orden, el alcance por fase y la definición de
"hecho".

## Non-negotiables

- `SAVK core` se mantiene `stdlib-only`
- no se aceptan dependencias externas en parser, engine, reporter ni tests de `v0.1`
- `v0.1` soporta solo `linux-systemd`
- no se implementa remediación, remote execution ni introspección de contratos
- primero se cierra un slice útil completo; después se expanden dominios

## Build strategy

SAVK se construye en slices verticales, no por capas aisladas.

Orden de prioridad:

1. contrato válido
2. tipos base
3. engine mínimo
4. primer dominio real
5. reporte JSON estable
6. endurecimiento y expansión

Regla práctica:

- no abrir `services`, `sockets` ni `identity` antes de que `paths` funcione
  de punta a punta
- no abrir concurrencia antes de tener orden estable y prerequisitos cerrados
- no abrir `v0.2` antes de cerrar un `v0.1` usable

## Current state

Hecho:

- blueprint cerrado
- spec del contrato `savk/v1`
- spec del reporte `savk-report/v1`
- CLI con `validate`, `check` y `version`
- engine secuencial con orden estable, prerequisitos, panic recovery y
  timeout por collector
- reporters `json` y `table`
- dominios `paths`, `identity`, `sockets` y `services`
- preflight de namespace para `paths`, `sockets` y `services` sobre `/proc/1/comm`
- `--host-root` para dominios filesystem-backed
- `identity` alineado con el rediseño `runtime process identity`
- contratos de ejemplo en `examples/`

No hecho:

- endurecimiento E2E por distro fuera del unit scope

## Phase 0: Docs locked

Objetivo:

- congelar la semántica mínima antes de escribir código

Salida esperada:

- [contract-v1.md](spec/contract-v1.md) usable como input contract
- [report-v1.md](spec/report-v1.md) usable como output contract
- roadmap alineado con blueprint

Estado:

- completado

## Phase 1: Contract and core types

Objetivo:

- poder ejecutar `savk validate --contract <file>` con salida correcta

Archivos objetivo:

- `internal/evidence/evidence.go`
- `internal/contract/schema.go`
- `internal/contract/parser.go`
- `cmd/savk/main.go`

Trabajo:

- definir `EvalStatus`, `ReasonCode`, `Evidence`, `CheckResult`
- definir tipos canónicos del contrato
- implementar parser zero-deps para el subset YAML soportado
- validar `apiVersion`, `kind`, `metadata`, unknown fields y enums
- sembrar fixtures agresivos desde el primer día
- cablear `savk validate`

Definition of done:

- `savk validate --contract valid.yaml` sale `0`
- `savk validate --contract invalid.yaml` sale `3`
- errores de parseo tienen path + hint cuando aplique
- `invalid/` ya cubre duplicate keys, tabs, flow style, relative paths,
  unknown fields, enum inválido y contract vacío
- no hay dependencias externas en `go.mod`

## Phase 1.5: Golden tests for parser and report

Objetivo:

- congelar temprano el comportamiento observable del parser y del JSON

Archivos objetivo:

- `testdata/fixtures/valid/`
- `testdata/fixtures/invalid/`
- `testdata/golden/parser/`
- `testdata/golden/report/`
- `internal/reporters/json.go`

Trabajo:

- convertir los fixtures agresivos del parser en tests reproducibles
- fijar golden outputs mínimos para errores de parseo y validación
- fijar golden JSON outputs mínimos usando `CheckResult` sintéticos
- fijar assertions de exit code

Definition of done:

- parser fixtures cubren casos válidos y maliciosos
- existe al menos un golden JSON estable para `PASS`, `FAIL` y `ERROR`
- cambios en el shape del JSON rompen tests de inmediato
- el reporter JSON mínimo queda congelado antes de abrir más collectors

## Phase 2: Minimal engine

Objetivo:

- tener un engine secuencial con orden estable y prerequisitos

Archivos objetivo:

- `internal/engine/check.go`
- `internal/engine/engine.go`
- `internal/engine/evaluator.go`

Trabajo:

- definir interfaz `Check`
- ordenar checks por `CheckID`
- resolver prerequisitos
- detectar ciclos antes de ejecutar checks
- recuperar panics por check

Definition of done:

- el engine ejecuta checks en orden estable
- la propagación de prerequisitos coincide con `report-v1`
- un panic en un check no aborta el run completo

## Phase 3: First real slice with `paths`

Objetivo:

- tener `savk check` funcionando de punta a punta sobre el dominio `paths`

Archivos objetivo:

- `internal/collectors/interfaces.go`
- `internal/collectors/paths.go`
- `internal/reporters/json.go`
- `cmd/savk/main.go`

Trabajo:

- implementar observación real de rutas
- derivar checks `path.<path>.exists`, `path.<path>.type`, `path.<path>.mode`,
  `path.<path>.owner`, `path.<path>.group`
- usar el reporter JSON ya congelado en Phase 1.5
- soportar `--format json`

Definition of done:

- un contrato con `paths` produce `PASS`, `FAIL` y `ERROR` reales
- `NOT_FOUND` funciona como evidencia sólida cuando corresponda
- `evidence.raw` es opcional y nunca se reparsea para evaluar
- el JSON emitido cumple [report-v1.md](spec/report-v1.md)
- owner/group en `paths` se comparan por nombre, no por UID/GID

## Phase 4: CLI hardening

Objetivo:

- hacer el binario usable sin inflar alcance

Trabajo:

- `--domain`
- `--collector-timeout`
- `--format json|table`
- `--host-root` para `paths` y `sockets`
- exit codes finales
- mensajes accionables consistentes

Definition of done:

- `savk check --contract x --format json` es estable
- `savk check --contract x --domain paths` limita ejecución correctamente
- timeout por collector produce `ERROR/TIMEOUT`

## Phase 5: Complete `v0.1` domains

Objetivo:

- completar `services`, `identity` y `sockets`

Orden recomendado:

1. `identity`
2. `sockets`
3. `services`

Razonamiento:

- `identity` ya no modela cuentas locales; modela runtime process identity
- `identity` depende de `service.state`, `MainPID`, `ControlGroup`,
  `/proc/<pid>/status` y `/proc/<pid>/cgroup`
- `sockets` sigue siendo filesystem-first
- `services` introduce más superficie y más riesgo

Definition of done:

- los cuatro dominios prometidos existen
- todos respetan `context.Context`
- shell-out, si existe, pasa por `CommandRunner`
- `NAMESPACE_ISOLATION` está implementado para el target soportado
- `identity` usa selector `service` y capability sets explícitos

## Phase 6: Release hardening for `v0.1`

Objetivo:

- dejar SAVK listo para etiquetar una primera versión honesta

Trabajo:

- fixtures `valid/` e `invalid/`
- contratos de ejemplo
- tests unitarios de parser, engine y collectors
- tabla human-readable
- `README.md`
- `savk version`

Definition of done:

- un usuario nuevo puede validar y ejecutar un contrato sin leer el código
- los contratos de ejemplo cubren al menos un caso real por dominio
- el JSON y la CLI son coherentes entre sí

## Future work

Posibles líneas de trabajo después de `v0.1`:

- snapshots
- snapshot comparison model
- SARIF
- DAG scheduler
- matrix E2E por distro

Regla:

- nada de esto entra en `v0.1` si retrasa el primer slice usable

Fuera de alcance por ahora:

- plugin architecture
- contract inheritance
- DSL propia
- generación automática de contratos
- firmado criptográfico de evidencia

## Success condition for `v0.1`

`v0.1` está logrado cuando SAVK puede:

- validar contratos `savk/v1`
- ejecutar checks reales sobre `paths`, `identity`, `sockets` y `services`
- producir JSON estable conforme a spec
- fallar de forma honesta y accionable
- seguir siendo `stdlib-only`

Nota:

- la semántica de `identity` en `v0.1` ya quedó fijada como runtime process
  identity; nuevas iteraciones deberían construir sobre eso, no reabrirlo
