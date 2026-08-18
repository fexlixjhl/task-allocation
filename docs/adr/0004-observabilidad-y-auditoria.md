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
