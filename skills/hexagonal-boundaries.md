---
name: hexagonal-boundaries
description: Dónde colocar cada pieza según la arquitectura hexagonal (ADR-0001)
---

## Reglas de import
1. `internal/domain` no importa nada del proyecto. Solo stdlib.
2. `internal/app` importa `domain`. Nunca `adapters`.
3. `internal/adapters/*` importan `app` y `domain`. Nunca entre sí.
4. Solo `cmd/` importa `adapters`.

Las verifica `depguard`. Si dudas de dónde va algo, la pregunta correcta es
"¿qué necesita importar?", no "¿de qué trata?".

## Tabla de decisión
| Si la pieza... | va en |
|---|---|
| Es una regla de negocio invariante | `internal/domain` |
| Es una entidad con identidad propia | `internal/domain` |
| Orquesta varias entidades para cumplir un caso de uso | `internal/app` |
| Comprueba autorización sobre un recurso | `internal/app` |
| Es una interfaz que el núcleo necesita que alguien cumpla | `internal/app/ports` |
| Habla HTTP, SQL, reloj, UUID, red o disco | `internal/adapters/<tecnología>` |
| Cablea dependencias concretas | `cmd/api` |

## Puertos
Un puerto es una interfaz **definida por el núcleo** y expresada en su
vocabulario, no en el de la tecnología que lo implementará.

Correcto: `SlotRepository.ByOwnerAndRange(ctx, MemberID, TimeRange)`.
Incorrecto: `SlotRepository.Query(ctx, sql string)`.

Si el nombre de un método de puerto menciona SQL, HTTP, JSON o una librería,
la abstracción está mal: el detalle se ha filtrado hacia dentro.

## La regla del DTO
Los tipos generados desde OpenAPI son DTOs del adaptador HTTP. Viven en
`internal/adapters/http/` y **no cruzan** hacia `app` ni `domain`.

El adaptador traduce DTO a entidad en la entrada y entidad a DTO en la
salida. Lo mismo con los structs generados por sqlc, en sentido inverso.

Esta regla tiene razón de seguridad, no de estética: si el DTO cruzara,
existiría una vía para construir una entidad sin pasar por su constructor,
y por tanto en estado inválido.

## Autorización
Va en el caso de uso, en `internal/app`, y siempre antes de cualquier efecto
lateral. Nunca en el handler HTTP: cada nuevo punto de entrada (un consumidor
de mensajes, un CLI) se saltaría el control.

## Errores
El dominio devuelve errores del dominio. El adaptador HTTP los traduce a los
códigos declarados en la spec. Un error de `database/sql` que llega al
cliente filtra estructura interna.
