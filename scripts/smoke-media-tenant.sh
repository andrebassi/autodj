#!/bin/bash
# Smoke do isolamento de mídia por estação (0.2.16).
#
# Sobe o servidor com HOME/dirs descartáveis, envia o MESMO nome de arquivo
# (`intro.mp3`) por duas sessões diferentes e prova que:
#   1. cada upload cai em media/<tenant>/;
#   2. os conteúdos NÃO se sobrescrevem;
#   3. os ids dos tracks diferem (id = sha1(path), path agora é distinto);
#   4. cada estação enxerga só a própria faixa.
#
# Não toca em ~/.autodj do usuário: tudo em mktemp -d, limpo no fim.
set -euo pipefail

PORT="${PORT:-8399}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
SRV_PID=""

cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "== tmp: $TMP"
mkdir -p "$TMP/home"

# ffmpeg gera um mp3 real (o upload roda ffprobe: arquivo falso é rejeitado).
gen_mp3() { # $1=dest $2=freq (conteúdo diferente por estação)
  ffmpeg -v quiet -y -f lavfi -i "sine=frequency=$2:duration=1" -c:a libmp3lame "$1"
}
gen_mp3 "$TMP/a.mp3" 440
gen_mp3 "$TMP/b.mp3" 880

echo "== build"
cd "$ROOT"
GOWORK=off go build -o "$TMP/autodj-server" ./cmd/server

echo "== up (HOME=$TMP/home, port $PORT)"
HOME="$TMP/home" PORT="$PORT" AUTODJ_AUTOPILOT=0 WEB_DIR=/nonexistent \
  "$TMP/autodj-server" >"$TMP/server.log" 2>&1 &
SRV_PID=$!

for _ in $(seq 1 40); do
  curl -sf "http://localhost:$PORT/api/v1/health" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -sf "http://localhost:$PORT/api/v1/health" >/dev/null || { echo "FAIL: server não subiu"; cat "$TMP/server.log"; exit 1; }

upload() { # $1=sid $2=arquivo -> json do track
  curl -sf -X POST "http://localhost:$PORT/api/v1/library/upload" \
    -H "X-AutoDJ-Session: $1" \
    -F "file=@$2;filename=intro.mp3" \
    -F "category=music" -F "title=Intro $1"
}

echo "== upload mesmo nome por 2 sessões"
RA="$(upload aaa111 "$TMP/a.mp3")"
RB="$(upload bbb222 "$TMP/b.mp3")"

ID_A="$(printf '%s' "$RA" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
ID_B="$(printf '%s' "$RB" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
echo "   id aaa111 = $ID_A"
echo "   id bbb222 = $ID_B"

MEDIA="$TMP/home/.autodj/media"
echo "== árvore do media"
find "$MEDIA" -type f | sed "s|$TMP/home/.autodj/|  |"

FAIL=0
chk() { if [ "$1" = "ok" ]; then echo "  PASS: $2"; else echo "  FAIL: $2"; FAIL=1; fi; }

[ -f "$MEDIA/aaa111/intro.mp3" ] && chk ok "aaa111 gravou em media/aaa111/" || chk no "aaa111 gravou em media/aaa111/"
[ -f "$MEDIA/bbb222/intro.mp3" ] && chk ok "bbb222 gravou em media/bbb222/" || chk no "bbb222 gravou em media/bbb222/"
[ -f "$MEDIA/intro.mp3" ] && chk no "raiz do media NÃO deve receber upload de tenant" || chk ok "raiz do media limpa"
cmp -s "$MEDIA/aaa111/intro.mp3" "$MEDIA/bbb222/intro.mp3" && chk no "conteúdos deveriam diferir (sobrescrita!)" || chk ok "conteúdos preservados (sem sobrescrita)"
[ "$ID_A" != "$ID_B" ] && chk ok "ids distintos (sha1 do path)" || chk no "ids colidiram: $ID_A"

echo "== cada estação vê só a própria faixa"
list() { curl -sf "http://localhost:$PORT/api/v1/library/tracks" -H "X-AutoDJ-Session: $1" \
  | python3 -c 'import json,sys; print(",".join(sorted(t["id"] for t in json.load(sys.stdin))))'; }
LA="$(list aaa111)"; LB="$(list bbb222)"
[ "$LA" = "$ID_A" ] && chk ok "biblioteca de aaa111 = só $ID_A" || chk no "aaa111 viu: $LA"
[ "$LB" = "$ID_B" ] && chk ok "biblioteca de bbb222 = só $ID_B" || chk no "bbb222 viu: $LB"

echo "== delete de aaa111 não pode tocar no arquivo de bbb222"
curl -sf -X DELETE "http://localhost:$PORT/api/v1/library/tracks/$ID_A" -H "X-AutoDJ-Session: aaa111" >/dev/null
[ ! -f "$MEDIA/aaa111/intro.mp3" ] && chk ok "arquivo de aaa111 removido" || chk no "arquivo de aaa111 sobrou"
[ -f "$MEDIA/bbb222/intro.mp3" ] && chk ok "arquivo de bbb222 intacto" || chk no "arquivo de bbb222 sumiu (VAZAMENTO)"

echo
[ "$FAIL" = 0 ] && echo "SMOKE OK" || { echo "SMOKE FALHOU"; exit 1; }
