# REMOTE deploy image only (Fly.io). LOCAL dev uses Nix, never Docker (rule 23).
# Go server + static web UI + ffprobe (metadata). Playout roda no browser;
# broadcast por servidor (Liquidsoap/Icecast) foi descontinuado.

# ── web (static export) ──────────────────────────────────────────────
FROM node:22-bookworm-slim AS web
WORKDIR /web
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml* ./
RUN pnpm install --no-frozen-lockfile
COPY web/ ./
# versão exibida no titlebar (git tag), passada por `fly deploy --build-arg`.
# NEXT_PUBLIC_* é inlinado no bundle Next em build-time. Ver task `deploy`.
ARG APP_VERSION=v0.0.1
ENV NEXT_PUBLIC_APP_VERSION=$APP_VERSION
RUN pnpm build

# ── go build ─────────────────────────────────────────────────────────
FROM golang:1.27-bookworm AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# APP_VERSION precisa ser redeclarado neste stage (ARG não atravessa FROM). Vai pro
# binário via ldflags -X, na MESMA variável que virou NEXT_PUBLIC_APP_VERSION no bundle
# — é a comparação dos dois que detecta deploy novo com a aba aberta. Sem isso o
# binário fica em "dev" e o front desliga a checagem.
ARG APP_VERSION=v0.0.1
RUN go build -ldflags "-X autodj/adapters/driving/httpapi.Version=$APP_VERSION" \
      -o /out/server ./cmd/server \
 && go build -o /out/migrate ./cmd/migrate

# ── runtime (slim + ffprobe) ─────────────────────────────────────────
# ── separação de vocal por IA (demucs.cpp) — DESLIGADA, não removida ──────────
#
# Este estágio FUNCIONOU na v0.2.35 e está comentado desde a 0.2.36. O motivo é a
# CONTA, não a técnica: exige `performance-4x` com 8 GB (medido: 5,9 GB de pico),
# o que leva a fatura de ~US$ 7 para ~US$ 125/mês — 17× — para rodar de 8 a 15 min
# por faixa, contra ~6 min na máquina do operador.
#
# ⚠️ NÃO BASTA ENCOLHER A VM DEIXANDO O BINÁRIO NA IMAGEM. Com o `demucs`
# presente, `StemSeparator.Available()` responde true, as rotas sobem, a UI oferece
# a ação — e o primeiro clique estoura a memória, matando a MÁQUINA (não só o
# recurso) e tirando a estação do ar. O binário ficar de fora é o que mantém a
# degradação graciosa honesta: sem ele o servidor responde 404, a UI esconde a
# função e nada quebra.
#
# PARA REATIVAR: descomente este bloco E o par COPY/ENV do runtime, volte o
# `[[vm]]` do fly.toml para performance/4/8gb e reponha AUTODJ_DEMUCS_THREADS=2.
#
# FROM debian:bookworm-slim AS demucs
# RUN apt-get update \
#  && apt-get install -y --no-install-recommends git cmake g++ make curl ca-certificates \
#       libopenblas-dev \
#  && rm -rf /var/lib/apt/lists/*
# # BLAS é OBRIGATÓRIO e não aparece no build local: no macOS o CMake acha o
# # Accelerate da Apple sozinho, então o `scripts/build-demucs.sh` nunca precisou
# # instalar nada. No Debian não há equivalente implícito e o configure aborta com
# # "Could NOT find BLAS" — foi exatamente assim que o primeiro deploy quebrou.
# WORKDIR /src
# RUN git clone --recurse-submodules --depth 1 https://github.com/sevagh/demucs.cpp.git .
# # O CMakeLists do upstream declara compat com CMake <3.5, removido no CMake 4.x —
# # a política afrouxa a checagem sem tocar no código deles.
# RUN cmake -B build -DCMAKE_BUILD_TYPE=Release -DCMAKE_POLICY_VERSION_MINIMUM=3.5 \
#  && cmake --build build --parallel "$(nproc)"
# # demucs_mt = multi-thread; o single-thread não entrega tempo utilizável.
# RUN install -m 0755 build/demucs_mt.cpp.main /usr/local/bin/demucs
# # pesos ggml pré-convertidos (~80 MB), os mesmos que o script local baixa.
# RUN curl -fL -o /htdemucs.bin \
#     https://huggingface.co/datasets/Retrobear/demucs.cpp/resolve/main/ggml-model-htdemucs-4s-f16.bin

FROM debian:bookworm-slim AS runtime
# ffmpeg: análise de reprodução (trim, loudness, entrada do vocal) e extração de
#   capa embutida.
# aubio-tools: BPM. Traz `aubiotrack` (tempos das batidas); o binário unificado
#   `aubio` pode não vir, e por isso o analisador tenta os dois — ver CLI.bpm.
#   ~2 MB. Sem ele, prod fica com BPM zerado em todo o acervo, em silêncio.
#   NÃO instala keyfinder-cli (tom musical): não existe no repositório do Debian e
#   exigiria compilar, o que não se paga por um campo de exibição.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg aubio-tools ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
# Separação por IA — DESLIGADA junto com o estágio de build acima (ver o porquê lá).
# Sem estas duas linhas `StemSeparator.Available()` é false, as rotas nem são
# registradas e a UI esconde a função: degradação graciosa, o servidor responde 404
# e nada quebra.
#
# libopenblas0 e libgomp1 saíram do apt junto: são as bibliotecas compartilhadas
# que só o binário do demucs carrega.
#
# COPY --from=demucs /usr/local/bin/demucs /usr/local/bin/demucs
# COPY --from=demucs /htdemucs.bin         /opt/autodj/models/htdemucs.bin
# ENV AUTODJ_DEMUCS_BIN=/usr/local/bin/demucs \
#     AUTODJ_DEMUCS_MODEL=/opt/autodj/models/htdemucs.bin

# ── usuário sem privilégio ───────────────────────────────────────────
# O container rodava como ROOT. Escapar de um container a partir de root é bem
# mais fácil, e este servidor executa ffmpeg/aubio SOBRE ARQUIVOS QUE O USUÁRIO
# ENVIA — é exatamente a superfície onde isso importa.
#
# ⚠️ HOME CONTINUA /root, DE PROPÓSITO — não é sobra da versão anterior.
# O servidor guarda tudo em `~/.autodj` (os.UserHomeDir) e grava `Track.Path`
# ABSOLUTO no banco. Mover o HOME junto com o usuário (tentado na v0.2.42) fez
# cada faixa existente continuar apontando para /root/.autodj/... enquanto o
# volume estava noutro lugar: a biblioteca listava e NENHUMA música tocava (403).
# O ganho de segurança é o processo não ser root; onde ficam os dados é outro
# assunto, e mudá-lo custaria reescrever caminho em todos os bancos.
RUN useradd --system --uid 10001 --home-dir /root --shell /usr/sbin/nologin app
ENV HOME=/root

COPY --from=build /out/server  /usr/local/bin/server
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=web   /web/out     /app/web/out
COPY migrations   /app/migrations
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENV PORT=8095 \
    WEB_DIR=/app/web/out

EXPOSE 8095
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
