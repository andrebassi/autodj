# AutoDJ — Product Requirements Document (PRD)

> **⚠️ DESCONTINUADO (2026-07-25): broadcast por servidor (Liquidsoap + Icecast) foi
> removido.** Não transmitimos mais no servidor — o **playout roda no browser** (2
> motores: Spotify Web Playback SDK / `<audio>` local) e o servidor só orquestra
> fila/now-playing (`stubengine`). Os requisitos **FR6/FR7** (renderizar `.liq`,
> comandar Liquidsoap por socket, transmitir por `output.icecast`/mount `/radio`) e as
> partes de NFR1/NFR6/NFR7 que citam Liquidsoap/Icecast estão **obsoletos** — registro
> histórico do desenho original do Épico 1. O invariante `spotify ⇒ mixable=false`
> permanece (domínio + banco). Ver `docs/architecture.md` (changelog 0.1.16).

## Goals and Background Context

### Goals

- Entregar um software de automação de rádio + DJ ("radio-as-software"), rodando como **web + desktop**, com paridade visual/funcional em relação ao RadioPro e superando-o via integração Spotify e mixagem DJ.
- Modelar o domínio em torno do **split de dois motores de áudio** (Spotify = DRM/sem PCM; Local = PCM próprio) como invariante estrutural — não como um toggle de UI — garantindo que uma faixa Spotify **nunca** possa ser transmitida (`spotify ⇒ mixable=false`).
- Provar a arquitetura com um **protótipo funcional end-to-end (walking skeleton)** no Épico 1: scanear pasta → montar fila → tocar via Liquidsoap → transmitir por Icecast → Player web mostrando now-playing ao vivo por WebSocket.
- Usar **Go hexagonal** (domain/ports/adapters/cmd) como núcleo, com Liquidsoap 2.4 como motor de playout comandado por socket Unix (Go é o cérebro de scheduling, padrão AzuraCast) e Icecast para streaming.
- Manter recursos locais em **Nix, nunca Docker** (regra global 23); Docker apenas para deploy remoto (Fly.io).
- Garantir qualidade por gates de cobertura: domínio 100%, pacotes de adapter/app ≥90%, CI bloqueando abaixo disso.

### Background

AutoDJ nasce da necessidade de um sistema de automação de rádio moderno, web + desktop, com UX igual ou superior ao RadioPro (produto comercial BR de UI Windows de 2018), agregando integração com Spotify e um deck de mixagem DJ. A fonte de mídia é dupla: arquivos locais e Spotify. O ponto de decisão que define todo o resto é que **Spotify entrega áudio sob DRM/EME, sem acesso ao PCM**, enquanto arquivos locais nos dão o PCM bruto. Isso bifurca capacidade em toda a plataforma: crossfade real, beatmatch, waveform/marcadores, entrada no bus Icecast e relevância ECAD só existem para fontes PCM. Por isso o modelo carrega um `SourceKind` com uma **matriz de capacidade** consultada pela porta `PlaybackEngine`, pelo value object `Playable` e pela fila de playout.

O projeto adota **AIOX Spec-Driven Development** desde o kickoff e a arquitetura hexagonal Go (inspirada no template `labs/polytrader`: Go hexagonal + Next.js `web/` + Taskfile + `fly.toml`). Cortes explícitos de escopo (fora do produto em qualquer épico): Controle de Contratos (gestor comercial/CRM/financeiro), Módulo Gerencial e Módulos Por Setor. Comerciais permanecem **apenas como categoria de mídia** na grade — sem contrato, cliente ou financeiro associados. O Épico 1 já está **implementado**: uma fatia vertical mínima que transmite de ponta a ponta.

### Change Log

| Date | Version | Description | Author |
|------|---------|-------------|--------|
| 2026-07-24 | 0.1.0 | PRD inicial AIOX; Épico 1 (walking skeleton) implementado e documentado como Done | @pm |

## Requirements

### Functional

- **FR1**: O sistema DEVE modelar `SourceKind` ∈ {`local`, `spotify`, `line_in`, `tts`, `youtube`} e derivar de cada um uma matriz fixa de capacidade (PCM, crossfade, beatmatch, waveform, playout, ECAD).
- **FR2**: O sistema DEVE garantir a invariante `spotify ⇒ mixable=false`, impedindo faixas Spotify de entrarem no bus de playout em quatro camadas: domínio, fila, motor e banco.
- **FR3**: Uma `Track` local DEVE derivar `pcm_available=true` e `mixable=true`; o sistema NÃO DEVE permitir que essas flags sejam definidas livremente por um caller.
- **FR4**: O sistema DEVE escanear recursivamente uma pasta em busca de arquivos de áudio (mp3, flac, wav, ogg, m4a, aac, opus, aiff), extrair metadados via `ffprobe` (título, artista, álbum, duração, bitrate) e fazer upsert de `Track`s locais com id estável derivado do path (re-scans atualizam em vez de duplicar).
- **FR5**: O sistema DEVE montar uma `PlayQueue` de playout que aceita apenas fontes mixáveis e valida a transição contra o `SourceKind` de destino (crossfade só para fontes PCM).
- **FR6**: O motor de playout DEVE renderizar um script `.liq`, comandar o Liquidsoap por socket Unix (`push`, `skip`, `nowplaying`, `version`) e recusar qualquer fonte não-mixável com `ErrNotMixable`.
- **FR7**: O sistema DEVE transmitir o áudio produzido pelo Liquidsoap via `output.icecast` num mount tocável (ex.: `http://localhost:8000/radio`), com fallback `mksafe` garantindo que a saída nunca fique em silêncio.
- **FR8**: A API REST DEVE expor: `GET /api/health`, `POST /api/library/scan {root}`, `GET /api/library/tracks`, `POST /api/playback/load`, `POST /api/playback/next`, `GET /api/playback/nowplaying`.
- **FR9**: O sistema DEVE expor um endpoint WebSocket `/ws` que envia snapshots de now-playing + fila a cada 2s (e um snapshot imediato na conexão), no shape `nowPlayingMsg{type, now_playing, queue[]}`.
- **FR10**: O Player web (Next.js) DEVE mostrar a faixa tocando agora, a fila e um botão "Próxima", atualizando ao vivo por WebSocket e reconectando automaticamente quando a conexão cai.
- **FR11**: O `trackDTO` da API DEVE surfacer as flags de capacidade (`mixable`, `pcm_available`) para que o frontend possa gatear features PCM-only por fonte.
- **FR12** (YouTube — login OPCIONAL, gate só na busca; **revisado na v0.2.17**, antes era login obrigatório): A aba YouTube DEVE ter **duas sub-abas**. (a) **"Colar links"** DEVE funcionar **sem login nenhum**: um campo multi-linha onde o operador cola endereços de vídeo separados por `;` (também vírgula/quebra de linha/tab/espaço) e o sistema resolve, importa e enfileira o lote de uma vez (`POST /api/v1/youtube/import-links`, cap **50** links, dedupe preservando a ordem colada, `watch?v=`/`youtu.be`/`shorts`/`embed`/id cru). (b) **"Buscar"** DEVE oferecer **dois motores chaveáveis** (ver FR13): o motor **`free`** (default) DEVE buscar **sem login nenhum**, e apenas o motor **`api`** (YouTube Data API) DEVE exigir conta Google conectada (OAuth auth-code **com client secret**), com o card de conexão **dentro** dessa sub-aba e visível só nesse motor. Racional do gate assimétrico: `search.list` custa ~100 unidades das ~10k/dia, enquanto `videos.list` custa 1 por lote — o login é **escudo de cota do `search.list`** num serviço público multi-tenant, não requisito de identidade; os caminhos que não consomem cota (motor `free` e colar links) NÃO DEVEM ser gateados. **Nenhuma operação de import DEVE exigir conta** — nem `import-links`, nem `import` de vídeo avulso, nem `import` de playlist: todas resolvem metadados pela **API KEY** (o token OAuth é ignorado pelo catálogo) a 1 unidade por lote. Em particular, enfileirar/arrastar pro deck um resultado achado no motor `free` NÃO DEVE pedir login — gatear ali torna a busca livre inútil na prática. O painel DEVE abrir na sub-aba que funciona no estado atual: "Buscar" sempre que houver motor de busca disponível (grátis **ou** conta ligada), caindo em "Colar links" só quando buscar for impossível (**revisado na v0.2.18** — antes abria em "Colar links" sempre que desconectado, o que escondia o chaveador de motor). O sistema DEVE guardar 1 `youtube_account` por estação (fingerprint), com refresh de token, roteado ao tenant correto via state-smuggling `<sid>~<state>`.
- **FR13** (YouTube — DOIS motores de busca chaveáveis; **revisado na v0.2.17**, antes só existia a Data API): A busca DEVE expor dois motores atrás da MESMA porta de catálogo, selecionáveis pelo operador via `?backend=` (`free` | `api`, default `free`), **sem remover nenhum dos dois**. (a) Motor **`free`** DEVE consultar o YouTube pelo protocolo InnerTube (lib Go `github.com/raitonoberu/ytsearch`), **sem API key, sem conta e com custo ZERO de cota**. (b) Motor **`api`** DEVE usar a **YouTube Data API v3 com API KEY própria** (`AUTODJ_YOUTUBE_API_KEY`, não o token OAuth): `search.list` (type=video,playlist) + `playlistItems.list` + `videos.list` para durações (ISO-8601 `PT#M#S` → ms) e thumbnail → `ArtworkURL`. Em AMBOS os motores, a resolução de ids no import (`videos.list`/`playlistItems.list`, 1 unidade por lote) DEVE continuar na Data API, para preservar duração exata e o filtro `embeddable`; falha de parse no motor `free` DEVE cair no motor `api` (degradar, não quebrar). A UI DEVE indicar que no motor `free` um vídeo não-incorporável só é descartado no import. Import mapeia `CatalogItem → Track(source=youtube, YouTubeURI)`.
- **FR14** (YouTube — invariante sem PCM): O sistema DEVE garantir a invariante **`youtube ⇒ mixable=false`** em 2 camadas — domínio (`Track.Validate`, `SourceYouTube ⇒ YouTubeURI != ""`) e banco (CHECK `youtube_never_mixable`). Faixa YouTube NUNCA entra no bus de mix PCM (sem crossfade/beatmatch/waveform); toca **out-of-band** com `hard_cut`/`fade_tts`.
- **FR15** (YouTube — playout no deck): Ao tocar uma faixa YouTube, o deck ao vivo DEVE chavear para o **YouTube IFrame Player API** (grátis, sem premium) no **MESMO slot de capa 60×60** (`.cover-deck`), com progresso/timers reais e auto-avanço ao terminar (evento `ENDED` + fallback por duração). O playout NUNCA DEVE ser gated em "Premium" (best-effort, só paridade do chip). O player IFrame DEVE ficar visível (ToS), o que o slot 60×60 satisfaz.
- **FR16** (YouTube — degradação graciosa em **dois níveis**; **revisado na v0.2.17**, antes exigia as 3 vars juntas): Sem `AUTODJ_YOUTUBE_API_KEY`, as rotas `/youtube/*` DEVEM responder **404** e a aba DEVE sumir da UI (mesmo padrão do Spotify sem `AUTODJ_SPOTIFY_CLIENT_ID`). **Com a API key mas sem `AUTODJ_YOUTUBE_CLIENT_ID`/`_CLIENT_SECRET`**, a aba DEVE subir, a sub-aba "Colar links" DEVE funcionar por inteiro e a busca DEVE funcionar no motor `free`; apenas `POST /youtube/connect` e `GET /youtube/callback` DEVEM responder **503** (integração de conta indisponível), nunca 404/500. O status DEVE publicar `freeSearch` (motor grátis fiado no servidor); quando falso, a UI DEVE esconder o chaveador e usar o motor `api`. Segredos SEMPRE via env/`pass`, nunca no código.

### Non Functional

- **NFR1**: Recursos locais (Icecast, Liquidsoap) DEVEM subir via **Nix** (`nix run .#services` + `task engine:up`), **nunca Docker** (regra global 23). Persistência local é **SQLite por usuário logado** (1 arquivo/user, sem daemon) — não sobe serviço de DB; Postgres é opcional (só p/ exercitar o adapter remoto). Docker é permitido apenas para a imagem de deploy remoto (Fly.io).
- **NFR2**: O núcleo Go DEVE seguir arquitetura hexagonal com direção de dependência `adapters → ports → domain`; o pacote `domain/` NÃO DEVE importar libs externas.
- **NFR3**: Cobertura de testes: pacotes `domain/` em **100%**; pacotes de adapter/app em **≥90%**; CI bloqueia builds abaixo do gate. Pacotes `cmd/*` e `adapters/driven/postgres` são integration-tier e ficam fora do gate unitário.
- **NFR4**: O módulo Go DEVE ser **local** (`module autodj`), com imports no formato `autodj/domain/shared` etc. — nunca `github.com/...`.
- **NFR5**: Metadados DEVEM ser extraídos out-of-process via `ffprobe`, mantendo o servidor cgo-free (`CGO_ENABLED=0`) e evitando dor de cross-compile nativo (relevante para o futuro empacotamento Wails).
- **NFR6**: O playout DEVE ser 24/7-safe: `mksafe` no Liquidsoap para nunca ficar mudo; no Fly.io, `auto_stop_machines = "off"` e `min_machines_running = 1`.
- **NFR7**: O servidor DEVE degradar graciosamente no repositório: default local = **SQLite por usuário** (arquivo durável), Postgres quando remoto/escala (`DATABASE_URL`), memória como fallback dev; e usar Liquidsoap se o socket responder, senão o stub in-process (a UI continua funcionando, sem playout). *Estado atual: só `memory` e `postgres` implementados; adapter `sqlite` (`modernc.org/sqlite`) é o próximo passo.*
- **NFR8** (erros legíveis + LIMITAÇÃO da cota YouTube — v0.2.5): Erros das integrações (YouTube/Spotify) DEVEM chegar à UI como frase pt-BR clara, nunca código cru (`youtubeSearch: 500`). Backend mapeia erros de domínio → status HTTP correto (cota esgotada → **429**, não 500, via `ErrQuotaExceeded` embrulhado em `catalog.go` e traduzido em `writeYouTubeErr`); frontend humaniza status+corpo num só lugar (`web/app/lib/errors.ts`, aplicado por `throwHuman` em `api.ts` preservando `.status`). **LIMITAÇÃO CONHECIDA**: a busca do YouTube usa a Data API v3 com **teto grátis 10k unidades/dia por projeto** ÷ ~100 por `search.list` ≈ **~100 buscas/dia** (reset ~meia-noite Pacífico). Esse teto vale **só para o motor `api`**, que depende da API key **e** do login OAuth (escudo da cota); o motor **`free`** (InnerTube, default desde a v0.2.17) busca **sem conta e sem gastar cota**, e **colar links importa sem login** custando ~1 unidade por lote de até 50 — playout não gasta cota nenhuma. Aumentar: chavear para o motor `free` (custo zero) / importar por URL (`videos.list` = 1 unidade, 100× mais barato) / múltiplos projetos + round-robin de keys (não implementado — key única) / audit oficial do Google. Detalhes em `docs/architecture.md` → "Limitações conhecidas".

## Technical Assumptions

### Repository Structure

Monorepo hexagonal único: `domain/`, `ports/`, `adapters/`, `app/`, `cmd/`, `migrations/`, `web/` (Next.js), `deploy/`, `scripts/`, `docs/` (AIOX). O `go.work` da raiz do workspace é opcional; o módulo AutoDJ builda standalone com `GOWORK=off`.

### Service Architecture

Um único binário `server` orquestra REST + WebSocket + serve o export estático do Next.js. O playout roda como processos irmãos: Icecast (streaming) e Liquidsoap (playout, comandado pelo Go via socket Unix). Go é o cérebro de scheduling (padrão AzuraCast); Liquidsoap é o mixer/output e nunca é reimplementado em Go. Metadados via `ffprobe` out-of-process. Repositório com adapters intercambiáveis por trás da mesma `TrackRepo`: **SQLite por usuário** (default local, `modernc.org/sqlite`, planejado), Postgres via pgx (remoto/escala) e memória (fallback dev); e dois motores intercambiáveis (Liquidsoap / stub).

### Testing Requirements

Testes table-driven em Go, com fakes determinísticos: fake Unix socket telnet-style para o Liquidsoap engine; mp3 real gerado por ffmpeg (skip se ausente) para o ffprobe; `httptest` + `coder/websocket` para o hub WS. Gate de cobertura via `scripts/coverage-gate.sh` no CI (domain 100%, adapters/app ≥90%). Postgres e `cmd/*` são integration-tier (excluídos do gate unitário). Frontend validado por build estático (`pnpm build`) no CI.

### Additional

- Go 1.26, Liquidsoap 2.4, Icecast (Icecast-KH em prod, icecast2 fallback), Next.js 15 + React 19 (static export), `modernc.org/sqlite` (persistência local por usuário, planejado), pgx/v5 (remoto/escala), chi/v5, coder/websocket.
- Deploy Fly.io region `gru`, imagem única com Icecast + Liquidsoap + ffprobe + server.
- Créditos/pass/segredos fora do repositório (senha Icecast injetada como Fly secret).

## Epic List

- **Épico 1 — Protótipo Funcional (walking skeleton)**: entregar uma fatia vertical mínima que transmite de ponta a ponta — scanear pasta, montar fila, tocar via Liquidsoap, streamar Icecast e mostrar now-playing ao vivo no Player web. **[IMPLEMENTADO]**
- **Épico 2 — Library Rica**: metadados completos + árvore de categorias + browser UI + análise de BPM/key/waveform para fontes PCM.
- **Épico 3 — Markers + Mixagem**: editor de waveform, marcadores (cue/intro/outro/mix points) e mixagem fina entre faixas PCM.
- **Épico 4 — Scheduler Completo**: clocks/hour-templates, grade de programação, geração de playlist, eventos agendados e auto-pilot.
- **Épico 5 — DJ Deck**: deck duplo em Web Audio (cue, EQ, pitch/beatmatch) + cartwall.
- **Épico 6 — Spotify + AI DJ**: OAuth Spotify, import de metadados/playlists, Web Playback SDK e AI DJ falado (TTS) respeitando o split de dois motores.
- **Épico 7 — Playout Pro + RBAC**: múltiplas saídas (RDS/HLS), line-in de hardware, RBAC e `PlayLog` append-only imutável.
- **Épico 8 — Relatórios / Social / Público**: relatório ECAD + comprovante, pedidos de ouvinte, site público, integrações sociais e info diária.
- **Épico 9 — Desktop / Multi-estação**: empacotamento Wails, layouts 2-monitores/touch e escala multi-estação.

## Epic Details

### Épico 1 — Protótipo Funcional (walking skeleton) — [IMPLEMENTADO]

**Goal**: Provar a arquitetura de ponta a ponta com a menor fatia vertical possível que efetivamente transmite: subir a stack local via Nix, scanear uma pasta de áudio, montar a fila de playout, tocar via Liquidsoap, streamar por Icecast e exibir now-playing ao vivo no Player web. Ao final, `ffplay http://localhost:8000/radio` toca a rádio e o Player web mostra a faixa atual atualizando por WebSocket. O split de dois motores já está gravado no domínio e é enforçado nas quatro camadas.

#### Story 1.1 — Scaffold + Nix

**As a** engenheiro de plataforma,
**I want** um esqueleto hexagonal Go com stack local em Nix e CI com gate de cobertura,
**so that** todo trabalho subsequente parte de uma base reprodutível, sem Docker para dev.

Acceptance Criteria:
1. `nix run .#services` sobe a stack local (Icecast; Postgres opcional) sem Docker — persistência local é SQLite por usuário (arquivo).
2. O módulo Go é `autodj` (local), com pastas `domain/ports/adapters/app/cmd`, e `task -l` lista as tasks.
3. CI (`.github/workflows/ci.yml`) roda `go vet`, `go test -cover` e o `coverage-gate.sh` (domain 100%, adapters ≥90%); o job web faz `pnpm build`.
4. Docs AIOX (`docs/prd.md`, `docs/architecture.md`, `docs/stories/`) existem.

#### Story 1.2 — Domínio Track + Queue

**As a** desenvolvedor de domínio,
**I want** as entidades `Track`, `Playable`/`SourceKind` com a matriz de capacidade e a `PlayQueue`,
**so that** o split de dois motores fique enforçado no domínio com cobertura 100%.

Acceptance Criteria:
1. `shared.Capabilities(SourceKind)` retorna a matriz correta para local/spotify/line_in/tts.
2. `library.NewTrack` deriva `PCMAvailable`/`Mixable` da fonte e `Validate` os re-deriva mesmo após mutação de campos.
3. `PlayQueue{Playout:true}.Enqueue` rejeita fonte não-mixável (`ErrNotMixable`) e transição inválida (`ErrBadTransition`).
4. Cobertura dos pacotes `domain/*` = 100%.

#### Story 1.3 — Folder Scan Básico

**As a** operador de rádio,
**I want** escanear uma pasta e popular a biblioteca com metadados,
**so that** eu tenha faixas locais mixáveis para tocar.

Acceptance Criteria:
1. `POST /api/library/scan {root}` percorre a pasta, extrai metadados via `ffprobe` e faz upsert de `Track`s locais com `pcm_available=true`.
2. Arquivos que falham extração/validação são pulados (scan best-effort); o id é estável por path (re-scan atualiza).
3. `GET /api/library/tracks` lista o catálogo com as flags de capacidade no DTO.

#### Story 1.4 — Liquidsoap Playback

**As a** operador de rádio,
**I want** que a fila seja empurrada para o Liquidsoap por socket, com skip e crossfade,
**so that** o áudio local toque de fato com transições suaves.

Acceptance Criteria:
1. O engine renderiza um `.liq` com `request.queue`, `crossfade`, `mksafe`, `output.icecast` e o comando custom `autodj.nowplaying`.
2. `Push` empurra faixas mixáveis via socket (`file://` prefixado quando sem scheme); `Skip` avança; `NowPlaying` lê o título.
3. `Push` recusa fonte Spotify com `ErrNotMixable`; crossfade é a transição default no load.
4. Se o socket não responder, o servidor cai para o `stubengine` (UI funciona, sem playout).

#### Story 1.5 — Icecast Broadcast

**As a** ouvinte,
**I want** um stream Icecast tocável do áudio da rádio,
**so that** eu possa ouvir a rádio em qualquer player.

Acceptance Criteria:
1. O Liquidsoap emite `output.icecast` no mount `/radio`; `ffplay http://localhost:8000/radio` toca o stream.
2. Icecast sobe localmente via `flake.nix` (`nix run .#services`) — sem Docker.
3. A imagem de deploy remoto (Fly.io) empacota Icecast + Liquidsoap + server num entrypoint único (Icecast → Liquidsoap → migrate → server).

#### Story 1.6 — Player UI Now-Playing

**As a** operador/ouvinte,
**I want** um Player web mostrando now-playing e fila ao vivo com botão Próxima,
**so that** eu veja e controle a rádio pelo navegador.

Acceptance Criteria:
1. O Player Next.js mostra "Tocando agora", a fila e os botões "Carregar biblioteca" e "Próxima".
2. As atualizações chegam ao vivo por WebSocket (`/ws`, snapshot a cada 2s + snapshot imediato na conexão).
3. Ao cair a conexão, o Player mostra "reconectando" e reconecta automaticamente.

### Épico 2 — Library Rica

**Goal**: Transformar o catálogo mínimo numa biblioteca completa com metadados ricos, categorias hierárquicas, browser navegável e análise de áudio (BPM/key/waveform) para fontes PCM.

Stories (esboço): 2.1 metadados estendidos + tags; 2.2 árvore de categorias (rotation buckets); 2.3 browser UI com busca/filtro; 2.4 análise BPM/key (Camelot)/LUFS; 2.5 geração de waveform/peaks.

### Épico 3 — Markers + Mixagem

**Goal**: Editor de waveform com marcadores (cue in/out, intro, outro, mix points) e controle fino de mixagem entre faixas PCM.

Stories (esboço): 3.1 editor de waveform (wavesurfer); 3.2 modelo `Marker`/`Beatgrid` PCM-only; 3.3 persistência de markers; 3.4 aplicação de mix points no crossfade.

### Épico 4 — Scheduler Completo

**Goal**: Programação automática via clocks/hour-templates, grade, geração de playlist, eventos agendados e auto-pilot.

Stories (esboço): 4.1 clocks/hour-templates; 4.2 grade semanal; 4.3 geração de playlist por regras de rotação; 4.4 eventos agendados; 4.5 auto-pilot + supervisão.

### Épico 5 — DJ Deck

**Goal**: Deck duplo de DJ no navegador (Web Audio), com cue, EQ, pitch/beatmatch e cartwall.

Stories (esboço): 5.1 deck duplo Web Audio; 5.2 cue + hot cues; 5.3 EQ3 + pitch/beatmatch; 5.4 cartwall de efeitos/jingles.

### Épico 6 — Spotify + YouTube + AI DJ

**Goal**: Integrar Spotify e YouTube respeitando o split de motores (sem PCM, sem mix) e adicionar AI DJ falado por TTS. YouTube = espelho do Spotify (busca via key própria, playout no deck), com duas divergências: a estrutural do IFrame Player e a de acesso — **login Google é opcional** (gateia só o motor de busca que gasta cota; colar links e o motor grátis não pedem conta).

Stories (esboço): 6.1 OAuth Spotify; 6.2 import de metadados/playlists (source=spotify); 6.3 Web Playback SDK (fade-only, nunca no playout); 6.4 AI DJ TTS (source=tts) com voiceover pré-gerado; **6.5 OAuth YouTube (Google, com secret) + domínio/ports/service/repos/migração; 6.6 busca/import YouTube (Data API v3 com key) + aba na Operacional; 6.7 playout YouTube no deck (IFrame Player persistente no slot 60×60, terceiro branch do seletor de motor); 6.8 import por links colados (sem login); 6.9 busca sem login (InnerTube) + chaveador de motor**.

### Épico 7 — Playout Pro + RBAC

**Goal**: Playout profissional com múltiplas saídas, line-in de hardware, RBAC e `PlayLog` append-only imutável.

Stories (esboço): 7.1 line-in (source=line_in); 7.2 múltiplas saídas (RDS/HLS); 7.3 RBAC + JWT/argon2id; 7.4 `PlayLog` append-only.

### Épico 8 — Relatórios / Social / Público

**Goal**: Relatório ECAD + comprovante, pedidos de ouvinte, site público e integrações sociais.

Stories (esboço): 8.1 relatório ECAD (ecad_relevant); 8.2 comprovante de execução; 8.3 pedidos/dedicatórias; 8.4 site público + info diária; 8.5 integrações sociais.

### Épico 9 — Desktop / Multi-estação

**Goal**: Empacotar o mesmo domínio Go em desktop via Wails e escalar para múltiplas estações.

Stories (esboço): 9.1 wrapper Wails v2; 9.2 SQLite (modernc) desktop; 9.3 layouts 2-monitores/touch; 9.4 multi-estação.

### Épico 10 — Áudio das faixas locais (trim, ganho, vocal) — [IMPLEMENTADO]

**Goal**: Fazer o acervo próprio soar como rádio: sem vazio nas pontas, no mesmo
volume, com a entrada da voz conhecida para encaixar vinheta/locução — e, ligando um
botão, **sem o vocal**.

Os três sinais saem de **uma única passada de `ffmpeg`** (o binário que já existe nos
dois ambientes — `aubio`/`keyfinder-cli`/`audiowaveform` nunca estiveram na imagem de
produção, então o analisador ali era o `Noop`). Metade da configuração já estava
declarada em `mixing.Settings` desde a Story 3.3 **sem nenhum código lendo**; este
épico é, sobretudo, ligar o que estava pendurado.

Stories: [10.1 trim de silêncio](stories/10.1.trim-de-silencio.md);
[10.2 ganho constante](stories/10.2.ganho-constante.md);
[10.3 entrada do vocal e LOOP](stories/10.3.entrada-do-vocal-e-loop.md);
[10.4 remover o vocal (modo VOZ)](stories/10.4.remover-vocal.md);
[10.5 separação por IA](stories/10.5.separacao-por-ia.md);
[10.6 separação na nuvem](stories/10.6.separacao-na-nuvem.md) (proposta).

A 10.4 entregou o botão **VOZ** por cancelamento de centro (instantâneo, exigindo
imagem estéreo) e a 10.5 a separação por **IA** (Demucs via `demucs.cpp`,
pré-render por faixa). **A 0.2.32 aposentou o cancelamento de centro**: medido em
música real com o próprio modelo como régua, ele reduzia a voz em **2,0 dB** contra
**31,9 dB** do instrumental separado — um botão que promete tirar a voz e tira 2 dB
engana mais do que ajuda. O botão VOZ da 10.4 continua existindo e passou a ser a
**interface da 10.5**: liga só em faixa já preparada, e o `stereo_side_db` que a 10.4
media saiu do domínio, dos repos e dos DTOs junto com o DSP.

⚠️ A justificativa antiga para recusar IA (Cross-Origin Isolation quebraria
YouTube/Spotify; sem implementação Go) estava **errada** — ver a correção em
`docs/pendencias.md`. O impedimento real é **velocidade** (~1,4× o tempo real,
medido), o que faz da IA um **pré-render por faixa**, opt-in e local — nunca um
botão ao vivo, e nunca em produção (`shared-cpu-1x`, 1 GB). O que a medição da 0.2.32
acrescentou é que **não há alternativa instantânea aceitável**: preparar a faixa é o
preço de tirar a voz de verdade.

Escopo explícito: **só faixas locais (PCM)**. Spotify e YouTube tocam out-of-band, sem
PCM deste lado — não há o que medir nem o que cortar, e o invariante
`spotify|youtube ⇒ !mixable` continua intocado.

## Next Steps

### Architect Prompt

Use este PRD para produzir `docs/architecture.md` seguindo o template fullstack AIOX. Ancore tudo no split de dois motores (matriz de capacidade em `domain/shared`), na direção hexagonal `adapters → ports → domain`, no padrão AzuraCast (Go comanda Liquidsoap por socket Unix; Icecast streama) e na regra de infra local Nix-não-Docker. Detalhe o schema real (`migrations/001_tracks.sql`, incluindo as duas CHECK constraints), a API REST + shape WS e o wiring condicional em `cmd/server` (default local SQLite-por-usuário — planejado; Postgres-se-DATABASE_URL para remoto/escala; senão memória; Liquidsoap-se-socket-saudável-senão-stub).

### UX Expert Prompt

A partir do Player mínimo já entregue (`web/app/page.tsx`), evolua a UX rumo à paridade com o RadioPro: browser de biblioteca, editor de waveform/markers, deck DJ duplo e grade do scheduler — sempre gateando features PCM-only (crossfade/beatmatch/waveform) pelas flags de capacidade expostas no `trackDTO`. Mantenha o tema escuro atual como base do design system.
