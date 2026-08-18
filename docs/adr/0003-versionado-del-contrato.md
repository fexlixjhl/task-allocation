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
