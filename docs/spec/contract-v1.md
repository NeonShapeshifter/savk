# SAVK Contract Spec v1

Status: normative for `apiVersion: savk/v1`

## Scope

Este documento define el contrato de entrada que consume `savk check`
y `savk validate` en `v0.1`.

Objetivos:

- fijar el subset YAML soportado por el parser zero-deps
- fijar el schema del contrato
- fijar validaciones, errores y límites del formato
- fijar el target soportado en `v0.1`

No define:

- el formato del reporte JSON
- detalles internos del engine
- remediación o generación automática de contratos

## Versioning

- `apiVersion` versiona el contrato de entrada
- este documento aplica solo a `apiVersion: savk/v1`
- cambios incompatibles en el contrato requieren una nueva `apiVersion`
- `schemaVersion` pertenece al reporte JSON y se define aparte

## Supported target

`savk/v1` soporta un único target en producción:

```text
linux-systemd
```

Reglas:

- `metadata.target` DEBE ser `linux-systemd`
- un target desconocido o no soportado es `USER_ERROR`
- `NOT_APPLICABLE` no reemplaza un target inválido; solo aplica a checks
  válidos dentro de un target soportado

## Input format

El contrato DEBE ser un archivo de texto UTF-8.

### YAML subset

`savk/v1` no soporta YAML completo. El parser solo DEBE aceptar:

- un único documento YAML
- mappings por indentación
- listas simples con `-`
- el literal inline `[]` solo para listas vacías
- strings planas o quoted
- integers base 10
- booleans `true` y `false`

Restricciones:

- indentación solo con espacios
- tabs son inválidos
- comentarios de línea completa son válidos
- comentarios inline no son requeridos por la spec
- keys duplicadas son inválidas
- fields desconocidas son inválidas

No soportado:

- anchors
- aliases
- merge keys
- tags
- flow style general (`{}` o `[a, b]`)
- multiline strings
- múltiples documentos
- tipos implícitos fuera de `string`, `int`, `bool`

Una feature fuera de este subset DEBE fallar con error explícito.

## Root schema

El documento raíz DEBE ser un mapping con estas fields:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `apiVersion` | string | yes | exacto `savk/v1` |
| `kind` | string | yes | exacto `ApplianceContract` |
| `metadata` | mapping | yes | metadatos del contrato |
| `services` | mapping | no | dominio `services` |
| `sockets` | mapping | no | dominio `sockets` |
| `paths` | mapping | no | dominio `paths` |
| `identity` | mapping | no | dominio `identity` |

Reglas:

- `kind` DEBE ser `ApplianceContract`
- al menos uno de `services`, `sockets`, `paths` o `identity` DEBE estar
  presente y no vacío
- el orden de las fields no importa
- una field desconocida en cualquier nivel es error de validación

## Metadata

`metadata` DEBE ser un mapping con estas fields:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `name` | string | yes | identificador humano del contrato |
| `target` | string | yes | exacto `linux-systemd` en `v0.1` |

Reglas:

- `metadata.name` DEBE ser un string no vacío
- `metadata.target` DEBE coincidir con la support matrix

## Domain schemas

Las domains pueden omitirse. Una domain omitida no genera checks.

### Services

`services` DEBE ser un mapping `service_name -> ServiceSpec`.

El nombre del servicio identifica la unidad observada. En `v0.1`,
usar el nombre explícito de la unidad es lo más seguro; usar el unit
name completo es RECOMMENDED.

`ServiceSpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `state` | string | yes | `active`, `inactive`, `failed` |
| `run_as` | mapping | no | identidad esperada del proceso |
| `restart` | string | no | `always`, `on-failure`, `no` |
| `capabilities` | list[string] | no | `AmbientCapabilities` esperadas |

`run_as`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `user` | string | yes | nombre de usuario esperado, no UID |
| `group` | string | no | nombre de grupo esperado, no GID |

Reglas:

- `state` DEBE ser uno de `active`, `inactive`, `failed`
- `restart` DEBE ser uno de `always`, `on-failure`, `no`
- `capabilities` DEBE ser una lista de strings no vacíos
- los nombres de capability DEBEN usar la forma canónica Linux
  como `CAP_NET_BIND_SERVICE`
- en `v0.1`, `services.<name>.capabilities` compara contra la propiedad
  `AmbientCapabilities` observada vía `systemctl show`
- `run_as.user` y `run_as.group`, si existen, se comparan por nombre

### Sockets

`sockets` DEBE ser un mapping `absolute_path -> SocketSpec`.

La presencia de una entrada en `sockets` implica que el socket DEBE existir.

`SocketSpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `owner` | string | no | nombre de owner esperado, no UID |
| `group` | string | no | nombre de group esperado, no GID |
| `mode` | string | no | octal quoted |

Reglas:

- la key del mapping DEBE ser una ruta absoluta
- `mode`, si existe, DEBE usar notación octal quoted
- un `SocketSpec` vacío es válido y significa “solo verificar existencia”
- `owner` y `group`, si existen, se comparan por nombre
- `sockets` observa el nodo con `lstat`; no sigue symlinks

### Paths

`paths` DEBE ser un mapping `absolute_path -> PathSpec`.

La presencia de una entrada en `paths` implica que la ruta DEBE existir.

`PathSpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `owner` | string | no | nombre de owner esperado, no UID |
| `group` | string | no | nombre de group esperado, no GID |
| `mode` | string | no | octal quoted |
| `type` | string | no | `file`, `directory` |

Reglas:

- la key del mapping DEBE ser una ruta absoluta
- `type`, si existe, DEBE ser `file` o `directory`
- `mode`, si existe, DEBE cumplir `^0[0-7]{3,4}$`
- un `PathSpec` vacío es válido y significa “solo verificar existencia”
- `owner` y `group`, si existen, se comparan por nombre
- `paths` observa el nodo con `lstat`; no sigue symlinks

### Identity

`identity` DEBE ser un mapping `label -> RuntimeIdentitySpec`.

La key es un label lógico del sujeto runtime observado. No representa
necesariamente un usuario local del host.

En `v0.1`, `identity` modela la identidad efectiva de un proceso en ejecución.
El único selector soportado en `v0.1` es un servicio systemd.

`RuntimeIdentitySpec`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `service` | string | yes | unidad systemd usada para resolver el sujeto runtime |
| `uid` | int | no | UID efectiva del proceso observado |
| `gid` | int | no | GID efectiva del proceso observado |
| `capabilities` | mapping | no | expectations por capability set |

`capabilities`:

| Field | Type | Required | Notes |
|---|---|---:|---|
| `effective` | list[string] | no | compara contra `CapEff` |
| `permitted` | list[string] | no | compara contra `CapPrm` |
| `inheritable` | list[string] | no | compara contra `CapInh` |
| `bounding` | list[string] | no | compara contra `CapBnd` |
| `ambient` | list[string] | no | compara contra `CapAmb` |

Reglas:

- `service` DEBE ser un string no vacío
- en `v0.1`, `service` es obligatorio en cada `RuntimeIdentitySpec`
- al menos una de `uid`, `gid` o `capabilities` DEBE existir
- `uid` y `gid` DEBEN ser enteros no negativos
- `capabilities`, si existe, DEBE ser un mapping no vacío
- cada capability set soportado DEBE ser una lista de strings no vacíos
- los nombres de capability DEBEN usar la forma canónica Linux
  como `CAP_NET_BIND_SERVICE`
- la observación runtime de `identity` en `v0.1` se resuelve como:
  `systemctl show <unit> --property=MainPID --property=ControlGroup`
  + lectura de `/proc/<pid>/status` y `/proc/<pid>/cgroup`
- los checks de `identity` dependen de `service.<unit>.state`
- si `identity.<label>.service` referencia una entrada declarada en `services`,
  `services.<unit>.state` DEBE ser `active`
- si la unidad referenciada no está declarada en `services`, `savk check`
  PUEDE sintetizar el prerequisito `service.<unit>.state`
- si `MainPID` ya no puede probarse contra el `ControlGroup` observado,
  SAVK DEBE degradar el resultado a `INSUFFICIENT_DATA`

## Scalars and value rules

### Modes

`mode` se representa como string quoted en octal.

Ejemplos válidos:

```yaml
mode: "0640"
mode: "0750"
mode: "04755"
```

Ejemplos inválidos:

```yaml
mode: 640
mode: "640"
mode: "0x1ff"
```

### Paths

Rutas en `paths` y `sockets`:

- DEBEN ser absolutas
- NO DEBEN estar vacías
- DEBEN referirse al filesystem visto por SAVK o por `--host-root`
- `--host-root`, si se usa, remapea rutas absolutas bajo ese root solo para
  `paths` y `sockets`
- `--host-root` NO aplica a `services` ni `identity` en `v0.1`

### Strings

Strings estructurales como `metadata.name`, users, groups, service names
y capability names DEBEN ser no vacíos.

## Defaults

`savk/v1` evita defaults implícitos siempre que sea posible.

Reglas:

- omitir una field opcional significa “no afirmar esa propiedad”
- la presencia de una key en `paths` o `sockets` siempre afirma existencia
- una domain omitida no genera checks
- una domain presente pero vacía no genera checks y SHOULD tratarse como
  contrato sospechoso

## Check ID convention

Los checks derivados del contrato DEBEN tener IDs predecibles.

Convención inicial:

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

Notas:

- los IDs son estables y deterministas
- los consumers externos DEBEN tratarlos como strings opacos
- el orden de serialización de resultados depende del `CheckID`
- SAVK también puede emitir IDs reservados de preflight:
  `path.__preflight__.namespace`, `socket.__preflight__.namespace`,
  `service.__preflight__.namespace`

## Validation model

La validación ocurre en tres capas:

1. Syntax
2. Structure
3. Semantics

### Syntax errors

Errores del subset YAML:

- indentación inválida
- tabs
- flow style
- multiline strings
- duplicate keys

### Structure errors

Errores del schema:

- field desconocida
- tipo inválido
- `kind` inválido
- `apiVersion` inválida
- `metadata` incompleto

### Semantic errors

Errores del contenido:

- target no soportado
- ruta relativa
- enum inválido
- contract vacío
- ciclo en el grafo de prerequisitos derivado del contrato

Los errores de contrato DEBEN abortar antes del engine y salir con exit code `3`.

## Error message guidance

Los errores de parseo y validación DEBEN ser accionables.

Ejemplos:

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
