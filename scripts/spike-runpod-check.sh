#!/bin/bash
# spike-runpod-check.sh — lê o resultado do spike e VIGIA o gasto.
#
# Duas perguntas, uma tela: "o pod devolveu a medição?" e "sobrou GPU ligada?".
# A segunda importa tanto quanto a primeira — GPU esquecida ligada é o tipo de erro
# que só aparece na fatura.
set -euo pipefail
# Caminhos do cofre: local (gitignorado) se existir, senão o exemplo genérico.
# Ver scripts/secrets.example.sh — o porquê está lá.
_SECRETS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$_SECRETS_DIR/secrets.local.sh" 2>/dev/null || . "$_SECRETS_DIR/secrets.example.sh"



AUTODJ="${AUTODJ_BASE:-https://autodj.andrebassi.com.br}"
KEY="$(pass show "$PASS_RUNPOD_API_KEY")"

echo "== faixas SPIKE no AutoDJ (o resultado chega como faixa) =="
curl -sS -m 30 "$AUTODJ/api/v1/library/tracks" -o /tmp/spike-tracks.json 2>/dev/null || true
python3 -c "
import json
try:
    ts = json.load(open('/tmp/spike-tracks.json'))
except Exception:
    print('  (não consegui ler o catálogo)'); raise SystemExit
hits = [t for t in ts if t.get('title','').startswith('SPIKE')]
print('  nenhuma ainda' if not hits else '')
for t in hits:
    print(' •', t['title'])
    print('   id:', t['id'])
"

echo
echo "== pods ativos (vigilância de gasto) =="
curl -sS -X POST https://api.runpod.io/graphql -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KEY" \
  -d '{"query":"query { myself { clientBalance pods { id name desiredStatus costPerHr runtime { uptimeInSeconds } } } }"}' \
  | python3 -c "
import json,sys
d = json.load(sys.stdin)['data']['myself']
print('  saldo US\$ %.4f' % d['clientBalance'])
ps = d.get('pods') or []
if not ps:
    print('  nenhum pod ativo ✓')
for p in ps:
    up = (p.get('runtime') or {}).get('uptimeInSeconds')
    print('  •', p['id'], p['desiredStatus'], 'US\$%.3f/h' % (p.get('costPerHr') or 0), 'uptime', up, 's')
    if up is not None and up < 30:
        print('    ⚠️ uptime baixo e reiniciando = LOOP DE RESTART (container morrendo na partida)')
"
