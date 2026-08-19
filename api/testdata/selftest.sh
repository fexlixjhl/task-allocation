#!/usr/bin/env bash
# Verifica que el ruleset de Spectral hace lo que decimos que hace:
# la spec correcta pasa sin errores y la defectuosa dispara todas las reglas.
set -euo pipefail
cd "$(dirname "$0")/.."

EXPECTED_RULES=(
  ta-operation-source-issue
  ta-operation-security-declared
  ta-public-operation-is-visible
  ta-operation-required-scope
  ta-owner-check-required
  ta-owner-check-shape
  ta-owner-check-responses
  ta-path-id-is-uuid
  ta-string-has-maxlength
  ta-collection-has-pagination
  ta-info-version-semver
  ta-servers-v1-prefix
)

fail=0

echo "== good.yaml no debe producir errores =="
if npx --yes @stoplight/spectral-cli lint testdata/good.yaml --ruleset .spectral.yaml --fail-severity=error >/dev/null 2>&1; then
  echo "   OK"
else
  echo "   FALLO: la spec de referencia produce errores"
  npx --yes @stoplight/spectral-cli lint testdata/good.yaml --ruleset .spectral.yaml 2>/dev/null | grep error || true
  fail=1
fi

echo "== bad.yaml debe disparar todas las reglas propias =="
out=$(npx --yes @stoplight/spectral-cli lint testdata/bad.yaml --ruleset .spectral.yaml 2>/dev/null || true)
for rule in "${EXPECTED_RULES[@]}"; do
  if grep -q "$rule" <<<"$out"; then
    echo "   OK   $rule"
  else
    echo "   FALLO $rule no disparó"
    fail=1
  fi
done

exit $fail
