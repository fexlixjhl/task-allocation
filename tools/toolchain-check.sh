#!/usr/bin/env bash
# Verifica que la versión de Go declarada en .mise.toml, en el devcontainer y
# en la directiva toolchain de backend/go.mod es la misma.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

mise_ver=$(grep -E '^go[[:space:]]*=' mise.toml | head -1 | cut -d'"' -f2)
mod_ver=$(grep -E '^toolchain[[:space:]]+go' backend/go.mod | head -1 | sed 's/^toolchain[[:space:]]*go//')
dev_ver=$(grep -o '"ghcr.io/devcontainers/features/go:1"[^}]*}' .devcontainer/devcontainer.json \
          | grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4)

echo "  .mise.toml        : ${mise_ver:-AUSENTE}"
echo "  backend/go.mod    : ${mod_ver:-AUSENTE}"
echo "  devcontainer.json : ${dev_ver:-AUSENTE}"

for v in "$mise_ver" "$mod_ver" "$dev_ver"; do
  [ -n "$v" ] || { echo "ERROR: falta una de las tres declaraciones"; exit 1; }
done

if [ "$mise_ver" != "$mod_ver" ] || [ "$mise_ver" != "$dev_ver" ]; then
  echo "ERROR: las tres declaraciones de la versión de Go deben coincidir."
  fail=1
fi

exit $fail
