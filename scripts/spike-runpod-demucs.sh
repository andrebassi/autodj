#!/bin/bash
# spike-runpod-demucs.sh — MEDE o custo real de separar vocal numa GPU alugada.
#
# Não é integração: é um experimento descartável para responder UMA pergunta antes
# de escrever qualquer adapter — "quanto tempo (e portanto quanto dinheiro) leva
# uma faixa numa GPU do RunPod?".
#
# Existe porque estimativa não medida já custou caro nesta sessão: a v0.2.35 subiu
# separação para produção com base em "~1,4× o tempo real" e virou uma VM de US$ 125
# por mês, revertida no dia seguinte. Aqui o número vem ANTES da decisão.
#
# ── O RESULTADO VOLTA PELO PRÓPRIO AUTODJ ────────────────────────────────────
# A primeira versão lia o log do pod por `rest.runpod.io/v1/pods/{id}/logs` — rota
# que NÃO EXISTE MAIS (responde erro de spec; o `Logs()` do finetuning-platform
# provavelmente está quebrado pelo mesmo motivo). Sem log eu fiquei cego, e o pod
# entrou em loop de restart sem que eu soubesse por quê.
#
# Agora o pod devolve o resultado por um caminho que já sabemos que funciona: faz
# `POST /library/upload` no AutoDJ de produção, com o TEMPO MEDIDO no título — e a
# leitura é pela API de faixas. Bônus: isso valida o fluxo real da story 10.6 (pod
# puxa áudio do AutoDJ → separa → devolve ao AutoDJ) antes de ele existir.
#
# ⚠️ AUTO-TERMINAÇÃO É ITEM DE SEGURANÇA, não detalhe: sem ela o RunPod REINICIA o
# container ao fim do comando (loop de GPU paga). Aqui ela roda em Python, dentro de
# `finally` — vale para sucesso, erro e timeout. O padrão veio de
# `finetuning-platform/internal/backend/bootstrap.go`.
#
# Uso:  scripts/spike-runpod-demucs.sh [URL_DO_AUDIO]
set -euo pipefail
# Caminhos do cofre: local (gitignorado) se existir, senão o exemplo genérico.
# Ver scripts/secrets.example.sh — o porquê está lá.
_SECRETS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$_SECRETS_DIR/secrets.local.sh" 2>/dev/null || . "$_SECRETS_DIR/secrets.example.sh"



AUDIO_URL="${1:-https://autodj.andrebassi.com.br/api/v1/library/tracks/9644cecb0f6130ae1d17dadbd1788cda18775ed2/audio}"
AUTODJ="${AUTODJ_BASE:-https://autodj.andrebassi.com.br}"
# RTX A5000 (24 GB) a ~US$ 0,16/h é a mais barata que serve — CONSULTADA na API, não
# escolhida de memória: a tabela do site mostra preços de Serverless (US$ 0,69/h) e
# "NVIDIA L4" sequer existe no secure cloud, então o pod nem teria subido.
# LISTA, não uma GPU só: a primeira tentativa levou SUPPLY_CONSTRAINT ("no longer
# any instances available") — capacidade de nuvem some e volta, e prender o spike a
# um modelo específico faz o experimento falhar por motivo que não é o dele.
# Ordem = mais barata primeiro (preços consultados na API, não do site).
GPUS_DEFAULT=("NVIDIA RTX A5000" "NVIDIA RTX A4000" "NVIDIA RTX A4500" "NVIDIA RTX 4000 Ada Generation" "NVIDIA GeForce RTX 3090" "NVIDIA GeForce RTX 4090" "NVIDIA RTX A6000")
if [ -n "${RUNPOD_GPU:-}" ]; then GPUS=("$RUNPOD_GPU"); else GPUS=("${GPUS_DEFAULT[@]}"); fi
# Imagem oficial do PyTorch (Docker Hub), tag pública e estável.
IMAGE="${RUNPOD_IMAGE:-pytorch/pytorch:2.4.0-cuda12.4-cudnn9-runtime}"
API="https://api.runpod.io/graphql"

KEY="$(pass show "$PASS_RUNPOD_API_KEY")"
[ -n "$KEY" ] || { echo "sem chave em $PASS_RUNPOD_API_KEY"; exit 1; }

gql() { curl -sS -X POST "$API" -H "Content-Type: application/json" \
          -H "Authorization: Bearer $KEY" --data-binary "$1"; }

echo "== saldo (preflight) =="
gql '{"query":"query { myself { clientBalance } }"}' | python3 -c \
  'import json,sys; print("US$", json.load(sys.stdin)["data"]["myself"]["clientBalance"])'

# ── script que roda DENTRO do pod ─────────────────────────────────────────────
# Tudo em Python de propósito: a versão anterior usava curl no bash, e a imagem do
# PyTorch não garante `curl` instalado. Python está garantido — é a imagem dele.
read -r -d '' POD_SCRIPT <<'PY' || true
import os, subprocess, time, urllib.request, json, uuid, traceback

AUDIO = os.environ["AUDIO_URL"]
BASE = os.environ["AUTODJ_BASE"]
WORK = "/workspace"


def terminate():
    """Mata o próprio pod. SEMPRE — sucesso, erro ou timeout. Sem isto o RunPod
    reinicia o container e a GPU roda (paga) indefinidamente."""
    key, pid = os.environ.get("RUNPOD_API_KEY"), os.environ.get("RUNPOD_POD_ID")
    if not key or not pid:
        return
    body = json.dumps({"query": 'mutation { podTerminate(input:{podId:"%s"}) }' % pid}).encode()
    req = urllib.request.Request("https://api.runpod.io/graphql?api_key=" + key,
                                 data=body, headers={"Content-Type": "application/json"})
    try:
        urllib.request.urlopen(req, timeout=30).read()
    except Exception as e:
        print("SPIKE aviso: auto-terminacao falhou:", e, flush=True)


def upload(path, title):
    """Devolve o resultado pelo AutoDJ — o canal que já sabemos que funciona."""
    boundary = uuid.uuid4().hex
    body = b""
    for k, v in {"title": title, "artist": "SPIKE", "category": "music"}.items():
        body += (f'--{boundary}\r\nContent-Disposition: form-data; '
                 f'name="{k}"\r\n\r\n{v}\r\n').encode()
    with open(path, "rb") as f:
        data = f.read()
    body += (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; '
             f'filename="{os.path.basename(path)}"\r\n'
             f'Content-Type: audio/mpeg\r\n\r\n').encode() + data + b"\r\n"
    body += f"--{boundary}--\r\n".encode()
    req = urllib.request.Request(BASE + "/api/v1/library/upload", data=body,
                                 headers={"Content-Type":
                                          f"multipart/form-data; boundary={boundary}"})
    return urllib.request.urlopen(req, timeout=180).read().decode()[:160]


try:
    print("SPIKE baixando", AUDIO, flush=True)
    urllib.request.urlretrieve(AUDIO, WORK + "/in.mp3")
    print("SPIKE bytes", os.path.getsize(WORK + "/in.mp3"), flush=True)

    dur = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration",
                          "-of", "default=nk=1:nw=1", WORK + "/in.mp3"],
                         capture_output=True, text=True).stdout.strip() or "?"

    # A GPU é o PONTO do experimento: se o torch não a enxergar, o número medido
    # seria de CPU e responderia a pergunta errada.
    import torch
    gpu = torch.cuda.get_device_name(0) if torch.cuda.is_available() else "SEM-GPU"
    print("SPIKE cuda", gpu, flush=True)

    t0 = time.time()
    r = subprocess.run(["python", "-m", "demucs", "-n", "htdemucs", "--two-stems", "vocals",
                        "-o", WORK + "/out", WORK + "/in.mp3"],
                       capture_output=True, text=True, timeout=900)
    el = time.time() - t0
    print("SPIKE demucs rc", r.returncode, (r.stderr or "")[-600:], flush=True)

    stem = None
    for root, _, files in os.walk(WORK + "/out"):
        for fn in files:
            if "no_vocals" in fn:
                stem = os.path.join(root, fn)
    ratio = (el / float(dur)) if dur not in ("?", "") else 0

    mp3 = WORK + "/stem.mp3"
    if stem:
        subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-i", stem, "-b:a", "128k", mp3],
                       check=False)
    title = f"SPIKE gpu={el:.1f}s audio={dur}s ratio={ratio:.2f}x rc={r.returncode} {gpu}"
    print("SPIKE_RESULT", title, flush=True)
    print("SPIKE upload:", upload(mp3 if os.path.exists(mp3) else WORK + "/in.mp3", title),
          flush=True)
except Exception:
    traceback.print_exc()
    try:
        upload("/workspace/in.mp3", "SPIKE FALHOU " + traceback.format_exc()[-70:])
    except Exception:
        pass
finally:
    terminate()
PY

B64=$(printf '%s' "$POD_SCRIPT" | base64 | tr -d '\n')

# ⚠️ `mkdir -p /workspace` É O QUE FALTAVA NA PRIMEIRA TENTATIVA — e explica o loop
# de restart (uptime caindo de 10s para 3s entre duas consultas). O pod sobe com
# `volumeInGb: 0`, então /workspace NÃO existe; escrever ali falhava, o comando
# morria na hora e o RunPod reiniciava o container. O Bootstrap do
# finetuning-platform faz este mkdir na primeira linha, e foi o que eu não copiei.
BOOT='bash -lc '"'"'set -uo pipefail; mkdir -p /workspace; echo "$SCRIPT_B64" | base64 -d > /workspace/spike.py; apt-get update -qq >/dev/null 2>&1; apt-get install -y -qq ffmpeg >/dev/null 2>&1; pip install -q --no-cache-dir demucs >/dev/null 2>&1; python /workspace/spike.py; echo SPIKE_DONE; sleep 5'"'"

launch() { # launch <gpu> <cloudType>
  local gpu="$1" cloud="$2" payload
  payload=$(python3 - "$gpu" "$IMAGE" "$BOOT" "$B64" "$AUDIO_URL" "$KEY" "$AUTODJ" "$cloud" <<'INNER'
import json, sys
gpu, image, boot, b64, audio, key, base, cloud = sys.argv[1:9]
q = """mutation Launch($input: PodFindAndDeployOnDemandInput!) {
  podFindAndDeployOnDemand(input: $input) { id name desiredStatus costPerHr }
}"""
inp = {
    "cloudType": cloud, "gpuCount": 1, "gpuTypeId": gpu,
    "name": "autodj-spike-demucs", "imageName": image,
    "containerDiskInGb": 30, "volumeInGb": 0, "volumeMountPath": "/workspace",
    "dockerArgs": boot,
    "env": [{"key": "SCRIPT_B64", "value": b64},
            {"key": "AUDIO_URL", "value": audio},
            {"key": "AUTODJ_BASE", "value": base},
            {"key": "RUNPOD_API_KEY", "value": key}],
}
print(json.dumps({"query": q, "variables": {"input": inp}}))
INNER
)
  gql "$payload"
}

POD_ID=""
# SECURE primeiro (rede e disco melhores); COMMUNITY é o plano B — mais barato e com
# mais capacidade livre, que é o que falta justamente quando dá SUPPLY_CONSTRAINT.
for cloud in SECURE COMMUNITY; do
  for gpu in "${GPUS[@]}"; do
    echo "== tentando $gpu ($cloud) =="
    RESP=$(launch "$gpu" "$cloud")
    POD_ID=$(echo "$RESP" | python3 -c 'import json,sys; d=json.load(sys.stdin); p=(d.get("data") or {}).get("podFindAndDeployOnDemand") or {}; print(p.get("id",""))')
    if [ -n "$POD_ID" ]; then GPU="$gpu"; break 2; fi
    echo "   indisponivel: $(echo "$RESP" | python3 -c 'import json,sys; e=(json.load(sys.stdin).get("errors") or [{}])[0]; print(e.get("message","?")[:70])')"
  done
done

[ -n "$POD_ID" ] || { echo "FALHOU: nenhuma GPU disponivel em SECURE nem COMMUNITY"; exit 1; }
echo "pod: $POD_ID ($GPU)"
echo
echo "terminar na mão, se precisar:"
echo "  KEY=\$(pass show "$PASS_RUNPOD_API_KEY"); curl -sS -X POST $API -H \"Authorization: Bearer \$KEY\" -H 'Content-Type: application/json' -d '{\"query\":\"mutation { podTerminate(input:{podId:\\\"$POD_ID\\\"}) }\"}'"
echo
echo "O resultado chega como FAIXA no AutoDJ (título começa com SPIKE)."
echo "Acompanhe com:  scripts/spike-runpod-check.sh"
