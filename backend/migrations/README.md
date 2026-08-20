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
