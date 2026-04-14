# SAVK Report Spec v1

Status: normative for `schemaVersion: savk-report/v1`

## Scope

Este documento define el reporte JSON emitido por `savk check` cuando la
ejecución llega al engine.

Objetivos:

- fijar un contrato JSON estable y machine-readable
- fijar la semántica de estados y reason codes
- fijar el shape de `results`
- fijar la semántica de evidence, truncado y redacción
- fijar reglas de orden y determinismo

No define:

- el contrato de entrada YAML
- el formato table/human-readable
- el shape de futuras snapshots

## Versioning

- `schemaVersion` versiona el reporte JSON
- este documento aplica solo a `schemaVersion: savk-report/v1`
- cambios incompatibles en el reporte requieren nueva `schemaVersion`
- `contractVersion` refleja la `apiVersion` del contrato usado en el run

## Emission rules

Este documento aplica a `savk check`.

Reglas:

- si el contrato o la CLI fallan antes de arrancar el engine, SAVK PUEDE no
  emitir reporte JSON y DEBE salir con exit code `3`
- si el engine arranca, el reporte JSON DEBE respetar esta spec aunque haya
  resultados `FAIL`, `ERROR` o `INSUFFICIENT_DATA`

## Top-level object

El reporte raíz DEBE ser un JSON object.

Fields mínimas obligatorias:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `schemaVersion` | string | yes | exacto `savk-report/v1` |
| `toolVersion` | string | yes | versión de SAVK |
| `contractVersion` | string | yes | exacto `savk/v1` en `v0.1` |
| `contractHash` | string | yes | `sha256:<hex>` del contrato consumido |
| `runID` | string | yes | identificador opaco del run |
| `target` | string | yes | target del contrato |
| `host` | string | yes | identidad del host observador |
| `startedAt` | string | yes | RFC3339 UTC |
| `durationMs` | integer | yes | duración total del run |
| `results` | array | yes | lista de `CheckResult` |

Fields recomendadas:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `exitCode` | integer | no | código de salida final |
| `hostRoot` | string | no | root filesystem remapeado usado por `--host-root` |
| `summary` | object | no | conteos por estado |

### Top-level semantics

- `contractHash` DEBE ser el SHA-256 de los bytes exactos consumidos por el parser
- `runID` DEBE tratarse como string opaco
- `host` identifica al observador que ejecutó SAVK
- `hostRoot`, si existe, DEBE reflejar el root filesystem remapeado usado para
  `paths` y `sockets`
- `startedAt` DEBE serializarse en UTC
- `durationMs` DEBE ser `>= 0`
- `results` DEBE estar ordenado de forma estable

Si existe `summary`, SHOULD usar este shape:

```json
{
  "pass": 0,
  "fail": 0,
  "notApplicable": 0,
  "insufficientData": 0,
  "error": 0
}
```

## Result object

Cada entrada de `results` DEBE ser un object con este shape:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `checkID` | string | yes | ID estable y determinista |
| `domain` | string | yes | `services`, `sockets`, `paths`, `identity` |
| `status` | string | yes | ver sección de estados |
| `reasonCode` | string or null | no | ver sección de reason codes |
| `expected` | JSON value | no | valor esperado usado por la evaluación |
| `observed` | JSON value | no | valor observado estructurado |
| `evidence` | object | yes | evidencia asociada al check |
| `durationMs` | integer | yes | duración del check |
| `message` | string | yes | mensaje accionable |

Reglas:

- `durationMs` DEBE ser `>= 0`
- `message` DEBE ser no vacío
- `observed` SHOULD ser estructurado cuando sea posible
- `expected` y `observed` NO DEBEN depender de parsear `evidence.raw`

## Evaluation states

Valores válidos de `status`:

```text
PASS
FAIL
NOT_APPLICABLE
INSUFFICIENT_DATA
ERROR
```

Semántica:

- `PASS`: observado == esperado con evidencia suficiente
- `FAIL`: observado != esperado con evidencia suficiente
- `NOT_APPLICABLE`: el check no aplica en este contexto
- `INSUFFICIENT_DATA`: el collector corrió, pero la evidencia no alcanza
- `ERROR`: el collector no pudo ejecutarse correctamente

## Reason codes

Valores válidos:

```text
TIMEOUT
PERMISSION_DENIED
NOT_FOUND
PARSE_ERROR
NAMESPACE_ISOLATION
INTERNAL_ERROR
PREREQUISITE_FAILED
```

### Reason code usage

- `reasonCode` MAY omitirse o ser `null` cuando no agrega información útil
- `TIMEOUT` SHOULD acompañar `ERROR`
- `PERMISSION_DENIED` SHOULD acompañar `ERROR`
- `NOT_FOUND` MAY acompañar `FAIL` cuando la ausencia observada es evidencia sólida
- `PARSE_ERROR` SHOULD acompañar `INSUFFICIENT_DATA` o `ERROR`
- `NAMESPACE_ISOLATION` SHOULD acompañar `ERROR`
- `INTERNAL_ERROR` MUST acompañar `ERROR` por panic o invariante rota
- `PREREQUISITE_FAILED` MUST acompañar resultados propagados por prerequisitos

## Prerequisite propagation

Cuando un check no corre porque un prerequisito no hizo `PASS`, el resultado
serializado DEBE seguir estas reglas:

| Prerequisite status | Dependent status | reasonCode |
|---|---|---|
| `FAIL` | `NOT_APPLICABLE` | `PREREQUISITE_FAILED` |
| `NOT_APPLICABLE` | `NOT_APPLICABLE` | `PREREQUISITE_FAILED` |
| `ERROR` | `INSUFFICIENT_DATA` | `PREREQUISITE_FAILED` |
| `INSUFFICIENT_DATA` | `INSUFFICIENT_DATA` | `PREREQUISITE_FAILED` |

El `message` DEBE nombrar el `checkID` bloqueante.

## Evidence object

`evidence` es respaldo de auditoría y explicación humana. No es el modelo
canónico de evaluación.

Shape:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `source` | string | yes | origen de la observación |
| `collector` | string | yes | collector que produjo la evidencia |
| `collectedAt` | string | yes | RFC3339 UTC |
| `command` | array[string] | no | argv ejecutado, sin shell |
| `exitCode` | integer | no | exit code del comando, si aplica |
| `raw` | string | no | evidencia textual opcional |
| `redacted` | boolean | yes | hubo redacción |
| `truncated` | boolean | yes | el `raw` visible fue truncado |

Reglas:

- `source` y `collector` DEBEN ser no vacíos
- `collectedAt` DEBE estar en UTC
- `command`, si existe, DEBE reflejar `argv` exacto, nunca `sh -c`
- `exitCode` SHOULD existir cuando `command` exista
- `raw` PUEDE omitirse por completo
- cuando `raw` exista, `redacted` y `truncated` DEBEN reflejar el contenido visible

### Evidence semantics

- `observed` es la verdad estructurada usada por la evaluación
- `evidence.raw` es texto auxiliar para auditoría humana
- `evidence.raw` PUEDE ser parcial, truncado o redactado
- un collector PUEDE interpretar su fuente primaria antes de serializarla
  como `evidence.raw`
- un consumer machine-readable NO DEBE depender de `evidence.raw`
- el reporter y los consumers externos NO DEBEN reparsear `evidence.raw`
  para decidir el `status`

## Error classes

`savk-report/v1` no requiere un field explícito para `USER_ERROR`,
`SYSTEM_ERROR` o `INTERNAL_ERROR`.

Semántica:

- `USER_ERROR` describe fallos previos al engine y normalmente no produce reporte
- `SYSTEM_ERROR` se refleja en `status`, `reasonCode` y `message`
- `INTERNAL_ERROR` se refleja con `status: ERROR` y `reasonCode: INTERNAL_ERROR`

## Determinism and ordering

Reglas:

- timestamps DEBEN serializarse en UTC
- el orden de `results` DEBE ser estable entre runs equivalentes
- el orden RECOMMENDED para `results` es lexicográfico por `checkID`
- el orden de fields emitidas SHOULD ser estable aunque JSON no lo requiera

Un cambio de estrategia interna de ejecución NO DEBE alterar el orden del
reporte si el conjunto de resultados es el mismo.

## Exit codes

Si el reporte incluye `exitCode`, DEBE respetar estas reglas:

```text
0  -> solo PASS / NOT_APPLICABLE
1  -> al menos un FAIL, sin ERROR / INSUFFICIENT_DATA
2  -> al menos un ERROR o INSUFFICIENT_DATA
3  -> contract / CLI user error antes del engine
```

Precedencia:

```text
3 > 2 > 1 > 0
```

## Non-normative example

```json
{
  "schemaVersion": "savk-report/v1",
  "toolVersion": "0.1.0",
  "contractVersion": "savk/v1",
  "contractHash": "sha256:4c54e5b4...",
  "runID": "20260412T160000Z-8f2d",
  "target": "linux-systemd",
  "host": "sensor-prod-01",
  "startedAt": "2026-04-12T16:00:00Z",
  "durationMs": 14,
  "exitCode": 1,
  "summary": {
    "pass": 1,
    "fail": 1,
    "notApplicable": 0,
    "insufficientData": 0,
    "error": 0
  },
  "results": [
    {
      "checkID": "path./etc/sensor-agent/config.yaml.exists",
      "domain": "paths",
      "status": "PASS",
      "expected": true,
      "observed": true,
      "evidence": {
        "source": "fs.stat",
        "collector": "paths",
        "collectedAt": "2026-04-12T16:00:00Z",
        "redacted": false,
        "truncated": false
      },
      "durationMs": 1,
      "message": "path exists"
    },
    {
      "checkID": "path./etc/sensor-agent/config.yaml.mode",
      "domain": "paths",
      "status": "FAIL",
      "reasonCode": null,
      "expected": "0640",
      "observed": "0666",
      "evidence": {
        "source": "fs.stat",
        "collector": "paths",
        "collectedAt": "2026-04-12T16:00:00Z",
        "raw": "mode=0666",
        "redacted": false,
        "truncated": false
      },
      "durationMs": 1,
      "message": "expected mode 0640, observed 0666"
    }
  ]
}
```
