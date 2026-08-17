# Excepciones de seguridad

Registro de gates saltados deliberadamente. Fichero bajo CODEOWNERS: **ningún
agente puede escribir aquí**. Los agentes ejecutan la política; solo un humano
la modifica.

Toda excepción requiere fecha de caducidad. Un workflow nocturno falla el build
de `main` cuando una excepción vence. Sin caducidad forzada, las excepciones se
vuelven permanentes y el SDL se vacía sin que nadie decida vaciarlo.

## Cómo añadir una

1. Añade una fila a la tabla con todos los campos rellenos.
2. Abre el issue de remediación y enlázalo.
3. La fecha de caducidad máxima es de 90 días desde la aprobación.
4. El PR que añade la excepción requiere tu aprobación como CODEOWNER.

## Excepciones activas

| ID | Gate afectado | Alcance exacto | Motivo | Caduca | Issue |
|---|---|---|---|---|---|
| — | — | — | — | — | — |

## Excepciones cerradas

| ID | Gate afectado | Cerrada el | Cómo se resolvió |
|---|---|---|---|
| — | — | — | — |
