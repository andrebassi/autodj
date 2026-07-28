#!/bin/bash
# build-demucs.sh — prepara a separação de vocal por IA (Demucs v4) para uso LOCAL.
#
# Por que shell-out e não uma lib Go: `demucs.cpp` (MIT) é C++17 + Eigen
# header-only, sem PyTorch. Poderia entrar por cgo, mas aí o binário do AutoDJ
# passaria a exigir toolchain C++ para compilar — inclusive na imagem de deploy,
# onde este recurso NEM RODA (a VM de produção é `shared-cpu-1x` com 1 GB, e a
# separação leva ~1,4× o tempo real). Shell-out mantém o servidor Go puro e o
# recurso é opt-in: sem o binário instalado, o AutoDJ degrada como sempre fez com
# aubio/keyfinder — a função some, nada quebra.
#
# Instala em (sobrescrevíveis por env):
#   ~/.autodj/bin/demucs            (AUTODJ_DEMUCS_BIN)
#   ~/.autodj/models/htdemucs.bin   (AUTODJ_DEMUCS_MODEL)
set -euo pipefail

DEST_BIN="${AUTODJ_DEMUCS_BIN:-$HOME/.autodj/bin/demucs}"
DEST_MODEL="${AUTODJ_DEMUCS_MODEL:-$HOME/.autodj/models/htdemucs.bin}"
WORK="${DEMUCS_WORK:-$HOME/.autodj/src}"
# Pesos ggml pré-convertidos (o repo do demucs.cpp aponta para este dataset).
MODEL_URL="https://huggingface.co/datasets/Retrobear/demucs.cpp/resolve/main/ggml-model-htdemucs-4s-f16.bin"

mkdir -p "$(dirname "$DEST_BIN")" "$(dirname "$DEST_MODEL")" "$WORK"

if [ ! -f "$DEST_MODEL" ]; then
  echo "== baixando pesos htdemucs 4-source (~80 MB) =="
  curl -fL --progress-bar -o "$DEST_MODEL.part" "$MODEL_URL"
  mv "$DEST_MODEL.part" "$DEST_MODEL"
fi
echo "modelo: $DEST_MODEL ($(du -h "$DEST_MODEL" | cut -f1))"

if [ -x "$DEST_BIN" ]; then
  echo "binário já instalado: $DEST_BIN"
  exit 0
fi

cd "$WORK"
[ -d demucs.cpp ] || git clone --recurse-submodules --depth 1 https://github.com/sevagh/demucs.cpp.git
cd demucs.cpp

# ⚠️ O CMakeLists do upstream declara compatibilidade com CMake < 3.5, que o CMake
# 4.x removeu — sem esta política o configure aborta. Não é um patch no código
# deles, é só afrouxar a checagem de versão do nosso lado.
echo "== compilando (Release) =="
cmake -B build -DCMAKE_BUILD_TYPE=Release -DCMAKE_POLICY_VERSION_MINIMUM=3.5 >/dev/null
cmake --build build --parallel "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)" >/dev/null

# demucs_mt = multi-threaded; é o único que entrega tempo utilizável.
install -m 0755 build/demucs_mt.cpp.main "$DEST_BIN"
echo "binário: $DEST_BIN"
"$DEST_BIN" 2>&1 | head -1 || true
