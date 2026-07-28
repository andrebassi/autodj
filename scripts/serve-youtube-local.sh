#!/usr/bin/env bash
# Local smoke test do YouTube: sobe server com Spotify + YouTube habilitados,
# DB em memória, web/out estático, redirect loopback na porta 8095 (bate com o
# redirect URI registrado no Google Cloud). Creds YouTube vêm do pass.
set -euo pipefail
# Caminhos do cofre: local (gitignorado) se existir, senão o exemplo genérico.
# Ver scripts/secrets.example.sh — o porquê está lá.
_SECRETS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$_SECRETS_DIR/secrets.local.sh" 2>/dev/null || . "$_SECRETS_DIR/secrets.example.sh"


cd "$(dirname "$0")/.."

PORT="${PORT:-8095}"
export PORT
export AUTODJ_DB_PATH="${AUTODJ_DB_PATH:-memory}"
export WEB_DIR="$(pwd)/web/out"
# lê do pass só se a var não veio do ambiente; ausência de pass não aborta.
from_pass() { pass show "$1" 2>/dev/null | tr -d '[:space:]' || true; }

if [ -z "${AUTODJ_SPOTIFY_CLIENT_ID:-}" ] && [ -f scripts/spotify-client-id.txt ]; then
  AUTODJ_SPOTIFY_CLIENT_ID="$(tr -d '[:space:]' < scripts/spotify-client-id.txt)"
fi
export AUTODJ_SPOTIFY_CLIENT_ID="${AUTODJ_SPOTIFY_CLIENT_ID:-}"
export AUTODJ_YOUTUBE_CLIENT_ID="${AUTODJ_YOUTUBE_CLIENT_ID:-$(from_pass "$PASS_YOUTUBE_CLIENT_ID")}"
export AUTODJ_YOUTUBE_CLIENT_SECRET="${AUTODJ_YOUTUBE_CLIENT_SECRET:-$(from_pass "$PASS_YOUTUBE_CLIENT_SECRET")}"
export AUTODJ_YOUTUBE_API_KEY="${AUTODJ_YOUTUBE_API_KEY:-$(from_pass "$PASS_YOUTUBE_API_KEY")}"
# redirect segue no loopback IP (é o registrado no Google Cloud), mas o RETORNO
# pós-callback é absoluto p/ localhost: o YouTube não incorpora clipe monetizado a
# partir de origem IP (erro 150 → faixa pulada). Ver lib/ytOrigin.ts.
export AUTODJ_YOUTUBE_REDIRECT="http://127.0.0.1:${PORT}/api/v1/youtube/callback"
export AUTODJ_YOUTUBE_RETURN="http://localhost:${PORT}/?op=youtube"
echo "abra: http://localhost:${PORT}  (não use 127.0.0.1 — embed musical bloqueado em IP)"

GOWORK=off go build -o /tmp/autodj-server ./cmd/server
exec /tmp/autodj-server
