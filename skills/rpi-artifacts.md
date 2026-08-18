---
name: rpi-artifacts
description: Plantillas y reglas de los artefactos RESEARCH.md y PLAN.md
---

## Ubicación y slug
`docs/features/<slug>/RESEARCH.md` y `docs/features/<slug>/PLAN.md`.
El slug es kebab-case, sin acentos, derivado del issue, y el mismo que se
usa en el nombre de rama.

## Plantilla de RESEARCH.md

    # Research: <slug>
    Issue: #<n>   ·   Spec: api/openapi.yaml@<sha>

    ## Operaciones afectadas
    | operationId | método y ruta | ¿existe ya? |

    ## Esquemas implicados
    Nombre, campos relevantes y restricciones de validación.

    ## ADRs aplicables
    ## Amenazas relevantes
    De docs/threats/. Si no hay modelo para estas operaciones, escríbelo.

    ## Ficheros que se tocarán
    | fichero | capa hexagonal |

    ## Preguntas abiertas

Regla del research: todo lo que escribas debe ser comprobable señalando un
fichero o una línea de la spec. Un hueco se anota como pregunta abierta,
nunca se rellena con una suposición.

## Plantilla de PLAN.md

    # Plan: <slug>
    Research: ./RESEARCH.md   ·   Aprobado por: <humano>

    ## Alcance
    Qué entra.

    ## Fuera de alcance
    Qué no entra, explícitamente.

    ## Pasos
    | # | Acción | Ficheros | Capa | Criterio de aceptación |

    ## Criterios de seguridad
    | Amenaza | Control ASVS | Implementación | Verificación |

## Reglas del plan
- Cada paso indica su capa hexagonal. Un paso que toca dos capas son dos
  pasos.
- El criterio de aceptación es verificable: un test, un comando, un código
  de respuesta. "Funciona correctamente" no es un criterio.
- No repitas lo que ya dice la spec. Referencia el `operationId`.
- La tabla de criterios de seguridad no puede quedar vacía. Si el cambio no
  tiene implicaciones, escribe por qué en una línea.
- Cada fila de seguridad traza hacia atrás a una amenaza y hacia adelante a
  un test concreto.

## Presupuesto
Máximo 15 pasos. Un plan más largo es un plan que nadie revisa a fondo, y un
plan que nadie revisa no es un gate. Si te pasas, propón partir la feature.
