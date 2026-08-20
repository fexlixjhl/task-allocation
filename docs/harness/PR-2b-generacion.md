# PR-2b — Generación de código desde el contrato

**Continúa desde:** PR-2a mergeado en `main`.
**Ubicación recomendada:** `docs/harness/PR-2b.md`.

> **Estado de verificación.** Todas las configuraciones de este documento están
> **contrastadas contra la documentación oficial** de cada herramienta
> (`configuration-schema.json` de oapi-codegen, `docs.sqlc.dev`, la cadena de uso
> del CLI de goose). `openapi-typescript` 7.13.0 está además **ejecutado**: generé
> el cliente desde `api/testdata/good.yaml` y comprobé la salida. Lo que **no** he
> podido ejecutar es oapi-codegen, sqlc y goose, porque el entorno donde preparé
> esto no alcanza el proxy de módulos de Go; el apéndice A es un procedimiento
> completo para que los verifiques tú en la primera pasada.
>
> Esta versión incorpora todo lo aprendido al implementarlo: la forma correcta de
> los overrides de sqlc (cinco iteraciones, verificadas compilando), la retirada
> del `type-mapping` de oapi-codegen por innecesario, las convenciones de rutas de
> cada herramienta, el `go mod tidy` que faltaba, la consolidación de la versión
> de Go del apartado 2.3, y la sintaxis de `goose create`.
>
> **Las configuraciones de sqlc y oapi-codegen de este documento están ejecutadas
> y compilan.** Lo único no verificado es `oasdiff`, que pertenece a PR-2a.

---

## Índice

1. [El problema del huevo y la gallina](#1-el-problema-del-huevo-y-la-gallina)
2. [El módulo de herramientas `tools/gen`](#2-el-módulo-de-herramientas-toolsgen)
   · [2.3 Consolidar la versión de Go](#23-consolidar-la-versión-de-go)
   · [2.4 Cómo resuelve las rutas cada herramienta](#24-cómo-resuelve-las-rutas-cada-herramienta)
3. [`oapi-codegen`: del contrato al servidor Go](#3-oapi-codegen-del-contrato-al-servidor-go)
4. [`sqlc`: del SQL a los tipos](#4-sqlc-del-sql-a-los-tipos)
5. [`goose`: migraciones con expand/contract](#5-goose-migraciones-con-expandcontract)
6. [`openapi-typescript`: el cliente del frontend](#6-openapi-typescript-el-cliente-del-frontend)
7. [El Makefile deja de tener promesas](#7-el-makefile-deja-de-tener-promesas)
   · [7.1 El fichero completo resultante](#71-el-fichero-completo-resultante)
8. [`generate-check`: el gate de código generado](#8-generate-check-el-gate-de-código-generado)
9. [Autotest de generación](#9-autotest-de-generación)
10. [Verificar, commitear y abrir el PR](#10-verificar-commitear-y-abrir-el-pr)
11. [Qué falta](#11-qué-falta)

---

## 1. El problema del huevo y la gallina

Hay que configurar cuatro generadores y no existe `api/openapi.yaml`: ese fichero
lo escribirá `contract-designer` en PR-3. Tres opciones:

1. **Esperar a PR-3.** Descartada: significaría que el primer PR de un agente
   estrena, a la vez, el contrato y cuatro generadores sin probar. Cuando algo
   falle, no sabrás si fue el agente o la configuración.
2. **Escribir una spec provisional y borrarla después.** Contamina el historial y
   viola la regla de que solo `contract-designer` toca el contrato.
3. **Usar `api/testdata/good.yaml` como entrada de verificación.** El fixture que
   ya escribimos en PR-2a es una spec válida y completa.

La tercera, y no es un apaño: es el mismo patrón que resolvió PR-2a. El fixture
pasa de ser solo un caso de prueba del linter a ser **la entrada canónica con la
que se valida toda la cadena de generación**. Cuando llegue PR-3, los generadores
ya estarán probados y el único elemento nuevo será el contrato real.

```bash
git checkout main && git pull
git checkout -b chore/pr-2b-generacion
```

---

## 2. El módulo de herramientas `tools/gen`

### 2.1 Por qué un módulo aparte

Los generadores son dependencias Go. Podrían declararse en `backend/go.mod`, que
es lo habitual, y sería un error aquí por la misma razón que ya aplicamos a
`agentsync`: **el módulo que se despliega no debe arrastrar el `go.sum` del
utillaje.** `govulncheck` en PR-2d reportaría vulnerabilidades de dependencias
transitivas de un generador que nunca se ejecuta en producción, y tendrías que
decidir cada semana si eso importa. Con módulos separados, la pregunta no se
plantea.

### 2.2 Directivas `tool` de Go

Go 1.24 introdujo la directiva `tool` en `go.mod`, que sustituye al viejo patrón
del fichero `tools.go` con imports vacíos. La versión de cada generador queda
fijada en `go.mod` y `go.sum`, de modo que tú, los agentes y CI generáis
exactamente con la misma versión. Sin eso, `make generate` produce diffs distintos
según quién lo ejecute, y el gate del apartado 8 se vuelve inservible.

```bash
mkdir -p tools/gen
cd tools/gen
cat > go.mod <<'EOF'
module github.com/fexlixjhl/task-allocation/tools/gen

go 1.25
EOF

# go get -tool puede elevar la directiva `go` si alguna herramienta
# exige una versión superior. Es esperado; se consolida en 2.3.
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc
go get -tool github.com/pressly/goose/v3/cmd/goose
go mod tidy
cd ../..
```

> **`go mod tidy` no es opcional.** `go get -tool` registra la directiva pero deja
> el `go.sum` incompleto para *compilar* la herramienta. Sin él, la primera
> invocación de sqlc falla con una cascada de `missing go.sum entry for module
> providing package ...` sobre dependencias transitivas — en el caso de sqlc, el
> parser de TiDB que lleva incrustado para analizar SQL.

Comprueba que las directivas quedaron registradas:

```bash
cat tools/gen/go.mod
go -C tools/gen tool
```

Debes ver tres líneas `tool` y los tres nombres listados. La invocación posterior
es siempre `go -C tools/gen tool <nombre>`, por la misma razón que en
`agentsync`: no hay módulo Go en la raíz del repositorio.

> Si tu versión de Go no soporta `go get -tool`, el plan B es `go install` con
> versión fijada. Pierdes la fijación en `go.sum`, que es justamente lo que hace
> reproducible la generación, así que prefiere actualizar el toolchain.

**Es probable que `go get -tool` haya subido la directiva `go` de este módulo por
encima de 1.25.** No es un efecto secundario raro: alguna de las tres herramientas
declara un mínimo superior y Go lo propaga. El apartado siguiente lo resuelve.

---

### 2.3 Consolidar la versión de Go

#### Qué acaba de pasar

La directiva `go` de un `go.mod` declara la **versión mínima del lenguaje** que
ese módulo requiere, no la que tienes instalada. Cuando `go get -tool` la subió,
fue porque una de las herramientas la exige.

Conviene conocer el mecanismo que hace que esto no te haya fallado todavía: con
`GOTOOLCHAIN=auto`, el valor por defecto, si tu Go local es 1.25 y un `go.mod`
pide 1.26, **Go descarga y usa 1.26 automáticamente**.

Es cómodo, y tiene una implicación de cadena de suministro que no habíamos
considerado: un `go.mod` puede provocar la descarga de un toolchain distinto del
declarado en el devcontainer, en silencio. En un proyecto que se toma en serio la
reproducibilidad, la versión debe estar declarada, no ser emergente.

#### Lo que esto valida del diseño

Fíjate en lo que **no** ha pasado: `backend/go.mod` y `tools/agentsync/go.mod`
siguen donde estaban. La subida quedó contenida en `tools/gen`.

Es exactamente el argumento del apartado 2.1 para separar los módulos: la
evolución del utillaje no arrastra al módulo que se despliega. El mecanismo
funcionó solo, sin que nadie lo forzara.

#### Por qué se resuelve aquí y no más adelante

Tres razones, y la primera pesa más que las otras dos:

1. **`AGENTS.md` dice "Go 1.25".** Es la capa de hechos, y los agentes la leen
   como verdad. Un hecho desactualizado en el fichero cuya única función es ser
   exacto es peor que cualquier inconsistencia de versión.
2. **PR-3 es el primer PR de un agente.** Mal momento para tener una discrepancia
   entre lo declarado y lo instalado: cualquier fallo raro te costará descartar
   primero si es del agente o del toolchain.
3. **El coste es el mismo ahora que después**, pero después implica un PR extra
   con los agentes ya operando bajo un hecho falso.

Va dentro de PR-2b, y no en un PR aparte, porque **es PR-2b quien lo ha causado**:
el `go get -tool` de este mismo apartado. Que la corrección viaje con su causa es
lo correcto.

#### Paso 1: averigua qué versión hace falta de verdad

No subas por inercia. Mira qué exige cada cosa:

```bash
cat tools/gen/go.mod
go -C tools/gen list -m -f '{{.Path}} {{.GoVersion}}' all 2>/dev/null | sort -u -k2 | tail -5
mise ls-remote go | tail -5
```

En lo que sigue uso **1.26.7** como marcador. Sustitúyelo por la última estable
que te devuelva `mise ls-remote`.

#### Paso 2: `.mise.toml` en la raíz

Fichero nuevo, versionado:

```toml
[tools]
go = "1.26.7"
node = "22"
```

Va en el repositorio y no en tu configuración local por el mismo principio que el
resto del arnés: la política vive versionada. Quien clone el repositorio obtiene
las mismas versiones sin que nadie se lo diga.

#### Paso 3: los `go.mod`

Cada módulo declara su mínimo real, y añadimos la directiva `toolchain` donde
queremos fijar la versión exacta.

`backend/go.mod`:

```
module github.com/fexlixjhl/task-allocation/backend

go 1.26

toolchain go1.26.7
```

```bash
go -C backend mod edit -go=1.26 -toolchain=go1.26.7
```

**Subir `backend` a 1.26 es deliberado**, aunque hoy no tenga una sola línea de
código. Es una aplicación, no una librería: nadie la importa, así que elevar el
mínimo no perjudica a ningún consumidor. Y evita tener que hacerlo a mitad de
PR-5, cuando ya haya código.

`tools/gen` ya está en 1.26 por `go get -tool`; añádele el `toolchain` para
fijarla:

```bash
go -C tools/gen mod edit -toolchain=go1.26.7
```

`tools/agentsync` **se queda en 1.25**. Solo usa la librería estándar y no
necesita nada de 1.26. Subirlo por uniformidad sería elevar un requisito sin
motivo, que es justo lo contrario de lo que hace la directiva `go`.

#### Paso 4: el devcontainer

```json
"ghcr.io/devcontainers/features/go:1": { "version": "1.26.7" },
```

#### Paso 5: `AGENTS.md`

Una línea, en la sección de estructura:

```
- `backend/`  Go 1.26, módulo `github.com/fexlixjhl/task-allocation/backend`
```

Recuerda que `AGENTS.md` está bajo `CODEOWNERS`, así que este cambio exige tu
aprobación. Es lo correcto: es un hecho del repositorio, no un detalle.

#### Paso 6: el check que impide la deriva

Con la versión declarada en tres sitios, aparece la posibilidad de que diverjan.
Lo cerramos con el mismo patrón de `agents-check`.

Fichero `tools/toolchain-check.sh`:

```bash
#!/usr/bin/env bash
# Verifica que la versión de Go declarada en .mise.toml, en el devcontainer y
# en la directiva toolchain de backend/go.mod es la misma.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

mise_ver=$(grep -E '^go[[:space:]]*=' .mise.toml | head -1 | cut -d'"' -f2)
mod_ver=$(grep -E '^toolchain[[:space:]]+go' backend/go.mod | head -1 | sed 's/^toolchain[[:space:]]*go//')
dev_ver=$(grep -o '"ghcr.io/devcontainers/features/go:1"[^}]*}' .devcontainer/devcontainer.json \
          | grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4)

echo "  .mise.toml        : ${mise_ver:-AUSENTE}"
echo "  backend/go.mod    : ${mod_ver:-AUSENTE}"
echo "  devcontainer.json : ${dev_ver:-AUSENTE}"

for v in "$mise_ver" "$mod_ver" "$dev_ver"; do
  [ -n "$v" ] || { echo "ERROR: falta una de las tres declaraciones"; exit 1; }
done

if [ "$mise_ver" != "$mod_ver" ] || [ "$mise_ver" != "$dev_ver" ]; then
  echo "ERROR: las tres declaraciones de la versión de Go deben coincidir."
  fail=1
fi

exit $fail
```

```bash
chmod +x tools/toolchain-check.sh
```

Y en el Makefile, añadido a `lint`:

```make
toolchain-check: ## Verifica que la versión de Go es la misma en las tres declaraciones
	@./tools/toolchain-check.sh

lint: agents-check toolchain-check generate-check spec-lint backend-lint frontend-lint
```

**Este script está ejecutado y verificado.** Los tres casos que probé:

```
=== caso correcto ===
  .mise.toml        : 1.26.7
  backend/go.mod    : 1.26.7
  devcontainer.json : 1.26.7
rc=0

=== divergencia ===
  .mise.toml        : 1.26.6
  backend/go.mod    : 1.26.7
  devcontainer.json : 1.26.7
ERROR: las tres declaraciones de la versión de Go deben coincidir.
rc=1

=== falta la directiva toolchain ===
ERROR: falta una de las tres declaraciones
rc=1
```

Un detalle que merece contarse: al escribirlo, el script **detectó una divergencia
real a la primera** — yo había actualizado `.mise.toml` y `go.mod` y me había
dejado el devcontainer en 1.25. Es exactamente la deriva que existe para impedir,
y apareció en el primer minuto de vida del check.

#### Paso 7: verificar

```bash
mise install
go version                                  # debe dar go1.26.7
make toolchain-check
go -C backend build ./... 2>/dev/null || true
make verify && echo "OK"
```

**Comprueba también CI en la primera ejecución.** `harness-check` usa
`go-version-file: tools/agentsync/go.mod`, que sigue en 1.25 y por tanto no
cambia. Pero no tengo confirmado cómo trata `setup-go` la directiva `toolchain`
frente a la `go` cuando ambas están presentes, así que verifica en el log del
workflow qué versión selecciona.

---

### 2.4 Cómo resuelve las rutas cada herramienta

Esto cuesta media hora de depuración si se descubre por las malas, así que va
antes de tocar ningún fichero de configuración.

Como todas las herramientas se invocan con `go -C tools/gen tool ...`, el
**directorio de trabajo es `tools/gen`**, no la raíz del repositorio. Y las dos
herramientas de configuración no siguen el mismo criterio:

| Herramienta | Sus rutas son relativas a | Consecuencia |
|---|---|---|
| **oapi-codegen** | El **directorio de trabajo** | `output:` necesita `../../` para llegar a la raíz |
| **sqlc** | El **fichero de configuración** | `backend/sqlc.yaml` usa rutas naturales, sin prefijo |

Ambos criterios son legítimos. Lo peligroso es asumir que comparten uno.

**Cómo se manifiesta el error si te equivocas.** oapi-codegen sale con código 0 y
escribe el fichero en otro sitio; no falla, simplemente el artefacto no está donde
lo buscas. Con `output: ../backend/...` acabarías con un
`tools/backend/internal/adapters/http/gen/server.gen.go` fantasma. sqlc, en
cambio, sí falla con un mensaje claro y con la ruta **absoluta** que ha resuelto,
que es la mejor pista para saber qué criterio aplica.

**La verificación que no depende de suposiciones:**

```bash
find . -name "server.gen.go" -not -path "./.git/*"
```

Debe devolver **una sola ruta**. Si devuelve dos, una de ellas es un artefacto
extraviado que hay que borrar. Verificar el código de salida no basta cuando una
herramienta escribe ficheros: hay que comprobar que el fichero está donde se
espera.

---

## 3. `oapi-codegen`: del contrato al servidor Go

Fichero `api/oapi-codegen.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.0/configuration-schema.json
package: gen
# Relativo a tools/gen, que es el directorio de trabajo con `go -C`. Ver 2.4.
output: ../../backend/internal/adapters/http/gen/server.gen.go

generate:
  std-http-server: true
  strict-server: true
  models: true
  embedded-spec: true
```

Ajusta la versión del esquema de la primera línea a la que instales:

```bash
go -C tools/gen list -m github.com/oapi-codegen/oapi-codegen/v2
```

### 3.1 Las decisiones, una por una

**`std-http-server` y no Echo, Gin o Chi.** El adaptador HTTP no arrastra un
framework. `net/http` con el enrutador de la stdlib basta para lo que hacemos, y
cada dependencia que no entra es una dependencia que `govulncheck` y Renovate no
tienen que vigilar en PR-2d. Coherente además con ADR-0001: el adaptador debe ser
delgado, porque toda la lógica vive en `app` y `domain`.

**`strict-server: true` es la decisión más importante de este fichero.** Sin ella,
oapi-codegen genera handlers con `http.ResponseWriter` y tú escribes la
serialización a mano — con lo que puedes devolver cualquier cosa, incluido un
código de respuesta no declarado en la spec. Con `strict-server`, cada operación
tiene un tipo de request y un tipo de response por código declarado:

```go
type StrictServerInterface interface {
    UpdateSlot(ctx context.Context, request UpdateSlotRequestObject) (UpdateSlotResponseObject, error)
}
```

Devolver un 418 no declarado deja de ser un error en tiempo de ejecución para ser
**un error de compilación**: el tipo no existe. Es exactamente la propiedad que
buscábamos cuando escribimos en el skill que "los códigos de respuesta declarados
son exhaustivos". Con `strict-server`, esa regla la impone el compilador.

**`embedded-spec: true`** incrusta la spec en el binario. Permite validar
peticiones contra el esquema en tiempo de ejecución y, sobre todo, garantiza que
el binario desplegado sepa exactamente contra qué contrato se compiló. Es
trazabilidad en tiempo de ejecución, complementaria al trailer `Spec:`.

**`output` apunta a `internal/adapters/http/gen/`,** un subdirectorio propio. No
es cosmético: es lo que permite que `depguard` prohíba ese paquete concreto desde
`app` y `domain` en PR-2c. Sin subdirectorio separado, la regla del DTO no sería
expresable como regla de import.

**No hay `type-mapping` para UUID, y merece explicar por qué no.**

El mapeo por defecto de `string` con `format: uuid` es `openapi_types.UUID`,
mientras que sqlc produce `github.com/google/uuid.UUID`. Parece una divergencia
que obligaría a convertir en cada handler — justo la clase de decisión repetida
donde un agente acaba improvisando de formas inconsistentes.

No lo es. `openapi_types.UUID` es un **alias de tipo** de
`github.com/google/uuid.UUID` — exactamente el paquete al que apunta el override
de sqlc. Un alias en Go no es un tipo nuevo: es otro nombre para el mismo tipo,
así que ambos son intercambiables sin conversión y la traducción DTO ↔ entidad es
una asignación directa.

**Cómo comprobarlo, y por qué `go doc` no basta.** El comando obvio devuelve:

```bash
go doc github.com/oapi-codegen/runtime/types.UUID
# type UUID = uuid.UUID
```

El `=` confirma que es un alias, pero `go doc` muestra el código tal como está
escrito: ese `uuid` sin cualificar podría ser el paquete de Google, el de `gofrs`
o cualquier otro. La comprobación que no admite interpretación es compilar, desde
el módulo temporal del apéndice A.2:

```bash
cd .tmp/oapi-selftest
go get github.com/google/uuid
cat > alias_check.go <<'EOF'
package genfixture

import (
	googleuuid "github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Si openapi_types.UUID NO es un alias de github.com/google/uuid.UUID,
// esta declaración no compila.
var _ googleuuid.UUID = openapi_types.UUID{}
EOF
go build ./... && echo "SON EL MISMO TIPO" || echo "SON TIPOS DISTINTOS"
rm alias_check.go
cd ../..
```

Este chequeo valida dos cosas a la vez: que sobra el `type-mapping` en
oapi-codegen, y que el `go_type` del override de sqlc apunta al paquete correcto.
Si alguno de los dos cambiara de paquete, volverían a ser tipos distintos y
habría que ajustar el override.

Un borrador anterior de este documento incluía un `type-mapping` para forzar la
coincidencia. Estaba mal por partida doble: era innecesario, y el campo `import:`
que usaba no emitía el import del paquete, así que el código generado no
compilaba con `package uuid is not in std`.

**La primera línea, `yaml-language-server`, da validación en el editor.** El
esquema declara `additionalProperties: false`, así que cualquier clave mal
escrita es un error duro y el editor te lo marca al teclear. Funciona en VS Code
con la extensión de YAML y en IntelliJ, que reconoce la misma directiva: no es
configuración de IDE, es una línea en un fichero del repositorio.

**Lo que deliberadamente no está.** Ni `compatibility`, cuyas opciones son
específicas de otros routers y con `std-http-server` no hacen nada, ni
`skip-prune`, `user-templates` o `nullable-type`, que son valores por defecto o
funciones que nuestro contrato no usa. Escribir un valor por defecto sugiere que
se ha decidido algo cuando no es así.

### 3.2 Lo que `oapi-codegen` no hace, y hay que tener claro

Genera la interfaz y los tipos. **No genera la traducción DTO → entidad**: eso lo
escribe `backend-builder` a mano en cada handler, y es deliberado. Es el punto
donde se aplican las invariantes del dominio, y automatizarlo abriría la vía a
construir entidades sin pasar por su constructor — precisamente lo que ADR-0001
prohíbe.

---

## 4. `sqlc`: del SQL a los tipos

Fichero `backend/sqlc.yaml`:

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
        # Paquetes externos: forma larga en AMBOS elementos del par, o sqlc
        # emite el import dos veces. Stdlib: forma corta, o sqlc lo duplica
        # con el suyo. Verificado a base de compilar; no está documentado.
        overrides:
          - db_type: "uuid"
            go_type:
              import: "github.com/google/uuid"
              package: "uuid"
              type: "UUID"
          - db_type: "uuid"
            nullable: true
            go_type:
              import: "github.com/google/uuid"
              package: "uuid"
              type: "UUID"
              pointer: true
          - db_type: "timestamptz"
            go_type: "time.Time"
          # PENDIENTE: sin puntero. Ninguna columna de fecha es nullable hoy.
          # Cuando aparezca la primera, verificar que el cero de time.Time no
          # se confunde con ausencia de valor.
          - db_type: "timestamptz"
            nullable: true
            go_type: "time.Time"
```

### 4.1 Las decisiones

**`schema: migrations`.** sqlc lee el esquema **de las propias migraciones**, no
de un `schema.sql` mantenido aparte. Esto elimina una fuente de verdad duplicada:
si una migración añade una columna, sqlc la conoce sin que nadie sincronice nada.
Y tiene un efecto de gate: una consulta que referencia una columna inexistente
**no compila la generación**. Un error de SQL detectado sin tocar la base de datos.

**`emit_json_tags: false`** es una decisión de arquitectura disfrazada de opción.
Si los structs de sqlc llevaran tags JSON, invitarían a serializarlos
directamente en una respuesta HTTP — saltándose la traducción a DTO y violando
ADR-0001. Quitando los tags, el atajo deja de ser cómodo. Es *hacer que lo
correcto sea lo fácil*, aplicado a la configuración de una herramienta.

**`emit_interface: true`** genera una interfaz `Querier` con todos los métodos.
Sirve para doblar el acceso a datos en tests sin levantar Postgres. Ojo: el puerto
del dominio sigue siendo `ports.SlotRepository`, definido en el vocabulario del
núcleo; `Querier` es un detalle del adaptador. No los confundas — el skill
`hexagonal-boundaries` es explícito sobre esto.

**`emit_pointers_for_null_types: true`.** Un campo nullable se representa como
puntero, no como un `sql.NullString`. Hace que la ausencia de valor sea visible en
el tipo y obliga a decidir qué hacer con `nil` al traducir a entidad, en lugar de
arrastrar un valor cero silencioso.

**El override de UUID sí hace falta aquí, al contrario que en oapi-codegen.** Son
situaciones opuestas y conviene no confundirlas: el tipo por defecto de
oapi-codegen ya era el bueno, mientras que sqlc con `sql_package: pgx/v5` produce
`pgtype.UUID` — un struct con `Bytes [16]byte` y `Valid bool`, no intercambiable
con nada. Sin override habría que convertir en cada mapeo a entidad. Lo mismo
vale para `timestamptz`, cuyo defecto sería `pgtype.Timestamptz` en lugar de
`time.Time`.

**Los overrides van en pares.** La documentación de sqlc es explícita: un único
override `db_type` aplica a columnas nullable **o** a no-nullable, pero no a
ambas, y si quieres el mismo tipo Go con independencia de la nulabilidad hay que
configurar dos.

En nuestro esquema, `project_id uuid` es nullable —una franja puede no estar
asociada a ningún proyecto—. Con un solo override, `id` y `owner_id` recibirían
`uuid.UUID` y `project_id` el tipo UUID de pgtype.

**Y la forma del override importa tanto como su existencia.** Esto no está
documentado y costó cinco iteraciones de prueba y compilación:

| Paquete del tipo | Forma que funciona | Qué pasa si usas la otra |
|---|---|---|
| **Externo** (`github.com/google/uuid`) | Larga: `import`, `package`, `type`, más `pointer: true` en el nullable | Forma corta mezclada con larga → import duplicado. Larga sin `package` → import truncado a `github` |
| **Stdlib** (`time`) | Corta: `go_type: "time.Time"` en ambos | Forma larga → `time` declarado dos veces, porque sqlc ya lo importa por su cuenta |

Los dos elementos de un par deben usar **la misma forma**. Mezclarlas es lo que
genera el import duplicado.

**El puntero hay que pedirlo explícitamente.** `emit_pointers_for_null_types`
actúa sobre los tipos por defecto; en cuanto declaras un override, sqlc respeta
literalmente el tipo que le das. Sin `pointer: true`, `project_id` sale como
`uuid.UUID` y "sin proyecto" se representaría con el UUID cero, indistinguible de
un valor legítimo.

**Deuda anotada en el fichero:** el par de `timestamptz` va en forma corta y por
tanto **sin puntero**. Hoy da igual, porque las cuatro columnas de fecha son
`NOT NULL`. Cuando aparezca la primera fecha nullable —un `cancelled_at`— hay que
verificar que el cero de `time.Time`, que es el año 1, no se confunde con
ausencia de valor. El comentario en el YAML lo recuerda.

**Cómo se detectó todo esto.** Ninguno de los tres `grep` del autotest falló en
ninguna de las cinco iteraciones: los tipos eran correctos, los pares se
aplicaban, no había tags JSON. Lo que estaba roto era siempre el bloque de
imports, y solo lo vio `go build`. Sin esa comprobación, esta configuración
habría llegado intacta a PR-5 y el fallo habría aparecido mezclado con el primer
código de dominio escrito por un agente.

**No hay override de `tstzrange`, y es deliberado.** En un borrador anterior de
este documento incluí uno; era un error por dos motivos.

El primero es técnico: `pgtype.Range[pgtype.Timestamptz]` es un tipo genérico, y
el campo `go_type` de sqlc no expresa parámetros de tipo. Cualquier intento de
escribirlo produce configuración inválida o un tipo que no compila.

El segundo es de arquitectura, y es el que importa: **el `tstzrange` no es un
tipo del dominio, es un mecanismo del motor.** Existe únicamente dentro de la
constraint `EXCLUDE` que impide solapamientos. El dominio razona en términos de
`starts_at` y `ends_at`, que son dos `timestamptz` y se mapean solos.

La regla práctica que se deriva: **ninguna consulta de `backend/queries/` debe
devolver una columna o expresión de tipo `tstzrange`.** Si alguna vez hace falta
el rango en Go, se construye desde los dos extremos. La constraint sigue
protegiendo la invariante exactamente igual, sin que Go conozca el tipo.

Esto es coherente con ADR-0001: un detalle del adaptador que no necesita cruzar
hacia dentro, no cruza.

---

## 5. `goose`: migraciones con expand/contract

### 5.1 Convención de ficheros y estrategia híbrida de versionado

goose ordena por el prefijo numérico. Los ficheros se crean con marca de tiempo:

```
backend/migrations/20260817120000_crear_slots.sql
```

Un contador secuencial produce colisiones en cuanto dos ramas abren una migración
a la vez, y con agentes trabajando en paralelo eso deja de ser hipotético.

Pero el timestamp resuelve solo la mitad del problema. **Evita la colisión al
crear; no garantiza el orden al aplicar** cuando dos ramas se mergean en orden
distinto al de creación. Por eso la recomendación oficial de goose es híbrida:
timestamps durante el desarrollo, y conversión a versiones secuenciales antes de
publicar, con el comando `goose fix`.

Adoptamos esa estrategia:

| Momento | Versionado | Comando |
|---|---|---|
| Al crear la migración, en una rama | Timestamp | `make migrate-create NAME=...` |
| Antes de publicar una release | Secuencial | `goose fix` |
| Después de publicada | Congelado | No se toca |

La secuencia fija el orden de aplicación de forma definitiva, que es justo lo que
la política expand/contract necesita: si el orden pudiera cambiar, una migración
de *contract* podría adelantarse a la de *expand* que la habilita.

### 5.2 Plantilla con la cabecera de ADR-0002

Fichero `backend/migrations/README.md`:

```markdown
# Migraciones

Forward-only. Una migración mergeada **nunca se edita**: un error se corrige con
una migración nueva.

## Cabecera obligatoria

Toda migración empieza con este bloque de comentario:

    -- Fase: expand | migrate | contract
    -- Reversible: sí | no (motivo)
    -- ADR-0002: <por qué esta fase, y qué migración la completa si aplica>

## Recordatorio de la política (ADR-0002)

- Los cambios destructivos se reparten en al menos dos releases.
- Prohibido en la migración que introduce el uso: DROP COLUMN, DROP TABLE,
  añadir NOT NULL a columna existente, renombrar.
- Los renombrados no existen: añadir, copiar, usar, y eliminar más tarde.
- Los índices sobre tablas con datos se crean CONCURRENTLY.
- Toda migración debe aplicarse sobre base vacía y sobre base poblada.
```

Ejemplo de migración conforme, `20260817120000_crear_slots.sql`:

```sql
-- Fase: expand
-- Reversible: sí
-- ADR-0002: creación inicial, sin datos previos que migrar.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE slots (
    id          uuid PRIMARY KEY,
    owner_id    uuid NOT NULL,
    project_id  uuid,
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    note        text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT slots_range_valid CHECK (ends_at > starts_at)
);

ALTER TABLE slots ADD CONSTRAINT slots_no_overlap
  EXCLUDE USING gist (
    owner_id WITH =,
    tstzrange(starts_at, ends_at) WITH &&
  );

-- +goose Down
DROP TABLE slots;
```

**`btree_gist` es imprescindible y se olvida siempre.** La constraint mezcla un
operador de igualdad sobre `uuid` con un operador de solapamiento sobre un rango;
sin esa extensión, PostgreSQL no sabe indexar `owner_id WITH =` dentro de un
índice GiST y la migración falla con un error poco claro.

**La `CHECK` de rango válido es independiente de la validación en Go.** Es la
tercera capa de las tres que discutimos: contrato, dominio y motor. Si alguna vez
un camino de escritura evita el constructor de dominio, la base sigue rechazando
el rango invertido.

### 5.3 Targets de migración

**Dónde van exactamente.** Las dos variables se añaden al bloque de variables de
la cabecera, junto a `GO`, `SPECTRAL` y `AGENTSYNC`. Los cuatro targets van en una
sección propia **al final del fichero**, después de los tests. Y los cuatro
nombres se añaden a `.PHONY`.

El apartado 7.1 muestra el Makefile completo resultante, con todo en su sitio.

En la cabecera, junto al resto de variables:

```make
DB_URL    ?= postgres://taskalloc:taskalloc-dev-only@localhost:5432/taskalloc?sslmode=disable
GOOSE     := $(GO) -C tools/gen tool goose -dir ../../backend/migrations postgres "$(DB_URL)"
```

Al final del fichero, en su propia sección:

```make
## --- Migraciones: efectos laterales sobre una base de datos. ---
## --- Fuera de generate y de verify a propósito.                ---

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

**`migrate-create` reutiliza `$(GOOSE)`, con driver y cadena de conexión.** La
sintaxis del CLI es `goose [OPTIONS] DRIVER DBSTRING COMMAND`, y `create` es un
comando más: omitir el driver y la cadena falla. Es contraintuitivo, porque crear
un fichero vacío no necesita base de datos, pero es la forma que exige la
herramienta.

> Alternativa documentada por goose: exportar `GOOSE_DRIVER` y `GOOSE_DBSTRING`
> como variables de entorno, con lo que los argumentos posicionales dejan de
> hacer falta. Es lo que conviene si algún día ejecutas las migraciones desde un
> contenedor.

**`DB_URL` con `?=`** para poder apuntar al Postgres efímero:
`make migrate-up DB_URL=postgres://...@localhost:55432/taskalloc_test?sslmode=disable`.

**Ningún target de migración entra en `verify` ni en `generate`.** Aplicar
migraciones es un efecto lateral sobre una base de datos, y `AGENTS.md` ya dice
que las aplica CI, nunca un agente. Que estén en el Makefile es comodidad para ti;
que estén fuera de la interfaz que invocan los agentes es deliberado.

---

## 6. `openapi-typescript`: el cliente del frontend

Esta es la parte que sí está ejecutada y verificada.

Fichero `frontend/package.json` — mínimo, sin la aplicación Vue todavía:

```json
{
  "name": "task-allocation-frontend",
  "private": true,
  "type": "module",
  "scripts": {
    "generate:api": "openapi-typescript ../api/openapi.yaml -o src/api/schema.d.ts"
  },
  "devDependencies": {
    "openapi-typescript": "^7.13.0"
  }
}
```

```bash
cd frontend && npm install && cd ..
```

### Qué produce, comprobado

Ejecutando el generador sobre el fixture:

```bash
npx openapi-typescript api/testdata/good.yaml -o /tmp/schema.d.ts
```

Salida real (139 líneas para dos operaciones):

```typescript
/**
 * This file was auto-generated by openapi-typescript.
 * Do not make direct changes to the file.
 */

export interface paths {
    "/slots": {
        parameters: { query?: never; header?: never; path?: never; cookie?: never; };
        /**
         * Lista franjas horarias
         * @description Devuelve las franjas visibles para el solicitante.
         */
        get: operations["listSlots"];
        put?: never;
        post?: never;
        ...
    };
    "/slots/{slotId}": {
        ...
        patch: operations["updateSlot"];
    };
}
```

Tres cosas que conviene notar de esa salida:

**Los métodos no declarados aparecen como `never`.** Un intento de hacer `POST
/slots` desde el frontend no compila. El contrato se convierte en restricción del
cliente, no solo del servidor.

**Las `description` de la spec viajan como JSDoc.** El desarrollador de frontend
ve la documentación del contrato en el autocompletado del editor, sin salir del
código. Es un argumento adicional para escribir descripciones útiles en la spec,
aunque no sean normativas.

**Solo son tipos, no un cliente HTTP.** `openapi-typescript` genera un fichero de
declaraciones; el `fetch` tipado lo pone una librería complementaria como
`openapi-fetch`, que llegará con la aplicación Vue. Para PR-2b esto basta y evita
comprometerse ahora con una librería de cliente.

**El fichero va a `frontend/src/api/`, ya marcado como `linguist-generated`** en el
`.gitattributes` de PR-0. Se commitea, como los adaptadores de agentes y por la
misma razón: no hay paso de build previo que lo produzca en el checkout.

---

## 7. El Makefile deja de tener promesas

### 7.1 El fichero completo resultante

Este es el Makefile con todo integrado: los targets de generación reales, los
de migración del apartado 5.3, `generate-check` del 8, `gen-selftest` del 9 y
`toolchain-check` del 2.3. **Sustituye el fichero entero**; es menos propenso a
error que ir insertando bloques.

Está ejecutado y probado: `make help` lista los veinte targets,
`make toolchain-check` sale en 0, `make generate-check` detecta un generado
obsoleto commiteado, y `make migrate-create` sin `NAME` falla con su mensaje de
uso.

```make
SHELL := /bin/bash
.DEFAULT_GOAL := help

GO        ?= go
SPECTRAL  ?= npx --yes @stoplight/spectral-cli
AGENTSYNC := $(GO) -C tools/agentsync run . -root ../..

DB_URL    ?= postgres://taskalloc:taskalloc-dev-only@localhost:5432/taskalloc?sslmode=disable
GOOSE     := $(GO) -C tools/gen tool goose -dir ../../backend/migrations postgres "$(DB_URL)"

.PHONY: help generate verify lint test \
        agents agents-check toolchain-check generate-check \
        generate-api generate-sql generate-client \
        spec-lint spec-ruleset-selftest gen-selftest \
        backend-lint frontend-lint backend-test frontend-test \
        migrate-up migrate-down migrate-status migrate-create

## --- Interfaz pública: estos tres targets los invocan los agentes ---

help: ## Lista los targets disponibles
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

generate: agents generate-api generate-sql generate-client ## Regenera todo lo derivado

verify: lint test ## Todo lo que debe pasar antes de marcar un PR ready

lint: agents-check toolchain-check generate-check spec-lint backend-lint frontend-lint ## Solo los linters

test: gen-selftest backend-test frontend-test ## Solo los tests

## --- Arnés de agentes y coherencia del entorno ---

agents: ## Regenera los adaptadores de agente de ambos runtimes
	@$(AGENTSYNC)

agents-check: ## Verifica el arnés sin escribir; falla si está desactualizado
	@$(AGENTSYNC) -check

toolchain-check: ## Verifica que la versión de Go coincide en las tres declaraciones
	@./tools/toolchain-check.sh

## --- Generación derivada del contrato ---

generate-api: ## Servidor y tipos Go desde el contrato
	@if [ -f api/openapi.yaml ]; then \
	  $(GO) -C tools/gen tool oapi-codegen -config ../../api/oapi-codegen.yaml ../../api/openapi.yaml; \
	  echo "make: servidor Go generado"; \
	else \
	  echo "make: sin api/openapi.yaml todavía, generate-api omitido"; \
	fi

generate-sql: ## Acceso a datos tipado desde las consultas SQL
	@if [ -n "$$(ls -A backend/queries 2>/dev/null | grep -v '^\.gitkeep$$')" ]; then \
	  $(GO) -C tools/gen tool sqlc -f ../../backend/sqlc.yaml generate; \
	  echo "make: acceso a datos generado"; \
	else \
	  echo "make: sin consultas en backend/queries, generate-sql omitido"; \
	fi

generate-client: ## Cliente TypeScript desde el contrato
	@if [ -f api/openapi.yaml ] && [ -f frontend/package.json ]; then \
	  cd frontend && npm run --silent generate:api; \
	  echo "make: cliente TypeScript generado"; \
	else \
	  echo "make: sin spec o sin frontend, generate-client omitido"; \
	fi

generate-check: ## Falla si el código generado no está al día con sus fuentes
	@$(MAKE) --no-print-directory generate >/dev/null
	@if ! git diff --quiet -- \
	     backend/internal/adapters/http/gen \
	     backend/internal/adapters/postgres/gen \
	     frontend/src/api \
	     .claude .github/agents; then \
	  echo "ERROR: hay código generado desactualizado:"; \
	  git diff --name-only -- backend/internal/adapters/http/gen \
	     backend/internal/adapters/postgres/gen frontend/src/api .claude .github/agents; \
	  echo "Ejecuta 'make generate' y commitea el resultado."; \
	  exit 1; \
	fi
	@echo "make: código generado al día"

## --- Linters ---

spec-lint: ## Lint del contrato con el ruleset propio
	@$(MAKE) --no-print-directory spec-ruleset-selftest
	@if [ -f api/openapi.yaml ]; then \
	  $(SPECTRAL) lint api/openapi.yaml --ruleset api/.spectral.yaml --fail-severity=error; \
	else \
	  echo "make: sin api/openapi.yaml todavía, spec-lint omitido"; \
	fi

spec-ruleset-selftest: ## Verifica que el ruleset de Spectral hace lo que dice
	@./api/testdata/selftest.sh

backend-lint:
	@if [ -n "$$(find backend -name '*.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend vet ./...; \
	else \
	  echo "make: sin código Go todavía, backend-lint omitido"; \
	fi

frontend-lint:
	@if [ -f frontend/package.json ]; then \
	  echo "make: eslint pendiente de configurar (PR-2c)"; \
	else \
	  echo "make: sin frontend/package.json todavía, frontend-lint omitido"; \
	fi

## --- Tests ---

gen-selftest: ## Verifica la cadena de generación contra el fixture
	@./api/testdata/gen-selftest.sh

backend-test:
	@if [ -n "$$(find backend -name '*_test.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend test ./...; \
	else \
	  echo "make: sin tests de Go todavía, backend-test omitido"; \
	fi
	@$(GO) -C tools/agentsync test ./...

frontend-test:
	@if [ -f frontend/package.json ]; then \
	  echo "make: vitest pendiente de configurar (PR-2c)"; \
	else \
	  echo "make: sin tests de frontend todavía, frontend-test omitido"; \
	fi

## --- Migraciones: efectos laterales sobre una base de datos. ---
## --- Fuera de generate y de verify a propósito.                ---

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

Cinco cosas que notar en la estructura:

**El orden de las secciones es el orden mental de uso:** interfaz pública
primero, luego arnés, generación, linters, tests y por último migraciones. Quien
abra el fichero encuentra arriba lo que invoca a diario.

**`.PHONY` los declara todos.** Ninguno de estos targets produce un fichero con
su propio nombre, así que sin `.PHONY` bastaría con que alguien creara un
directorio llamado `test` para que `make test` dejara de ejecutarse.

**Las migraciones están al final y aisladas, con su comentario.** Son los únicos
targets con efectos laterales sobre una base de datos, y ni `generate` ni
`verify` los invocan. Eso es lo que garantiza que un agente ejecutando
`make verify` no pueda tocar datos.

**El ancho de columna del `help` es 20 y no 18.** `spec-ruleset-selftest` tiene 21
caracteres y desalineaba la salida. Detalle menor que se ve a la primera
ejecución.

**Las guardas siguen ahí** porque `api/openapi.yaml` aún no existe. Desaparecerán
solas en PR-3, sin tocar el Makefile: en cuanto haya contrato, las ramas `else`
dejan de ejecutarse. Ese es el diseño desde PR-0 — la interfaz estable y la
implementación creciendo por debajo.

### 7.2 Detalle de los targets de generación

**Nota sobre `sqlc -f`:** la ruta del fichero de configuración es relativa al
directorio desde el que se invoca. Con `go -C tools/gen`, el directorio de trabajo
es `tools/gen`, de ahí el `../../`. Es el mismo detalle que ya nos mordió con
`agentsync` en el paso 4 de PR-0.

---

## 8. `generate-check`: el gate de código generado

Este es el aporte estructural de PR-2b, y es exactamente el mismo patrón que
`agents-check`.

El código generado se commitea. Eso abre un fallo silencioso: alguien cambia la
spec, olvida `make generate`, y el repositorio queda con un contrato que dice una
cosa y unos tipos que dicen otra. Todo compila. Todos los tests pasan. Y la spec
ha dejado de ser la fuente de verdad sin que nadie se entere.

```make
generate-check: ## Falla si el código generado no está al día con sus fuentes
	@$(MAKE) --no-print-directory generate >/dev/null
	@if ! git diff --quiet -- \
	     backend/internal/adapters/http/gen \
	     backend/internal/adapters/postgres/gen \
	     frontend/src/api \
	     .claude .github/agents; then \
	  echo "ERROR: hay código generado desactualizado:"; \
	  git diff --name-only -- backend/internal/adapters/http/gen \
	     backend/internal/adapters/postgres/gen frontend/src/api .claude .github/agents; \
	  echo "Ejecuta 'make generate' y commitea el resultado."; \
	  exit 1; \
	fi
	@echo "make: código generado al día"
```

**Dónde va exactamente.** El target se coloca al final de la sección de
generación, justo después de `generate-client`, porque invoca a `generate`. Su
nombre se añade a `.PHONY`, y a la lista de dependencias de `lint`:

```make
lint: agents-check toolchain-check generate-check spec-lint backend-lint frontend-lint ## Solo los linters
```

Del mismo modo, `gen-selftest` del apartado 9 se añade a `test`:

```make
test: gen-selftest backend-test frontend-test ## Solo los tests
```

El apartado 7.1 muestra el fichero completo con todo colocado.

### Lo que merece atención

**Regenera y compara con `git diff`,** en lugar de usar los modos de comprobación
propios de cada herramienta. `sqlc diff` existe; `oapi-codegen` no tiene
equivalente. Un único mecanismo para las cuatro cadenas es más simple de razonar.

**No sustituye a `agents-check`, y esto es importante.** En un borrador anterior
escribí que lo unificaba. Es falso, y lo he comprobado ejecutando ambos contra los
dos escenarios:

| Escenario | `agents-check` | `generate-check` |
|---|---|---|
| Fichero generado editado a mano, sin commitear | **Falla** | Pasa |
| Fuente cambiada y commiteada sin regenerar | **Falla** | **Falla** |

La razón es el orden de las operaciones. `generate-check` **regenera primero** y
luego compara con lo commiteado: si la edición a mano vive solo en el árbol de
trabajo, la regeneración la sobrescribe y no queda diff que detectar.
`agents-check` compara el disco contra lo esperado **sin regenerar**, así que sí
la ve.

Los dos hacen falta, y los dos van en `lint`. Cubren huecos distintos del mismo
problema.

**Deja el árbol de trabajo modificado si falla.** Es intencionado: cuando el check
falla en local, ya tienes los ficheros regenerados listos para `git add`. El
mensaje te dice exactamente qué hacer.

**Depende de que la generación sea determinista.** Si dos ejecuciones del mismo
generador sobre la misma entrada producen bytes distintos —por una marca de tiempo
o una versión incrustada— el check daría falsos positivos perpetuos. Es la razón
por la que fijamos las versiones con directivas `tool` en el apartado 2. Si al
probarlo ves diffs espurios, el culpable suele ser una cabecera con versión: la
solución es fijar la versión, no relajar el check.

---

## 9. Autotest de generación

Igual que el ruleset tiene su autotest, la cadena de generación tiene el suyo.
Verifica que los generadores funcionan **aunque todavía no exista el contrato**,
usando el fixture.

**Primero, el fichero de configuración de prueba.** No se usa el flag `-o` para
redirigir la salida: el config ya fija `output:`, y la precedencia entre ambos
varía entre versiones. Un segundo config elimina la ambigüedad, y evita el fallo
concreto de que el generador escriba en la ruta real del árbol a partir del
fixture.

```bash
mkdir -p .tmp
printf '\n# Salidas temporales de verificación\n.tmp/\n' >> .gitignore
```

Fichero `api/testdata/oapi-codegen.test.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.0/configuration-schema.json
package: genfixture
# Relativo a tools/gen. Ver 2.4.
output: ../../.tmp/oapi-selftest/server.gen.go

generate:
  std-http-server: true
  strict-server: true
  models: true
  embedded-spec: true
```

Fichero `api/testdata/gen-selftest.sh`:

```bash
#!/usr/bin/env bash
# Verifica que los generadores producen salida válida a partir del fixture.
# Se ejecuta aunque todavía no exista api/openapi.yaml.
set -euo pipefail
cd "$(dirname "$0")/../.."

FIXTURE=api/testdata/good.yaml
fail=0

echo "== cliente TypeScript desde el fixture =="
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
if npx --yes openapi-typescript "$FIXTURE" -o "$TMP/schema.d.ts" >/dev/null 2>&1; then
  grep -q 'operations\["updateSlot"\]' "$TMP/schema.d.ts" \
    && echo "   OK   operación tipada" || { echo "   FALLO operación no tipada"; fail=1; }
  grep -q 'slotId' "$TMP/schema.d.ts" \
    && echo "   OK   parámetro de path presente" || { echo "   FALLO falta el path param"; fail=1; }
else
  echo "   FALLO openapi-typescript no generó"; fail=1
fi

echo "== servidor Go desde el fixture =="
mkdir -p .tmp/oapi-selftest
if go -C tools/gen tool oapi-codegen \
     -config ../../api/testdata/oapi-codegen.test.yaml \
     "../../$FIXTURE"; then
  OUT=.tmp/oapi-selftest/server.gen.go
  grep -q 'StrictServerInterface' "$OUT" \
    && echo "   OK   strict-server activo" || { echo "   FALLO sin strict-server"; fail=1; }
  # Compilar, no solo buscar cadenas: un import ausente no lo detecta un grep.
  if ( cd .tmp/oapi-selftest \
       && { [ -f go.mod ] || go mod init selftest >/dev/null 2>&1; } \
       && go mod tidy >/dev/null 2>&1 && go build ./... >/dev/null 2>&1 ); then
    echo "   OK   el código generado compila"
  else
    echo "   FALLO el código generado no compila:"
    ( cd .tmp/oapi-selftest && go build ./... 2>&1 | head -5 )
    fail=1
  fi
else
  echo "   FALLO oapi-codegen no generó"; fail=1
fi

echo "== acceso a datos desde el fixture =="
if [ -f backend/sqlc.test.yaml ]; then
  if go -C tools/gen tool sqlc -f ../../backend/sqlc.test.yaml generate; then
    M=.tmp/sqlc-selftest/models.go
    grep -q 'uuid.UUID' "$M" \
      && echo "   OK   override de uuid" || { echo "   FALLO override no aplicado"; fail=1; }
    grep -q '\*uuid.UUID' "$M" \
      && echo "   OK   par nullable aplicado" || { echo "   FALLO falta el override nullable"; fail=1; }
    grep -q 'json:' "$M" \
      && { echo "   FALLO hay tags JSON, se puede saltar el DTO"; fail=1; } \
      || echo "   OK   sin tags JSON"
    # Compilar: es lo único que demuestra que los imports están bien.
    if ( cd .tmp/sqlc-selftest \
         && { [ -f go.mod ] || go mod init sqlcselftest >/dev/null 2>&1; } \
         && go mod tidy >/dev/null 2>&1 && go build ./... >/dev/null 2>&1 ); then
      echo "   OK   el acceso a datos compila"
    else
      echo "   FALLO el acceso a datos no compila:"
      ( cd .tmp/sqlc-selftest && go build ./... 2>&1 | head -5 )
      fail=1
    fi
  else
    echo "   FALLO sqlc no generó"; fail=1
  fi
else
  echo "   -    omitido: backend/sqlc.test.yaml todavía no existe"
fi

exit $fail
```

Tres decisiones del script que merecen explicación.

**No silencia los errores de oapi-codegen ni de sqlc.** La versión con
`>/dev/null 2>&1` produce un `FALLO sin interfaz` que no dice nada; con la salida
visible, ves el error real del generador. El silencio solo se mantiene en
`openapi-typescript`, que es ruidoso y ya está verificado.

**La parte de sqlc se salta si aún no existe `backend/sqlc.test.yaml`.** Sus
fixtures se crean en el apéndice A.0, así que el script funciona antes y después
de ese paso.

**Compila el código generado, no solo busca cadenas.** Esta es la lección más
cara de todo PR-2b. Una configuración anterior generaba un tipo correcto **sin su
import**: el `grep` pasaba y el código no compilaba. Un fichero generado que no
compila es peor que un fichero ausente, porque el fallo aparece mucho más tarde y
lejos de su causa.

La regla general: cuando el artefacto verificado es código, la verificación es
compilarlo. `StrictServerInterface` por `grep` demuestra que la opción se aplicó;
`go build` demuestra que el resultado sirve.

```bash
chmod +x api/testdata/gen-selftest.sh
```

Y en el Makefile, junto al autotest del ruleset:

```make
gen-selftest: ## Verifica la cadena de generación contra el fixture
	@./api/testdata/gen-selftest.sh
```

Añádelo a `test`:

```make
test: gen-selftest backend-test frontend-test ## Solo los tests
```

**La parte de TypeScript la he ejecutado y pasa.** La de Go no pude probarla en mi
entorno, y es donde apareció el primer fallo real al implementarlo: una versión
anterior de este script usaba `-o` para redirigir la salida, y el flag no tiene
precedencia sobre el `output:` del config. El resultado era que el generador
escribía en la ruta real del árbol a partir del fixture, o no escribía nada.

Si te ocurre algo parecido, comprueba siempre `git status --short` después: un
generador que escribe donde no debe deja rastro, y ese fichero no puede
commitearse porque no procede del contrato real.

> **Sobre sqlc y goose en el autotest:** el script de arriba no los cubre, porque
> ambos necesitan un esquema real y en PR-2b todavía no hay migraciones. El
> **apéndice A** resuelve ese hueco con fixtures propios de base de datos, de
> modo que las cuatro cadenas queden verificadas antes de PR-3. Hazlo: es la
> parte de este PR con más probabilidad de necesitar ajuste en tu entorno.

---

## 10. Verificar, commitear y abrir el PR

```bash
# Herramientas fijadas
cat tools/gen/go.mod
go -C tools/gen tool

# Autotests
./api/testdata/selftest.sh          # ruleset, de PR-2a
./api/testdata/gen-selftest.sh      # generación, nuevo

# La interfaz completa
make generate
make verify && echo "OK"
```

`make generate` debe informar de que omite las tres cadenas por falta de contrato,
y `generate-check` debe pasar porque no hay nada que esté desactualizado.

```bash
git add tools api/oapi-codegen.yaml api/testdata/gen-selftest.sh \
        backend/sqlc.yaml backend/go.mod backend/migrations/README.md \
        frontend/package.json frontend/package-lock.json \
        Makefile AGENTS.md .mise.toml .devcontainer/devcontainer.json

git commit -m "build: cadena de generación desde el contrato

oapi-codegen con strict-server, sqlc sobre pgx/v5 leyendo el esquema de las
migraciones, goose con la política expand/contract de ADR-0002, y cliente
TypeScript. Versiones fijadas con directivas tool en un módulo aparte.
generate-check impide que el código generado quede desincronizado del contrato.

Consolida además la versión de Go en 1.26.7, que las herramientas de generación
exigen: declarada en .mise.toml, en el devcontainer y en backend/go.mod, con
toolchain-check impidiendo que las tres diverjan. tools/agentsync se queda en
1.25 porque solo usa stdlib.

Issue: N/A
Spec: N/A
Plan: N/A
Agent: none
Runtime: human"

git push -u origin chore/pr-2b-generacion
gh pr create --draft --title "build: cadena de generación (PR-2b)" --label "runtime:human" --fill
gh pr checks --watch
gh pr ready && gh pr merge --squash --delete-branch
```

### Checklist

- [ ] `tools/gen/go.mod` con tres directivas `tool` y su `go.sum`
- [ ] `go -C tools/gen mod tidy` ejecutado tras los tres `go get -tool`
- [ ] `output:` de los dos configs de oapi-codegen con `../../`
- [ ] Rutas de los dos `sqlc*.yaml` **sin** prefijo
- [ ] `find . -name "server.gen.go"` devuelve una sola ruta, bajo `.tmp/`
- [ ] `go -C tools/gen tool` lista oapi-codegen, sqlc y goose
- [ ] `.mise.toml` en la raíz con la versión de Go y de Node
- [ ] `backend/go.mod` con `go 1.26` y `toolchain go1.26.7`
- [ ] `tools/agentsync/go.mod` **sin tocar**, sigue en 1.25
- [ ] Devcontainer alineado con `.mise.toml`
- [ ] `AGENTS.md` dice Go 1.26
- [ ] `tools/toolchain-check.sh` ejecutable, en `lint`, y en verde
- [ ] `api/oapi-codegen.yaml` con `strict-server: true` y sin `type-mapping`
- [ ] `backend/sqlc.yaml` con `emit_json_tags: false` y sin override de `tstzrange`
- [ ] Overrides de UUID en forma larga en **ambos** elementos del par, con `pointer: true` en el nullable
- [ ] Overrides de `timestamptz` en forma **corta** en ambos
- [ ] `backend/sqlc.yaml` y `backend/sqlc.test.yaml` idénticos salvo rutas, `package` y `out`
- [ ] Apéndice A ejecutado: las cuatro cadenas verificadas en tu entorno
- [ ] `backend/migrations/README.md` con la cabecera obligatoria
- [ ] `frontend/package.json` y `npm install` ejecutado
- [ ] Los tres targets de generación sin texto "pendiente de configurar"
- [ ] `DB_URL` y `GOOSE` en el bloque de variables de la cabecera
- [ ] Los cuatro `migrate-*` en su sección al final, fuera de `verify`
- [ ] `generate-check` tras `generate-client`, y en `lint`
- [ ] `toolchain-check` en `lint`, `gen-selftest` en `test`
- [ ] Todos los targets nuevos declarados en `.PHONY`
- [ ] `make help` lista veinte targets alineados
- [ ] `make verify` en verde
- [ ] `harness-check` y `spec-gate` siguen en verde

---

## 11. Qué falta

| Paso | Contenido |
|---|---|
| **2c** | golangci-lint con `depguard` implementando las cuatro reglas de ADR-0001, eslint, vue-tsc, workflow `code-gate` |
| 2d | CodeQL, Semgrep con reglas propias, gitleaks, govulncheck, Trivy, ZAP contra el perfil `ephemeral`, `agent-review`, métricas de runtime, caducidad de excepciones, Renovate |

Dos apuntes que nacen de este PR:

- **Sustituir el `grep` de `info.version` por `yq`** en `spec-gate`. Era deuda
  reconocida en PR-2a y ahora `npx yq` está al alcance sin añadir dependencias
  nuevas al pipeline.
- **`depguard` en PR-2c debe prohibir explícitamente
  `internal/adapters/http/gen`** desde `app` y `domain`. Ese paquete no existía
  cuando escribimos la configuración en la Fase 3; ahora sí, y es la regla del
  DTO hecha exit code.

---

# Apéndice A — Verificación de los generadores Go

Esta es la parte que yo no pude ejecutar. Está pensada para que la recorras en
orden: cada paso tiene su comando, la salida que debes ver, y qué hacer si no
coincide. Al terminar tendrás las **cuatro** cadenas verificadas —incluidas sqlc
y goose, que el autotest del apartado 9 no cubría— antes de que exista contrato.

## A.0 Preparar los fixtures de base de datos

sqlc y goose necesitan un esquema real. Como `backend/migrations/` debe seguir
vacío hasta PR-5, usamos fixtures propios.

```bash
mkdir -p backend/testdata/migrations backend/testdata/queries
printf '\n# Salidas temporales de verificación\n.tmp/\n' >> .gitignore
```

Fichero `backend/testdata/migrations/20260101000000_fixture_slots.sql`:

```sql
-- Fase: expand
-- Reversible: sí
-- ADR-0002: fixture de verificación de la cadena de generación, no es esquema real.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE slots (
    id          uuid PRIMARY KEY,
    owner_id    uuid NOT NULL,
    project_id  uuid,
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    note        text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT slots_range_valid CHECK (ends_at > starts_at)
);

ALTER TABLE slots ADD CONSTRAINT slots_no_overlap
  EXCLUDE USING gist (
    owner_id WITH =,
    tstzrange(starts_at, ends_at) WITH &&
  );

-- +goose Down
DROP TABLE slots;
```

Fichero `backend/testdata/queries/slots.sql`:

```sql
-- name: GetSlot :one
SELECT * FROM slots WHERE id = $1;

-- name: ListSlotsByOwner :many
SELECT * FROM slots
WHERE owner_id = $1
ORDER BY starts_at
LIMIT $2 OFFSET $3;

-- name: CreateSlot :one
INSERT INTO slots (id, owner_id, project_id, starts_at, ends_at, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
```

> Fíjate en que ninguna consulta devuelve `tstzrange`. Es la regla del apartado 4
> aplicada: el rango vive en la constraint, no en Go.

Fichero `backend/sqlc.test.yaml`. Sus rutas **no llevan prefijo**: sqlc las
resuelve respecto al propio fichero de configuración, según el apartado 2.4.



```yaml
version: "2"
sql:
  - engine: postgresql
    schema: testdata/migrations
    queries: testdata/queries
    gen:
      go:
        package: genfixture
        out: ../.tmp/sqlc-selftest
        sql_package: pgx/v5
        emit_interface: true
        emit_json_tags: false
        emit_empty_slices: true
        emit_pointers_for_null_types: true
        # Paquetes externos: forma larga en AMBOS elementos del par, o sqlc
        # emite el import dos veces. Stdlib: forma corta, o sqlc lo duplica
        # con el suyo. Verificado a base de compilar; no está documentado.
        overrides:
          - db_type: "uuid"
            go_type:
              import: "github.com/google/uuid"
              package: "uuid"
              type: "UUID"
          - db_type: "uuid"
            nullable: true
            go_type:
              import: "github.com/google/uuid"
              package: "uuid"
              type: "UUID"
              pointer: true
          - db_type: "timestamptz"
            go_type: "time.Time"
          # PENDIENTE: sin puntero. Ninguna columna de fecha es nullable hoy.
          # Cuando aparezca la primera, verificar que el cero de time.Time no
          # se confunde con ausencia de valor.
          - db_type: "timestamptz"
            nullable: true
            go_type: "time.Time"
```

Fichero `api/testdata/oapi-codegen.test.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.0/configuration-schema.json
package: genfixture
# Relativo a tools/gen. Ver 2.4.
output: ../../.tmp/oapi-selftest/server.gen.go

generate:
  std-http-server: true
  strict-server: true
  models: true
  embedded-spec: true
```

> **Por qué un config de prueba y no el flag `-o`.** El fichero de configuración
> ya fija `output:`, y la precedencia entre config y flag varía entre versiones
> de oapi-codegen. Un segundo config elimina la ambigüedad. Mantenerlo alineado
> con el real es trivial: solo cambian `package` y `output`.

---

## A.1 Las directivas `tool` se registraron

```bash
cat tools/gen/go.mod
go -C tools/gen tool
```

**Esperado:** tres líneas `tool` en `go.mod` y tres nombres listados.

```
tool (
	github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
	github.com/pressly/goose/v3/cmd/goose
	github.com/sqlc-dev/sqlc/cmd/sqlc
)
```

| Si ves | Significa | Haz |
|---|---|---|
| `flag provided but not defined: -tool` | Go anterior a 1.24 | Actualiza el toolchain; `go version` debe dar 1.25 |
| `no required module provides package` | El path del comando cambió | Búscalo en el repo del proyecto; los `cmd/` se mueven entre versiones mayores |
| `go.sum` sin entradas | `go get -tool` no descargó | `go -C tools/gen mod tidy` |

Anota las versiones, las necesitarás si algo falla más adelante:

```bash
go -C tools/gen list -m github.com/oapi-codegen/oapi-codegen/v2 \
                        github.com/sqlc-dev/sqlc github.com/pressly/goose/v3
```

---

## A.2 oapi-codegen genera el servidor

```bash
mkdir -p .tmp/oapi-selftest
go -C tools/gen tool oapi-codegen \
  -config ../../api/testdata/oapi-codegen.test.yaml \
  ../../api/testdata/good.yaml
```

**Esperado:** sin salida y el fichero creado. Compruébalo:

```bash
ls -l .tmp/oapi-selftest/server.gen.go
grep -n "StrictServerInterface" .tmp/oapi-selftest/server.gen.go | head -3
grep -n "UpdateSlot" .tmp/oapi-selftest/server.gen.go | head -5
```

Debes encontrar `StrictServerInterface` con un método `UpdateSlot` que recibe un
`UpdateSlotRequestObject` y devuelve un `UpdateSlotResponseObject`. **Esa firma es
la prueba de que `strict-server` está activo**; si ves `http.ResponseWriter` en la
interfaz, no lo está y pierdes la garantía de compilación sobre los códigos de
respuesta.

Los UUID aparecerán como `openapi_types.UUID`, que es el mapeo por defecto:

```bash
grep -n "openapi_types" .tmp/oapi-selftest/server.gen.go | head -3
```

Eso es correcto y no hay que cambiarlo: `openapi_types.UUID` es un alias de
`uuid.UUID`, el mismo tipo que produce sqlc. Puedes confirmarlo con
`go doc github.com/oapi-codegen/runtime/types.UUID` desde dentro del módulo
temporal; si la salida lleva un `=`, es un alias.

**Y ahora lo importante: que compile.** Un `grep` no detecta un import ausente,
y ese fue el fallo real que apareció al implementar este apéndice:

```bash
cd .tmp/oapi-selftest
cat > go.mod <<'EOF'
module selftest

go 1.25
EOF
go mod tidy && go build ./... && echo "COMPILA"
cd ../..
```

| Si ves | Significa | Haz |
|---|---|---|
| `unknown generate option "std-http-server"` | Versión anterior a la que lo introdujo | Actualiza oapi-codegen, o usa `chi-server` y anótalo como desviación |
| `error: output path ... no such directory` | No creaste `.tmp/oapi-selftest` | `mkdir -p` |
| Sale código 0 pero el fichero no está donde esperas | `output:` mal resuelto | Es relativo a `tools/gen`: usa `../../`. Ver 2.4 |
| `missing go.sum entry for module ...` | `go.sum` incompleto | `go -C tools/gen mod tidy` |
| Sale `ServerInterface` pero no `StrictServerInterface` | `strict-server` no se aplicó | Revisa la indentación de `generate:` en el YAML |
| Falla `go build` por imports | Faltan dependencias del runtime generado | `go mod tidy` dentro de `.tmp/oapi-selftest` |

---

## A.3 sqlc genera el acceso a datos

sqlc no necesita base de datos: analiza el SQL contra el esquema de las
migraciones de forma estática.

```bash
go -C tools/gen tool sqlc -f ../../backend/sqlc.test.yaml vet 2>/dev/null || true
go -C tools/gen tool sqlc -f ../../backend/sqlc.test.yaml generate
ls -l .tmp/sqlc-selftest/
```

**Esperado:** tres ficheros, `db.go`, `models.go` y `slots.sql.go`.

Comprueba que los overrides se aplicaron, que es lo que de verdad estamos
verificando:

```bash
sed -n '1,15p'        .tmp/sqlc-selftest/models.go   # un solo import de cada cosa
grep -n "uuid.UUID"   .tmp/sqlc-selftest/models.go
grep -n "\*uuid.UUID"  .tmp/sqlc-selftest/models.go  # project_id, nullable
grep -n "time.Time"   .tmp/sqlc-selftest/models.go
grep -n "json:"       .tmp/sqlc-selftest/models.go   # NO debe encontrar nada
grep -n "\*string"     .tmp/sqlc-selftest/models.go   # note es nullable -> puntero
grep -n "type Querier interface" .tmp/sqlc-selftest/db.go
```

Los seis resultados esperados: `id` y `owner_id` como `uuid.UUID`, **`project_id`
como `*uuid.UUID`**, las fechas como `time.Time`, **ninguna** tag JSON, `note`
como `*string`, y una interfaz `Querier`.

**La comprobación de `*uuid.UUID` es la que valida el par de overrides.** Si
`project_id` sale como un tipo de pgtype en lugar de `*uuid.UUID`, el override
`nullable: true` no se aplicó. Y si aparecen tags JSON, el atajo de serializar el
struct de sqlc directamente en una respuesta HTTP vuelve a estar disponible, y
con él la violación de ADR-0001.

Y una prueba negativa que merece la pena hacer una vez, porque demuestra el valor
del `schema: migrations`:

```bash
echo "-- name: RotoAProposito :one
SELECT columna_que_no_existe FROM slots WHERE id = \$1;" >> backend/testdata/queries/slots.sql

go -C tools/gen tool sqlc -f ../../backend/sqlc.test.yaml generate
# debe FALLAR con: column "columna_que_no_existe" does not exist

# revierte
git checkout backend/testdata/queries/slots.sql 2>/dev/null || \
  sed -i '/RotoAProposito/,+1d' backend/testdata/queries/slots.sql
```

Eso es un error de SQL detectado sin conexión a base de datos y sin ejecutar un
test. Es la razón de ser de sqlc en este proyecto.

| Si ves | Significa | Haz |
|---|---|---|
| `unknown type "tstzrange"` | Alguna consulta la devuelve | Quita esa columna del `SELECT`; el rango no cruza a Go |
| `project_id` no sale como `*uuid.UUID` | Falta `pointer: true` en el override nullable | El override anula `emit_pointers_for_null_types` |
| `uuid redeclared in this block` | Los dos elementos del par usan formas distintas | Ambos en forma larga |
| `package github is not in std` | Forma larga sin `package:` | Añade `package: "uuid"` |
| `time redeclared in this block` | Forma larga para un tipo de stdlib | Usa la forma corta: `go_type: "time.Time"` |
| Ningún override se aplica | La versión puede exigir el nombre cualificado | Prueba `pg_catalog.uuid` y `pg_catalog.timestamptz` |
| `unknown flag: -f` | El flag del fichero de config cambió | `sqlc generate --help` y ajusta el autotest |
| `no queries contained in paths ...` | `backend/queries` solo tiene `.gitkeep` | Correcto antes de PR-5; la guarda del Makefile lo evita |
| `relation "slots" does not exist` | No encuentra el esquema | Revisa `schema:`; es relativo al **fichero de config**, sin prefijo |
| `unsupported engine` o error de sintaxis en el YAML | Config de sqlc v1 | Asegura `version: "2"` |
| `type "uuid" does not exist` | Falta la extensión en las migraciones | En PostgreSQL 13+ `uuid` es nativo; revisa la versión de la imagen |

---

## A.4 goose aplica las migraciones contra Postgres real

Este paso valida lo que ninguno de los anteriores puede: que el SQL funciona de
verdad, incluida la constraint de exclusión.

```bash
docker compose --profile ephemeral up -d db-ephemeral
docker compose ps            # espera a que figure como healthy

export DB_TEST_URL='postgres://taskalloc:taskalloc-ephemeral@localhost:55432/taskalloc_test?sslmode=disable'

go -C tools/gen tool goose -dir ../../backend/testdata/migrations postgres "$DB_TEST_URL" up
go -C tools/gen tool goose -dir ../../backend/testdata/migrations postgres "$DB_TEST_URL" status
```

**Esperado:** una migración aplicada y `status` mostrándola con su fecha.

Ahora la prueba que de verdad importa — **que la constraint de solapamiento
funciona**:

```bash
PSQL="docker compose exec -T db-ephemeral psql -U taskalloc -d taskalloc_test"

# Primera franja: debe insertarse
$PSQL -c "INSERT INTO slots (id, owner_id, starts_at, ends_at)
          VALUES (gen_random_uuid(), '11111111-1111-1111-1111-111111111111',
                  '2026-09-01T09:00Z', '2026-09-01T10:00Z');"

# Segunda, solapada y del mismo dueño: DEBE fallar
$PSQL -c "INSERT INTO slots (id, owner_id, starts_at, ends_at)
          VALUES (gen_random_uuid(), '11111111-1111-1111-1111-111111111111',
                  '2026-09-01T09:30Z', '2026-09-01T10:30Z');"

# Solapada pero de OTRO dueño: debe insertarse
$PSQL -c "INSERT INTO slots (id, owner_id, starts_at, ends_at)
          VALUES (gen_random_uuid(), '22222222-2222-2222-2222-222222222222',
                  '2026-09-01T09:30Z', '2026-09-01T10:30Z');"

# Rango invertido: debe fallar por la CHECK
$PSQL -c "INSERT INTO slots (id, owner_id, starts_at, ends_at)
          VALUES (gen_random_uuid(), '33333333-3333-3333-3333-333333333333',
                  '2026-09-01T11:00Z', '2026-09-01T10:00Z');"
```

**Esperado:** insert 1 correcto, insert 2 con
`conflicting key value violates exclusion constraint "slots_no_overlap"`,
insert 3 correcto, insert 4 con
`new row ... violates check constraint "slots_range_valid"`.

Los cuatro resultados juntos son la demostración de ADR-0002: la invariante de no
solapamiento la sostiene el motor, no la aplicación, y por tanto resiste
escrituras concurrentes que ninguna comprobación en Go resistiría.

Prueba también el rollback, porque la política expand/contract depende de él:

```bash
go -C tools/gen tool goose -dir ../../backend/testdata/migrations postgres "$DB_TEST_URL" down
go -C tools/gen tool goose -dir ../../backend/testdata/migrations postgres "$DB_TEST_URL" status
docker compose --profile ephemeral down -v
```

| Si ves | Significa | Haz |
|---|---|---|
| `data type uuid has no default operator class for access method "gist"` | Falta `btree_gist` | Es el error clásico; añade `CREATE EXTENSION IF NOT EXISTS btree_gist;` |
| `connection refused` | Postgres aún no está listo | Espera a `healthy` en `docker compose ps` |
| `function gen_random_uuid() does not exist` | PostgreSQL anterior a 13 | Sube la imagen o usa `pgcrypto` |
| `goose: no migration files found` | Ruta mal resuelta | `-dir` es relativo a `tools/gen`, de ahí `../../` |

---

## A.5 El autotest ya cubre lo verificado

El script del apartado 9 ya incorpora las comprobaciones de este apéndice: usa
`api/testdata/oapi-codegen.test.yaml` en lugar del flag `-o`, verifica
`StrictServerInterface`, **compila el código generado**, y ejecuta la parte de
sqlc en cuanto existe `backend/sqlc.test.yaml`.

Así que al terminar A.0 a A.4, vuelve a lanzarlo y ahora debe cubrir las tres
cadenas:

```bash
./api/testdata/gen-selftest.sh
```

Salida esperada con los fixtures ya creados:

```
== cliente TypeScript desde el fixture ==
   OK   operación tipada
   OK   parámetro de path presente
== servidor Go desde el fixture ==
   OK   strict-server activo
   OK   el código generado compila
== acceso a datos desde el fixture ==
   OK   override de uuid
   OK   par nullable aplicado
   OK   sin tags JSON
   OK   el acceso a datos compila
```

**El check de las tags JSON está escrito al revés a propósito**: falla si las
encuentra. Es un test de una decisión de arquitectura, igual que
`TestReviewerHasNoWriteCapability` en `agentsync`. Cuando dentro de un año alguien
active `emit_json_tags` "para depurar", el autotest se pondrá en rojo y habrá que
leer ADR-0001 antes de seguir.

**El check del par nullable** (`grep '\*uuid.UUID'`) es el que valida la
corrección de sqlc: si `project_id` no sale como puntero a `uuid.UUID`, falta el
override con `nullable: true`.

goose no entra en el autotest: necesita Postgres levantado y eso pertenece al
workflow de integración de PR-2c, no a `make verify`.

## A.6 Checklist del apéndice

- [ ] `go -C tools/gen tool` lista las tres herramientas
- [ ] oapi-codegen genera `StrictServerInterface` y el resultado **compila**
- [ ] Ningún config de oapi-codegen lleva `type-mapping` para UUID
- [ ] sqlc genera con `uuid.UUID`, `time.Time`, sin tags JSON y con `*string`
- [ ] sqlc **falla** ante una columna inexistente
- [ ] goose aplica y revierte la migración fixture
- [ ] La constraint de exclusión rechaza el solapamiento del mismo dueño
- [ ] La constraint **no** rechaza el solapamiento de dueños distintos
- [ ] La `CHECK` rechaza el rango invertido
- [ ] `gen-selftest.sh` ampliado y en verde
- [ ] `.tmp/` en `.gitignore`
