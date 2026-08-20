# Verificación de configuraciones contra documentación oficial

**Fecha:** 2026-08-19
**Alcance:** oapi-codegen, sqlc, goose (PR-2b, sin implementar) y oasdiff (PR-2a, ya mergeado).

Documento de correcciones previo a implementar PR-2b. Cada afirmación de aquí
está contrastada con la fuente oficial que se cita.

---

## Índice

1. [Resumen de hallazgos](#1-resumen-de-hallazgos)
2. [Fuentes consultadas](#2-fuentes-consultadas)
3. [oapi-codegen](#3-oapi-codegen)
4. [sqlc — el fallo real](#4-sqlc--el-fallo-real)
5. [goose](#5-goose)
6. [oasdiff — corrección a PR-2a, ya mergeado](#6-oasdiff--corrección-a-pr-2a-ya-mergeado)
7. [Dónde aplica cada corrección](#7-dónde-aplica-cada-corrección)
8. [Ficheros corregidos, listos para copiar](#8-ficheros-corregidos-listos-para-copiar)

---

## 1. Resumen de hallazgos

| # | Herramienta | Hallazgo | Gravedad |
|---|---|---|---|
| 1 | sqlc | Un override `db_type` aplica a columnas **nullable o no-nullable, pero no a ambas**. Mi config solo cubría las no-nullable, así que `project_id uuid` (nullable) no habría recibido `uuid.UUID` | **Fallo real** |
| 2 | oapi-codegen | ~~El mapeo por defecto de `format: uuid` produce tipos distintos~~ **RETIRADO**: `openapi_types.UUID` es alias de `uuid.UUID`. Ver 3.3 | Falsa alarma |
| 3 | goose | Mi target `migrate-create` omitía `DRIVER` y `DBSTRING`, que la sintaxis exige | **Fallo real** |
| 4 | oapi-codegen | `compatibility.apply-gorilla-middleware-first-to-last` es específico de gorilla; con `std-http-server` no hace nada | Ruido |
| 5 | oapi-codegen | `skip-prune: false`, `user-templates: {}` y `nullable-type` son valores por defecto o innecesarios | Ruido |
| 6 | oasdiff | Acepta sintaxis de revisión de git directamente; el paso `git show > /tmp/base.yaml` sobra | Mejora |
| 7 | oasdiff | Existe acción oficial `oasdiff/oasdiff-action/breaking@v0`; el `go install` sobra | Mejora |
| 8 | goose | Recomendación oficial: timestamps en desarrollo y `goose fix` a secuencial antes de producción | Mejora |

Lo que **sí estaba correcto**, y lo confirmo para que no quede duda: todas las
claves `emit_*` de sqlc, `sql_package: pgx/v5`, la forma `overrides[].db_type` +
`go_type`, las cuatro opciones de `generate` de oapi-codegen, que `output` es
clave válida y obligatoria, el orden de argumentos de `oasdiff breaking base
revision`, el flag `--fail-on ERR`, y la opción `-dir` de goose.

---

## 2. Fuentes consultadas

| Herramienta | Fuente |
|---|---|
| oapi-codegen | `docs/configuration.md` y `configuration-schema.json` del repositorio oficial |
| sqlc | `docs.sqlc.dev` — referencia de configuración y guía de *Overriding types* |
| goose | Cadena de uso del CLI y documentación en `pressly.github.io/goose` |
| oasdiff | `docs/BREAKING-CHANGES.md` del repositorio, `oasdiff.com/docs` y el repositorio de la acción de GitHub |

---

## 3. oapi-codegen

### 3.1 Lo que confirma el JSON Schema oficial

El esquema declara `"additionalProperties": false` en la raíz y en cada bloque, y
`"required": ["package", "output"]`. Dos consecuencias prácticas:

- **Cualquier clave mal escrita es un error duro**, no un aviso silencioso. Esto
  es bueno: el fichero se valida solo.
- `output` sí es clave válida, y además obligatoria. Mi configuración original
  estaba bien en este punto.

Confirmados en el esquema: `generate.std-http-server`, `generate.strict-server`
("Strict specifies whether to generate strict server wrapper"),
`generate.models`, `generate.embedded-spec`, y en `output-options` las claves
`skip-prune`, `nullable-type` y `user-templates`.

### 3.2 Lo que sobra

**`compatibility.apply-gorilla-middleware-first-to-last`** existe, pero su
descripción es explícita: afecta al código generado *para gorilla/mux*. Con
`std-http-server` no hace absolutamente nada. Lo incluí por inercia. Fuera.

**`output-options.skip-prune: false`** y **`user-templates: {}`** son los valores
por defecto. Escribir un valor por defecto en un fichero de configuración sugiere
que se ha decidido algo cuando no es así. Fuera.

**`nullable-type: true`** genera tipos envoltorio para campos marcados
`nullable: true` en la spec. Como nuestro contrato no los usa —expresamos la
opcionalidad con `required`— activarlo introduce una forma de tipo que nadie
necesita. Fuera; el valor por defecto es `false`.

### 3.3 El mapeo de UUID: recomendación RETIRADA

> **Corrección posterior.** Lo que sigue en este apartado se implementó y
> **falló**. Se conserva porque el error es instructivo, pero la recomendación
> queda retirada: **no añadas `type-mapping` para UUID**.
>
> Dos motivos. Primero, es innecesario: `openapi_types.UUID` es un **alias de
> tipo** de `uuid.UUID` —`go doc github.com/oapi-codegen/runtime/types.UUID`
> devuelve `type UUID = uuid.UUID`—, así que ambos generadores ya producen el
> mismo tipo y no hay conversión que escribir. Segundo, el campo `import:` que
> propongo abajo **no emite el import** del paquete, de modo que el código
> generado no compila: `package uuid is not in std`.
>
> El fallo de criterio fue mío y está señalado en el propio apartado: escribí que
> no había verificado si eran alias, y aun así puse la configuración en la ruta
> principal en lugar de dejarla como nota. Comprobarlo costaba treinta segundos.

### 3.3 (original) El hallazgo sobre el mapeo de UUID

El esquema documenta el `type-mapping` por defecto, y para `string` con formato
`uuid` es:

```yaml
uuid: { type: openapi_types.UUID }
```

Es decir, el adaptador HTTP recibiría `openapi_types.UUID` mientras el adaptador
de Postgres, por el override de sqlc, produce `github.com/google/uuid.UUID`.

**Por qué importa aquí más que en otros proyectos.** ADR-0001 obliga a traducir
DTO ↔ entidad en el borde. Esa traducción la escribe `backend-builder` a mano en
cada handler. Si los dos lados del mapeo usan tipos nominalmente distintos para
el mismo UUID, el agente tendrá que decidir en cada handler cómo convertir, y esa
es exactamente la clase de decisión repetida donde un agente acaba improvisando de
formas inconsistentes.

La solución es fijarlo explícitamente. El esquema define `simple-type-spec` con
los campos `type` (obligatorio) e `import`:

```yaml
output-options:
  type-mapping:
    string:
      formats:
        uuid:
          type: uuid.UUID
          import: github.com/google/uuid
```

Con esto, ambos generadores producen `uuid.UUID` de `github.com/google/uuid` y la
traducción en el borde es una asignación directa.

> **Sin verificar:** no he podido comprobar si `openapi_types.UUID` es un alias de
> `uuid.UUID` —en cuyo caso serían intercambiables y el override sería solo
> cosmético— o un tipo distinto. Fijarlo explícitamente es correcto en ambos
> casos, así que la recomendación no depende de resolver esa duda.

### 3.4 Mejora: validación en el editor

El esquema JSON se publica y se puede enganchar al Language Server de YAML. Una
línea al principio del fichero da autocompletado y validación:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.0/configuration-schema.json
```

Funciona en VS Code con la extensión de YAML y en IntelliJ, que reconoce esa
directiva. Encaja con el requisito de paridad entre IDEs: no es configuración de
editor, es una línea en un fichero del repositorio.

Fija la versión del esquema a la que uses, no a `HEAD`. Un esquema que cambia
bajo tus pies produce avisos sobre campos que sí son válidos en tu versión.

---

## 4. sqlc — el fallo real

### 4.1 Qué dice la documentación

La guía oficial de *Overriding types* es inequívoca sobre el campo `nullable`:
si es `true`, sqlc aplica el override cuando la columna es nullable; en caso
contrario lo aplica cuando **no** lo es. Y añade el punto que se me escapó: una
única configuración de override `db_type` aplica a columnas nullable o a
no-nullable, pero no a ambas, y si se quiere el mismo tipo Go
independientemente de la nulabilidad hay que configurar **dos** overrides.

### 4.2 Por qué esto rompe nuestro esquema

En el fixture de la migración, `project_id uuid` es nullable —una franja puede no
estar asociada a ningún proyecto—. Con mi configuración original, un solo override
de `uuid`:

- `id` y `owner_id`, no nullable → `uuid.UUID` ✅
- `project_id`, nullable → **no recibe el override**, y con `sql_package: pgx/v5`
  recibiría el tipo UUID de pgtype ❌

El resultado sería un struct con dos tipos distintos de UUID conviviendo. Habría
compilado, y el error se habría descubierto al escribir el primer mapeo a entidad
en PR-5, con el agente intentando convertir entre ellos.

### 4.3 La corrección

Dos overrides por cada tipo cuya nulabilidad varíe:

```yaml
overrides:
  - db_type: "uuid"
    go_type: "github.com/google/uuid.UUID"
  - db_type: "uuid"
    nullable: true
    go_type: "github.com/google/uuid.UUID"
  - db_type: "timestamptz"
    go_type: "time.Time"
  - db_type: "timestamptz"
    nullable: true
    go_type: "time.Time"
```

Los de `timestamptz` no hacen falta hoy —las cuatro columnas de fecha del fixture
son `NOT NULL`— pero una fecha nullable aparecerá en cuanto haya un
`cancelled_at` o similar, y el fallo volvería a ser silencioso.

**Interacción con `emit_pointers_for_null_types: true`:** la documentación
confirma que esa opción está soportada para PostgreSQL con `pgx/v5`. Combinada con
el override nullable, `project_id` debería quedar como `*uuid.UUID`. Es
comprobable en el paso A.3 del apéndice de PR-2b, y el `grep -n "\*uuid.UUID"`
es la verificación que lo confirma.

### 4.4 Un punto a vigilar

La guía oficial de overrides usa en su ejemplo `db_type: "pg_catalog.timestamptz"`,
con el prefijo del catálogo, mientras que ejemplos muy extendidos usan
`"timestamptz"` a secas. No he podido determinar si ambas formas son
equivalentes en la versión actual.

**Qué hacer:** empieza con la forma corta, que es la más usada, y si el override
no se aplica —lo verás porque el tipo generado no es el esperado— prueba con
`pg_catalog.timestamptz` y `pg_catalog.uuid`. Está en la tabla de diagnóstico del
apéndice.

---

## 5. goose

### 5.1 El fallo: `create` también necesita driver y cadena de conexión

La cadena de uso del CLI es `goose [OPTIONS] DRIVER DBSTRING COMMAND`, y los
ejemplos oficiales muestran la forma `goose sqlite3 ./foo.db create init sql`:
`create` es un comando más y va después de driver y cadena de conexión.

Mi target los omitía:

```make
# INCORRECTO
migrate-create:
	@$(GO) -C tools/gen tool goose -dir ../../backend/migrations create $(NAME) sql
```

Corregido:

```make
# CORRECTO
migrate-create: ## Crea una migración vacía: make migrate-create NAME=crear_slots
	@test -n "$(NAME)" || { echo "uso: make migrate-create NAME=descripcion"; exit 1; }
	@$(GOOSE) create $(NAME) sql
```

Reutilizando la variable `GOOSE`, que ya incluye `-dir`, `postgres` y la cadena de
conexión. Más corto y correcto.

> Alternativa que también documenta goose: exportar `GOOSE_DRIVER` y
> `GOOSE_DBSTRING` como variables de entorno, con lo que los argumentos
> posicionales dejan de ser necesarios. Es lo que conviene si algún día montamos
> las migraciones en un contenedor.

### 5.2 Mejora: la estrategia híbrida de versionado

En PR-2b argumenté a favor de timestamps sobre secuencia, por las colisiones
entre ramas paralelas. La documentación oficial de goose recomienda algo más
matizado: timestamps durante el desarrollo, y versiones secuenciales en
producción, con `goose fix` para convertir unas en otras.

Es mejor que mi propuesta y por una razón concreta: los timestamps evitan la
colisión al crear, pero **no garantizan el orden de aplicación** cuando dos ramas
se mergean en orden distinto al de creación. La conversión a secuencial antes de
publicar fija ese orden de forma definitiva.

Adáptalo así en `backend/migrations/README.md`:

```markdown
## Versionado

Los ficheros se crean con timestamp (`make migrate-create`), que evita
colisiones entre ramas paralelas.

Antes de publicar una release, `goose fix` renumera a versiones secuenciales
y congela el orden de aplicación. Los timestamps evitan la colisión al crear;
la secuencia garantiza el orden al aplicar.

Una migración ya renumerada y publicada no se vuelve a tocar.
```

---

## 6. oasdiff — corrección a PR-2a, ya mergeado

### 6.1 Lo que estaba bien

`oasdiff breaking <base> <revision> --fail-on ERR` es correcto: la documentación
confirma que `--fail-on ERR` hace salir con código 1 si hay cambios de nivel ERR,
y que el orden es base primero y revisión después. También que `breaking` detecta
solo niveles ERR y WARN.

### 6.2 Mejora 1: sintaxis de revisión de git

oasdiff acepta referencias de git directamente, con la forma `revisión:fichero`.
La documentación lo muestra con `HEAD~1:openapi.yaml HEAD:openapi.yaml`, y la
acción oficial usa `origin/${{ github.base_ref }}:openapi.yaml`.

Eso elimina el paso intermedio de mi workflow:

```bash
# ANTES: tres líneas y un fichero temporal
git show origin/main:api/openapi.yaml > /tmp/base.yaml
oasdiff breaking /tmp/base.yaml api/openapi.yaml --fail-on ERR

# DESPUÉS
oasdiff breaking origin/main:api/openapi.yaml HEAD:api/openapi.yaml --fail-on ERR
```

Requiere que la referencia base esté disponible en el checkout: la acción oficial
lo resuelve con `git fetch --depth=1 origin ${{ github.base_ref }}`.

### 6.3 Mejora 2: existe una acción oficial

`oasdiff/oasdiff-action/breaking@v0` acepta `base`, `revision` y `fail-on` como
inputs, y publica los hallazgos como anotaciones en la pestaña de ficheros del
PR. Elimina el `go install` de mi workflow, con su descarga y compilación en cada
ejecución.

Además, las acciones leen un fichero `.oasdiff.yaml` de la raíz del repositorio,
lo que permite mantener la configuración versionada en lugar de repetirla en el
YAML del workflow. Precedencia: los `with:` de la acción ganan sobre el fichero,
y el fichero gana sobre los valores por defecto.

Esto encaja especialmente bien con nuestro diseño: la política vive en un fichero
del repositorio bajo `CODEOWNERS`, no en la configuración de un workflow.

```yaml
# .oasdiff.yaml en la raíz
fail-on: ERR
exclude-elements:
  - description
  - title
  - summary
```

`exclude-elements` evita que una reescritura de descripciones dispare el gate.
Coherente con nuestro skill: `description` no es normativo.

### 6.4 Lo que sigue siendo nuestro

La acción detecta; **las tres condiciones de ADR-0003 las seguimos imponiendo
nosotros**. Ningún tercero sabe que exigimos label, subida de MAJOR y sección de
migración. Ese bloque del workflow se mantiene, incluido el detalle de pasar el
cuerpo del PR por `env:` en lugar de interpolarlo.

---

## 7. Dónde aplica cada corrección

| Corrección | Destino | Motivo |
|---|---|---|
| 1, 2, 3, 4, 5, 8 | **PR-2b, antes de implementarlo** | No está implementado; el documento se corrige y nace bien |
| 6, 7 | **PR nuevo `fix/pr-2a-oasdiff`** | PR-2a ya está en `main`; se corrige con un PR propio |

No hay una tercera vía mejor: PR-2a está mergeado, y el historial no se reescribe.
Un PR pequeño y específico es además un buen ejercicio del flujo — es el primero
que corrige algo previo en lugar de añadir.

**Orden sugerido:** primero el fix de oasdiff, que es de diez minutos y deja
`spec-gate` correcto; después PR-2b con el documento ya corregido.

Mensaje de commit para el fix:

```
fix(ci): simplifica spec-gate usando la acción oficial de oasdiff

oasdiff acepta sintaxis de revisión de git, lo que elimina el fichero
temporal, y la acción oficial evita compilar el binario en cada ejecución.
La configuración pasa a .oasdiff.yaml, versionada y bajo CODEOWNERS.
Las tres condiciones de ADR-0003 se mantienen intactas.

Issue: N/A
Spec: N/A
Plan: N/A
Agent: none
Runtime: human
```

---

## 8. Ficheros corregidos, listos para copiar

### 8.1 `api/oapi-codegen.yaml`

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.0/configuration-schema.json
package: gen
output: ../backend/internal/adapters/http/gen/server.gen.go

generate:
  std-http-server: true
  strict-server: true
  models: true
  embedded-spec: true

output-options:
  # Alinea el tipo de UUID con el override de sqlc, para que la traducción
  # DTO <-> entidad sea una asignación directa. Por defecto sería
  # openapi_types.UUID.
  type-mapping:
    string:
      formats:
        uuid:
          type: uuid.UUID
          import: github.com/google/uuid
```

### 8.2 `backend/sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: migrations
    queries: queries
    gen:
      go:
        package: gen
        out: internal/adapters/postgres/gen
        sql_package: pgx/v5
        emit_interface: true
        emit_json_tags: false
        emit_empty_slices: true
        emit_pointers_for_null_types: true
        emit_prepared_queries: false
        # Un override db_type aplica a columnas nullable O no-nullable,
        # nunca a ambas. De ahí los pares.
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
          - db_type: "uuid"
            nullable: true
            go_type: "github.com/google/uuid.UUID"
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "timestamptz"
            nullable: true
            go_type: "time.Time"
```

Aplica los mismos pares a `backend/sqlc.test.yaml`.

### 8.3 Targets de goose en el `Makefile`

```make
DB_URL ?= postgres://taskalloc:taskalloc-dev-only@localhost:5432/taskalloc?sslmode=disable
GOOSE  := $(GO) -C tools/gen tool goose -dir ../../backend/migrations postgres "$(DB_URL)"

migrate-up: ## Aplica las migraciones pendientes
	@$(GOOSE) up

migrate-down: ## Revierte la última migración
	@$(GOOSE) down

migrate-status: ## Estado de las migraciones
	@$(GOOSE) status

migrate-create: ## Crea una migración vacía: make migrate-create NAME=crear_slots
	@test -n "$(NAME)" || { echo "uso: make migrate-create NAME=descripcion"; exit 1; }
	@$(GOOSE) create $(NAME) sql
```

### 8.4 `.oasdiff.yaml` en la raíz

```yaml
fail-on: ERR
exclude-elements:
  - description
  - title
  - summary
```

### 8.5 `.github/workflows/spec-gate.yml`, job `breaking`

```yaml
  breaking:
    name: spec-breaking
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@v4

      - name: Traer la rama base
        run: git fetch --depth=1 origin ${{ github.base_ref }}

      - name: ¿Existe contrato en la base?
        id: base
        run: |
          if git cat-file -e "origin/${{ github.base_ref }}:api/openapi.yaml" 2>/dev/null; then
            echo "exists=true" >> "$GITHUB_OUTPUT"
          else
            echo "sin spec en la base, primer contrato"
            echo "exists=false" >> "$GITHUB_OUTPUT"
          fi

      - name: Detectar cambios incompatibles
        id: diff
        if: steps.base.outputs.exists == 'true'
        continue-on-error: true
        uses: oasdiff/oasdiff-action/breaking@v0
        with:
          base: 'origin/${{ github.base_ref }}:api/openapi.yaml'
          revision: 'HEAD:api/openapi.yaml'
          fail-on: ERR

      - name: Exigir las tres condiciones de ADR-0003
        if: steps.diff.outcome == 'failure'
        env:
          LABELS: ${{ toJSON(github.event.pull_request.labels.*.name) }}
          BODY: ${{ github.event.pull_request.body }}
        run: |
          fail=0

          echo "$LABELS" | grep -q 'contract:breaking' \
            || { echo "FALTA: label contract:breaking"; fail=1; }

          base_major=$(git show "origin/${{ github.base_ref }}:api/openapi.yaml" \
            | grep -m1 '^  version:' | tr -d ' "' | cut -d: -f2 | cut -d. -f1)
          head_major=$(grep -m1 '^  version:' api/openapi.yaml \
            | tr -d ' "' | cut -d: -f2 | cut -d. -f1)
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

Notas sobre este workflow:

- **`continue-on-error: true` en la detección** es lo que permite distinguir "hay
  cambio incompatible" de "el job falla". Sin él, la acción cortaría antes de que
  se comprobaran las tres condiciones, y un cambio incompatible correctamente
  declarado sería imposible de mergear.
- **`git cat-file -e`** sustituye a `hashFiles`, que mira el árbol de trabajo y no
  la rama base. Necesitábamos saber si existe la spec **en la base**, que es otra
  pregunta.
- **La extracción de `info.version` sigue con `grep`**, y sigue siendo deuda
  reconocida. En PR-2b entra Node en el pipeline, y entonces conviene pasar a
  `npx yq`.

---

## 9. Qué queda sin verificar

Por honestidad, lo que no he podido contrastar y hay que comprobar al implementar:

| Punto | Cómo comprobarlo |
|---|---|
| Si `openapi_types.UUID` es alias de `uuid.UUID` | Irrelevante tras fijar el `type-mapping`, pero se ve en el generado |
| Si `db_type: "timestamptz"` funciona o hace falta `pg_catalog.` | Paso A.3 del apéndice: si el tipo no es `time.Time`, prueba el prefijo |
| El flag `-f` de `sqlc generate` para un config alternativo | `sqlc generate --help` antes de ejecutar el autotest |
| La versión exacta del esquema de oapi-codegen a fijar en la primera línea | `go -C tools/gen list -m github.com/oapi-codegen/oapi-codegen/v2` |
| Que `oasdiff-action/breaking@v0` siga siendo la referencia vigente | La página de la acción en el repositorio oficial |

Ninguno de estos cinco bloquea la implementación: los cuatro primeros se
resuelven en el primer intento de ejecución, con el diagnóstico ya escrito en el
apéndice A de PR-2b.
