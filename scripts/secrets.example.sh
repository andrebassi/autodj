#!/usr/bin/env bash
# Onde ficam suas credenciais no gerenciador de senhas (`pass`).
#
# COPIE para `scripts/secrets.local.sh` e ajuste os caminhos aos SEUS. O arquivo
# local é gitignorado de propósito: os caminhos do cofre não são segredos — saber
# o caminho não abre nada sem a sua chave GPG —, mas publicá-los entrega de graça
# como o seu cofre é organizado e quais serviços você usa.
#
# Nada disto é obrigatório: toda variável `AUTODJ_*` correspondente, se já estiver
# exportada no ambiente, vence o `pass`. Quem não usa `pass` simplesmente exporta.

PASS_RUNPOD_API_KEY="autodj/runpod/api-key"
PASS_FLY_API_TOKEN="autodj/flyio/api-token"
PASS_YOUTUBE_CLIENT_ID="autodj/youtube/client-id"
PASS_YOUTUBE_CLIENT_SECRET="autodj/youtube/client-secret"
PASS_YOUTUBE_API_KEY="autodj/youtube/api-key"
