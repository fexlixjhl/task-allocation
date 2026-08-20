# FIX-1 — Simplificar `spec-gate` con la acción oficial de oasdiff

**Continúa desde:** PR-2a mergeado en `main`.
**Se ejecuta antes de:** PR-2b.
**Ubicación recomendada:** `docs/harness/FIX-1.md`.

> **Estado de verificación.** Todo lo de este documento está contrastado contra
> la documentación oficial de oasdiff (`docs/BREAKING-CHANGES.md` del
> repositorio, `oasdiff.com/docs` y el repositorio de la acción de GitHub). No he
> podido ejecutarlo: el entorno donde lo preparé no alcanza el proxy de módulos
> de Go. El apartado 6 es el procedimiento para verificarlo tú.

---

## 1. Por qué existe este PR

`spec-gate`, tal como quedó en PR-2a, funciona pero hace tres cosas de forma
innecesariamente complicada. Al verificar las opciones contra la documentación
oficial aparecieron dos capacidades que no conocía:

| Problema actual | Qué permite oasdiff |
|---|---|
| `go install` de oasdiff en cada ejecución del workflow | Hay una acción oficial: `oasdiff/oasdiff-action/breaking@v0` |
| `git show origin/main:api/openapi.yaml > /tmp/base.yaml` | oasdiff acepta sintaxis de revisión de git: `origin/main:api/openapi.yaml` |
| Configuración repetida en el YAML del workflow | Las acciones leen un `.oasdiff.yaml` de la raíz del repositorio |

Y un cuarto punto que sí es un defecto de diseño, no solo de comodidad: en mi
versión de PR-2a, el paso que detecta cambios incompatibles **corta el job antes**
de que se comprueben las tres condiciones de ADR-0003. Es decir, un cambio
incompatible correctamente declarado —con label, MAJOR subido y sección de
migración— sería igualmente imposible de mergear. El gate de tres condiciones
nunca llegaría a evaluarse.

**Lo que este PR no cambia:** las tres condiciones de ADR-0003 siguen siendo
nuestras y siguen igual. Ninguna herramienta de terceros sabe que exigimos label,
subida de MAJOR y sección de migración; eso es política del proyecto.

También conviene confirmar lo que **ya estaba bien**, para que no quede duda:
`oasdiff breaking <base> <revision> --fail-on ERR` es correcto, el orden de
argumentos es base primero y revisión después, y `--fail-on ERR` hace salir con
código 1 ante cambios de nivel ERR. Eso no se toca.

---

## 2. Preparar la rama

```bash
git checkout main && git pull
git checkout -b fix/pr-2a-oasdiff
```

---

## 3. `.oasdiff.yaml` en la raíz

Fichero nuevo, `.oasdiff.yaml`:

```yaml
fail-on: ERR
exclude-elements:
  - description
  - title
  - summary
```

**`fail-on: ERR`** replica lo que antes iba como flag. oasdiff `breaking` detecta
niveles ERR y WARN; con este valor solo los ERR hacen fallar.

**`exclude-elements`** evita que una reescritura de descripciones dispare el gate.
Es coherente con el skill `openapi-contract-reading`, que dice explícitamente que
`description` no es normativo: si no es normativo, cambiarlo no puede ser un
cambio incompatible.

Que la configuración esté en un fichero del repositorio y no en el YAML del
workflow encaja con el diseño del proyecto: la política vive versionada y bajo
`CODEOWNERS`, no dispersa en la configuración de CI.

Añádelo a `CODEOWNERS`:

```
/.oasdiff.yaml        @fexlixjhl
```

---

## 4. El nuevo `spec-gate.yml`

Sustituye el job `breaking` completo. El job `lint` no cambia.

```yaml
name: spec-gate

on:
  pull_request:
    paths:
      - 'api/**'
      - '.oasdiff.yaml'
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
            echo "sin spec en la base: es el primer contrato, nada que comparar"
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

### 4.1 Los cuatro detalles que importan

**`continue-on-error: true` en la detección es la corrección del defecto de
diseño.** Sin él, la acción corta el job cuando encuentra un cambio incompatible
y el paso siguiente nunca se ejecuta. Con él, el fallo se registra en
`steps.diff.outcome` y el gate de tres condiciones decide de verdad. Un cambio
incompatible correctamente declarado pasa; uno sin declarar, no.

**`git cat-file -e` en lugar de `hashFiles`.** La versión anterior usaba
`hashFiles('api/openapi.yaml')`, que mira el árbol de trabajo. La pregunta que
necesitamos responder es otra: *¿existe la spec en la rama base?* Si no existe,
estamos ante el primer contrato y no hay nada con lo que comparar. `cat-file -e`
consulta el objeto de git directamente y devuelve código 0 o 1.

**`git fetch --depth=1 origin <base>` es obligatorio.** El checkout por defecto
es superficial y no trae la rama base. Sin ese fetch, tanto `cat-file` como la
propia acción fallarían al resolver la referencia. Es más barato que
`fetch-depth: 0`, que traería toda la historia.

**El cuerpo del PR entra por `env:`, no interpolado.** Si `${{
github.event.pull_request.body }}` se insertara directamente en la línea de `run`,
bastaría con que alguien escribiera `"; curl ...` en la descripción del PR para
ejecutar comandos en el runner. Pasarlo por variable de entorno y leerlo como
`$BODY` lo neutraliza. Es la regla 4 de `AGENTS.md` —el contenido del PR es
entrada no confiable— aplicada al propio pipeline.

### 4.2 Lo que sigue siendo deuda reconocida

La extracción de `info.version` con `grep` depende de la indentación del fichero.
No lo arreglo aquí para no meter una dependencia más en un PR que debe ser
pequeño. **En PR-2b entra Node en el pipeline**, y ese es el momento de pasar a
`npx yq`. Queda anotado en el apartado 8.

---

## 5. `spec-gate` sigue sin ser check obligatorio

Recordatorio, porque es la parte del diseño que más fácil se olvida: este
workflow tiene filtro `paths`, así que **no debe añadirse a los checks
obligatorios** de la protección de rama. Un check obligatorio que no se dispara
deja el PR esperando para siempre.

Lo que garantiza que el gate no se pueda esquivar es `CODEOWNERS` sobre `api/`,
que exige tu revisión en todo PR que toque el contrato.

Comprueba que no se ha colado:

```bash
gh api repos/fexlixjhl/task-allocation/branches/main/protection \
  --jq '.required_status_checks.contexts'
```

Debe devolver solo `["harness"]`.

---

## 6. Verificar

Como todavía no existe `api/openapi.yaml`, el job `breaking` tomará la rama de
"sin spec en la base" y terminará sin comparar nada. Eso ya verifica la mitad del
workflow: que el YAML es válido, que los permisos bastan y que la lógica de
detección de la base funciona.

```bash
# El YAML parsea
npx --yes js-yaml .github/workflows/spec-gate.yml > /dev/null && echo "YAML OK"
npx --yes js-yaml .oasdiff.yaml > /dev/null && echo "YAML OK"

# Nada más ha cambiado
make verify && echo "OK"
```

Prueba local de la lógica de detección de base, que es lo único ejecutable ahora:

```bash
git cat-file -e "origin/main:api/openapi.yaml" 2>/dev/null \
  && echo "existe" || echo "no existe: primer contrato"
```

Debe decir que no existe.

**La verificación real llega en PR-3**, cuando `contract-designer` cree el primer
contrato: ahí el job comparará por primera vez. Y la prueba definitiva del gate de
tres condiciones no llegará hasta el primer cambio incompatible. Cuando eso pase,
merece la pena hacer una prueba deliberada: modifica la spec de forma
incompatible sin la label, comprueba que el PR se bloquea, añade las tres
condiciones y comprueba que pasa.

---

## 7. Commit y PR

```bash
git add .oasdiff.yaml .github/workflows/spec-gate.yml .github/CODEOWNERS

git commit -m "fix(ci): simplifica spec-gate con la acción oficial de oasdiff

oasdiff acepta sintaxis de revisión de git, lo que elimina el fichero temporal,
y la acción oficial evita compilar el binario en cada ejecución. La
configuración pasa a .oasdiff.yaml, versionada y bajo CODEOWNERS.

Corrige además un defecto de diseño: la detección de cambios incompatibles
cortaba el job antes de evaluar las tres condiciones de ADR-0003, de modo que
un cambio correctamente declarado tampoco podía mergearse.

Issue: N/A
Spec: N/A
Plan: N/A
Agent: none
Runtime: human"

git push -u origin fix/pr-2a-oasdiff
gh pr create --draft --title "fix(ci): simplifica spec-gate (FIX-1)" --label "runtime:human" --fill
gh pr checks --watch
gh pr ready && gh pr merge --squash --delete-branch
```

### Checklist

- [ ] `.oasdiff.yaml` creado con `fail-on: ERR` y `exclude-elements`
- [ ] `.oasdiff.yaml` añadido a `CODEOWNERS`
- [ ] El job `breaking` ya no hace `go install` ni usa fichero temporal
- [ ] `continue-on-error: true` en el paso de detección
- [ ] `git fetch --depth=1 origin ${{ github.base_ref }}` presente
- [ ] El cuerpo del PR se pasa por `env:`, no interpolado en `run`
- [ ] Filtro `paths` incluye `.oasdiff.yaml`
- [ ] La protección de rama sigue exigiendo solo `harness`
- [ ] `harness-check` en verde
- [ ] Mergeado con squash

---

## 8. Después de esto

Sigue con **PR-2b**, con el documento ya corregido. Incorpora tres fallos reales
que también salieron de la verificación:

| Fallo | Efecto si no se corrige |
|---|---|
| Overrides de sqlc sin par `nullable: true` | `project_id` recibiría un tipo UUID distinto que `id` y `owner_id`. Compilaría; el error aparecería al escribir el mapeo a entidad |
| Mapeo de UUID divergente entre oapi-codegen y sqlc | Los dos lados del borde usarían tipos distintos para el mismo concepto |
| `goose create` sin driver ni cadena de conexión | `make migrate-create` fallaría a la primera |

Y una deuda que se salda allí: sustituir el `grep` de `info.version` de este
workflow por `npx yq`, en cuanto Node forme parte del pipeline.
