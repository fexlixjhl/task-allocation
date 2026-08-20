#!/usr/bin/env bash
# Verifica que los generadores producen salida válida a partir del fixture.
# Se ejecuta aunque todavía no exista api/openapi.yaml.
set -euo pipefail
cd "$(dirname "$0")/../.."

FIXTURE=api/testdata/good.yaml
fail=0

echo "== cliente TypeScript desde el fixture =="
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
if npx --yes openapi-typescript "$FIXTURE" -o "$TMP/schema.d.ts" >/dev/null 2>&1; then
  grep -q 'operations\["updateSlot"\]' "$TMP/schema.d.ts" \
    && echo "   OK   operación tipada" || { echo "   FALLO operación no tipada"; fail=1; }
  grep -q 'slotId' "$TMP/schema.d.ts" \
    && echo "   OK   parámetro de path presente" || { echo "   FALLO falta el path param"; fail=1; }
else
  echo "   FALLO openapi-typescript no generó"; fail=1
fi

echo "== servidor Go desde el fixture =="
mkdir -p .tmp/oapi-selftest
if go -C tools/gen tool oapi-codegen \
     -config ../../api/testdata/oapi-codegen.test.yaml \
     "../../$FIXTURE"; then
  OUT=.tmp/oapi-selftest/server.gen.go
  grep -q 'StrictServerInterface' "$OUT" \
    && echo "   OK   strict-server activo" || { echo "   FALLO sin strict-server"; fail=1; }
  # Compilar, no solo buscar cadenas: un import ausente no lo detecta un grep.
  if ( cd .tmp/oapi-selftest \
       && { [ -f go.mod ] || go mod init selftest >/dev/null 2>&1; } \
       && go mod tidy >/dev/null 2>&1 && go build ./... >/dev/null 2>&1 ); then
    echo "   OK   el código generado compila"
  else
    echo "   FALLO el código generado no compila:"
    ( cd .tmp/oapi-selftest && go build ./... 2>&1 | head -5 )
    fail=1
  fi
else
  echo "   FALLO oapi-codegen no generó"; fail=1
fi

echo "== acceso a datos desde el fixture =="
if [ -f backend/sqlc.test.yaml ]; then
  if go -C tools/gen tool sqlc -f ../../backend/sqlc.test.yaml generate; then
    M=.tmp/sqlc-selftest/models.go
    grep -q 'uuid.UUID' "$M" \
      && echo "   OK   override de uuid" || { echo "   FALLO override no aplicado"; fail=1; }
    grep -q '\*uuid.UUID' "$M" \
      && echo "   OK   par nullable aplicado" || { echo "   FALLO falta el override nullable"; fail=1; }
    grep -q 'json:' "$M" \
      && { echo "   FALLO hay tags JSON, se puede saltar el DTO"; fail=1; } \
      || echo "   OK   sin tags JSON"
    # Compilar: es lo único que demuestra que los imports están bien.
    if ( cd .tmp/sqlc-selftest \
         && { [ -f go.mod ] || go mod init sqlcselftest >/dev/null 2>&1; } \
         && go mod tidy >/dev/null 2>&1 && go build ./... >/dev/null 2>&1 ); then
      echo "   OK   el acceso a datos compila"
    else
      echo "   FALLO el acceso a datos no compila:"
      ( cd .tmp/sqlc-selftest && go build ./... 2>&1 | head -5 )
      fail=1
    fi
  else
    echo "   FALLO sqlc no generó"; fail=1
  fi
else
  echo "   -    omitido: backend/sqlc.test.yaml todavía no existe"
fi

exit $fail
