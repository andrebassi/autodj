#!/usr/bin/env bash
# Fly.io entrypoint: schema (idempotente) + API Go, largando privilégio de root.
# Playout roda no browser; broadcast por servidor (Liquidsoap/Icecast) foi
# descontinuado.
set -euo pipefail

APP_USER=app
APP_UID=10001
# ⚠️ OS DADOS FICAM EM /root/.autodj MESMO SEM ROOT. `Track.Path` é gravado
# ABSOLUTO no banco, então mover isto invalidaria toda faixa já existente — foi o
# que quebrou o áudio na v0.2.42 (403, permissão negada ao atravessar /root).
DATA_DIR=/root/.autodj

# Schema só quando há Postgres; SQLite migra sozinho no boot do server.
if [[ -n "${DATABASE_URL:-}" ]]; then
  migrate /app/migrations || true
fi

# Já não somos root (execução local como outro usuário, ou imagem com USER): não
# há o que ajustar nem de onde descer.
if [[ "$(id -u)" -ne 0 ]]; then
  exec server
fi

# `chmod o+x /root` (executar, NÃO ler) é o que permite ao usuário sem privilégio
# ATRAVESSAR /root até os próprios dados. Sem isto a travessia falha com permissão
# negada e o `http.ServeFile` responde 403 — a biblioteca lista e nada toca. Não é
# `o+r`: ninguém precisa LISTAR /root, só passar por ele.
chmod o+x /root || true

# O volume do Fly é montado pertencendo a ROOT. Sem este ajuste o processo sem
# privilégio não escreve no próprio banco.
#
# A checagem do dono ANTES do chown não é economia à toa: `chown -R` num acervo
# com milhares de arquivos custa tempo A CADA BOOT, e o health check do Fly tem
# grace period. Depois da primeira vez o teste falha rápido e seguimos.
mkdir -p "$DATA_DIR"
if [[ "$(stat -c '%u' "$DATA_DIR" 2>/dev/null || echo 0)" != "$APP_UID" ]]; then
  echo "entrypoint: ajustando dono de $DATA_DIR para $APP_USER (primeira subida sem root)"
  chown -R "$APP_UID:$APP_UID" "$DATA_DIR" || true
fi

# ⚠️ FALHA ABERTA, DE PROPÓSITO. Se a ferramenta de troca de usuário não existir
# nesta imagem, seguimos como root COM AVISO em vez de derrubar o container.
# Root é o comportamento ANTIGO — ruim, mas conhecido; um entrypoint que aborta
# tira a estação DO AR, que é pior e mais difícil de diagnosticar. O aviso é o que
# impede a regressão de passar despercebida: se ele aparecer nos logs, o ganho de
# segurança não aconteceu.
if command -v setpriv >/dev/null 2>&1; then
  exec setpriv --reuid="$APP_UID" --regid="$APP_UID" --init-groups server
elif command -v su >/dev/null 2>&1; then
  exec su -s /bin/bash -c 'exec server' "$APP_USER"
fi

echo "entrypoint: AVISO — sem setpriv/su nesta imagem; seguindo como ROOT" >&2
exec server
