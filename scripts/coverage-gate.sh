#!/usr/bin/env bash
# Coverage gate: domain packages must be 100%, adapter/app packages >=90%.
# Integration-tier packages (live DB / process main) are excluded — covered by
# end-to-end runs, not unit coverage.
set -euo pipefail
export GOWORK=off

# integration-tier: pacotes cujo trabalho é INVOCAR PROCESSO EXTERNO ou falar com
# um banco de verdade. O CI não tem ffmpeg/demucs/Postgres, então exigir 90% ali
# só produziria teste de fachada — a lógica pura desses adapters mora em funções
# separadas (ex.: `audioanalyze/playout.go`), essas sim testadas.
EXCLUDE_RE='autodj/(cmd/|adapters/driven/postgres|adapters/driven/audioanalyze|adapters/driven/stemsep|adapters/driven/tts)'
FAIL=0

# `go test` PRIMEIRO, com a saída num arquivo e o status guardado.
#
# Por que não `< <(go test ...)` direto: pacote que NÃO COMPILA (ou cujo teste
# falha) escreve no stderr e não emite linha "coverage:" nenhuma — com o stderr
# descartado, ele simplesmente SUMIA da lista e o gate dizia "passed" sobre um
# pacote que nem buildava. Além disso o status do process substitution não é
# visível pro `set -e`. Aconteceu de verdade: o `httpapi` ficou com um fake
# desatualizado (sem os métodos novos de pastas) e o gate passou mesmo assim.
# ⚠️ TEMPLATE COM XXXXXX, não `-t prefixo`: `mktemp -t autodj-cover` funciona no
# macOS (BSD) e FALHA no Linux (GNU), onde -t exige o template — "too few X's in
# template". Ficou escondido porque este script só rodava no Mac do autor: o
# workflow do GitHub existia mas o repositório vivia no GitLab, então o CI nunca
# tinha sido executado uma única vez. A primeira execução, ao publicar, o pegou.
OUT="$(mktemp "${TMPDIR:-/tmp}/autodj-cover.XXXXXX")"
trap 'rm -f "$OUT"' EXIT
if ! go test ./... -cover >"$OUT" 2>&1; then
  echo "FAIL  go test não passou (build quebrado ou teste vermelho):"
  grep -vE '^(ok|---|\s*$)' "$OUT" | head -20
  echo "coverage gate FAILED"
  exit 1
fi

while IFS= read -r line; do
  case "$line" in
    *"coverage:"*) ;;
    *) continue ;;
  esac
  # pkg token is whichever field looks like an autodj/ path (handles both the
  # "ok  autodj/x  coverage:" and leading-tab "  autodj/x  coverage:" forms).
  pkg="$(grep -oE 'autodj/[^[:space:]]+' <<<"$line" | head -1)"
  cov="$(sed -E 's/.*coverage: ([0-9.]+)%.*/\1/' <<<"$line")"

  echo "$pkg" | grep -Eq "$EXCLUDE_RE" && { echo "skip  $pkg  (integration-tier)"; continue; }

  if [[ "$pkg" == autodj/domain/* ]]; then min=100.0; else min=90.0; fi

  if awk "BEGIN{exit !($cov < $min)}"; then
    echo "FAIL  $pkg  ${cov}%  (min ${min}%)"; FAIL=1
  else
    echo "ok    $pkg  ${cov}%"
  fi
done <"$OUT"

if [[ "$FAIL" == "1" ]]; then echo "coverage gate FAILED"; exit 1; fi
echo "coverage gate passed"
