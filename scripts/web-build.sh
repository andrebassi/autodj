#!/bin/bash
# web-build.sh — build estático do frontend (web/out) com a versão vinda da git tag
# e o gate de changelog. Único ponto de build da UI (usado por `task web:build` e
# `task desktop:build`). Injeta NEXT_PUBLIC_APP_VERSION (inlinado no bundle Next).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Versão exibida + gate de changelog (app-version.sh roda o changelog-check dentro).
# Build fora da tag vira "-dev.<sha>", pra não passar por release (ver app-version.sh).
VER="$("$ROOT/scripts/app-version.sh")"

echo "web-build: NEXT_PUBLIC_APP_VERSION=$VER"
cd "$ROOT/web"
pnpm install
NEXT_PUBLIC_APP_VERSION="$VER" pnpm build
