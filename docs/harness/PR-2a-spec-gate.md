# PR-2a — El gate del contrato

**Continúa desde:** PR-1 mergeado en `main`.
**Ubicación recomendada:** `docs/harness/PR-2a.md`.

> El ruleset de Spectral está **ejecutado y verificado** con Spectral 6.16.3:
> las doce reglas propias disparan sobre una spec defectuosa y ninguna sobre
> una correcta. Lo relativo a `oasdiff` está diseñado pero no ejecutado; lo
> señalo donde corresponde.

---

## Índice

1. [Por qué PR-2 se parte en cuatro](#1-por-qué-pr-2-se-parte-en-cuatro)
2. [Qué contiene este paso](#2-qué-contiene-este-paso)
3. [El ruleset: `api/.spectral.yaml`](#3-el-ruleset-apispectralyaml)
4. [Explicación regla por regla](#4-explicación-regla-por-regla)
5. [Fixtures y autotest del ruleset](#5-fixtures-y-autotest-del-ruleset)
6. [Cablear `make spec-lint`](#6-cablear-make-spec-lint)
7. [`oasdiff` y el gate de cambios incompatibles](#7-oasdiff-y-el-gate-de-cambios-incompatibles)
8. [El workflow `spec-gate`](#8-el-workflow-spec-gate)
9. [Verificar, commitear y abrir el PR](#9-verificar-commitear-y-abrir-el-pr)
10. [Qué falta](#10-qué-falta)

---

## 1. Por qué PR-2 se parte en cuatro

PR-2 tal como lo planteamos reúne unas veinte herramientas y siete workflows.
Como un único pull request tiene dos problemas serios:

- **Diagnóstico imposible.** Si el PR se pone en rojo, no sabrás cuál de veinte
  configuraciones falla. Y una configuración de CI no se depura en local: cada
  intento es un push y una espera.
- **Revisión imposible.** Es exactamente el tipo de PR que se aprueba sin leer,
  y estamos construyendo un proyecto cuya tesis es que los gates de revisión
  importan.

El troceado:

| Paso | Contenido | Verificable por sí solo |
|---|---|---|
| **2a** | Contrato: Spectral, oasdiff, `spec-gate` | Sí — este documento |
| 2b | Generación: oapi-codegen, sqlc, goose, openapi-typescript | Sí |
| 2c | Calidad: golangci-lint con depguard, eslint, `code-gate` | Sí |
| 2d | Seguridad y operación: CodeQL, Semgrep, gitleaks, ZAP, `agent-review`, métricas, Renovate | Sí |

Cada uno es un PR real que pasa por el mismo flujo. Y hay un orden implícito:
2a valida el contrato, 2b lo convierte en código, 2c comprueba ese código, 2d lo
somete a seguridad. El mismo orden del ciclo.

---

## 2. Qué contiene este paso

Este es el gate más importante de todo el proyecto, y conviene entender por qué.

`AGENTS.md` dice que la spec es la fuente de verdad. Hasta ahora eso es una
declaración: nada impide que `contract-designer` produzca una operación sin
modelo de autorización, con un identificador secuencial y sin paginación. Este
paso convierte cada regla que hemos escrito en prosa a lo largo de cinco fases en
**un exit code**.

Y tiene un efecto que va mucho más allá del contrato: **una operación que no
declara su autorización nunca llegará a implementarse**, porque el PR de contrato
no se mergea. Ningún revisor tiene que acordarse. Eso es desplazar a la izquierda
de verdad. **[SDL: requisitos de seguridad · OWASP: ASVS V1, V4, V5]**

```bash
git checkout main && git pull
git checkout -b chore/pr-2a-spec-gate
```

Necesitas Spectral disponible. En el devcontainer y en CI usaremos `npx`, de modo
que no hay nada que instalar globalmente:

```bash
npx --yes @stoplight/spectral-cli --version
```

---

## 3. El ruleset: `api/.spectral.yaml`

```yaml
extends: ["spectral:oas"]

rules:
  # Falso positivo conocido: con openIdConnect los scopes no se enumeran en
  # el securityScheme, así que esta regla no puede resolverlos.
  oas3-operation-security-defined: off

  # --- Trazabilidad (ADR-0003, PR-1) ---
  ta-operation-source-issue:
    description: Toda operación declara el issue de requisito que la motivó.
    message: "{{error}}"
    severity: error
    given: "$.paths[*][get,put,post,delete,patch]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          required: ["x-source-issue"]
          properties:
            x-source-issue: { type: integer, minimum: 1 }

  # --- Autorización declarada en el contrato (ASVS V4) ---
  ta-operation-security-declared:
    description: >
      Toda operación declara security. Un array vacío significa endpoint
      público de forma explícita; omitirlo no es una opción.
    severity: error
    given: "$.paths[*][get,put,post,delete,patch]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          required: ["security"]

  ta-public-operation-is-visible:
    description: Endpoint público declarado. Requiere justificación en el plan.
    severity: warn
    given: "$.paths[*][get,put,post,delete,patch].security"
    then:
      function: length
      functionOptions:
        min: 1

  ta-operation-required-scope:
    description: Toda operación no pública declara x-required-scope.
    severity: error
    given: "$.paths[*][get,put,post,delete,patch]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          if:
            properties:
              security: { type: array, minItems: 1 }
            required: ["security"]
          then:
            required: ["x-required-scope"]
            properties:
              x-required-scope:
                type: string
                pattern: "^[a-z]+:(read|write|admin)$"

  ta-owner-check-required:
    description: >
      Una operación con parámetro de path opera sobre una instancia concreta
      y debe declarar x-owner-check.
    severity: error
    given: "$.paths[*][get,put,post,delete,patch]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          if:
            required: ["parameters"]
            properties:
              parameters:
                type: array
                contains:
                  type: object
                  required: ["in"]
                  properties:
                    in: { const: path }
          then:
            required: ["x-owner-check"]

  ta-owner-check-shape:
    description: x-owner-check usa el enum cerrado de reglas de autorización.
    severity: error
    given: "$.paths[*][get,put,post,delete,patch].x-owner-check"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          required: ["resource", "param", "rule"]
          additionalProperties: false
          properties:
            resource: { enum: ["slot", "project", "assignment"] }
            param: { type: string, minLength: 1 }
            rule:
              enum:
                - owner-only
                - owner-or-project-lead
                - team-member
                - project-lead-only

  ta-owner-check-responses:
    description: >
      Una operación con x-owner-check debe declarar 403 y 404. Confundirlos
      filtra la existencia de recursos ajenos.
    severity: error
    given: "$.paths[*][get,put,post,delete,patch]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          if:
            required: ["x-owner-check"]
          then:
            required: ["responses"]
            properties:
              responses:
                type: object
                required: ["403", "404"]

  # --- Validación de entrada (ASVS V5) ---
  ta-path-id-is-uuid:
    description: Los identificadores de path son UUID, nunca secuenciales.
    severity: error
    given: "$..parameters[?(@.in == 'path')]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          if:
            required: ["name"]
            properties:
              name: { pattern: "Id$" }
          then:
            required: ["schema"]
            properties:
              schema:
                type: object
                required: ["format"]
                properties:
                  format: { const: uuid }

  ta-string-has-maxlength:
    description: Todo campo de texto libre declara maxLength.
    severity: error
    given: "$.components.schemas..properties[*]"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          if:
            allOf:
              - required: ["type"]
                properties:
                  type: { const: string }
              - not: { required: ["enum"] }
              - not: { required: ["format"] }
          then:
            required: ["maxLength"]

  ta-collection-has-pagination:
    description: Una colección sin paginación es un vector de agotamiento.
    severity: warn
    given: "$.paths[*].get"
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          if:
            not:
              allOf:
                - required: ["parameters"]
                - properties:
                    parameters:
                      type: array
                      contains:
                        type: object
                        required: ["in"]
                        properties:
                          in: { const: path }
          then:
            required: ["parameters"]
            properties:
              parameters:
                type: array
                contains:
                  type: object
                  required: ["name"]
                  properties:
                    name: { const: limit }

  # --- Versionado (ADR-0003) ---
  ta-info-version-semver:
    description: info.version sigue versionado semántico.
    severity: error
    given: "$.info.version"
    then:
      function: pattern
      functionOptions:
        match: "^\\d+\\.\\d+\\.\\d+$"

  ta-servers-v1-prefix:
    description: Las rutas van bajo el prefijo de versión mayor.
    severity: error
    given: "$.servers[*].url"
    then:
      function: pattern
      functionOptions:
        match: "/v\\d+$"
```

### Dos decisiones estructurales

**Prefijo `ta-` en todas las reglas propias.** Las distingue de las de
`spectral:oas` de un vistazo en la salida, y permite que el autotest las cuente
con un `grep`. Cuando dentro de un año alguien vea `ta-owner-check-shape` en un
log, sabrá que es una regla nuestra y dónde buscarla.

**Solo se usan funciones nativas de Spectral**, ninguna función JavaScript
personalizada. Todo se expresa con `schema`, `pattern` y `length`. Es una
restricción autoimpuesta y merece la pena: las funciones custom requieren un
fichero `.js`, complican el empaquetado y —lo importante— convierten el ruleset
en código que a su vez necesitaría tests. Con JSON Schema, la regla *es* su
propia especificación.

El coste de esa restricción está en `ta-collection-has-pagination`, la única
regla que no puede ser exacta. Volveré sobre ello.

---

## 4. Explicación regla por regla

### Trazabilidad

**`ta-operation-source-issue`.** Cierra el hueco de SDD que detectamos: desde
cualquier `operationId` se llega al requisito de negocio. En PR-1 lo documentamos
en el skill; aquí deja de depender de que el agente se acuerde.

Fíjate en que valida presencia **y** tipo. Un `x-source-issue: "diecisiete"` es
tan inútil como su ausencia, y es exactamente el error que un modelo comete al
copiar el formato sin entenderlo.

### Autorización

**`ta-operation-security-declared`** exige que `security` esté presente. La sutileza
está en que un array vacío **es** una declaración válida: `security: []` significa
"público", explícitamente. La regla no prohíbe endpoints públicos; prohíbe que
existan por omisión.

Esa distinción es la clave de todo el bloque de autorización. El fallo que
queremos evitar no es "alguien hizo público un endpoint", sino "nadie decidió si
este endpoint es público".

**`ta-public-operation-is-visible`** es un `warn`, no un `error`. No bloquea: hace
visible en cada revisión qué endpoints son públicos. Un aviso que aparece en la
salida cada vez es un recordatorio; un error sería un obstáculo que empujaría a
la gente a rodear el sistema.

**`ta-operation-required-scope`** solo aplica cuando `security` no está vacío —de
ahí el `if`—. Un endpoint público no necesita scope. Y el `pattern`
`^[a-z]+:(read|write|admin)$` impone la taxonomía de permisos: `slots:write` vale,
`ESCRITURA` no. Sin esa restricción, cada operación inventaría su propio
vocabulario y la autorización dejaría de ser verificable mecánicamente.

**`ta-owner-check-required`** es la regla más sofisticada del conjunto. Usa
`contains` de JSON Schema para detectar si la operación tiene algún parámetro
`in: path`; si lo tiene, opera sobre una instancia concreta y necesita declarar
quién puede tocarla.

Esto ataca el IDOR en el punto más temprano posible del ciclo. No en revisión de
código, no en un test: en el contrato, antes de que exista implementación.

**`ta-owner-check-shape`** impone el enum cerrado de ADR-0003 y lleva
`additionalProperties: false`. Ese detalle importa: sin él, un
`x-owner-check: { resource: slot, param: slotId, rule: owner-only, note: "revisar" }`
pasaría, y ese `note` sería un campo que ninguna herramienta procesa y que alguien
leería como normativo.

**`ta-owner-check-responses`** exige 403 y 404 juntos. Es la política de no
filtrar existencia del skill `openapi-contract-reading`, hecha ejecutable. Un
contrato que solo declara 403 fuerza al implementador a revelar que el recurso
existe.

### Validación de entrada

**`ta-path-id-is-uuid`** se aplica solo a parámetros cuyo nombre acaba en `Id`.
Un identificador secuencial no es un fallo de control de acceso, pero convierte
un IDOR en enumerable: descubierta una franja, se descubren todas.

**`ta-string-has-maxlength`** exime a los campos con `enum` o `format`, que ya
están acotados por otra vía. Un `string` sin límite es un vector de agotamiento
de recursos y, en cuanto se persiste, un problema de base de datos.

**`ta-collection-has-pagination` es la regla imperfecta, y conviene ser honesto.**
Aproxima "es una colección" por "no tiene parámetro de path", lo cual falla con
singletons como `/me` o `/health`. Por eso es `warn` y no `error`: prefiero un
aviso ocasionalmente injusto a una regla que bloquea PRs legítimos y enseña a la
gente a desactivar reglas. Si la aproximación resulta molesta en la práctica, la
solución limpia es una extensión `x-collection: true` explícita, no relajarla.

### Versionado

**`ta-info-version-semver`** y **`ta-servers-v1-prefix`** implementan ADR-0003.
La segunda acepta `/v\d+`, no literalmente `/v1`, para que el día que exista `/v2`
la regla siga sirviendo.

---

## 5. Fixtures y autotest del ruleset

Aquí aplicamos al ruleset el mismo principio que a `agentsync`: **la verificación
vive en una herramienta con tests propios.**

Un ruleset sin probar es una promesa. El fallo típico no es que una regla dé
falsos positivos —eso se nota enseguida— sino que **una regla no dispare nunca**:
un `given` con un JSONPath que no resuelve produce silencio, y el silencio es
indistinguible de "todo correcto". Un gate que no puede fallar no es un gate.

### 5.1 `api/testdata/good.yaml`

La spec de referencia: correcta según nuestras reglas. Es también, en la
práctica, la documentación ejecutable de cómo debe verse una operación bien
declarada — y le será útil a `contract-designer` en PR-3.

```yaml
openapi: 3.0.3
info:
  title: task-allocation
  version: 0.1.0
  description: Calendario de equipo con franjas, proyectos y asignaciones.
  contact:
    name: fexlixjhl
    url: https://github.com/fexlixjhl/task-allocation
tags:
  - name: slots
    description: Franjas horarias
servers:
  - url: https://api.example.test/v1
paths:
  /slots:
    get:
      operationId: listSlots
      summary: Lista franjas horarias
      description: Devuelve las franjas visibles para el solicitante.
      tags: [slots]
      x-source-issue: 17
      x-required-scope: slots:read
      security:
        - oidc: [slots:read]
      parameters:
        - name: limit
          in: query
          schema: { type: integer, minimum: 1, maximum: 100, default: 20 }
        - name: offset
          in: query
          schema: { type: integer, minimum: 0, default: 0 }
      responses:
        '200':
          description: Colección de franjas
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Slot' }
  /slots/{slotId}:
    patch:
      operationId: updateSlot
      summary: Actualiza una franja
      description: Reprograma o anota una franja existente.
      tags: [slots]
      x-source-issue: 17
      x-required-scope: slots:write
      x-owner-check:
        resource: slot
        param: slotId
        rule: owner-or-project-lead
      security:
        - oidc: [slots:write]
      parameters:
        - name: slotId
          in: path
          required: true
          schema: { type: string, format: uuid }
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/SlotUpdate' }
      responses:
        '204': { description: Actualizada }
        '403': { description: Prohibido }
        '404': { description: No encontrada }
components:
  securitySchemes:
    oidc:
      type: openIdConnect
      openIdConnectUrl: https://idp.example.test/.well-known/openid-configuration
  schemas:
    Slot:
      type: object
      required: [id, startsAt, endsAt]
      properties:
        id: { type: string, format: uuid }
        startsAt: { type: string, format: date-time }
        endsAt: { type: string, format: date-time }
        note: { type: string, maxLength: 280 }
        status: { type: string, enum: [free, booked] }
    SlotUpdate:
      type: object
      properties:
        startsAt: { type: string, format: date-time }
        endsAt: { type: string, format: date-time }
        note: { type: string, maxLength: 280 }
```

### 5.2 `api/testdata/bad.yaml`

Cada defecto es deliberado y está pensado para disparar una regla concreta.

```yaml
openapi: 3.0.3
info:
  title: task-allocation
  version: "0.1"                      # ta-info-version-semver
  description: Spec de prueba con defectos deliberados.
  contact: { name: fexlixjhl }
tags:
  - name: slots
    description: Franjas horarias
servers:
  - url: https://api.example.test     # ta-servers-v1-prefix
paths:
  /slots:
    get:                              # ta-operation-source-issue
      operationId: listSlots          # ta-operation-required-scope
      summary: Lista franjas          # ta-collection-has-pagination
      description: Sin paginación, a propósito.
      tags: [slots]
      security:
        - oidc: [slots:read]
      responses:
        '200': { description: OK }
  /slots/{slotId}:
    patch:
      operationId: updateSlot
      summary: Actualiza una franja
      description: Con varios defectos deliberados.
      tags: [slots]
      x-source-issue: "diecisiete"    # ta-operation-source-issue (tipo)
      x-required-scope: ESCRITURA     # ta-operation-required-scope (patrón)
      x-owner-check:
        resource: franja              # ta-owner-check-shape
        param: slotId
        rule: si-es-suya              # ta-owner-check-shape
      security:
        - oidc: [slots:write]
      parameters:
        - name: slotId
          in: path
          required: true
          schema: { type: integer }   # ta-path-id-is-uuid
      responses:
        '204': { description: Actualizada }
                                      # ta-owner-check-responses: faltan 403/404
  /projects:
    post:
      operationId: createProject
      summary: Crea un proyecto
      description: Sin security declarado.
      tags: [slots]
      x-source-issue: 21
      responses:                      # ta-operation-security-declared
        '201': { description: Creado }
  /projects/{projectId}:
    delete:
      operationId: deleteProject
      summary: Elimina un proyecto
      description: Opera sobre una instancia y no declara x-owner-check.
      tags: [slots]
      x-source-issue: 21
      x-required-scope: projects:write
      security:
        - oidc: [projects:write]
      parameters:
        - name: projectId
          in: path
          required: true
          schema: { type: string, format: uuid }
      responses:                      # ta-owner-check-required
        '204': { description: Eliminado }
        '403': { description: Prohibido }
        '404': { description: No encontrado }
  /health:
    get:
      operationId: health
      summary: Estado del servicio
      description: Endpoint público declarado explícitamente.
      tags: [slots]
      x-source-issue: 3
      security: []                    # ta-public-operation-is-visible (warn)
      parameters:
        - name: limit
          in: query
          schema: { type: integer }
      responses:
        '200': { description: OK }

components:
  securitySchemes:
    oidc:
      type: openIdConnect
      openIdConnectUrl: https://idp.example.test/.well-known/openid-configuration
  schemas:
    Project:
      type: object
      properties:
        id: { type: string, format: uuid }
        name: { type: string }        # ta-string-has-maxlength
```

### 5.3 `api/testdata/selftest.sh`

```bash
#!/usr/bin/env bash
# Verifica que el ruleset de Spectral hace lo que decimos que hace:
# la spec correcta pasa sin errores y la defectuosa dispara todas las reglas.
set -euo pipefail
cd "$(dirname "$0")/.."

EXPECTED_RULES=(
  ta-operation-source-issue
  ta-operation-security-declared
  ta-public-operation-is-visible
  ta-operation-required-scope
  ta-owner-check-required
  ta-owner-check-shape
  ta-owner-check-responses
  ta-path-id-is-uuid
  ta-string-has-maxlength
  ta-collection-has-pagination
  ta-info-version-semver
  ta-servers-v1-prefix
)

fail=0

echo "== good.yaml no debe producir errores =="
if spectral lint testdata/good.yaml --ruleset .spectral.yaml --fail-severity=error >/dev/null 2>&1; then
  echo "   OK"
else
  echo "   FALLO: la spec de referencia produce errores"
  spectral lint testdata/good.yaml --ruleset .spectral.yaml 2>/dev/null | grep error || true
  fail=1
fi

echo "== bad.yaml debe disparar todas las reglas propias =="
out=$(spectral lint testdata/bad.yaml --ruleset .spectral.yaml 2>/dev/null || true)
for rule in "${EXPECTED_RULES[@]}"; do
  if grep -q "$rule" <<<"$out"; then
    echo "   OK   $rule"
  else
    echo "   FALLO $rule no disparó"
    fail=1
  fi
done

exit $fail
```

```bash
chmod +x api/testdata/selftest.sh
```

> **Nota sobre el binario.** El script invoca `spectral` directamente. Si no lo
> tienes global, cambia la invocación por `npx --yes @stoplight/spectral-cli` o
> exporta `SPECTRAL` como variable. En el devcontainer conviene tenerlo global
> por velocidad: `npm i -g @stoplight/spectral-cli`.

Salida esperada:

```
== good.yaml no debe producir errores ==
   OK
== bad.yaml debe disparar todas las reglas propias ==
   OK   ta-operation-source-issue
   OK   ta-operation-security-declared
   OK   ta-public-operation-is-visible
   OK   ta-operation-required-scope
   OK   ta-owner-check-required
   OK   ta-owner-check-shape
   OK   ta-owner-check-responses
   OK   ta-path-id-is-uuid
   OK   ta-string-has-maxlength
   OK   ta-collection-has-pagination
   OK   ta-info-version-semver
   OK   ta-servers-v1-prefix
```

**La lista `EXPECTED_RULES` es el contrato del autotest.** Cuando añadas una
regla nueva al ruleset, añádela también aquí y provoca su fallo en `bad.yaml`.
Si no, la regla existe pero nadie ha comprobado que dispare.

---

## 6. Cablear `make spec-lint`

Sustituye el target provisional del Makefile y añade la variable:

```make
GO        ?= go
SPECTRAL  ?= npx --yes @stoplight/spectral-cli
AGENTSYNC := $(GO) -C tools/agentsync run . -root ../..
```

```make
spec-lint: ## Lint del contrato con el ruleset propio
	@$(MAKE) --no-print-directory spec-ruleset-selftest
	@if [ -f api/openapi.yaml ]; then \
	  $(SPECTRAL) lint api/openapi.yaml --ruleset api/.spectral.yaml --fail-severity=error; \
	else \
	  echo "make: sin api/openapi.yaml todavía, spec-lint omitido"; \
	fi

spec-ruleset-selftest: ## Verifica que el ruleset de Spectral hace lo que dice
	@./api/testdata/selftest.sh
```

Añade `spec-ruleset-selftest` a la lista de `.PHONY`.

### Dos decisiones

**El autotest corre *antes* del lint real, siempre.** Aunque todavía no exista
`api/openapi.yaml`. Es lo que hace que el gate esté vivo desde hoy: si alguien
rompe el ruleset, `make verify` se pone en rojo aunque no haya contrato. Un
ruleset roto sin spec es un problema latente que estallaría en PR-3 con el peor
timing posible.

**`--fail-severity=error`.** Los `warn` se muestran pero no bloquean. Es la misma
gradación de severidades de `owasp-asvs-review`: si todo bloquea, la gente busca
el bypass. `ta-public-operation-is-visible` y `ta-collection-has-pagination` son
avisos deliberados.

Pruébalo:

```bash
make spec-lint
make verify && echo "rc=0"
```

---

## 7. `oasdiff` y el gate de cambios incompatibles

> **Aviso de honestidad:** a diferencia del ruleset, esta sección está diseñada
> pero **no la he ejecutado**. Contrasta las opciones de CLI con la versión que
> instales antes de darla por buena.

ADR-0003 define un gate de tres condiciones para cambios incompatibles. `oasdiff`
aporta la detección; el workflow, la política.

### 7.1 Instalación

En CI y en local, vía la acción oficial o el binario:

```bash
go install github.com/oasdiff/oasdiff@latest
oasdiff breaking --help
```

Uso básico, comparando la spec de `main` con la de la rama:

```bash
git show origin/main:api/openapi.yaml > /tmp/base-openapi.yaml
oasdiff breaking /tmp/base-openapi.yaml api/openapi.yaml --fail-on ERR
```

`oasdiff breaking` clasifica los cambios; `--fail-on ERR` devuelve código distinto
de cero cuando encuentra alguno incompatible.

### 7.2 La política, no la herramienta

Lo importante no es el comando: es que el gate implemente **las tres condiciones
simultáneas** de ADR-0003, porque cualquiera de ellas por separado se cumple por
accidente.

| Condición | Cómo se comprueba |
|---|---|
| Label `contract:breaking` aplicada | API de GitHub sobre el PR |
| `info.version` sube de MAJOR | Comparar el campo entre base y rama |
| El cuerpo del PR tiene sección "Migración" | `grep` sobre el cuerpo del PR |

Un agente no aplica una label, no sube un MAJOR y no escribe una sección de
migración sin darse cuenta de lo que está haciendo. Ese es el diseño: no impedir
el cambio incompatible, sino hacerlo imposible por descuido.

Crea la label:

```bash
gh label create "contract:breaking" --repo fexlixjhl/task-allocation \
  --color B60205 --description "Cambio incompatible del contrato, aprobado" --force
```

---

## 8. El workflow `spec-gate`

Fichero `.github/workflows/spec-gate.yml`:

```yaml
name: spec-gate

on:
  pull_request:
    paths:
      - 'api/**'
      - '.github/workflows/spec-gate.yml'

permissions:
  contents: read
  pull-requests: read

concurrency:
  group: spec-gate-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    name: spec-lint
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '22'

      - name: Autotest del ruleset y lint del contrato
        run: make spec-lint

  breaking:
    name: spec-breaking
    runs-on: ubuntu-latest
    timeout-minutes: 5
    if: hashFiles('api/openapi.yaml') != ''
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Instalar oasdiff
        run: go install github.com/oasdiff/oasdiff@latest

      - name: Detectar cambios incompatibles
        id: diff
        run: |
          git show origin/${{ github.base_ref }}:api/openapi.yaml > /tmp/base.yaml 2>/dev/null || {
            echo "sin spec en la base, primer contrato"; echo "breaking=false" >> "$GITHUB_OUTPUT"; exit 0; }
          if oasdiff breaking /tmp/base.yaml api/openapi.yaml --fail-on ERR; then
            echo "breaking=false" >> "$GITHUB_OUTPUT"
          else
            echo "breaking=true" >> "$GITHUB_OUTPUT"
          fi

      - name: Exigir las tres condiciones de ADR-0003
        if: steps.diff.outputs.breaking == 'true'
        env:
          LABELS: ${{ toJSON(github.event.pull_request.labels.*.name) }}
          BODY: ${{ github.event.pull_request.body }}
        run: |
          fail=0

          echo "$LABELS" | grep -q 'contract:breaking' \
            || { echo "FALTA: label contract:breaking"; fail=1; }

          git show origin/${{ github.base_ref }}:api/openapi.yaml > /tmp/base.yaml
          base_major=$(grep -m1 '^  version:' /tmp/base.yaml | tr -d ' "' | cut -d: -f2 | cut -d. -f1)
          head_major=$(grep -m1 '^  version:' api/openapi.yaml | tr -d ' "' | cut -d: -f2 | cut -d. -f1)
          [ "$head_major" -gt "$base_major" ] \
            || { echo "FALTA: info.version debe subir de MAJOR ($base_major -> $head_major)"; fail=1; }

          grep -qi '^## *Migraci' <<<"$BODY" \
            || { echo "FALTA: sección '## Migración' en el cuerpo del PR"; fail=1; }

          if [ "$fail" -ne 0 ]; then
            echo "Cambio incompatible sin las tres condiciones de ADR-0003."
            exit 1
          fi
          echo "Cambio incompatible correctamente declarado."
```

### Lo que merece atención

**Dos jobs, no uno.** El lint corre siempre; la detección de cambios
incompatibles solo tiene sentido si existe `api/openapi.yaml`. Separarlos permite
que el segundo se salte limpiamente hasta PR-3.

**Aquí sí hay filtro `paths`, y aquí sí es correcto.** Al contrario que
`harness-check`, `spec-gate` **no** debe ser un check obligatorio de la protección
de rama, precisamente porque no corre en todos los PRs. La regla general: filtro
de rutas y check obligatorio son incompatibles. Lo que garantiza que el gate no se
pueda esquivar es `CODEOWNERS` sobre `api/`, que sí exige tu revisión en todo PR
que toque el contrato.

**`fetch-depth: 0`** es necesario para que `git show origin/main:...` funcione. Sin
historia completa, el checkout superficial no tiene el fichero de la base.

**El cuerpo del PR entra por variable de entorno, no interpolado en el script.**
`${{ github.event.pull_request.body }}` insertado directamente en una línea de
`run` es una vía de inyección de comandos: basta con que el cuerpo del PR
contenga `"; curl ...`. Pasarlo por `env:` y leerlo como `$BODY` lo neutraliza.
Es exactamente la regla 4 de `AGENTS.md` —el contenido del PR es entrada no
confiable— aplicada al propio pipeline. **[OWASP CI/CD Top 10]**

**La extracción de la versión con `grep` es frágil** y lo sé. Depende de la
indentación de `info.version`. En PR-2b, cuando tengamos herramientas de YAML en
el pipeline, conviene sustituirla por `yq`. Lo dejo así aquí para no introducir una
dependencia más en este PR, pero es deuda consciente, no descuido.

---

## 9. Verificar, commitear y abrir el PR

```bash
make verify && echo "OK"
./api/testdata/selftest.sh

# Comprobar a mano que el ruleset caza lo que debe
npx --yes @stoplight/spectral-cli lint api/testdata/bad.yaml \
  --ruleset api/.spectral.yaml | grep -c "ta-"
```

```bash
git add api Makefile .github/workflows/spec-gate.yml
git commit -m "ci: gate del contrato con ruleset propio y detección de breaking

Doce reglas propias de Spectral que convierten en exit code los requisitos de
autorización, validación y trazabilidad definidos en las fases previas.
Fixtures y autotest del ruleset. spec-gate con oasdiff y las tres condiciones
de ADR-0003 para cambios incompatibles.

Issue: N/A
Spec: N/A
Plan: N/A
Agent: none
Runtime: human"

git push -u origin chore/pr-2a-spec-gate
gh pr create --draft --title "ci: gate del contrato (PR-2a)" --label "runtime:human" --fill
gh pr checks --watch
gh pr ready && gh pr merge --squash --delete-branch
```

### Checklist

- [ ] `api/.spectral.yaml` con las doce reglas `ta-`
- [ ] `api/testdata/good.yaml` sin errores
- [ ] `api/testdata/bad.yaml` disparando las doce
- [ ] `selftest.sh` ejecutable y en verde
- [ ] `make spec-lint` corre el autotest incluso sin contrato
- [ ] `make verify` en verde
- [ ] Label `contract:breaking` creada
- [ ] `spec-gate.yml` con `permissions` mínimos y el cuerpo del PR por `env`
- [ ] `harness-check` sigue en verde

---

## 10. Qué falta

| Paso | Contenido |
|---|---|
| **2b** | oapi-codegen, sqlc, goose, openapi-typescript, cableados en `make generate`; el `Makefile` deja de tener targets provisionales |
| 2c | golangci-lint con depguard implementando las cuatro reglas de ADR-0001, eslint, vue-tsc, workflow `code-gate` |
| 2d | CodeQL, Semgrep con reglas propias, gitleaks, govulncheck, Trivy, ZAP contra el perfil `ephemeral`, `agent-review`, workflow de métricas de runtime, caducidad de excepciones, Renovate |

Dos apuntes para 2b y 2d que nacen de este paso:

- Sustituir la extracción de `info.version` por `yq` en cuanto esté disponible.
- La regla de Semgrep que comprueba que la `rule` de `x-owner-check` se
  corresponde con el método de `ports.Authorizer` invocado se apoya en el enum
  cerrado que acabamos de hacer obligatorio. Sin `ta-owner-check-shape`, esa regla
  no sería escribible.
