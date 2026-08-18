# PR-1 — Decisiones de arquitectura y artefactos de proceso

**Continúa desde:** PR-0 mergeado en `main`.
**Ubicación recomendada:** `docs/harness/PR-1.md`.

> Los cambios en skills están aplicados y verificados contra el presupuesto real
> con `agentsync`, y el `compose.yaml` está parseado y validado.

---

## Índice

1. [Qué contiene PR-1 y por qué](#1-qué-contiene-pr-1-y-por-qué)
2. [Preparar la rama](#2-preparar-la-rama)
3. [ADR-0001 — Arquitectura hexagonal](#3-adr-0001--arquitectura-hexagonal)
4. [ADR-0002 — Persistencia y política de migraciones](#4-adr-0002--persistencia-y-política-de-migraciones)
5. [ADR-0003 — Versionado del contrato](#5-adr-0003--versionado-del-contrato)
6. [ADR-0004 — Observabilidad y auditoría](#6-adr-0004--observabilidad-y-auditoría)
7. [Trazabilidad issue → operationId](#7-trazabilidad-issue--operationid)
8. [Plantillas RPI](#8-plantillas-rpi)
9. [Definition of Done](#9-definition-of-done)
10. [`compose.yaml`](#10-composeyaml)
11. [Regenerar, verificar y abrir el PR](#11-regenerar-verificar-y-abrir-el-pr)
12. [Qué falta](#12-qué-falta)

---

## 1. Qué contiene PR-1 y por qué

PR-0 montó el arnés: quién puede hacer qué. PR-1 escribe **las decisiones dentro
de las cuales van a operar los agentes**. Sigue sin haber código de producto.

El criterio para decidir qué entra aquí y qué puede esperar fue el que discutimos:
**una decisión va en PR-1 si su retrofit atraviesa el núcleo del código.** Todo lo
que sea fontanería —workflows, despliegue, configuración de herramientas— puede
llegar después sin coste. Lo que cambia la firma de cada caso de uso, no.

| Artefacto | Retrofit si se aplaza | Por eso está aquí |
|---|---|---|
| ADR-0001 hexagonal | Alto | Ya estaba en `AGENTS.md`; falta el documento con el razonamiento |
| ADR-0002 migraciones | **Irreversible** | Una migración destructiva ya aplicada no se rediseña |
| ADR-0003 versionado | Medio | Sin política, el primer cambio incompatible se cuela |
| ADR-0004 observabilidad y auditoría | **Alto** | Propagar contexto y auditar toca todos los casos de uso |
| Trazabilidad `Issue:` | Bajo, pero el historial pasado no se recupera | Cuanto antes, más historial cubierto |
| Plantillas RPI | Bajo | Las necesita `context-researcher` en PR-4 |
| Definition of Done | Bajo | Define "terminado" antes del primer entregable |
| `compose.yaml` | Bajo | Habilita testcontainers y el DAST de PR-2 |

**Nota sobre las ADRs: las escribes tú, no un agente.** Son el marco dentro del que
los agentes operan; que un agente redacte las reglas que le limitan invierte la
relación. Es la misma razón por la que `docs/adr/` está bajo `CODEOWNERS`.

Formato de todas las ADRs, consistente:

```
# ADR-NNNN: título
Estado · Fecha · Contexto · Decisión · Consecuencias · Alternativas descartadas
```

La sección de alternativas descartadas es la que hace útil una ADR dentro de dos
años: sin ella, quien la lea repetirá el debate que ya tuviste.

---

## 2. Preparar la rama

```bash
git checkout main && git pull
git checkout -b chore/pr-1-decisiones
mkdir -p docs/features/_template
```

---

## 3. ADR-0001 — Arquitectura hexagonal

Fichero `docs/adr/0001-arquitectura-hexagonal.md`:

```markdown
# ADR-0001: Arquitectura hexagonal en el backend

- **Estado:** aceptada
- **Fecha:** 2026-08-17

## Contexto

El backend se genera en gran parte a partir del contrato OpenAPI y lo escriben
agentes de codificación. Eso introduce dos riesgos que no existen igual en
desarrollo manual:

1. Un generador de código produce structs a partir de los esquemas de la spec.
   La ruta de menor resistencia es usarlos como entidades de dominio, lo que
   acopla el núcleo al formato de transporte.
2. Una instrucción de arquitectura en lenguaje natural ("mantén las capas
   separadas") se cumple parcialmente y se degrada sin que nadie lo note.

Necesitamos una arquitectura cuya conformidad sea verificable por herramienta,
no por revisión.

## Decisión

Arquitectura hexagonal (puertos y adaptadores) en `backend/internal`, con la
dirección de dependencias hacia dentro:

1. `internal/domain` no importa nada del proyecto. Solo stdlib.
2. `internal/app` importa `domain`. Nunca `adapters`.
3. `internal/adapters/*` importan `app` y `domain`. Nunca entre sí.
4. Solo `cmd/` importa `adapters`.

Los tipos generados desde OpenAPI son DTOs del adaptador HTTP: viven en
`internal/adapters/http/` y no cruzan hacia `app` ni `domain`. Lo mismo, en
sentido inverso, con los structs generados por sqlc.

La autorización vive en el caso de uso (`internal/app`), nunca en el handler.

Las cuatro reglas se verifican con `depguard` dentro de `golangci-lint`, y su
incumplimiento falla el build.

## Consecuencias

**Positivas**

- La conformidad arquitectónica es un exit code, no un juicio. Un agente no
  puede violarla y pasar el gate.
- La regla del DTO garantiza que toda entidad se construya por su constructor,
  y por tanto con sus invariantes aplicadas. Es un control de seguridad, no de
  estética: sin ella existiría una vía para tener una entidad en estado
  inválido.
- La autorización en el caso de uso sobrevive a nuevos puntos de entrada. Un
  consumidor de mensajes o un CLI futuros heredan el control; si estuviera en
  el handler, cada punto de entrada nuevo lo saltaría.
- El dominio es testeable sin base de datos ni HTTP.

**Negativas**

- Capa de mapeo DTO ↔ entidad que hay que escribir y mantener. Es trabajo
  mecánico y repetitivo; se acepta a cambio del aislamiento.
- Más ficheros y más indirección de la que pediría una app de este tamaño.
- Un puerto mal diseñado filtra el detalle igualmente: `Query(sql string)`
  cumple las cuatro reglas de import y rompe la abstracción. Eso no lo detecta
  `depguard`, y por eso está en la checklist de revisión.

## Alternativas descartadas

**Arquitectura por capas clásica.** No impone la dirección de dependencias de
forma verificable: la capa de servicio importando el repositorio concreto es
sintácticamente idéntica a importar su interfaz.

**Estructura plana por feature.** Más simple y muy razonable para una app de
este tamaño escrita a mano. Descartada porque no ofrece una frontera que un
linter pueda comprobar, que es el requisito que impone el desarrollo agéntico.

**Clean Architecture con más capas.** El aislamiento adicional no compensa la
ceremonia para el alcance de este proyecto.
```

### Lo que merece atención

La sección de contexto **no argumenta que hexagonal sea mejor arquitectura**.
Argumenta que es la que resuelve un problema específico de este proyecto: que las
reglas las tienen que cumplir agentes, y por tanto han de ser verificables por
máquina. Es la diferencia entre una ADR que documenta una preferencia y una que
documenta una decisión.

La negativa final —"un puerto mal diseñado filtra el detalle igualmente"— es
importante que esté escrita. Una ADR que solo lista ventajas es propaganda, y
además pierde la ocasión de explicar por qué existe el punto correspondiente en
la checklist de revisión.

---

## 4. ADR-0002 — Persistencia y política de migraciones

Fichero `docs/adr/0002-persistencia-postgres-sqlc.md`:

```markdown
# ADR-0002: PostgreSQL con sqlc, y migraciones expand/contract

- **Estado:** aceptada
- **Fecha:** 2026-08-17

## Contexto

El dominio central son franjas horarias con dueño, sobre las que se asignan
acciones. Dos requisitos condicionan la elección:

- Dos franjas del mismo miembro no pueden solaparse. Comprobar y después
  escribir desde la aplicación es una condición de carrera clásica que ningún
  test unitario detecta de forma fiable.
- Un calendario de equipo maneja zonas horarias de forma no trivial.

Además, el acceso a datos lo escriben agentes, lo que hace de la inyección SQL
un riesgo de primer orden si la capa de acceso admite concatenación.

## Decisión

**PostgreSQL** como motor. La exclusión de solapamientos se implementa en el
motor, no en la aplicación:

    ALTER TABLE slots ADD CONSTRAINT slots_no_overlap
      EXCLUDE USING gist (
        owner_id WITH =,
        tstzrange(starts_at, ends_at) WITH &&
      );

**sqlc** como capa de acceso. El SQL se escribe a mano en `backend/queries/` y
los tipos se generan. Prohibido construir SQL por concatenación o con
`fmt.Sprintf`.

**goose** para migraciones, en `backend/migrations/`, aplicadas por CI. Ningún
agente aplica migraciones contra un entorno con datos.

### Política de migraciones: expand/contract

1. Las migraciones son **forward-only** y **nunca se editan** una vez
   mergeadas. Un error se corrige con una migración nueva.
2. Todo cambio destructivo se reparte en al menos dos releases:
   **expand** (añadir), desplegar y migrar datos, y **contract** (eliminar) en
   una migración posterior.
3. Prohibido en la misma migración que introduce el uso: `DROP COLUMN`,
   `DROP TABLE`, añadir `NOT NULL` a una columna existente, o renombrar.
4. **Los renombrados no existen.** Se añade la columna nueva, se copian los
   datos, el código pasa a usarla, y la antigua se elimina en una migración
   posterior.
5. Los índices sobre tablas con datos se crean `CONCURRENTLY`.
6. Toda migración debe aplicarse correctamente sobre base vacía y sobre base
   poblada.
7. Una migración irreversible se marca explícitamente como tal en su
   cabecera, con el motivo.

## Consecuencias

**Positivas**

- La inyección SQL queda eliminada por construcción, no por revisión. La
  diferencia es que `security-reviewer` no tiene que buscar concatenaciones
  en cada PR: no se pueden escribir.
- Los structs generados por sqlc se quedan en el adaptador, igual que los
  DTOs de OpenAPI, coherente con ADR-0001.
- Expand/contract mantiene el rollback de despliegue siempre posible: la
  versión anterior del código sigue funcionando contra el esquema nuevo.
- La constraint de exclusión hace imposible el solapamiento incluso ante
  escrituras concurrentes.

**Negativas**

- SQL escrito a mano. Las consultas dinámicas complejas son incómodas con
  sqlc; si aparecen, se resuelven con consultas específicas, no con un
  constructor genérico.
- Expand/contract duplica el número de migraciones de cualquier cambio
  destructivo y obliga a un periodo con dos columnas conviviendo.
- Acoplamiento a PostgreSQL: `tstzrange` y `EXCLUDE` no son portables. Es
  aceptable, y la ADR lo hace explícito para que nadie lo descubra tarde.

## Alternativas descartadas

**ORM (GORM, ent).** Menos SQL a mano, pero tiende a filtrarse hacia el
dominio y hace opaco el SQL que acaba ejecutándose. Con agentes escribiendo el
código, la opacidad es un coste mayor que la comodidad.

**pgx en crudo.** Máximo control y ninguna barrera contra la concatenación.

**Exclusión de solapamientos en la aplicación con bloqueo explícito.**
Funciona, requiere disciplina en cada punto de escritura y es exactamente el
tipo de invariante que un agente puede omitir sin que ningún test lo revele.
```

### Lo que merece atención

**La política expand/contract es lo único de PR-1 que es literalmente
irreversible si se aplaza.** Una migración que borra una columna y ya se aplicó
no se puede rediseñar: el dato no está. Establecer la regla antes de la primera
migración cuesta cero; establecerla después de la quinta cuesta una recuperación
desde backup.

**El punto 4, "los renombrados no existen", parece extremo.** Es el error más
frecuente y el que peor se comporta: `ALTER TABLE ... RENAME COLUMN` es atómico
en la base, pero deja la versión anterior del código —la que aún se está
ejecutando durante el despliegue— consultando una columna que ya no existe.

---

## 5. ADR-0003 — Versionado del contrato

Fichero `docs/adr/0003-versionado-del-contrato.md`:

```markdown
# ADR-0003: Versionado semántico del contrato OpenAPI

- **Estado:** aceptada
- **Fecha:** 2026-08-17

## Contexto

El contrato es la fuente de verdad y de él se derivan el servidor y el cliente.
Sin una política explícita de evolución, un cambio incompatible entra en un PR
que parece inocuo —endurecer un `pattern`, quitar un campo de una respuesta— y
rompe consumidores en silencio.

Hoy el único consumidor es nuestro propio frontend, lo que hace tentador no
versionar. Es precisamente cuando conviene establecer el mecanismo: cuando
todavía no duele.

## Decisión

**Versionado semántico en `info.version`.**

| Cambio | Nivel |
|---|---|
| Eliminar una operación, un campo de respuesta o un código de respuesta | MAJOR |
| Añadir un campo obligatorio a una petición | MAJOR |
| Endurecer validación: `pattern`, `maxLength`, `enum` más estrecho, tipo distinto | MAJOR |
| Endurecer `security` o `x-required-scope` de una operación existente | MAJOR |
| Añadir una operación, un campo opcional o un código de respuesta nuevo | MINOR |
| Relajar validación, cambiar `description`, `example` o `summary` | PATCH |

**Prefijo de ruta `/v1` desde la primera operación.** No versionamos por
cabecera ni por parámetro. El coste hoy es una línea en `servers`; el coste de
añadirlo después es reescribir todas las rutas y todos los clientes.

**Gate de cambios incompatibles.** `oasdiff` compara la spec del PR con la de
`main` en el `spec-gate`. Si detecta un cambio incompatible, el PR falla salvo
que se cumplan **las tres** condiciones:

1. La label `contract:breaking` está aplicada.
2. `info.version` sube de MAJOR en el mismo PR.
3. El cuerpo del PR contiene una sección "Migración" describiendo qué deben
   hacer los consumidores.

**Deprecación antes de eliminar.** Una operación o campo que vaya a
desaparecer se marca primero con `deprecated: true` y una extensión
`x-sunset: YYYY-MM-DD` con al menos 30 días de margen. La eliminación es un
PR posterior a esa fecha.

## Consecuencias

**Positivas**

- Un cambio incompatible deja de ser un accidente y pasa a ser una decisión
  con tres pasos deliberados. Ninguno de los tres se ejecuta por descuido.
- El versionado de ruta permite convivencia de `/v1` y `/v2` si algún día
  hace falta, sin rediseñar nada.
- La tabla de niveles da a `contract-designer` un criterio mecánico en lugar
  de un juicio.

**Negativas**

- `/v1` es ceremonia visible para un proyecto con un único consumidor.
- `oasdiff` produce algún falso positivo en refactorizaciones de `$ref` que
  no cambian el esquema efectivo. Se resuelven con la label y una nota, que es
  precisamente el mecanismo previsto.

## Alternativas descartadas

**Sin versionado, contrato siempre compatible.** Funciona hasta el primer
cambio incompatible inevitable, momento en el que no hay mecanismo.

**Versionado por cabecera.** Más limpio conceptualmente, peor para depurar y
para cachear, y añade complejidad al cliente generado.

**Versionar por operación.** Máxima granularidad y una carga cognitiva que no
compensa a esta escala.
```

### Lo que merece atención

**El gate de tres condiciones es el patrón de "excepción con fricción" que ya
usamos en `exceptions.md`.** No prohíbe el cambio incompatible —prohibirlo sería
irreal— sino que lo hace imposible de hacer por accidente. Un agente no aplica una
label, no sube un MAJOR y no escribe una sección de migración sin darse cuenta de
lo que está haciendo.

**`/v1` desde el día uno es la decisión más barata de todo el proyecto.** Una
línea en `servers`. Añadirla después significa tocar todas las rutas de la spec,
todos los handlers y todo el cliente generado.

---

## 6. ADR-0004 — Observabilidad y auditoría

Fichero `docs/adr/0004-observabilidad-y-auditoria.md`:

```markdown
# ADR-0004: Observabilidad, auditoría y alcance de operación

- **Estado:** aceptada
- **Fecha:** 2026-08-17

## Contexto

El proyecto no tiene entorno de producción y no está previsto que lo tenga. La
tentación es dejar observabilidad y auditoría fuera de alcance por completo.

El problema es que dos de esas piezas atraviesan el núcleo. La propagación de
contexto y el registro de auditoría afectan a la firma y al cuerpo de cada caso
de uso; añadirlos después del décimo endpoint significa revisarlos todos. El
despliegue, en cambio, es fontanería que se puede añadir sin tocar el dominio.

Esta ADR separa deliberadamente lo que se decide ahora de lo que se deja fuera.

## Decisión

### Logging

`log/slog` de la stdlib, handler JSON. **El dominio no registra nada**: la
regla 1 de import de ADR-0001 ya lo garantiza, porque un logger inyectado
desde fuera violaría la frontera.

El logging de peticiones vive en el adaptador HTTP. Los casos de uso no
registran: devuelven errores con la información suficiente para que el
adaptador decida qué registrar y qué devolver.

### Propagación de contexto

Un middleware del adaptador HTTP genera un identificador de correlación por
petición y lo guarda en el `context.Context`. Todo caso de uso y todo puerto
recibe `ctx context.Context` como primer parámetro, sin excepciones.

Si la petición trae `X-Request-Id`, se acepta **solo** si es un UUID válido;
en cualquier otro caso se genera uno nuevo. Un identificador suministrado por
el cliente y no validado permite envenenar los logs y falsificar correlaciones.

El identificador se devuelve al cliente en la respuesta de error, de forma que
un usuario pueda reportar un fallo sin que el mensaje contenga detalle interno.

### Auditoría

La auditoría **no es observabilidad**: es un requisito de seguridad (ASVS V7)
y, en este dominio, información de negocio. Se implementa como un puerto
explícito, `ports.AuditLog`, no como logging.

Todo caso de uso que muta estado lo invoca **después de autorizar y antes de
persistir**. Se auditan:

- Creación, modificación y eliminación de franjas, proyectos y asignaciones.
- Toda denegación de autorización, con o sin mutación.

Cada entrada registra: quién (identificador de miembro), qué operación
(`operationId`), sobre qué recurso, cuándo, y el identificador de correlación.

**Nunca** se registran tokens, cabeceras de autorización, ni datos personales
más allá del identificador de miembro.

El almacenamiento es una tabla `audit_log` en PostgreSQL, **append-only**: el
rol de aplicación tiene `INSERT` y `SELECT`, y no tiene `UPDATE` ni `DELETE`.

### Fuera de alcance, explícitamente

- Métricas y trazas distribuidas (OpenTelemetry). La propagación de contexto
  que sí decidimos deja la puerta abierta a añadirlas sin refactor.
- Despliegue continuo, entornos gestionados y rollback automatizado.
- Alertado y respuesta a incidentes.

Lo que sí existe es un entorno efímero por `compose.yaml` para tests de
integración y para el DAST de PR-2.

## Consecuencias

**Positivas**

- El `ctx` en toda firma es lo único caro de retrofitear, y queda resuelto
  antes del primer caso de uso.
- La auditoría como puerto la hace testeable con un doble, y la separa
  conceptualmente del logging, que es donde suele diluirse.
- `append-only` a nivel de permisos de base de datos hace que el registro
  resista incluso a un fallo de lógica de aplicación.
- El alcance fuera queda documentado como decisión, no como olvido.

**Negativas**

- `ctx` como primer parámetro en todas partes es ruido sintáctico, incluso
  donde hoy no se usa.
- La tabla de auditoría crece sin política de retención definida. Aceptable
  sin producción; sería una decisión pendiente en un proyecto real.
- Un puerto más que implementar y cablear desde el primer caso de uso.

## Alternativas descartadas

**Auditoría como logging estructurado con un campo `audit: true`.** Más barato
y confunde dos cosas con requisitos distintos: los logs se rotan y se pierden,
un registro de auditoría no debe poder perderse.

**Auditoría por triggers de base de datos.** Captura el cambio pero no el
intento denegado, que es justo el evento de seguridad más interesante, y
tampoco el `operationId` ni la correlación.

**Dejar la observabilidad entera fuera.** Descartada por el coste de retrofit
de la propagación de contexto.
```

### Lo que merece atención

**La distinción entre auditoría y observabilidad es la idea central de esta
ADR.** Se confunden constantemente y tienen requisitos opuestos: los logs se
muestrean, se rotan y se pierden por diseño; un registro de auditoría no debe
poder perderse. Tratarlos como lo mismo produce sistemas donde el evento de
seguridad relevante desapareció con la rotación.

**"Toda denegación de autorización se audita"** es la línea más valiosa. Los
intentos fallidos son el evento de seguridad interesante, y el que ningún trigger
de base de datos puede capturar: la mutación nunca llegó a ocurrir.

**El `append-only` por permisos, no por convención.** Si el rol de aplicación
carece de `UPDATE` y `DELETE` sobre `audit_log`, ni un fallo de lógica ni un
agente confundido pueden borrar el rastro. **[OWASP: ASVS V7 · SDL: verificación]**

**La validación de `X-Request-Id`** es un detalle pequeño con nombre propio: log
injection. Un identificador arbitrario suministrado por el cliente y volcado sin
validar en un log estructurado permite falsificar entradas.

---

## 7. Trazabilidad issue → operationId

Este es el hueco de SDD que detectamos: teníamos código → spec y código → plan,
pero no requisito → contrato.

### 7.1 Extensión en el contrato

Cada operación declara qué requisito la motivó. Añade a
`skills/openapi-contract-reading.md`, en la sección de extensiones, tras el
párrafo de `x-owner-check`:

```markdown
`x-source-issue`: entero. Número del issue de requisito que motivó la
operación. Es el eslabón que conecta negocio con contrato: desde cualquier
`operationId` se puede llegar al requisito que lo justifica.
```

En la lista de lectura, amplía el punto 5:

```markdown
5. Las extensiones propias del proyecto: `x-required-scope`, `x-owner-check`
   y `x-source-issue`.
```

Y en "Señales de contrato incompleto", añade una línea:

```markdown
- Operación sin `x-source-issue`.
```

### 7.2 Trailer en los commits

En `skills/github-pr-protocol.md`, el bloque de formato de commit pasa a:

```
    Issue: #17
    Spec: api/openapi.yaml@a1b2c3d
    Plan: docs/features/slot-creation/PLAN.md
    Agent: backend-builder
    Runtime: claude-code
```

Y el párrafo siguiente:

```markdown
Los cinco trailers son obligatorios. `Issue:` es el requisito de negocio que
origina el cambio; `Spec:` lleva el SHA corto del commit de la spec contra la
que trabajas, no la rama.
```

### 7.3 Campo en la plantilla de PR

En `.github/pull_request_template.md`, sustituye el bloque final de trailers:

```markdown
Issue: #<n>
Spec: api/openapi.yaml@<sha>
Plan: docs/features/<slug>/PLAN.md
Agent: <nombre-del-agente|none>
Runtime: <claude-code|copilot|human>
```

### 7.4 La cadena completa

Con esto, la trazabilidad queda cerrada en los dos sentidos:

```
Issue #17  ──x-source-issue──▶  operationId createSlot
    ▲                                      │
    │                                      ▼
 Issue: #17  ◀──trailer──  commit  ──Spec:──▶  openapi.yaml@sha
                              │
                              └──Plan:──▶  PLAN.md ──▶ RESEARCH.md
```

Desde cualquier línea de código llegas al requisito de negocio, y desde cualquier
requisito llegas a las operaciones que lo implementan. En PR-2, una regla de
Spectral hará obligatorio `x-source-issue`, con lo que dejará de depender de la
disciplina del agente.

> **Sobre el historial pasado.** Los commits de PR-0 no llevan `Issue:` y no se
> van a reescribir: reescribir historia mergeada es peor que un hueco de
> trazabilidad conocido. La cadena empieza a ser completa desde PR-1.

---

## 8. Plantillas RPI

Los ficheros que `context-researcher` y `change-planner` copiarán en PR-4. El
skill `rpi-artifacts` ya describe su contenido; estos son los ficheros reales.

Fichero `docs/features/_template/RESEARCH.md`:

```markdown
# Research: <slug>

- **Issue:** #<n>
- **Spec:** api/openapi.yaml@<sha>

## Operaciones afectadas

| operationId | método y ruta | ¿existe ya? |
|---|---|---|

## Esquemas implicados

<!-- Nombre, campos relevantes y restricciones de validación declaradas. -->

## ADRs aplicables

<!-- Cuáles, y qué imponen sobre este cambio en concreto. -->

## Amenazas relevantes

<!-- De docs/threats/. Si no hay modelo para estas operaciones, dilo. -->

## Ficheros que se tocarán

| fichero | capa hexagonal |
|---|---|

## Preguntas abiertas

<!-- Contradicciones, huecos y ambigüedades. No las resuelvas aquí. -->
```

Fichero `docs/features/_template/PLAN.md`:

```markdown
# Plan: <slug>

- **Research:** ./RESEARCH.md
- **Issue:** #<n>
- **Aprobado por:** <humano>  ·  **Fecha:** <YYYY-MM-DD>

## Alcance

<!-- Qué entra. -->

## Fuera de alcance

<!-- Qué no entra, explícitamente. -->

## Pasos

| # | Acción | Ficheros | Capa | Criterio de aceptación |
|---|---|---|---|---|

## Criterios de seguridad

| Amenaza | Control ASVS | Implementación | Verificación |
|---|---|---|---|
```

**Por qué `_template` con guion bajo.** Ordena primero alfabéticamente en
`docs/features/`, y deja claro de un vistazo que no es una feature. Cuando haya
quince features, esa convención evita que alguien lo trate como una más.

**Los comentarios HTML son instrucciones para quien rellena** y desaparecen al
renderizar. Un agente que copia la plantilla los lee; un humano que lee el
documento terminado no los ve.

---

## 9. Definition of Done

Fichero `docs/DEFINITION-OF-DONE.md`:

```markdown
# Definition of Done

Una feature está terminada cuando **todo** lo siguiente es cierto. Los checks
automáticos cubren parte; el resto se verifica en la revisión.

## Contrato

- [ ] La operación existe en `api/openapi.yaml` con `operationId`, `security`,
      `x-required-scope`, `x-source-issue` y, si el recurso tiene dueño,
      `x-owner-check`.
- [ ] Todos los códigos de respuesta posibles están declarados.
- [ ] Spectral pasa limpio y `oasdiff` no reporta cambios incompatibles no
      declarados.

## Proceso

- [ ] Existe `RESEARCH.md` y un `PLAN.md` aprobado y mergeado en `main`.
- [ ] Cada paso implementado se corresponde con un paso del plan.
- [ ] Lo que quedó fuera de alcance sigue fuera, o el plan se enmendó.

## Código

- [ ] Respeta las cuatro reglas de import de ADR-0001; `depguard` en verde.
- [ ] Ningún DTO generado cruza hacia `app` o `domain`.
- [ ] La autorización está en el caso de uso, antes de cualquier efecto
      lateral.
- [ ] Todo caso de uso que muta estado invoca `ports.AuditLog`.
- [ ] Toda firma recibe `ctx context.Context` como primer parámetro.
- [ ] Las migraciones cumplen expand/contract (ADR-0002).

## Tests

- [ ] Tests unitarios del dominio, sin base de datos.
- [ ] Tests de contrato: la implementación responde conforme a la spec.
- [ ] Un test de abuso por cada control de la tabla de seguridad del plan,
      que comprueba también el efecto lateral, no solo el código de respuesta.

## Seguridad

- [ ] Cada fila de la tabla de criterios de seguridad del plan está
      implementada y verificada.
- [ ] `security-reviewer` sin hallazgos de severidad alta o crítica.
- [ ] Los hallazgos medios están resueltos o acordados explícitamente en el PR.
- [ ] Ningún log ni mensaje de error expone datos de otros usuarios.

## Trazabilidad

- [ ] Los cinco trailers presentes en el cuerpo del PR.
- [ ] El PR enlaza el issue y este se cierra al mergear.

## Documentación

- [ ] Si el cambio contradice una ADR, la ADR se ha actualizado o superado
      con una nueva.
- [ ] Si introduce una amenaza nueva, `docs/threats/` está actualizado.
```

**Va en fichero aparte y no en `AGENTS.md`** por presupuesto: la capa de hechos
está en 30 de 40 líneas, y esto son 35 más. Se referencia desde
`skills/github-pr-protocol.md`, que es donde un agente lo necesita.

**El último bloque, documentación, es el que más se olvida.** Una ADR que el
código contradice es peor que no tener ADR: enseña a desconfiar del directorio
entero.

---

## 10. `compose.yaml`

Fichero `compose.yaml` en la raíz:

```yaml
name: task-allocation

services:
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: taskalloc
      POSTGRES_PASSWORD: taskalloc-dev-only
      POSTGRES_DB: taskalloc
    ports:
      - "5432:5432"
    volumes:
      - db-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U taskalloc -d taskalloc"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s

  db-ephemeral:
    profiles: ["ephemeral"]
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: taskalloc
      POSTGRES_PASSWORD: taskalloc-ephemeral
      POSTGRES_DB: taskalloc_test
      PGDATA: /dev/shm/pgdata
    tmpfs:
      - /dev/shm
    ports:
      - "55432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U taskalloc -d taskalloc_test"]
      interval: 3s
      timeout: 3s
      retries: 15

volumes:
  db-data:
```

### Lo que merece atención

**Dos servicios y no uno.** `db` persiste en un volumen y es tu base de
desarrollo. `db-ephemeral` guarda sus datos en `tmpfs` —memoria— y por tanto es
rápida y desaparece al parar. Es la que usará el DAST de PR-2 y la que quieres
para pruebas que ensucian.

**El puerto `55432` del efímero evita la colisión** con la base de desarrollo o
con un Postgres instalado en tu máquina. Un conflicto de puerto en CI produce un
fallo desconcertante que cuesta media hora diagnosticar.

**Las credenciales son de desarrollo y están en claro, a propósito.** Nombres
como `taskalloc-dev-only` señalan su alcance. La regla que las acompaña: **estas
son las únicas credenciales que se versionan en este repositorio.** Cualquier otra
va por `.env`, que está en `.gitignore` desde el paso 1 de PR-0.

**Los healthchecks no son opcionales aquí.** `testcontainers-go` y los workflows
esperan a que el servicio esté sano; sin `healthcheck`, "el contenedor arrancó" no
significa "Postgres acepta conexiones", y obtienes fallos intermitentes que
parecen problemas de test.

Compruébalo:

```bash
docker compose up -d db
docker compose ps                          # db debe figurar como healthy
docker compose --profile ephemeral up -d db-ephemeral
docker compose down
```

---

## 11. Regenerar, verificar y abrir el PR

### 11.1 Regenerar los adaptadores

Has tocado dos skills, así que el arnés cambia. **Este es el momento donde
`agentsync` demuestra por qué existe:**

```bash
make agents
make agents-check
```

Si te saltas el `make agents`, el `harness-check` fallará en CI con
`.claude/agents/...: desactualizado`. Es el comportamiento correcto.

Comprueba el presupuesto tras los cambios:

| Fichero | Antes | Después | Límite |
|---|---:|---:|---:|
| `openapi-contract-reading.md` | 34 | 39 | 80 |
| `hexagonal-boundaries.md` | 34 | 40 | 80 |
| `github-pr-protocol.md` | 36 | 38 | 80 |

Holgura amplia. Si algún día uno se acerca a 80, la respuesta correcta no es
subir el límite: es partir el skill en dos.

### 11.2 Verificar

```bash
make verify && echo "OK"
git status --short
```

### 11.3 Commit

```bash
git add docs .github/pull_request_template.md skills .claude compose.yaml
git commit -m "docs: decisiones de arquitectura y artefactos de proceso

Cuatro ADRs: hexagonal, persistencia con política expand/contract, versionado
semántico del contrato, y observabilidad con auditoría como puerto explícito.
Plantillas RPI, Definition of Done, compose con base de desarrollo y efímera,
y trazabilidad issue -> operationId mediante x-source-issue y trailer Issue.

Issue: N/A
Spec: N/A
Plan: N/A
Agent: none
Runtime: human"
```

### 11.4 PR

```bash
git push -u origin chore/pr-1-decisiones

gh pr create --repo fexlixjhl/task-allocation --base main --draft \
  --title "docs: decisiones de arquitectura y artefactos de proceso (PR-1)" \
  --label "runtime:human" \
  --body "$(cat <<'BODY'
## Qué cambia

Cuatro ADRs que fijan el marco dentro del que operarán los agentes, plantillas
RPI, Definition of Done, entorno de base de datos y cierre de la trazabilidad
issue -> operationId. Sin código de producto.

## Plan aprobado

N/A — PR de decisiones, previo al primer contrato.

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

Issue: N/A
Spec: N/A
Plan: N/A
Agent: none
Runtime: human
BODY
)"

gh pr checks --watch
gh pr ready
gh pr merge --squash --delete-branch
```

### Checklist de cierre

- [ ] Cuatro ADRs en `docs/adr/`
- [ ] `docs/features/_template/` con `RESEARCH.md` y `PLAN.md`
- [ ] `docs/DEFINITION-OF-DONE.md`
- [ ] `compose.yaml` con `db` sana y perfil `ephemeral` funcionando
- [ ] `x-source-issue` documentado en `openapi-contract-reading`
- [ ] Sección de auditoría en `hexagonal-boundaries`
- [ ] Cinco trailers en `github-pr-protocol` y en la plantilla de PR
- [ ] `make agents` ejecutado y adaptadores commiteados
- [ ] `harness-check` en verde
- [ ] Mergeado con squash y trailers presentes en `main`

---

## 12. Qué falta

**PR-2 es el más grande del proyecto** y el último que escribes tú:

| Bloque | Contenido |
|---|---|
| Contrato | Spectral con ruleset propio (incluye exigir `x-source-issue`, `x-owner-check` y `format: uuid`), oasdiff con el gate de tres condiciones |
| Generación | oapi-codegen, sqlc, openapi-typescript, goose, cableados en `make generate` |
| Calidad | golangci-lint con depguard implementando ADR-0001, eslint, vue-tsc |
| Seguridad | CodeQL, Semgrep con reglas propias, gitleaks, govulncheck, Trivy |
| Dinámico | ZAP contra el perfil `ephemeral` |
| Workflows | `spec-gate`, `plan-gate`, `code-gate`, `agent-review`, `dast`, caducidad de excepciones, métricas de runtime |
| Mantenimiento | Renovate o Dependabot con `sec:debt` automático |

Y después, el hito de verdad:

| PR | Contenido | Autor |
|---|---|---|
| PR-3 | Primera spec OpenAPI mínima | `contract-designer` — **el primer PR de un agente** |
| PR-4 | `RESEARCH.md` + `PLAN.md` | `context-researcher`, `change-planner` |
| PR-5 | Primera implementación | `backend-builder`, revisado por `security-reviewer` |
