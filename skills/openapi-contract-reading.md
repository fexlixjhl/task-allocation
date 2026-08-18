---
name: openapi-contract-reading
description: Cómo interpretar api/openapi.yaml como fuente de verdad
---

## Orden de lectura
1. `info.version` y el SHA del commit. Cítalo en el trailer `Spec:`.
2. `security` global y los overrides por operación. Una operación sin
   `security` explícito hereda el global; si el global está vacío, es un
   endpoint público y eso exige justificación en el plan.
3. La operación por `operationId`. Nunca la referencies por método+ruta:
   el `operationId` es el identificador estable entre spec, código y plan.
4. Los `$ref` de request y response, resueltos hasta el final.
5. Las extensiones propias del proyecto: `x-required-scope` y `x-owner-check`.

## Extensiones del proyecto
`x-required-scope`: string. Permiso que el token debe portar.

`x-owner-check`: objeto con tres campos.
- `resource`: tipo de recurso (`slot`, `project`, `assignment`).
- `param`: nombre del parámetro que identifica la instancia.
- `rule`: enum cerrado. Valores admitidos: `owner-only`,
  `owner-or-project-lead`, `team-member`, `project-lead-only`.

Cada valor de `rule` corresponde a un método de `ports.Authorizer`. Un valor
fuera del enum es un error de contrato, no una decisión de implementación.

## Qué es normativo
- `required`, `format`, `pattern`, `minimum`, `maximum`, `enum` y `maxLength`
  son restricciones exigibles. Si el código no las aplica, es un defecto
  aunque los tests pasen.
- Los códigos de respuesta declarados son exhaustivos: devolver uno no
  declarado viola el contrato.
- `description` NO es normativo. Si el comportamiento solo está en prosa,
  el contrato está incompleto: para y señálalo.

## Política 403 frente a 404
- `404` cuando el recurso no existe, y también cuando el solicitante no
  tiene derecho a saber que existe.
- `403` solo cuando la pertenencia es legítima pero la acción no está
  permitida.
  Confundirlos filtra la existencia de recursos ajenos.

## Señales de contrato incompleto
Cualquiera de estas es motivo de `needs-contract`, nunca de improvisar:
- Operación sin `security` ni justificación de que es pública.
- Colección sin parámetros de paginación.
- Recurso con identificador en el path y sin `x-owner-check`.
- Campo de texto libre sin `maxLength`.
- Identificador de path que no es `format: uuid`.
