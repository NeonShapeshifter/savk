# SAVK Report Spec v1

Status: normative for `schemaVersion: savk-report/v1`

## Scope

This document defines the JSON report emitted by `savk check` when execution
reaches the engine.

Objectives:

- define a stable, machine-readable JSON contract
- define the semantics of states and reason codes
- define the shape of `results`
- define the semantics of evidence, truncation, and redaction
- define ordering and determinism rules

It does not define:

- the YAML input contract
- the table or human-readable format
- the shape of future snapshots

## Versioning

- `schemaVersion` versions the JSON report
- this document applies only to `schemaVersion: savk-report/v1`
- incompatible report changes require a new `schemaVersion`
- `contractVersion` reflects the contract `apiVersion` used in the run

## Emission rules

This document applies to `savk check`.

Rules:

- if the contract or CLI fail before the engine starts, SAVK MAY omit the JSON
  report and MUST exit with code `3`
- if the engine starts, the JSON report MUST follow this spec even when results
  include `FAIL`, `ERROR`, or `INSUFFICIENT_DATA`

## Top-level object

The root report MUST be a JSON object.

Minimum required fields:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `schemaVersion` | string | yes | exact `savk-report/v1` |
| `toolVersion` | string | yes | SAVK version |
| `contractVersion` | string | yes | exact `savk/v1` in `v0.1` |
| `contractHash` | string | yes | `sha256:<hex>` of the consumed contract |
| `runID` | string | yes | opaque run identifier |
| `target` | string | yes | contract target |
| `host` | string | yes | observer host identity |
| `startedAt` | string | yes | RFC3339 UTC |
| `durationMs` | integer | yes | total run duration |
| `results` | array | yes | list of `CheckResult` |

Recommended fields:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `exitCode` | integer | no | final exit code |
| `hostRoot` | string | no | remapped filesystem root used by `--host-root` |
| `summary` | object | no | counts by state |

### Top-level semantics

- `contractHash` MUST be the SHA-256 of the exact bytes consumed by the parser
- `runID` MUST be treated as an opaque string
- `host` identifies the observer that executed SAVK
- in `v0.1.x`, `services` and `identity` results are observer-local relative
  to that `host`
- for `identity`, a `PASS` is limited to the current observer-local process
  observation at collection time under the current `MainPID` +
  `ControlGroup` linkage; it is not a durable provenance proof across time or
  namespaces
- `hostRoot`, when present, MUST reflect the normalized remapped filesystem root
  actually used for `paths` and `sockets`
- `startedAt` MUST be serialized in UTC
- `durationMs` MUST be `>= 0`
- `results` MUST be stably ordered

If `summary` exists, it SHOULD use this shape:

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

Each entry in `results` MUST be an object with this shape:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `checkID` | string | yes | stable, deterministic ID |
| `domain` | string | yes | `services`, `sockets`, `paths`, `identity` |
| `status` | string | yes | see the states section |
| `reasonCode` | string or null | no | see the reason codes section |
| `expected` | JSON value | no | expected value used in evaluation |
| `observed` | JSON value | no | structured observed value |
| `evidence` | object | yes | evidence associated with the check |
| `durationMs` | integer | yes | check duration |
| `message` | string | yes | actionable message |

Rules:

- `durationMs` MUST be `>= 0`
- `message` MUST be non-empty
- `observed` SHOULD be structured when possible
- `expected` and `observed` MUST NOT depend on reparsing `evidence.raw`

## Evaluation states

Valid `status` values:

```text
PASS
FAIL
NOT_APPLICABLE
INSUFFICIENT_DATA
ERROR
```

Semantics:

- `PASS`: observed == expected with sufficient evidence
- `FAIL`: observed != expected with sufficient evidence
- `NOT_APPLICABLE`: the check does not apply in this context
- `INSUFFICIENT_DATA`: the collector ran, but the evidence is not sufficient
- `ERROR`: the collector could not execute correctly

## Reason codes

Valid values:

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

- `reasonCode` MAY be omitted or set to `null` when it adds no useful detail
- an explicit unsupported-environment or trust-boundary limitation MAY use
  `ERROR` with omitted `reasonCode` when no existing reason code is precise
  enough and the `message` states the limit directly
- `TIMEOUT` SHOULD accompany `ERROR`
- `PERMISSION_DENIED` SHOULD accompany `ERROR`
- `NOT_FOUND` MAY accompany `FAIL` when the observed absence is solid evidence
- `PARSE_ERROR` SHOULD accompany `INSUFFICIENT_DATA` or `ERROR`
- `NAMESPACE_ISOLATION` SHOULD accompany `ERROR`
- `INTERNAL_ERROR` MUST accompany `ERROR` for panic or broken invariant
- `PREREQUISITE_FAILED` MUST accompany results propagated by prerequisites

## Prerequisite propagation

When a check does not run because a prerequisite did not return `PASS`, the
serialized result MUST follow these rules:

| Prerequisite status | Dependent status | reasonCode |
|---|---|---|
| `FAIL` | `NOT_APPLICABLE` | `PREREQUISITE_FAILED` |
| `NOT_APPLICABLE` | `NOT_APPLICABLE` | `PREREQUISITE_FAILED` |
| `ERROR` | `INSUFFICIENT_DATA` | `PREREQUISITE_FAILED` |
| `INSUFFICIENT_DATA` | `INSUFFICIENT_DATA` | `PREREQUISITE_FAILED` |

The `message` MUST name the blocking `checkID`.

## Evidence object

`evidence` is audit support and human explanation. It is not the canonical
evaluation model.

Shape:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `source` | string | yes | observation source |
| `collector` | string | yes | collector that produced the evidence |
| `collectedAt` | string | yes | RFC3339 UTC |
| `command` | array[string] | no | executed argv, without a shell |
| `exitCode` | integer | no | command exit code, when applicable |
| `raw` | string | no | optional textual evidence |
| `redacted` | boolean | yes | whether redaction happened |
| `truncated` | boolean | yes | whether the visible `raw` was truncated |

Rules:

- `source` and `collector` MUST be non-empty
- `collectedAt` MUST be in UTC
- `command`, when present, MUST reflect the exact `argv`, never `sh -c`
- `exitCode` SHOULD exist when `command` exists
- `raw` MAY be omitted entirely
- when `raw` exists, `redacted` and `truncated` MUST reflect the visible
  content

### Evidence semantics

- `observed` is the structured truth used by evaluation
- `evidence.raw` is supporting text for human audit
- `evidence.raw` MAY be partial, truncated, or redacted
- a collector MAY interpret its primary source before serializing it as
  `evidence.raw`
- a machine-readable consumer MUST NOT depend on `evidence.raw`
- the reporter and external consumers MUST NOT reparse `evidence.raw` to decide
  `status`

## Error classes

`savk-report/v1` does not require an explicit `USER_ERROR`, `SYSTEM_ERROR`, or
`INTERNAL_ERROR` field.

Semantics:

- `USER_ERROR` describes failures before the engine and normally does not
  produce a report
- `SYSTEM_ERROR` is reflected in `status`, `reasonCode`, and `message`
- `INTERNAL_ERROR` is reflected as `status: ERROR` and
  `reasonCode: INTERNAL_ERROR`

## Determinism and ordering

Rules:

- timestamps MUST be serialized in UTC
- the order of `results` MUST be stable across equivalent runs
- the RECOMMENDED order for `results` is lexicographic by `checkID`
- the order of emitted fields SHOULD be stable even though JSON does not
  require it

A change in internal execution strategy MUST NOT alter report order if the
result set is the same.

## Exit codes

If the report includes `exitCode`, it MUST follow these rules:

```text
0  -> only PASS / NOT_APPLICABLE
1  -> at least one FAIL, with no ERROR / INSUFFICIENT_DATA
2  -> at least one ERROR or INSUFFICIENT_DATA
3  -> contract / CLI user error before the engine
```

Precedence:

```text
3 > 2 > 1 > 0
```

## Non-normative example

```json
{
  "schemaVersion": "savk-report/v1",
  "toolVersion": "0.1.5",
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
