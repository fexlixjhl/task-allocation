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
