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
