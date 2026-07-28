#!/bin/bash
# app-version.sh — resolve a versão EXIBIDA no titlebar (NEXT_PUBLIC_APP_VERSION).
#
# Por que existe: a versão vinha direto de `git describe --tags --abbrev=0`, então
# um build local feito DEPOIS do release mostrava a mesma versão da produção — ou,
# pior, um `web/out` velho mostrava uma versão ANTERIOR à de prod sem nenhum sinal
# de que estava obsoleto. Não dava pra saber, olhando a tela, se aquele bundle era
# o de produção ou trabalho não lançado.
#
# Regra:
#   HEAD exatamente na tag + árvore limpa  → v<tag>                 (build de release)
#   qualquer outra coisa                   → v<próxima patch>-dev.<sha>[.dirty]
#
# Assim o local fica sempre À FRENTE de prod (v0.2.17-dev.abc1234 vs v0.2.16) e
# nunca é confundido com um release.
#
# O gate de changelog (scripts/changelog-check.sh) roda aqui dentro: continua
# obrigatório que a tag mais recente tenha entrada no changelog.
#
# Uso: scripts/app-version.sh   → imprime a versão COM o 'v' na última linha
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Gate + versão da tag (sem 'v'). Aborta o build se o changelog estiver atrás.
VER="$("$ROOT/scripts/changelog-check.sh")"

# Sem git (tarball, container de build sem .git) → usa a versão da tag como está.
if ! git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  echo "v$VER"
  exit 0
fi

# changelog-check devolve 0.0.1 quando não há tag nenhuma: nada a incrementar.
if [ "$VER" = "0.0.1" ]; then
  echo "v$VER"
  exit 0
fi

EXACT="$(git -C "$ROOT" describe --tags --exact-match --match "v[0-9]*" 2>/dev/null || true)"
DIRTY=""
[ -n "$(git -C "$ROOT" status --porcelain 2>/dev/null)" ] && DIRTY=".dirty"

if [ -n "$EXACT" ] && [ -z "$DIRTY" ]; then
  echo "app-version: release $EXACT" >&2
  echo "$EXACT"
  exit 0
fi

# Trabalho não lançado: rotula como a PRÓXIMA patch, marcada como -dev.
IFS=. read -r MA MI PA <<<"$VER"
NEXT="$MA.$MI.$((PA + 1))"
SHA="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
DEV="v$NEXT-dev.$SHA$DIRTY"

echo "app-version: dev $DEV (último release: v$VER)" >&2
echo "$DEV"
