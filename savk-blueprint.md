# SAVK — Security Appliance Verification Kit
### Blueprint v1.0 — Internal Design Notes

---

## Concepto

**Contract-driven, evidence-backed deployment verifier for Linux security appliances.**

No audita contra best practices genéricas.  
Verifica que tu sistema está desplegado exactamente como tú lo diseñaste.  
Produce evidencia auditable. Corre desde dentro del appliance.

> Lynis audita tu sistema contra best practices generales.  
> SAVK verifica tu sistema contra tu propio diseño.

---

## Principio arquitectural

> SAVK no confía en nada — ni en el contrato, ni en el sistema, ni en sí mismo.

```
Regla 1: Un contrato ambiguo no se evalúa.
Regla 2: Un collector no debe bloquear indefinidamente el resultado del run.
Regla 3: Los bordes del sistema deben poder simularse.
Regla 4: Los errores deben ser accionables, no solo presentes.
```

---

## Lenguaje y distribución

- **Go** — core stdlib-only; los artifacts oficiales se construyen como single binary estático
- `cp savk /usr/local/bin/savk` — instalación mínima
- Sin pip, sin virtualenv, sin runtime de terceros
- Shell-out a herramientas del host solo cuando el host es la fuente de verdad y sin pasar por shell

---

## Modelo conceptual central

```
Contract → Observation → Evaluation → Report
```

| Capa | Qué hace |
|------|----------|
| **Contract** | Estado esperado declarado por el operador (YAML) |
| **Observation** | Estado real del sistema, con fuente y timestamp |
| **Evaluation** | Comparación determinista: 5 estados posibles |
| **Report** | Salida human-readable y JSON estable; SARIF queda fuera de `v0.1` |

---

## Los 5 estados de evaluación

```
PASS              → observado == esperado, evidencia sólida
FAIL              → observado != esperado, evidencia sólida
NOT_APPLICABLE    → check no tiene sentido en este contexto
INSUFFICIENT_DATA → collector corrió pero evidencia es ambigua
ERROR             → collector no pudo correr
```

### ReasonCodes

```
TIMEOUT             → collector superó el deadline de context
PERMISSION_DENIED   → acceso insuficiente
NOT_FOUND           → recurso esperado no existe
PARSE_ERROR         → output del sistema no fue interpretable
NAMESPACE_ISOLATION → el proceso no observa el host que el contrato describe
INTERNAL_ERROR      → bug o pánico recuperado dentro del verifier
PREREQUISITE_FAILED → un check requerido no pasó primero
```

---

## Decisiones de arquitectura interna

### 1. Parseo paranoico del contrato

KnownFields only. Errores accionables con hints. Exit 3 inmediato.

SAVK v0.1 no promete YAML completo. Soporta un subset determinista:
maps, strings, ints, bools, listas simples y `[]` para listas vacías.
No anchors, no aliases, no merge keys, no tags, no multiline strings.
Cualquier feature fuera del subset falla con error explícito. Zero-deps
sin fingir compatibilidad.

```
unknown field "onwer" at services.sensor-agent.service
  hint: did you mean "owner"?

invalid restart policy "on_failure" at services.sensor-agent.service
  valid values: always, on-failure, no

unsupported apiVersion "savk/v2"
  supported: savk/v1
```

### 2. Contextos de ejecución implacables

Cada collector recibe `context.Context` con timeout estricto.  
Timeout → `ERROR` con `ReasonCode: TIMEOUT`. El run deja de esperar por ese
check aunque el collector ignore cancelación.

```go
ctx, cancel := context.WithTimeout(rootCtx, 2*time.Second)
defer cancel()
result := collector.Run(ctx)
```

### 3. Abstracción selectiva para testabilidad

Solo se abstrae lo que toca el sistema real.

```go
type ProcessReader interface {
    ReadStatus(ctx context.Context, pid int) (ProcessStatus, error)
}

type PathChecker interface {
    Stat(name string) (fs.FileInfo, error)
    Lstat(name string) (fs.FileInfo, error)
}

type CommandRunner interface {
    Run(ctx context.Context, argv []string) (CommandResult, error)
}
```

No DI ceremonial. `os.Lstat` directo donde no hace falta simularlo.  
Tests unitarios sin Docker ni root, más integración opt-in sobre host
`linux-systemd` real.

### 4. Check interface DAG-ready

Engine v0.1 secuencial. La interfaz permite DAG en v0.2 sin reescribir checks.

```go
type Check interface {
    ID()            string
    Domain()        string
    Prerequisites() []string  // IDs de checks que deben PASS primero
    Run(ctx context.Context) CheckResult
}
```

Dependencias naturales:
```
service.state  → prerequisito de identity.uid
path.exists    → prerequisito de path.mode
socket.exists  → prerequisito de socket.owner
```

### 5. Propagación explícita de prerequisitos

Un check solo corre si todos sus prerequisitos hicieron `PASS`.

Reglas de propagación:
```
prereq PASS                          → dependent puede correr
prereq FAIL / NOT_APPLICABLE         → dependent = NOT_APPLICABLE
prereq ERROR / INSUFFICIENT_DATA     → dependent = INSUFFICIENT_DATA
dependent ReasonCode                 → PREREQUISITE_FAILED
```

El mensaje debe nombrar el check bloqueante. Ciclos en el grafo de
prerequisitos son error de contrato y abortan con exit 3.

### 6. Borde de ejecución para shell-out

Los collectors pueden invocar herramientas del host solo cuando no haya
una interfaz razonable en stdlib y esa herramienta sea parte natural del
sistema observado.

Reglas:
```
- Nunca "sh -c" ni parsing vía shell
- Siempre exec.CommandContext con timeout del collector
- argv, exit code cuando exista y output relevante entran en Evidence
- CommandRunner abstraído para tests
```

---

## El Evidence model (canónico desde día uno)

```go
type Evidence struct {
    Source      string
    Collector   string
    CollectedAt time.Time
    Command     []string
    ExitCode    int
    Raw         string
    Redacted    bool
    Truncated   bool
}

type CheckResult struct {
    CheckID    string
    Domain     string
    Status     EvalStatus
    ReasonCode ReasonCode
    Expected   any
    Observed   any
    Evidence   Evidence
    DurationMs int64   // observabilidad interna
    Message    string  // accionable, no solo descriptivo
}
```

`DurationMs` por check — detecta collectors lentos, refuerza que  
SAVK no puede convertirse en un problema de disponibilidad.

Taxonomía de fallos:
```
USER_ERROR     → contrato, flags o input inválido; engine no arranca
SYSTEM_ERROR   → timeout, permiso, recurso ausente, namespace aislado
INTERNAL_ERROR → panic o invariante rota; se recupera y se reporta
```

`INTERNAL_ERROR` nunca se disfraza como `FAIL`.

---

## Product boundaries v0.1

### Non-goals

SAVK v0.1 explícitamente no:

- remedia ni modifica el sistema
- genera contratos desde el host
- audita contra benchmarks genéricos
- ejecuta verificación remota por SSH
- intenta autodetectar la intención del operador
- soporta targets fuera de Linux en producción

### Compatibility policy

- `apiVersion` versiona el contrato de entrada
- `schemaVersion` versiona el reporte JSON
- cambios incompatibles en contrato requieren nuevo `apiVersion`
- cambios incompatibles en JSON requieren nuevo `schemaVersion`
- nuevas fields opcionales dentro de `savk/v1` son válidas solo si no rompen parseo estricto del contrato existente

### JSON report contract v1

El reporte JSON de v0.1 debe ser estable y machine-readable desde día uno.

Campos top-level mínimos:
```json
{
  "schemaVersion": "savk-report/v1",
  "toolVersion": "0.1.0",
  "contractVersion": "savk/v1",
  "contractHash": "sha256:...",
  "runID": "...",
  "target": "linux-systemd",
  "host": "...",
  "startedAt": "2026-04-12T16:00:00Z",
  "durationMs": 1234,
  "results": []
}
```

### Evidence safety

- `Evidence.Raw` tiene límite de tamaño y puede truncarse
- secretos y tokens se redactan por defecto antes de persistirse
- raw completo solo se expone bajo flag explícito
- truncado y redacción deben quedar visibles en el reporte

### Determinismo e IDs

- timestamps en UTC
- orden de checks estable y reproducible
- mismo contrato + mismo host + mismo estado => mismo orden de resultados
- `CheckID` debe ser derivable y predecible

Convención inicial:
```
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

### Support matrix v0.1

Target soportado en producción:
```
linux-systemd
```

Targets fuera de esa matriz:
```
- error de contrato si el target no existe o no es soportado
- NOT_APPLICABLE solo para checks válidos dentro de un target soportado
```

### Panic policy

Si un collector paniquea:
```
- el engine recupera el panic
- ese check termina en ERROR
- ReasonCode = INTERNAL_ERROR
- el resto del run continúa
```

### Identity semantics v0.1

`identity` deja de modelar cuentas locales del host y pasa a modelar la
identidad runtime efectiva de un proceso observado.

Reglas:
```
- la key de identity es un label lógico, no un username
- el selector requerido en v0.1 es service: <unit>
- uid/gid son del proceso real observado
- capabilities se comparan por capability set, no como lista plana
- la observación sale de MainPID + ControlGroup + /proc/<pid>/{status,cgroup}
```

---

## Contract schema v1

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

sockets:
  /run/sensor-agent.sock:
    owner: sensor
    group: sensor
    mode: "0660"

paths:
  /etc/sensor-agent/config.yaml:
    owner: root
    group: sensor
    mode: "0640"
  /var/log/sensor-agent:
    owner: sensor
    mode: "0750"
    type: directory

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

---

## Estructura del repo actual

```
savk/
├── cmd/
│   └── savk/
│       ├── examples_test.go
│       ├── main.go
│       ├── main_test.go
│       └── systemd_integration_test.go
├── docs/
│   ├── integration.md
│   ├── public-repo-checklist.md
│   ├── release-checklist.md
│   ├── release.md
│   ├── roadmap.md
│   ├── spec/
│   │   ├── contract-v1.md
│   │   └── report-v1.md
├── examples/
│   ├── full-sensor-agent.yaml
│   ├── identity-runtime.yaml
│   ├── paths-only.yaml
│   ├── services-only.yaml
│   └── sockets-only.yaml
├── internal/
│   ├── capabilities/
│   │   └── capabilities.go
│   ├── collectors/
│   │   ├── filesystem.go
│   │   ├── identity.go
│   │   ├── identity_test.go
│   │   ├── interfaces.go
│   │   ├── paths.go
│   │   ├── paths_test.go
│   │   ├── service_preflight.go
│   │   ├── services.go
│   │   ├── services_test.go
│   │   ├── sockets.go
│   │   └── sockets_test.go
│   ├── contract/
│   │   ├── parser.go
│   │   ├── parser_test.go
│   │   └── schema.go
│   ├── evidence/
│   │   └── evidence.go
│   ├── engine/
│   │   ├── check.go
│   │   ├── engine.go
│   │   ├── engine_test.go
│   │   └── evaluator.go
│   └── reporters/
│       ├── evidence_sanitize.go
│       ├── json.go
│       ├── json_test.go
│       ├── table.go
│       └── table_test.go
├── testdata/
│   ├── fixtures/
│   └── golden/
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── Makefile
├── README.md
├── SECURITY.md
├── go.mod
└── .gitignore
```

---

## CLI

```bash
savk check    --contract appliance.yaml
savk check    --contract appliance.yaml --domain sockets,identity
savk check    --contract appliance.yaml --format json
savk check    --contract appliance.yaml --collector-timeout 5s
savk check    --contract appliance.yaml --domain paths --host-root /host
savk validate --contract appliance.yaml
savk version
```

## Exit codes

```
0  → solo PASS / NOT_APPLICABLE
1  → al menos un FAIL, sin ERROR / INSUFFICIENT_DATA
2  → al menos un ERROR o INSUFFICIENT_DATA
3  → contract / CLI user error antes del engine
```

Precedencia cerrada: `3 > 2 > 1 > 0`

---

## Estado del diseño

Este blueprint ya no es roadmap público ni pitch de release.

- para estado de ejecución real: [docs/roadmap.md](docs/roadmap.md)
- para contrato normativo: [docs/spec/contract-v1.md](docs/spec/contract-v1.md)
- para reporte normativo: [docs/spec/report-v1.md](docs/spec/report-v1.md)

---

## Reliability & Operations (notas de diseño)

**Namespace awareness** — En `v0.1` la detección es best-effort: preflight
sobre `/proc/1/comm` y errores típicos de `systemctl`. Si SAVK corre dentro
de un namespace aislado (contenedor privilegiado, DaemonSet) y el contrato
pide verificar el host, puede degradar a `NAMESPACE_ISOLATION`. En `v0.1`,
`--host-root=/host` remapea solo lecturas filesystem-backed (`paths`,
`sockets`); no aplica a `services` ni `identity`.

**Prerrequisitos** — Si un check depende de otro que no hizo `PASS`,
no corre. `FAIL/NOT_APPLICABLE` propaga `NOT_APPLICABLE`; `ERROR` e
`INSUFFICIENT_DATA` propagan `INSUFFICIENT_DATA`. Siempre con
`ReasonCode: PREREQUISITE_FAILED`.

**Execution boundary** — Shell-out permitido solo vía `exec.CommandContext`,
sin shell intermedio, con timeout estricto y evidencia suficiente del comando
ejecutado para auditar el resultado. Si un collector explota por panic, el
engine lo recupera y lo reporta como `ERROR` con clasificación `INTERNAL_ERROR`.

**Report stability** — El JSON es contrato público. `schemaVersion` debe
versionarse por separado del contrato de entrada y el orden de resultados
debe ser estable entre ejecuciones equivalentes.

**Evidence safety** — Evidence cruda debe truncarse, redactarse y marcarse
explícitamente cuando no sea completa. SAVK produce evidencia útil, no dumps
irresponsables del host.

**E2E distro matrix** — Harness reproducible que compila SAVK, levanta
targets reales o equivalentes por distro (Ubuntu 22.04, Debian 12, Alpine),
inyecta estado controlado y verifica ReasonCodes esperados por distro.
Alpine + OpenRC → `NOT_APPLICABLE` en checks de systemd.

**Execution budget** — `DurationMs` por check en CheckResult.
Duración total en el reporte JSON. SAVK no puede ser un problema
de disponibilidad en el appliance que audita.

---

*Blueprint v1.0 — design notes snapshot — April 2026*
