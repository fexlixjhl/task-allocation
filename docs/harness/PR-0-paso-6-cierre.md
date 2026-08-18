# PR-0 · Paso 6 — Cerrar el arnés y abrir el PR

**Continúa desde:** `PR-0-paso-5-skills.md`.
**Ubicación recomendada:** `docs/harness/PR-0-paso-6.md`.

> El Makefile, el workflow y el devcontainer de este documento están ejecutados
> y validados: `make verify` probado en verde y en rojo, YAML y JSON parseados,
> y el job de CI simulado paso a paso.

---

## Índice

1. [Qué cierra este paso](#1-qué-cierra-este-paso)
2. [El `Makefile`](#2-el-makefile)
3. [El workflow `harness-check`](#3-el-workflow-harness-check)
4. [Devcontainer y paridad de entornos](#4-devcontainer-y-paridad-de-entornos)
5. [Plantilla de PR](#5-plantilla-de-pr)
6. [`CODEOWNERS`](#6-codeowners)
7. [`docs/security/exceptions.md`](#7-docssecurityexceptionsmd)
8. [Labels del repositorio](#8-labels-del-repositorio)
9. [Commit y apertura del PR](#9-commit-y-apertura-del-pr)
10. [Protección de rama](#10-protección-de-rama)
11. [Merge y verificación final](#11-merge-y-verificación-final)
12. [Qué falta](#12-qué-falta)

---

## 1. Qué cierra este paso

Tienes las definiciones, los skills y el generador. Falta lo que los conecta con
GitHub y con tu máquina: una interfaz única de comandos, un gate que la ejecute en
CI, y las reglas de gobierno que hacen estructural —y no voluntaria— la separación
de funciones.

El orden de este paso importa y no es arbitrario:

```
Makefile → workflow → devcontainer → plantilla → CODEOWNERS → exceptions
   → labels → commit → push → PR draft → primera ejecución de CI
   → protección de rama → merge
```

La protección de rama va **después** de la primera ejecución de CI, no antes. La
razón es concreta: para exigir un check como obligatorio hay que saber cómo se
llama exactamente el contexto que reporta, y eso solo se sabe con certeza cuando
lo has visto correr una vez. Configurarlo antes es la forma más común de bloquear
un repositorio con un check obligatorio que nunca existirá por un error tipográfico.

---

## 2. El `Makefile`

### 2.1 El problema que resuelve

`AGENTS.md` promete a los agentes que `make generate` y `make verify` existen.
Si un agente ejecuta `make verify` y obtiene `No rule to make target`, se queda
atascado o —peor— improvisa comandos propios, y a partir de ahí cada agente
verifica de forma distinta.

Por eso el Makefile debe existir **con la interfaz completa** desde el primer día,
aunque casi ningún target tenga todavía nada que hacer. La interfaz es el
contrato; su implementación crece en PR-2.

Eso obliga a una propiedad poco habitual: **cada target debe degradar
correctamente cuando sus insumos no existen todavía**, sin fallar y sin mentir.
Un `spec-lint` que falla porque no hay spec haría que `make verify` fuera
imposible de pasar hoy. Un `spec-lint` que devuelve 0 en silencio sería peor:
parecería que se verificó algo.

La solución es una guarda por target que informa de lo que omite y por qué.

### 2.2 El fichero

Fichero `Makefile`, en la raíz:

```make
SHELL := /bin/bash
.DEFAULT_GOAL := help

GO        ?= go
AGENTSYNC := $(GO) -C tools/agentsync run . -root ../..

.PHONY: help generate verify lint test \
        agents agents-check \
        generate-api generate-sql generate-client \
        spec-lint backend-lint frontend-lint \
        backend-test frontend-test

## --- Interfaz pública: estos tres targets los invocan los agentes ---

help: ## Lista los targets disponibles
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

generate: agents generate-api generate-sql generate-client ## Regenera todo lo derivado

verify: lint test ## Todo lo que debe pasar antes de marcar un PR ready

lint: agents-check spec-lint backend-lint frontend-lint ## Solo los linters

test: backend-test frontend-test ## Solo los tests

## --- Arnés de agentes ---

agents: ## Regenera los adaptadores de agente de ambos runtimes
	@$(AGENTSYNC)

agents-check: ## Verifica el arnés sin escribir; falla si está desactualizado
	@$(AGENTSYNC) -check

## --- Generación derivada del contrato (se configura en PR-2) ---

generate-api:
	@if [ -f api/openapi.yaml ]; then \
	  echo "make: oapi-codegen pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin api/openapi.yaml todavía, generate-api omitido"; \
	fi

generate-sql:
	@if [ -n "$$(ls -A backend/queries 2>/dev/null | grep -v '^\.gitkeep$$')" ]; then \
	  echo "make: sqlc pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin consultas en backend/queries, generate-sql omitido"; \
	fi

generate-client:
	@if [ -f api/openapi.yaml ] && [ -f frontend/package.json ]; then \
	  echo "make: openapi-typescript pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin spec o sin frontend, generate-client omitido"; \
	fi

## --- Linters (se configuran en PR-2) ---

spec-lint:
	@if [ -f api/openapi.yaml ]; then \
	  echo "make: spectral pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin api/openapi.yaml todavía, spec-lint omitido"; \
	fi

backend-lint:
	@if [ -n "$$(find backend -name '*.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend vet ./...; \
	else \
	  echo "make: sin código Go todavía, backend-lint omitido"; \
	fi

frontend-lint:
	@if [ -f frontend/package.json ]; then \
	  echo "make: eslint pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin frontend/package.json todavía, frontend-lint omitido"; \
	fi

## --- Tests ---

backend-test:
	@if [ -n "$$(find backend -name '*_test.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend test ./...; \
	else \
	  echo "make: sin tests de Go todavía, backend-test omitido"; \
	fi
	@$(GO) -C tools/agentsync test ./...

frontend-test:
	@if [ -f frontend/package.json ]; then \
	  echo "make: vitest pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin tests de frontend todavía, frontend-test omitido"; \
	fi
```

> **Los tabuladores son obligatorios.** Las líneas de receta de `make` van
> indentadas con TAB, no con espacios. Si copias del documento y tu editor
> convierte tabs a espacios, obtendrás `missing separator`. Comprueba con
> `cat -A Makefile | head -20`: debes ver `^I` al inicio de cada receta.

### 2.3 Lo que merece atención

**`AGENTSYNC` como variable, no repetido nueve veces.** Recuerda el problema del
paso 4: la invocación correcta es `go -C tools/agentsync run . -root ../..`
porque no hay módulo Go en la raíz. Aislarla en una variable significa que el día
que cambie —por ejemplo si compilas un binario en lugar de usar `go run`— se toca
una línea.

**`GO ?= go` permite fijar el toolchain sin editar el fichero.** Si un día
necesitas probar con otra versión: `make verify GO=go1.26`. Es el tipo de detalle
que separa un Makefile utilizable de uno que la gente rodea.

**El target `help` es el `.DEFAULT_GOAL`.** Un `make` a secas no debe *hacer*
nada: debe decir qué se puede hacer. Y `help` se autogenera de los comentarios
`##`, así que no puede quedar desincronizado de los targets reales — el mismo
principio de fuente única que aplicamos a los agentes.

**`backend-test` ejecuta siempre los tests del generador.** Aunque no haya código
Go de producto, `tools/agentsync` tiene tests y son parte de lo que debe estar en
verde. Es coherente con tratar el arnés como código de primera clase: si el
generador se rompe, el proyecto se rompe.

**Los mensajes de omisión dicen "pendiente de configurar (PR-2)" y no "OK".**
Es deliberado. Un target que se salta debe decirlo en voz alta; si dijera `OK`,
en tres semanas alguien creería que el linter de la spec está corriendo cuando no
existe. La honestidad del pipeline es un requisito de seguridad, no de estilo.

### 2.4 Probarlo

```bash
make            # equivale a make help
make verify
echo "rc=$?"    # debe ser 0
```

Salida esperada de `make verify` en el estado actual:

```
agentsync: arnés al día
make: sin api/openapi.yaml todavía, spec-lint omitido
make: sin código Go todavía, backend-lint omitido
make: sin frontend/package.json todavía, frontend-lint omitido
make: sin tests de Go todavía, backend-test omitido
ok  	github.com/fexlixjhl/task-allocation/tools/agentsync	0.012s
make: sin tests de frontend todavía, frontend-test omitido
```

Y ahora lo importante: **comprobar que falla cuando debe**. Un `verify` que solo
sabe pasar no es un gate.

```bash
echo "editado" >> .claude/agents/backend-builder.md
make verify >/dev/null 2>&1; echo "rc=$?"   # debe ser distinto de 0
make agents                                  # restaura
make verify >/dev/null 2>&1; echo "rc=$?"   # vuelve a 0
```

---

## 3. El workflow `harness-check`

Fichero `.github/workflows/harness-check.yml`:

```yaml
name: harness-check

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

concurrency:
  group: harness-${{ github.ref }}
  cancel-in-progress: true

jobs:
  harness:
    name: harness
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Configurar Go
        uses: actions/setup-go@v5
        with:
          go-version-file: tools/agentsync/go.mod
          cache: false

      - name: Vet del generador
        run: go -C tools/agentsync vet ./...

      - name: Tests del generador
        run: go -C tools/agentsync test ./...

      - name: El arnés está al día
        run: make agents-check
```

### 3.1 Lo que merece atención

**`permissions: contents: read` a nivel de workflow.** Por defecto el
`GITHUB_TOKEN` puede llevar permisos amplios; declararlos explícitamente y al
mínimo es la aplicación directa del principio de mínimo privilegio al pipeline.
Este job solo necesita leer el código: no comenta, no etiqueta, no escribe.
**[SDL: cadena de suministro · OWASP CI/CD Top 10: permisos]**

**No hay filtro `paths`, y es intencionado.** La tentación es limitar el workflow
a los PRs que tocan `agents/`, `skills/` o `AGENTS.md`. No lo hagas: si haces
este check obligatorio en la protección de rama y añades un filtro de rutas, todo
PR que no toque esas rutas se quedará esperando eternamente un check que nunca se
dispara. Es una de las trampas clásicas de GitHub Actions. El job tarda menos de
un minuto y en repositorio público los minutos no se te cobran; la simplicidad
gana.

**`go-version-file: tools/agentsync/go.mod`** en lugar de una versión escrita a
mano. La versión de Go vive en un solo sitio, el `go.mod`, y CI la lee de ahí.
Otra fuente única.

**`cache: false`** porque el generador no tiene dependencias externas y por tanto
no hay `go.sum`. Sin este ajuste, `setup-go` intenta cachear a partir de un
`go.sum` inexistente y falla. Es la consecuencia práctica de la decisión de cero
dependencias del paso 4.

**`push: branches: [main]`** además de `pull_request`. Sirve para detectar
deriva si alguna vez algo entra a `main` sin pasar por PR, y para que el check
tenga historial en la rama por defecto.

**`concurrency` con `cancel-in-progress`.** En un flujo agéntico un agente
empuja varios commits seguidos a la misma rama; sin esto acumulas ejecuciones
redundantes de las que solo importa la última.

### 3.2 Sobre fijar las acciones por SHA

Las acciones están referenciadas por tag (`@v4`, `@v5`). Un tag es mutable: quien
controla el repositorio de la acción puede reapuntarlo. La práctica endurecida es
fijar el SHA completo del commit:

```bash
gh api repos/actions/checkout/git/ref/tags/v4 --jq .object.sha
gh api repos/actions/setup-go/git/ref/tags/v5 --jq .object.sha
```

Y usar `uses: actions/checkout@<sha>  # v4`.

Para este proyecto, con dos acciones de GitHub, el tag es aceptable. En un
proyecto real de tu organización, y sobre todo con acciones de terceros, fija el
SHA — Dependabot puede actualizarlos por ti. Lo anoto aquí porque es exactamente
el tipo de control que se olvida y que pertenece al SDL.

### 3.3 Simular el job antes de empujar

```bash
go -C tools/agentsync vet ./...
go -C tools/agentsync test ./...
make agents-check
```

Si los tres pasan en local, el job pasará en CI. Esa equivalencia es el objetivo
del devcontainer del apartado siguiente.

---

## 4. Devcontainer y paridad de entornos

Fichero `.devcontainer/devcontainer.json`:

```json
{
  "name": "task-allocation",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04",
  "features": {
    "ghcr.io/devcontainers/features/go:1": { "version": "1.25" },
    "ghcr.io/devcontainers/features/node:1": { "version": "22" },
    "ghcr.io/devcontainers/features/docker-in-docker:2": {},
    "ghcr.io/devcontainers/features/github-cli:1": {}
  },
  "postCreateCommand": "make help",
  "customizations": {
    "vscode": {
      "extensions": [
        "golang.go",
        "Vue.volar",
        "stoplight.spectral",
        "42Crunch.vscode-openapi",
        "GitHub.vscode-github-actions"
      ],
      "settings": {
        "go.lintTool": "golangci-lint",
        "files.eol": "\n"
      }
    }
  },
  "remoteUser": "vscode"
}
```

### Lo que merece atención

**`docker-in-docker` está aquí por los testcontainers.** Cuando lleguen los tests
de integración contra Postgres real, necesitarán levantar contenedores desde
dentro del devcontainer. Añadirlo ahora evita rehacer la imagen más adelante.

**Las extensiones son de VS Code, y eso es una asimetría real que hay que
nombrar.** El bloque `customizations.vscode` no lo lee IntelliJ. JetBrains tiene
soporte de devcontainers, pero es más limitado y no consume ese bloque.

**Cómo se consigue de verdad la paridad VS Code / IntelliJ.** No por
configuración duplicada de IDE, sino porque:

1. La versión de Go la fija `go.mod` y la lee tanto el devcontainer como CI.
2. Todos los comandos pasan por el `Makefile`, idéntico en ambos entornos.
3. Las reglas de los agentes viven en `AGENTS.md` y en `agents/`, que ningún IDE
   interpreta de forma propia.

Las extensiones son ergonomía: te dan resaltado y autocompletado. Si faltan, tu
experiencia empeora pero **el resultado de `make verify` es el mismo**. Esa es la
propiedad que perseguíamos desde la Fase 3: lo que determina el comportamiento
está en el repositorio, no en la configuración de tu editor.

En IntelliJ, el equivalente práctico es crear Run Configurations de tipo *Makefile
target* apuntando a `generate` y `verify`. El soporte nativo de OpenAPI de
IntelliJ, por cierto, es mejor que el de VS Code con extensión: es la única
ventaja genuina de uno sobre otro en este stack.

---

## 5. Plantilla de PR

Fichero `.github/pull_request_template.md`:

```markdown
## Qué cambia

<!-- Una o dos frases. Qué comportamiento cambia, no qué ficheros. -->

## Plan aprobado

<!-- Ruta al PLAN.md mergeado en main. "N/A" solo para PRs de arnés o de spec. -->

## Operaciones tocadas

| operationId | cambio |
|---|---|

## Seguridad

- [ ] Los criterios de seguridad del plan están implementados
- [ ] Hay tests de abuso para cada control nuevo o modificado
- [ ] No se han añadido dependencias sin justificación en el plan
- [ ] Ningún log o error expone datos de otros usuarios

## Verificación

- [ ] `make verify` pasa limpio en local

---

Spec: api/openapi.yaml@<sha>
Plan: docs/features/<slug>/PLAN.md
Agent: <nombre-del-agente|none>
Runtime: <claude-code|copilot|human>
```

### Lo que merece atención

**Los trailers al final no son decoración: son un requisito funcional del
squash.** Configuramos el repositorio en modo squash-only, y GitHub usa el cuerpo
del PR como mensaje del commit resultante. Si los trailers solo estuvieran en los
commits individuales de la rama, desaparecerían al mergear y el workflow de
métricas se quedaría sin datos — perderías la comparación entre Claude Code y
Copilot sin enterarte hasta el día que fueras a mirarla.

**"Qué comportamiento cambia, no qué ficheros."** El diff ya dice los ficheros.
Lo que un revisor necesita, y lo que un agente tiende a no escribir, es la
intención.

**La sección de seguridad es una checklist, no prosa.** Cuatro casillas que un
agente debe marcar afirmativamente, y que tú puedes contrastar contra el diff en
segundos. Es el gate humano 2 hecho barato: si marcar la casilla es mentira, se ve
en el diff. **[SDL: verificación de release]**

**"N/A" está permitido explícitamente para el plan.** Sin esa salida, los PRs de
arnés como este mismo PR-0 no podrían rellenar la plantilla, y una plantilla que
no se puede rellenar honestamente se rellena deshonestamente.

---

## 6. `CODEOWNERS`

Fichero `.github/CODEOWNERS`:

```
# Toda la base de código requiere tu revisión.
* @fexlixjhl

# Rutas críticas: se listan explícitamente para documentar la intención,
# aunque el patrón global ya las cubra. Ningún agente puede modificar
# las reglas bajo las que opera sin aprobación humana.
/api/                 @fexlixjhl
/agents/              @fexlixjhl
/skills/              @fexlixjhl
/AGENTS.md            @fexlixjhl
/.github/             @fexlixjhl
/tools/agentsync/     @fexlixjhl
/docs/adr/            @fexlixjhl
/docs/security/       @fexlixjhl
```

### Lo que merece atención

**Se usa `@fexlixjhl`, la cuenta, no el correo.** GitHub admite direcciones de
correo en CODEOWNERS, pero solo funcionan si están verificadas y asociadas a la
cuenta, y cuando no lo están **fallan en silencio**: el fichero no asigna a nadie
y nadie se enatera. La cuenta es inequívoca.

**El patrón global `*` ya cubre todo; las rutas explícitas son documentación
ejecutable.** En CODEOWNERS gana la última regla que coincide, así que
funcionalmente son redundantes. Las mantengo porque comunican qué rutas son
no negociables, y porque el día que añadas un colaborador y relajes el patrón
global, esas líneas seguirán protegiendo lo importante.

**Las cuatro rutas del arnés —`agents/`, `skills/`, `AGENTS.md`,
`tools/agentsync/`— más `.github/` son el control de gobierno central del
proyecto.** Un agente con permiso de escritura sobre sus propias definiciones o
sobre los workflows puede reescribir los límites bajo los que opera. En este
dominio, eso es el equivalente exacto de una escalada de privilegios. CODEOWNERS
es lo que lo convierte en imposible sin tu firma.
**[OWASP: ASVS V1 · CI/CD Top 10 · SDL: gobierno]**

Nota importante: **CODEOWNERS por sí solo no exige nada.** Solo asigna revisores
automáticamente. Lo que lo hace vinculante es `require_code_owner_reviews` en la
protección de rama, apartado 10.

---

## 7. `docs/security/exceptions.md`

Fichero `docs/security/exceptions.md`:

```markdown
# Excepciones de seguridad

Registro de gates saltados deliberadamente. Fichero bajo CODEOWNERS: **ningún
agente puede escribir aquí**. Los agentes ejecutan la política; solo un humano
la modifica.

Toda excepción requiere fecha de caducidad. Un workflow nocturno falla el build
de `main` cuando una excepción vence. Sin caducidad forzada, las excepciones se
vuelven permanentes y el SDL se vacía sin que nadie decida vaciarlo.

## Cómo añadir una

1. Añade una fila a la tabla con todos los campos rellenos.
2. Abre el issue de remediación y enlázalo.
3. La fecha de caducidad máxima es de 90 días desde la aprobación.
4. El PR que añade la excepción requiere tu aprobación como CODEOWNER.

## Excepciones activas

| ID | Gate afectado | Alcance exacto | Motivo | Caduca | Issue |
|---|---|---|---|---|---|
| — | — | — | — | — | — |

## Excepciones cerradas

| ID | Gate afectado | Cerrada el | Cómo se resolvió |
|---|---|---|---|
| — | — | — | — |
```

### Por qué este fichero existe antes de la primera excepción

Todo SDL necesita una salida documentada, o la gente inventa una peor:
desactivar un check, comentar un test, añadir un `nolint` sin explicación. Crear
el mecanismo **antes** de necesitarlo hace que la primera vez que haya que
saltarse un gate exista un camino evidente y con coste visible.

El campo de caducidad es el que sostiene todo el diseño. El workflow que la exige
llega en PR-2; sin él, este fichero sería una lista de buenas intenciones.
**[SDL: gestión de riesgo]**

Empieza vacío, con la fila de guiones para que la tabla renderice.

---

## 8. Labels del repositorio

Las labels que definimos en la Fase 2 tienen que existir antes de que un agente
intente aplicarlas, o fallará al etiquetar. Con `gh`:

```bash
REPO=fexlixjhl/task-allocation

# Agentes
for a in contract-designer context-researcher change-planner backend-builder security-reviewer; do
  gh label create "agent:$a" --repo "$REPO" --color BFD4F2 --description "PR producido por $a" --force
done

# Runtimes — el eje de la comparación
gh label create "runtime:claude-code" --repo "$REPO" --color 5319E7 --description "Producido con Claude Code" --force
gh label create "runtime:copilot"     --repo "$REPO" --color 1D76DB --description "Producido con GitHub Copilot" --force
gh label create "runtime:human"       --repo "$REPO" --color CCCCCC --description "Escrito a mano" --force

# Fase RPI
gh label create "phase:research"  --repo "$REPO" --color C2E0C6 --force
gh label create "phase:plan"      --repo "$REPO" --color C2E0C6 --force
gh label create "phase:implement" --repo "$REPO" --color C2E0C6 --force

# Bloqueos y seguridad
gh label create "needs-contract" --repo "$REPO" --color D93F0B --description "Falta declarar algo en la spec" --force
gh label create "needs-human"    --repo "$REPO" --color D93F0B --description "Agente bloqueado, requiere decisión humana" --force
gh label create "sec:required"   --repo "$REPO" --color B60205 --description "Control de seguridad obligatorio" --force
gh label create "sec:debt"       --repo "$REPO" --color FBCA04 --description "Hallazgo de severidad baja, no bloquea" --force
```

Verifica:

```bash
gh label list --repo "$REPO"
```

`needs-contract` y `needs-human` en rojo intenso a propósito: son las dos señales
de que el flujo se ha detenido esperándote, y deben destacar sobre el resto en la
lista de PRs.

---

## 9. Commit y apertura del PR

### 9.1 Commit final

```bash
git add Makefile .devcontainer .github docs/security
git commit -m "chore: interfaz de comandos, gate de CI y gobierno del repositorio

Makefile con la interfaz que promete AGENTS.md, degradando limpiamente
mientras faltan insumos. Workflow harness-check con permisos mínimos.
Devcontainer para paridad de entornos. Plantilla de PR con trailers que
sobreviven al squash, CODEOWNERS sobre el arnés y registro de excepciones.

Agent: none
Runtime: human"
```

### 9.2 Comprobación previa

Antes de empujar, la última verificación local:

```bash
make verify && echo "LISTO PARA PUSH"
git status --short          # debe estar limpio
git log --oneline           # cuatro commits de PR-0
```

### 9.3 Push y PR en draft

```bash
git push -u origin chore/pr-0-harness

gh pr create \
  --repo fexlixjhl/task-allocation \
  --base main \
  --head chore/pr-0-harness \
  --draft \
  --title "chore: arnés de agentes (PR-0)" \
  --label "runtime:human" \
  --body "$(cat <<'BODY'
## Qué cambia

Introduce el arnés completo de agentes: definiciones canónicas, skills,
generador de adaptadores para Claude Code y Copilot, interfaz de comandos
y gobierno del repositorio. No incluye código de producto.

## Plan aprobado

N/A — PR de arnés, previo a la existencia de contrato y de plan.

## Operaciones tocadas

| operationId | cambio |
|---|---|
| — | ninguna |

## Seguridad

- [x] Los criterios de seguridad del plan están implementados
- [x] Hay tests de abuso para cada control nuevo o modificado
- [x] No se han añadido dependencias sin justificación en el plan
- [x] Ningún log o error expone datos de otros usuarios

## Verificación

- [x] `make verify` pasa limpio en local

---

Spec: N/A
Plan: N/A
Agent: none
Runtime: human
BODY
)"
```

> **Sobre las casillas marcadas con N/A implícito.** Las cuatro de seguridad se
> marcan porque no hay código de producto: sin endpoints no hay autorización que
> verificar. Merece la pena ser consciente de que este es el único PR del
> proyecto donde esa respuesta es legítima. A partir de PR-3, marcar una casilla
> sin haberla comprobado es lo que convierte una checklist en teatro.

### 9.4 Ver correr el check por primera vez

```bash
gh pr checks --repo fexlixjhl/task-allocation --watch
```

Cuando termine, apunta el nombre exacto del contexto:

```bash
gh api repos/fexlixjhl/task-allocation/commits/chore/pr-0-harness/check-runs \
  --jq '.check_runs[].name'
```

Debe imprimir `harness`. **Ese es el string que va en la protección de rama**, y
por eso configuramos la protección ahora y no antes.

---

## 10. Protección de rama

### 10.1 El problema del auto-aprobado

Aquí hay una restricción que condiciona la configuración y que conviene entender
antes de aplicarla: **GitHub no permite aprobar tu propio pull request.** Eres el
único humano del repositorio. Si exiges una revisión aprobatoria sin más, tus
propios PRs quedarán bloqueados para siempre.

Las opciones reales:

| Configuración | Efecto |
|---|---|
| Sin revisiones requeridas | Nada estructural obliga al gate humano |
| Revisión requerida y `enforce_admins: true` | Tus propios PRs quedan bloqueados sin salida |
| Revisión requerida y `enforce_admins: false` | Los PRs de los bots requieren tu aprobación; los tuyos los puedes mergear como admin |

La tercera es la correcta para este proyecto, y merece que veas por qué no es una
concesión: **los agentes no son administradores.** Sus PRs pasan por la
protección completa, que es precisamente donde queremos el control. Tu capacidad
de bypass aplica a los PRs que escribes tú, donde ya eres el autor y el revisor
sería una ficción.

En un equipo real con dos o más personas, pondrías `enforce_admins: true` y la
revisión sería genuinamente cruzada. Anótalo como la diferencia entre este
ejemplo y un proyecto de tu organización.

### 10.2 Aplicarla

```bash
gh api -X PUT repos/fexlixjhl/task-allocation/branches/main/protection \
  --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["harness"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": true,
    "required_approving_review_count": 1
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true
}
JSON
```

Verifica:

```bash
gh api repos/fexlixjhl/task-allocation/branches/main/protection --jq '{
  checks: .required_status_checks.contexts,
  strict: .required_status_checks.strict,
  reviews: .required_pull_request_reviews.required_approving_review_count,
  codeowners: .required_pull_request_reviews.require_code_owner_reviews,
  dismiss_stale: .required_pull_request_reviews.dismiss_stale_reviews,
  conversations: .required_conversation_resolution.enabled
}'
```

### 10.3 Qué hace cada ajuste, y por qué importa en un flujo agéntico

| Ajuste | Efecto | Por qué en este proyecto |
|---|---|---|
| `contexts: ["harness"]` | El PR no se mergea sin ese check en verde | El arnés se valida a sí mismo en cada cambio |
| `strict: true` | La rama debe estar actualizada con `main` | Evita que dos PRs de agentes se pisen al mergear en paralelo |
| **`dismiss_stale_reviews`** | Un push nuevo **invalida** tu aprobación anterior | El control más importante de la lista, ver abajo |
| `require_code_owner_reviews` | Hace vinculante `CODEOWNERS` | Sin esto, CODEOWNERS solo sugiere revisores |
| `required_linear_history` | Prohíbe merge commits | Coherente con squash-only |
| `allow_force_pushes: false` | Nadie reescribe `main` | — |
| `required_conversation_resolution` | Los comentarios deben resolverse antes de mergear | Los hallazgos de `security-reviewer` no se pueden ignorar en silencio |

**`dismiss_stale_reviews` merece su párrafo.** En desarrollo humano es una
comodidad. En desarrollo agéntico es un control de seguridad: sin él, un agente
puede recibir tu aprobación, empujar tres commits más y mergear código que nunca
revisaste. El agente no lo haría con mala intención —simplemente está iterando
sobre los comentarios— pero el efecto es idéntico. Con este ajuste, cada push
posterior a tu aprobación exige que vuelvas a mirar.

**`required_conversation_resolution` es el que da dientes al revisor de
seguridad.** Un hallazgo de severidad media, que según nuestra política "bloquea
pero admite acuerdo explícito", se materializa exactamente aquí: la conversación
debe cerrarse, con tu comentario, antes de que el merge sea posible.
**[SDL: verificación de release · OWASP: ASVS V1]**

---

## 11. Merge y verificación final

```bash
gh pr ready --repo fexlixjhl/task-allocation   # sale de draft
gh pr checks --repo fexlixjhl/task-allocation  # harness en verde
gh pr merge --repo fexlixjhl/task-allocation --squash --delete-branch
```

El merge lo haces tú como admin, por la razón del apartado 10.1. A partir de
PR-3, cuando los autores sean agentes, la protección se aplicará completa.

Comprueba que los trailers sobrevivieron al squash — es la verificación que valida
todo el diseño de métricas:

```bash
git checkout main && git pull
git log -1 --format='%B'
```

Debes ver `Agent: none` y `Runtime: human` en el commit de `main`. Si no
aparecen, revisa que el cuerpo del PR los incluyera.

### Checklist de cierre de PR-0

- [ ] `Makefile` con tabuladores reales; `make verify` en 0 y en distinto de 0 cuando toca
- [ ] `harness-check.yml` con `permissions: contents: read`, sin filtro `paths`
- [ ] `devcontainer.json` válido
- [ ] Plantilla de PR con los cuatro trailers
- [ ] `CODEOWNERS` con la cuenta, cubriendo `agents/`, `skills/`, `AGENTS.md`, `tools/agentsync/`, `.github/`
- [ ] `docs/security/exceptions.md` creado y vacío
- [ ] Labels creadas y verificadas con `gh label list`
- [ ] PR abierto en draft y check `harness` ejecutado al menos una vez
- [ ] Protección de rama aplicada con el contexto `harness`
- [ ] `dismiss_stale_reviews` y `required_conversation_resolution` activos
- [ ] PR mergeado con squash y rama borrada
- [ ] Trailers presentes en el commit de `main`

### Estado del repositorio al cerrar PR-0

```
AGENTS.md                          capa de hechos
CLAUDE.md                          puntero
Makefile                           interfaz única de comandos
agents/            5 ficheros      definiciones canónicas
skills/            5 ficheros      procedimientos
tools/agentsync/   5 ficheros      generador con tests
.claude/agents/    5 generados     adaptador Claude Code
.github/agents/    5 generados     adaptador Copilot
.github/workflows/harness-check.yml
.github/CODEOWNERS
.github/copilot-instructions.md    puntero
.github/pull_request_template.md
.devcontainer/devcontainer.json
docs/security/exceptions.md
docs/{adr,threats,features,metrics}/   vacíos, esperando PR-1
api/                                   vacío, esperando PR-3
backend/, frontend/                    esqueleto sin código
```

Cero líneas de código de producto. Y sin embargo el repositorio ya tiene: cinco
agentes definidos y traducidos a dos runtimes, un gate que se valida a sí mismo,
separación de funciones estructural, y un mecanismo de excepciones con caducidad.
**Ese es el punto de PR-0: el arnés antes del producto.**

---

## 12. Qué falta

| PR | Contenido | Quién lo escribe |
|---|---|---|
| **PR-1** | ADR-0001 y ADR-0002, plantillas RPI en `docs/features/_template/` | Tú |
| **PR-2** | Spectral con ruleset propio, oasdiff, oapi-codegen, sqlc, goose, golangci-lint con depguard, CodeQL, Semgrep, gitleaks, workflow de caducidad de excepciones, workflow de métricas | Tú, y es el PR más grande |
| **PR-3** | Primera spec OpenAPI mínima | `contract-designer` — **el primer PR de un agente** |
| **PR-4** | `RESEARCH.md` + `PLAN.md` de la primera feature | `context-researcher` y `change-planner` |
| **PR-5** | Primera implementación en Go | `backend-builder`, revisado por `security-reviewer` |

PR-2 va antes de que exista código deliberadamente: los gates se estrenan contra
un repositorio vacío, donde todo pasa trivialmente, y así depuras la configuración
sin el ruido de hallazgos reales. Estrenar CodeQL y Semgrep a la vez que el primer
endpoint mezcla fallos de configuración con hallazgos legítimos.

PR-3 es el hito real del proyecto: el primero que no escribes tú. Es donde
descubrirás si el arnés funciona.
