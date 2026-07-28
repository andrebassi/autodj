#!/bin/bash
# changelog-check.sh — garante que o CHANGELOG está atualizado a cada bump de versão.
# Compara a git tag mais recente (fonte da versão exibida) com a entrada do TOPO de
# web/app/lib/changelog.ts. Se divergirem, ABORTA o build: toda versão nova precisa
# de uma entrada de changelog (regra DOC SEMPRE ATUALIZADA do CLAUDE.md do projeto).
#
# Uso: scripts/changelog-check.sh
# Saída: imprime a versão resolvida (sem 'v') em stdout na última linha.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CL="$ROOT/web/app/lib/changelog.ts"

# Git tag de VERSÃO mais recente (ex.: v0.1.7). `--match "v[0-9]*"` ignora tags-bookmark
# non-semver no mesmo commit (ex.: spotify.stable) — só versões contam pro gate.
# Sem git / sem tag → pula o gate (dev fora do git).
TAG=""
if git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  TAG="$(git -C "$ROOT" describe --tags --abbrev=0 --match "v[0-9]*" 2>/dev/null || true)"
fi

if [ -z "$TAG" ]; then
  echo "changelog-check: sem git tag — pulando gate" >&2
  echo "0.0.1"
  exit 0
fi

TAG_VER="${TAG#v}" # 0.1.7

# Gate: a versão da git tag PRECISA ter uma entrada no changelog. O changelog pode
# estar À FRENTE da tag (documentando trabalho ainda não taggeado) — por isso NÃO
# exigimos topo==tag, e sim que a versão taggeada exista em algum lugar do arquivo.
# Assim, todo `git tag vX.Y.Z` novo obriga uma entrada correspondente.
if grep -qE "version: *\"$TAG_VER\"" "$CL"; then
  echo "changelog-check: OK (v$TAG_VER tem entrada no changelog)" >&2
  echo "$TAG_VER"
  exit 0
fi

{
  echo "──────────────────────────────────────────────────────────────"
  echo "  CHANGELOG DESATUALIZADO — build abortado"
  echo "  git tag mais recente : v$TAG_VER"
  echo "  (sem entrada correspondente em web/app/lib/changelog.ts)"
  echo ""
  echo "  Adicione a entrada da v$TAG_VER no TOPO de:"
  echo "    web/app/lib/changelog.ts"
  echo "  (regra DOC SEMPRE ATUALIZADA — CLAUDE.md do projeto)"
  echo "──────────────────────────────────────────────────────────────"
} >&2
exit 1
