"""Serviço de separação que roda DENTRO do pod GPU.

⚠️ O POD É UM SERVIÇO, NÃO UM JOB — e essa é a decisão que simplifica tudo.

A alternativa (pod como job: baixa a faixa, separa, devolve por callback) exigiria
uma rota nova no AutoDJ para receber o stem, um token de escopo para o pod se
autenticar, e mudaria a porta `StemSeparator`, que hoje é arquivo-entra/arquivo-sai.
Como serviço, o AutoDJ envia os bytes e recebe os bytes de volta na mesma
requisição: a porta continua idêntica e nada novo é exposto na internet.

O outro ganho é o MODELO CARREGADO UMA VEZ. Num job por faixa, cada uma pagaria de
novo os ~3 min de subir pod e instalar o Demucs — mais caro que a separação em si
(medido: ~10 s por faixa de 3 min). Como serviço, o lote inteiro reusa o pod quente.

Só biblioteca padrão do Python: a imagem do PyTorch não garante nada além disso.
"""

import os
import subprocess
import sys
import tempfile
import hmac
import threading
import time
import urllib.request
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ.get("PORT", "8000"))
# Sem trabalho por este tempo, o pod se mata. É a SEGUNDA linha de defesa: quem
# manda é o watchdog do AutoDJ (só ele sabe se ainda há fila, e é o operador dele
# que paga a GPU parada). Por isso este valor é propositalmente MAIOR — ele só age
# se o AutoDJ tiver morrido antes de encerrar o pod.
IDLE_SECONDS = int(os.environ.get("IDLE_SECONDS", "300"))

# Segredo compartilhado com quem lançou este pod. Sorteado por subida, nunca
# reaproveitado. Vazio = pod aberto (uso manual para depurar).
POD_TOKEN = os.environ.get("POD_TOKEN", "")

_last_use = time.time()
_lock = threading.Lock()


def _terminate_self():
    """Pede a própria morte ao RunPod.

    ⚠️ NÃO DÁ PARA CONFIRMAR: a mutation responde `{"data":{"podTerminate":null}}`
    mesmo quando funciona. E, medido, o RunPod já reiniciou o container mesmo
    depois de o script terminar — por isso o AutoDJ mantém um watchdog próprio, e
    esta função é só a primeira tentativa, não a garantia.
    """
    key, pid = os.environ.get("RUNPOD_API_KEY"), os.environ.get("RUNPOD_POD_ID")
    if not key or not pid:
        return
    body = json.dumps({"query": 'mutation { podTerminate(input:{podId:"%s"}) }' % pid}).encode()
    req = urllib.request.Request(
        "https://api.runpod.io/graphql?api_key=" + key,
        data=body,
        headers={"Content-Type": "application/json"},
    )
    try:
        urllib.request.urlopen(req, timeout=30).read()
    except Exception as e:  # noqa: BLE001 — melhor seguir vivo que morrer aqui
        print("pod: auto-terminacao falhou:", e, flush=True)


def _idle_watch():
    while True:
        time.sleep(30)
        with _lock:
            idle = time.time() - _last_use
        if idle > IDLE_SECONDS:
            print(f"pod: ocioso ha {idle:.0f}s — terminando", flush=True)
            _terminate_self()
            os._exit(0)


def _separate(data: bytes) -> bytes:
    """Recebe áudio, devolve o mesmo áudio SEM VOZ.

    `--two-stems vocals` pede ao Demucs só a divisão voz/resto, que é metade do
    trabalho das 4 fontes — e o "resto" já é exatamente o instrumental. Somar
    drums+bass+other à mão (como faz o adapter local) daria o mesmo resultado por
    um caminho mais longo.
    """
    with tempfile.TemporaryDirectory() as td:
        src = os.path.join(td, "in.audio")
        with open(src, "wb") as f:
            f.write(data)
        out = os.path.join(td, "out")
        r = subprocess.run(
            [sys.executable, "-m", "demucs", "-n", "htdemucs",
             "--two-stems", "vocals", "-o", out, src],
            capture_output=True, text=True, timeout=1800,
        )
        if r.returncode != 0:
            raise RuntimeError((r.stderr or "")[-500:])
        stem = None
        for root, _dirs, files in os.walk(out):
            for fn in files:
                if "no_vocals" in fn:
                    stem = os.path.join(root, fn)
        if not stem:
            raise RuntimeError("demucs nao produziu o stem sem vocal")
        # ⚠️ DEVOLVE WAV, e isso é contrato, não preferência.
        #
        # A primeira versão devolvia mp3 (menos bytes na volta) — e o arquivo era
        # gravado no caminho `.instrumental.wav` que o StemService escolhe, ficando
        # com extensão mentindo sobre o conteúdo. Pior: o adapter LOCAL
        # (demucs.cpp) grava WAV de verdade, então os dois adapters da MESMA porta
        # produziriam formatos diferentes — o tipo de divergência que só aparece no
        # dia em que algo lê o arquivo pela extensão.
        #
        # O custo é transferência maior (~10 MB/min de áudio). Aceito: acontece uma
        # vez por faixa, num preparo que já leva segundos de GPU.
        with open(stem, "rb") as f:
            return f.read()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):  # silencia o log por requisição
        print("pod:", fmt % args, flush=True)

    def _authorized(self) -> bool:
        """O RunPod publica esta porta em https://<pod-id>-8000.proxy.runpod.net,
        que é PÚBLICO. Sem esta checagem, qualquer um que descobrisse o endereço
        mandaria áudio e gastaria a GPU que o operador paga — a única barreira
        seria o pod id ser difícil de adivinhar, ou seja, obscuridade.

        `compare_digest` em vez de `==`: comparação de string sai no primeiro byte
        diferente e o tempo de resposta vaza o prefixo correto.

        Sem POD_TOKEN no ambiente o serviço fica ABERTO, e é de propósito: um pod
        subido à mão para depurar continua utilizável. Quem lança pelo AutoDJ
        sempre recebe token.
        """
        if not POD_TOKEN:
            return True
        got = self.headers.get("Authorization", "")
        prefix = "Bearer "
        if not got.startswith(prefix):
            return False
        return hmac.compare_digest(got[len(prefix):], POD_TOKEN)

    def do_GET(self):  # noqa: N802 — nome exigido pela stdlib
        # /health é como o AutoDJ sabe que o pod terminou de instalar o Demucs.
        # Sem isso ele mandaria a primeira faixa antes do serviço existir.
        if self.path == "/health":
            # /health também exige o token: sem isso ele vira um confirmador
            # gratuito de "existe pod vivo neste endereço".
            if not self._authorized():
                self._reply(401, b"", "text/plain")
                return
            self._reply(200, b"ok", "text/plain")
            return
        self._reply(404, b"", "text/plain")

    def do_POST(self):  # noqa: N802
        global _last_use
        if self.path != "/separate":
            self._reply(404, b"", "text/plain")
            return
        if not self._authorized():
            # Não toca em _last_use: requisição não autorizada não pode segurar o
            # pod vivo, senão um estranho mantém a GPU do operador ligada de graça.
            self._reply(401, b"", "text/plain")
            return
        with _lock:
            _last_use = time.time()
        n = int(self.headers.get("Content-Length", "0"))
        data = self.rfile.read(n)
        t0 = time.time()
        try:
            out = _separate(data)
        except Exception as e:  # noqa: BLE001
            print("pod: falhou:", e, flush=True)
            self._reply(500, str(e).encode()[:500], "text/plain")
            return
        finally:
            with _lock:
                _last_use = time.time()
        print(f"pod: separou {n} bytes em {time.time() - t0:.1f}s", flush=True)
        self._reply(200, out, "audio/wav")

    def _reply(self, code, body, ctype):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if body:
            self.wfile.write(body)


if __name__ == "__main__":
    threading.Thread(target=_idle_watch, daemon=True).start()
    print(f"pod: servico ouvindo em :{PORT} (ocioso mata em {IDLE_SECONDS}s)", flush=True)
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
