# SAVK Contract Spec v1

Status: normative for `apiVersion: savk/v1`

## Scope

This document defines the input contract consumed by `savk check` and
`savk validate` in `v0.1`.

Objectives:

- define the YAML subset supported by the zero-dependency parser
- define the contract schema
- define validations, errors, and format limits
- define the supported target in `v0.1`

It does not define:

- the JSON report format
- internal engine details
- remediation or automatic contract generation

## Versioning

- `apiVersion` versions the input contract
- this document applies only to `apiVersion: savk/v1`
- incompatible contract changes require a new `apiVersion`
- `schemaVersion` belongs to the JSON report and is defined separately

## Supported target

`savk/v1` supports a single production target:

```text
linux-systemd
```

Rules:

- `metadata.target` MUST be `linux-systemd`
- an unknown or unsupported target is a `USER_ERROR`
- `NOT_APPLICABLE` does not replace an invalid target; it applies only to valid
  checks within a supported target

## Input format

The contract MUST be a UTF-8 text file.

### YAML subset

`savk/v1` does not support full YAML. The parser MUST accept only:

- a single YAML document
- indentation-based mappings
- simple lists with `-`
- the inline literal `[]` only for empty lists
- plain or quoted strings
- base-10 integers
- booleans `true` and `false`

Restrictions:

- indentation with spaces only
- tabs are invalid
- full-line comments are valid
- inline comments are not required by the spec
- duplicate keys are invalid
- unknown fields are invalid

Unsupported:

- anchors
- aliases
- merge keys
- tags
- general flow style (`{}` or `[a, b]`)
- multiline strings
- multiple documents
- implicit types outside `string`, `int`, and `bool`

Any feature outside this subset MUST fail with an explicit error.

## Root schema

The root document MUST be a mapping with these fields:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `apiVersion` | string | yes | exact `savk/v1` |
| `kind` | string | yes | exact `ApplianceContract` |
| `metadata` | mapping | yes | contract metadata |
| `services` | mapping | no | `services` domain |
| `sockets` | mapping | no | `sockets` domain |
| `paths` | mapping | no | `paths` domain |
| `identity` | mapping | no | `identity` domain |

Rules:

- `kind` MUST be `ApplianceContract`
- at least one of `services`, `sockets`, `paths`, or `identity` MUST be
  present and non-empty
- field order does not matter
- an unknown field at any level is a validation error

## Metadata

`metadata` MUST be a mapping with these fields:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `name` | string | yes | human contract identifier |
| `target` | string | yes | exact `linux-systemd` in `v0.1` |

Rules:

- `metadata.name` MUST be a non-empty string
- `metadata.target` MUST match the support matrix

## Domain schemas

Domains may be omitted. An omitted domain produces no checks.

### Services

`services` MUST be a mapping `service_name -> ServiceSpec`.

The service name identifies the observed unit. In `v0.1`, using the explicit
unit name is safest; using the full unit name is RECOMMENDED.

`ServiceSpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `state` | string | yes | `active`, `inactive`, `failed` |
| `run_as` | mapping | no | expected process identity |
| `restart` | string | no | `always`, `on-failure`, `no` |
| `capabilities` | list[string] | no | expected `AmbientCapabilities` |

`run_as`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `user` | string | yes | expected user name, not UID |
| `group` | string | no | expected group name, not GID |

Rules:

- `state` MUST be one of `active`, `inactive`, `failed`
- `restart` MUST be one of `always`, `on-failure`, `no`
- `capabilities` MUST be a list of non-empty strings
- capability names MUST use the canonical Linux form such as
  `CAP_NET_BIND_SERVICE`
- in `v0.1`, `services.<name>.capabilities` compares against the
  `AmbientCapabilities` property observed through `systemctl show`
- `run_as.user` and `run_as.group`, when present, compare by name
- `services` in `v0.1` is observer-local: `systemctl`, `/etc/passwd`, and
  `/etc/group` are interpreted on the same observer that runs SAVK
- if `systemctl` exposes numeric IDs or leaves `Group=` empty, SAVK can only
  normalize those values using local `/etc/passwd` and `/etc/group`
- if a numeric-looking `User=` or `Group=` exactly matches a local account name,
  SAVK MUST treat that exact name as the observed value before falling back to
  numeric UID/GID normalization
- if that evidence is insufficient, the result MUST degrade to
  `INSUFFICIENT_DATA`

### Sockets

`sockets` MUST be a mapping `absolute_path -> SocketSpec`.

The presence of an entry in `sockets` implies that the socket MUST exist.

`SocketSpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `owner` | string | no | expected owner name, not UID |
| `group` | string | no | expected group name, not GID |
| `mode` | string | no | quoted octal |

Rules:

- the mapping key MUST be an absolute path
- quoted mapping keys are valid and MUST be unquoted before path validation
- in this YAML subset, an absolute socket key containing `:` MUST be quoted
- `mode`, when present, MUST use quoted octal notation
- an empty `SocketSpec` is valid and means "only verify existence"
- `owner` and `group`, when present, compare by name
- `sockets` observes the node with `lstat`; it does not follow symlinks
- `owner` and `group` resolution MUST come from `/etc/passwd` and `/etc/group`
  visible to SAVK or to `--host-root`
- if no trustworthy mapping exists for an observed UID or GID, SAVK MUST
  degrade to `INSUFFICIENT_DATA`

### Paths

`paths` MUST be a mapping `absolute_path -> PathSpec`.

The presence of an entry in `paths` implies that the path MUST exist.

`PathSpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `owner` | string | no | expected owner name, not UID |
| `group` | string | no | expected group name, not GID |
| `mode` | string | no | quoted octal |
| `type` | string | no | `file`, `directory` |

Rules:

- the mapping key MUST be an absolute path
- quoted mapping keys are valid and MUST be unquoted before path validation
- in this YAML subset, an absolute path key containing `:` MUST be quoted
- `type`, when present, MUST be `file` or `directory`
- `mode`, when present, MUST match `^0[0-7]{3,4}$`
- an empty `PathSpec` is valid and means "only verify existence"
- `owner` and `group`, when present, compare by name
- `paths` observes the node with `lstat`; it does not follow symlinks
- `owner` and `group` resolution MUST come from `/etc/passwd` and `/etc/group`
  visible to SAVK or to `--host-root`
- if no trustworthy mapping exists for an observed UID or GID, SAVK MUST
  degrade to `INSUFFICIENT_DATA`

### Identity

`identity` MUST be a mapping `label -> RuntimeIdentitySpec`.

The key is a logical label for the observed runtime subject. It does not
necessarily represent a local user on the host.

In `v0.1`, `identity` models the effective identity of a running process.
The only selector supported in `v0.1` is a systemd service.

`RuntimeIdentitySpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `service` | string | yes | systemd unit used to resolve the runtime subject |
| `uid` | int | no | effective UID of the observed process |
| `gid` | int | no | effective GID of the observed process |
| `capabilities` | mapping | no | expectations by capability set |

`capabilities`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `effective` | list[string] | no | compares against `CapEff` |
| `permitted` | list[string] | no | compares against `CapPrm` |
| `inheritable` | list[string] | no | compares against `CapInh` |
| `bounding` | list[string] | no | compares against `CapBnd` |
| `ambient` | list[string] | no | compares against `CapAmb` |

Rules:

- `service` MUST be a non-empty string
- in `v0.1`, `service` is mandatory in every `RuntimeIdentitySpec`
- at least one of `uid`, `gid`, or `capabilities` MUST exist
- `uid` and `gid` MUST be non-negative integers
- `capabilities`, when present, MUST be a non-empty mapping
- each supported capability set MUST be a list of non-empty strings
- capability names MUST use the canonical Linux form such as
  `CAP_NET_BIND_SERVICE`
- runtime observation of `identity` in `v0.1` resolves as:
  `systemctl show <unit> --property=MainPID --property=ControlGroup`
  + reads of `/proc/<pid>/status` and `/proc/<pid>/cgroup`
- `identity` in `v0.1` is observer-local; SAVK does not support remapping or
  proving a runtime target distinct from the observer
- `identity` checks depend on `service.<unit>.state`
- if `identity.<label>.service` references a declared entry in `services`,
  `services.<unit>.state` MUST be `active`
- if the referenced unit is not declared in `services`, `savk check` MAY
  synthesize the prerequisite `service.<unit>.state`
- if `MainPID` can no longer be proven against the observed `ControlGroup`,
  SAVK MUST degrade the result to `INSUFFICIENT_DATA`
- if the observer-local context cannot prove the service-backed path, SAVK MUST
  fail closed with `ERROR/NAMESPACE_ISOLATION` or degrade to
  `INSUFFICIENT_DATA`

## Scalars and value rules

### Modes

`mode` is represented as a quoted octal string.

Valid examples:

```yaml
mode: "0640"
mode: "0750"
mode: "04755"
```

Invalid examples:

```yaml
mode: 640
mode: "640"
mode: "0x1ff"
```

### Paths

Paths in `paths` and `sockets`:

- MUST be absolute
- MUST NOT be empty
- MUST refer to the filesystem seen by SAVK or by `--host-root`
- `--host-root`, when used, remaps absolute paths under that root only for
  `paths` and `sockets`
- under `--host-root`, name-based `owner` and `group` resolution MUST also come
  from `<host-root>/etc/passwd` and `<host-root>/etc/group`
- `--host-root` DOES NOT apply to `services` or `identity` in `v0.1`; those
  domains remain observer-local only

### Strings

Structural strings such as `metadata.name`, users, groups, service names, and
capability names MUST be non-empty.

## Defaults

`savk/v1` avoids implicit defaults whenever possible.

Rules:

- omitting an optional field means "do not assert that property"
- the presence of a key in `paths` or `sockets` always asserts existence
- an omitted domain generates no checks
- a present but empty domain generates no checks and SHOULD be treated as a
  suspicious contract

## Check ID convention

Checks derived from the contract MUST have predictable IDs.

Initial convention:

```text
service.<name>.state
service.<name>.restart
path.<path>.exists
path.<path>.type
path.<path>.mode
path.<path>.owner
path.<path>.group
socket.<path>.exists
socket.<path>.owner
socket.<path>.group
socket.<path>.mode
identity.<label>.uid
identity.<label>.gid
identity.<label>.capabilities.effective
identity.<label>.capabilities.permitted
identity.<label>.capabilities.inheritable
identity.<label>.capabilities.bounding
identity.<label>.capabilities.ambient
```

Notes:

- IDs are stable and deterministic
- external consumers MUST treat them as opaque strings
- result serialization order depends on `CheckID`
- SAVK may also emit reserved preflight IDs:
  `path.__preflight__.namespace`, `socket.__preflight__.namespace`,
  `service.__preflight__.namespace`

## Validation model

Validation happens in three layers:

1. Syntax
2. Structure
3. Semantics

### Syntax errors

YAML subset errors:

- invalid indentation
- tabs
- flow style
- multiline strings
- duplicate keys

### Structure errors

Schema errors:

- unknown field
- invalid type
- invalid `kind`
- invalid `apiVersion`
- incomplete `metadata`

### Semantic errors

Content errors:

- unsupported target
- relative path
- invalid enum
- empty contract
- cycle in the prerequisite graph derived from the contract

Contract errors MUST abort before the engine and exit with code `3`.

## Error message guidance

Parse and validation errors MUST be actionable.

Examples:

```text
unknown field "onwer" at paths./etc/myapp/config.yaml
  hint: did you mean "owner"?

invalid restart policy "on_failure" at services.sensor-agent
  valid values: always, on-failure, no

unsupported target "linux-openrc"
  supported targets: linux-systemd

relative path "var/log/myapp" at paths
  hint: use an absolute path like "/var/log/myapp"
```

## Non-normative example

```yaml
apiVersion: savk/v1
kind: ApplianceContract
metadata:
  name: sensor-agent-prod
  target: linux-systemd

services:
  sensor-agent.service:
    state: active
    run_as:
      user: sensor
      group: sensor
    restart: on-failure
    capabilities: []

paths:
  /etc/sensor-agent/config.yaml:
    owner: root
    group: sensor
    mode: "0640"
    type: file
  /var/log/sensor-agent:
    owner: sensor
    mode: "0750"
    type: directory

sockets:
  /run/sensor-agent.sock:
    owner: sensor
    group: sensor
    mode: "0660"

identity:
  sensor_runtime:
    service: sensor-agent.service
    uid: 1001
    gid: 1001
    capabilities:
      effective: []
      permitted: []
      ambient: []
```
