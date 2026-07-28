#!/bin/bash
# serve-local.sh — sobe o server AutoDJ servindo o build estático web/out, com o
# Client ID do Spotify BAQUEADO (não some mais em restart). Uso caseiro.
#
# Client ID = client público PKCE (sem secret) → seguro ficar em arquivo local
# gitignored. Precedência: env AUTODJ_SPOTIFY_CLIENT_ID > scripts/spotify-client-id.txt.
set -euo pipefail
# Caminhos do cofre: local (gitignorado) se existir, senão o exemplo genérico.
# Ver scripts/secrets.example.sh — o porquê está lá.
_SECRETS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$_SECRETS_DIR/secrets.local.sh" 2>/dev/null || . "$_SECRETS_DIR/secrets.example.sh"



ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${PORT:-8099}"
# DB: vazio = default do serverkit = SQLite por fingerprint
# (~/.autodj/sessions/<sid>.db, e ~/.autodj/library.db pro tenant "default"). Era
# `memory` até a v0.2.18: como este script MATA o server antigo a cada execução, todo
# restart pra pegar código novo apagava biblioteca, fila e conta ligada — do lado do
# operador isso aparecia como "dou refresh e some tudo". Pra voltar ao efêmero
# (smoke em estado limpo): AUTODJ_DB_PATH=memory task serve
DB="${AUTODJ_DB_PATH:-}"
CID_FILE="$ROOT/scripts/spotify-client-id.txt"

CID="${AUTODJ_SPOTIFY_CLIENT_ID:-}"
[ -z "$CID" ] && [ -f "$CID_FILE" ] && CID="$(tr -d '[:space:]' < "$CID_FILE")"

# YouTube (OAuth Google code-flow COM secret + Data API key) — puxa do pass, uso
# caseiro. Precedência: env > pass. Se o pass estiver travado/ausente, ficam vazias
# e as rotas /youtube/* dão 404 (degradação graciosa, igual Spotify sem client id).
# O secret NÃO vai pra arquivo/binário — só env em runtime.
YT_CID="${AUTODJ_YOUTUBE_CLIENT_ID:-$(pass show "$PASS_YOUTUBE_CLIENT_ID" 2>/dev/null | tr -d '[:space:]' || true)}"
YT_SECRET="${AUTODJ_YOUTUBE_CLIENT_SECRET:-$(pass show "$PASS_YOUTUBE_CLIENT_SECRET" 2>/dev/null | tr -d '[:space:]' || true)}"
YT_KEY="${AUTODJ_YOUTUBE_API_KEY:-$(pass show "$PASS_YOUTUBE_API_KEY" 2>/dev/null | tr -d '[:space:]' || true)}"

# mata server antigo na porta
pkill -f "autodj-server" 2>/dev/null || true
sleep 1

echo "building server -> /tmp/autodj-server"
( cd "$ROOT" && GOWORK=off go build -o /tmp/autodj-server ./cmd/server )

YT_STATE="disabled"
[ -n "$YT_CID" ] && [ -n "$YT_SECRET" ] && [ -n "$YT_KEY" ] && YT_STATE="configured"
echo "serving: PORT=$PORT DB=${DB:-sqlite por fingerprint (persistente)} WEB_DIR=$ROOT/web/out spotify=${CID:+configured} youtube=$YT_STATE"

# ABRA EM localhost, NÃO em 127.0.0.1 (v0.2.10). O YouTube recusa incorporar clipe
# monetizado (VEVO/gravadora) quando a origem da página é IP literal: o player devolve
# onError 150 / errorCode "auth", o playout marca embed: e PULA a faixa — o sintoma é
# "arrastei pro deck e sumiu". A mesma URL em http://localhost:<porta> toca normal
# (medido lado a lado). Prod usa domínio real, então só afeta o dev.
#
# Os redirects de OAuth continuam em 127.0.0.1 (o Spotify EXIGE o IP de loopback
# literal e é o que está registrado nos dois consoles); só o RETORNO pós-callback
# passa a ser absoluto p/ localhost, senão o browser terminaria o login na origem IP.
APP_URL="http://localhost:$PORT"
echo "abra: $APP_URL   (não use 127.0.0.1 — YouTube bloqueia embed musical em IP)"
exec env \
  PORT="$PORT" \
  AUTODJ_DB_PATH="$DB" \
  WEB_DIR="$ROOT/web/out" \
  AUTODJ_SPOTIFY_CLIENT_ID="$CID" \
  AUTODJ_SPOTIFY_RETURN="${AUTODJ_SPOTIFY_RETURN:-$APP_URL/?op=spotify}" \
  AUTODJ_YOUTUBE_CLIENT_ID="$YT_CID" \
  AUTODJ_YOUTUBE_CLIENT_SECRET="$YT_SECRET" \
  AUTODJ_YOUTUBE_API_KEY="$YT_KEY" \
  AUTODJ_YOUTUBE_RETURN="${AUTODJ_YOUTUBE_RETURN:-$APP_URL/?op=youtube}" \
  /tmp/autodj-server
