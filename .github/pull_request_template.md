## Qué cambia

<!-- Una o dos frases. Qué comportamiento cambia, no qué ficheros. -->

## Plan aprobado

<!-- Ruta al PLAN.md mergeado en main. "N/A" solo para PRs de arnés o de spec. -->

## Operaciones tocadas

| operationId | cambio |
|---|---|

## Seguridad

- [ ] Los criterios de seguridad del plan están implementados
- [ ] Hay tests de abuso para cada control nuevo o modificado
- [ ] No se han añadido dependencias sin justificación en el plan
- [ ] Ningún log o error expone datos de otros usuarios

## Verificación

- [ ] `make verify` pasa limpio en local

---

Spec: api/openapi.yaml@<sha>
Plan: docs/features/<slug>/PLAN.md
Agent: <nombre-del-agente|none>
Runtime: <claude-code|copilot|human>
