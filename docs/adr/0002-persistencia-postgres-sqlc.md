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
