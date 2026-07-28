#!/bin/bash
# stems-cloud.sh — prepara o SEM-VOZ do acervo inteiro numa GPU alugada.
#
# O trabalho pesado é do servidor (StemService + adapter RunPod); este script só
# dispara e acompanha. A chave sai do `pass`, nunca de arquivo no repositório.
#
# CUSTO MEDIDO no spike: ~10 s de GPU por faixa de 3 min, ~US$ 0,20/h. Um acervo de
# 83 faixas sai por menos de US$ 0,10 — pago uma vez, não por mês.
set -euo pipefail

BASE="${AUTODJ_BASE:-http://localhost:8099}"
SID="${AUTODJ_SESSION:-}"
hdr=(); [ -n "$SID" ] && hdr=(-H "X-AutoDJ-Session: $SID")

echo "== acervo em $BASE =="
curl -sS "${hdr[@]}" "$BASE/api/v1/library/tracks" -o /tmp/stems-tracks.json
python3 - <<'PY'
import json
ts = json.load(open('/tmp/stems-tracks.json'))
loc = [t for t in ts if t.get('source') == 'local' and t.get('pcm_available')]
falta = [t for t in loc if not t.get('instrumental_ready')]
print(f"  locais: {len(loc)} | já preparadas: {len(loc)-len(falta)} | a preparar: {len(falta)}")
mins = sum((t.get('duration_ms') or 0) for t in falta) / 60000
# ~0,06x o tempo real (medido), mais ~3 min de subida do pod, uma vez.
print(f"  estimativa: {mins*0.06 + 3:.0f} min de GPU  ≈  US$ {((mins*0.06 + 3)/60)*0.20:.2f}")
open('/tmp/stems-todo.txt', 'w').write('\n'.join(t['id'] for t in falta))
PY

n=$(wc -l < /tmp/stems-todo.txt | tr -d ' ')
[ "$n" -gt 0 ] || { echo "nada a fazer"; exit 0; }
read -r -p "Enfileirar $n faixa(s)? [s/N] " ok
[ "$ok" = "s" ] || { echo "cancelado"; exit 0; }

while read -r id; do
  [ -n "$id" ] && curl -sS -X POST "${hdr[@]}" "$BASE/api/v1/library/tracks/$id/instrumental" >/dev/null
done < /tmp/stems-todo.txt
echo "enfileiradas. Acompanhe pela UI (barra na grade e na fila) ou:"
echo "  curl -s ${hdr[*]} $BASE/api/v1/library/instrumentals | python3 -m json.tool"
