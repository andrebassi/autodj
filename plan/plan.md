# AutoDJ — Plataforma de Automação de Rádio & DJ (Radio-as-Software)

## Context

Usuário quer software de automação de rádio + DJ, **visual/UX e funcionalidades iguais ou melhores que o RadioPro** (produto comercial BR, UI Windows de 2018), com **integração Spotify** e **mixagem DJ**. Roda como **web + desktop**. Fonte: **arquivos locais + Spotify**. Projeto novo, módulo Go **local `autodj`** (não `github.com/...`). Template mais próximo: `labs/polytrader` (Go hexagonal + Next.js `web/` + AIOX + Taskfile + fly.toml).

**Cortes explícitos do usuário (fora de escopo):** Controle de Contratos (gestor comercial/CRM/financeiro), Módulo Gerencial (dashboard abas CONFIG/COMERCIAL/MUSICAL/FINANCEIRO), Módulos Por Setor (deploy por sala). Comerciais permanecem **só como categoria de mídia** na grade — sem contrato/cliente/financeiro.

Decisões confirmadas: (1) **dois motores de áudio**; (2) motor Local/Pro = **Liquidsoap + Go**; (3) kickoff = **AIOX SDD completo**; (4) plataforma **web + desktop** (mesmo domínio Go, Wails embrulha web).

### Princípio central: split dois-motores define TODO o domínio

Spotify = DRM (sem PCM). Local = PCM próprio. Porta `PlaybackEngine` + value object `Playable{SourceKind}`. Matriz de capacidade (invariante):

```
SourceKind   PCM  crossfade  beatmatch  waveform  broadcast  ECAD
local        sim   sim        sim        sim       sim        sim
spotify      NÃO   fade-only  NÃO        NÃO       NÃO(ToS)   metadata
line_in      sim   sim        não        não       sim        parcial
tts          sim   sim        não        não       sim        n/a
```

Invariantes: `spotify ⇒ broadcast_allowed=false`, nunca entra no bus Icecast, só transição `fade_tts`/`hard_cut`. `Marker`/`Beatgrid`/`waveform` só p/ fontes PCM. `PlayLog.broadcasted/ecad_relevant=false` p/ Spotify.

## Regra de infra local (INVIOLÁVEL)

**Recursos locais (Postgres, Icecast, Liquidsoap) sobem via Nix — NUNCA Docker.**
`flake.nix` + `process-compose-flake`/`services-flake`. Toolchain via `nix develop`.
Docker/Dockerfile fica **só** para deploy remoto (Fly.io). Ver `~/.claude/rules/23-nix-local-not-docker.md`.
`task up` → `nix run .#services` (sobe stack local). Sem docker-compose para dev.

## Arquitetura

Monorepo Go hexagonal + Next.js + Wails. `domain/ports/adapters/cmd`, Taskfile, AIOX, cobertura domain 100% / adapters ≥90%.

### Por que Liquidsoap
Engine de rádio maduro (v2.4): scheduling, smart_crossfade, ReplayGain, fallback/mksafe/failover, harbor, output Icecast/HLS. Motor por trás de AzuraCast e LibreTime. Go gera `.liq` + comanda via socket Unix. Callbacks HTTP puxam próxima faixa da API Go.

### Tech stack
- Core: Go 1.26 hexagonal
- Broadcast: Liquidsoap 2.4 via socket Unix
- Streaming: Icecast-KH (fallback Icecast2)
- Metadata: `dhowden/tag` + `go-ffprobe`
- Análise: mixxx-analyzer (BPM+key+Camelot+LUFS) fallback aubio+keyfinder+rsgain
- Deck DJ browser: Web Audio + Tone.EQ3 + SoundTouchJS
- Waveform: audiowaveform → wavesurfer.js v7
- Spotify: Web API (search/playlist/metadata) + Web Playback SDK
- TTS: ElevenLabs (pt-BR) + Piper fallback
- DB: Postgres 17 (pgx) web / SQLite (modernc) desktop
- Realtime: WebSocket (coder/websocket)
- Auth: JWT + argon2id
- Frontend: Next.js 15 static export + Tailwind + shadcn
- Desktop: Wails v2
- Deploy: Fly.io

## Roadmap por fases (Épico 1 = PROTÓTIPO FUNCIONAL end-to-end)

- **Épico 1 — Protótipo funcional (walking skeleton)**: scaffold + fatia vertical mínima que transmite. `task up` sobe stack, scaneia pasta, monta fila, toca via Liquidsoap, streama Icecast, UI Player mostra now-playing ao vivo.
- **Épico 2 — Library rica**: metadata + árvore categorias + browser UI + análise BPM/key/waveform.
- **Épico 3 — Markers + mixagem**: editor waveform + marcadores + mixagem fina.
- **Épico 4 — Scheduler completo**: clocks/hour-templates + grade + geração + eventos + auto-pilot.
- **Épico 5 — DJ deck**: deck duplo Web Audio + cue + cartwall.
- **Épico 6 — Spotify + AI DJ**: OAuth + import + Web Playback SDK + auto-DJ falado.
- **Épico 7 — Broadcast pro + RBAC**: RDS + line-in + RBAC + PlayLog append-only.
- **Épico 8 — Relatórios/Social/Público**: ECAD + comprovante + pedidos + site público + social + info diária.
- **Épico 9 — Desktop/Multi-estação**: Wails + 2 monitores/touch + escala.

### Stories Épico 1
- `1.1.scaffold-e-nix` (@devops) — hexagonal skeleton, go.work, Taskfile, **Nix flake (Postgres+Icecast via services-flake; Liquidsoap por `task engine:up`)** — recursos locais via Nix, NUNCA Docker (regra 23). CI, fly.toml (Docker só remoto), docs AIOX.
- `1.2.domain-track-e-queue` (@dev) — Track, Playable/SourceKind + matriz, PlayQueue (domain 100%).
- `1.3.folder-scan-basico` (@dev) — scaneia pasta, metadata via ffprobe, insere Tracks.
- `1.4.liquidsoap-playback` (@dev, QG @architect) — gera .liq, fila, socket play/skip, crossfade.
- `1.5.icecast-broadcast` (@devops) — output Liquidsoap→Icecast, mount tocável.
- `1.6.player-ui-nowplaying` (@ux-design-expert, QG @dev) — Next.js Player + WebSocket ao vivo.

Grafo: 1.1→1.2→1.3→1.4→1.5; 1.2→1.6.

## Riscos
1. Spotify DRM/ToS + API encolhida → invariante `spotify⇒broadcast_allowed=false`.
2. Vazamento dois-motores → matriz na porta + invariantes.
3. Robustez broadcast 24/7 → Liquidsoap fallback/mksafe + supervisão Go.
4. Wails cross-compile → CLI out-of-process + modernc sqlite.
5. ECAD/comprovante → PlayLog imutável.
6. TTS latência → pré-gerar voiceover + cache + Piper fallback.
7. Correção scheduler → engine pure-domain + unit-test pesado.

## Verificação
- `task lib:scan` popula biblioteca.
- `task engine:up` + `task broadcast:test` → `ffplay http://localhost:8000/radio`.
- Player web: fila carrega, Play/Próxima, now-playing via WS.
- `go test ./...` verde, cobertura domain 100% / ≥90%.
