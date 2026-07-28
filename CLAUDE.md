# CLAUDE.md — AutoDJ

Tocador de música que anda sozinho, com mixagem de DJ. Go hexagonal + Next.js.
⚠️ **NÃO chamar de "rádio" nem de "automação de rádio"** (pedido do operador,
registrado em `AboutAutoDJ.tsx`): o padrão do app é o modo **Tocador** — o acervo
da pessoa tocando sozinho. Estação/rádio (grade, vinhetas, relógios, RDS) é um
**modo opcional**, não o que o produto é.
**Playout roda no browser** (3 motores: Spotify SDK / YouTube IFrame / áudio local);
o servidor só orquestra fila/now-playing. Deploy Fly.io. AIOX SDD (`docs/prd.md` →
`architecture.md` → `stories/`). **Broadcast por servidor (Liquidsoap/Icecast) foi
descontinuado** — não transmitimos mais no servidor.

> **O que NÃO está pronto?** [`docs/pendencias.md`](docs/pendencias.md) — o que foi
> pedido e não foi feito, o que funciona pela metade (com o limite explícito) e o que
> é impossível por plataforma. Consultar ANTES de prometer comportamento ao operador
> ou de "reimplementar" algo que já foi avaliado e recusado.
>
> **Algo não toca / some do deck / trava em 00:00?** Ler primeiro
> **[`docs/known-issues.md`](docs/known-issues.md)** — catálogo de erros conhecidos
> (sintoma → causa-raiz → fix → como reconhecer), com índice por sintoma. Este arquivo
> guarda o detalhe de implementação; o `known-issues.md` guarda o diagnóstico operacional.
> **Todo bug com causa-raiz provada entra lá no mesmo passo do fix.**
>
> Também vale para **ambiente de dev** (seção `DEV-*`), que é onde mais se perde tempo:
> feature nova que não aparece porque o binário Go é velho (`DEV-5`), "dou refresh e some
> tudo" (`DEV-6`, era o modo `memory`), deploy invisível por cache do HTML (`DEV-7`),
> `curl` e browser em tenants diferentes (`DEV-8`), `apiBase()` sem o `/api/v1` (`DEV-9`).
> E a pesquisa **YouTube sem API key** (`YT-11`): o que foi medido, o que não funcionou, e
> por que `playabilityStatus` medido do servidor não serve de filtro.

## Regras invioláveis

- **Módulo Go é LOCAL: `autodj`** (NÃO `github.com/...`). Imports: `autodj/domain/shared`.
- **Recursos locais via Nix, NUNCA Docker** (regra global 23). Docker só p/ deploy
  remoto (Fly.io). Local = `task serve` (server + web/out; playout no browser);
  Postgres opcional via `nix run .#services`; shell = `nix develop`.
- **Cobertura**: domain 100%, adapters/app ≥90%. CI bloqueia. `postgres` e `cmd/*`
  são integration-tier (fora do gate unitário). Gate: `task cover`.
- **`GOWORK=off`** — este módulo builda standalone (o `go.work` da raiz é opcional).
  O Taskfile já exporta `GOWORK=off`.
- **Frontend SEMPRE em componentes** (INVIOLÁVEL). Nada de JSX solto grande no
  `page.tsx` / rotas. Toda peça de UI = 1 componente em `web/app/components/*.tsx`
  (um por arquivo, `export function`). Dados/mocks em `web/app/lib/`, hooks em
  `web/app/hooks/`. `page.tsx` só orquestra. Import via alias `@/*` (→ `web/app/*`).
- **Persistência local = SQLite por browser** (1 arquivo por fingerprint FingerprintJS
  em `~/.autodj/sessions/<fp>.db`, `modernc.org/sqlite` pure-Go, sem daemon; isolamento
  por arquivo — cada browser = estação própria, ver seção "Estação por browser").
  Postgres é só remoto/multiusuário e entra **quando escalar**. Uso caseiro não sobe
  servidor de DB.
- **CÓDIGO EM INGLÊS, TEXTO EM PT-BR (INVIOLÁVEL — regra GLOBAL, vale em todos os
  projetos: `~/.claude/rules/24-code-english.md`)**: **todo objeto de programação
  em inglês** — função, variável, campo, tipo, prop, ref, estado, chave de objeto,
  constante, nome de teste, arquivo e **classe CSS**. **Comentários (`//`, `/* */`)
  e textos de UI ficam em pt-BR**, que é a língua do operador. Vale inclusive para
  código descartável (script de diagnóstico, snippet de console): se aparece numa
  tela ou num log, segue a regra.
  Exceção única e explícita: **valores de contrato com o servidor** (chaves de
  período `dia`/`semana`/`mes` do playlog, `Kind` de mídia como `hora`/`vinheta`,
  o kind de evento `hora_certa`) — renomear ali quebra o acordo com o backend; o
  identificador acompanha o valor de propósito.
  ⚠️ **Ao renomear em lote, script com regex NÃO basta** (aprendido no sweep da
  0.2.26): ele traduz texto de UI dentro de `${...}` de template literal, e um
  valor de estado que vira **classe CSS** (`"up-" + status`) exige mexer no
  `globals.css` na mesma passada, senão a tela perde o estilo em silêncio. Máscara
  de comentário/string ajuda, mas o que fecha a conta é `task web:build` +
  `go test ./...` e olhar a tela.
- **DOC SEMPRE ATUALIZADA (INVIOLÁVEL)**: qualquer mexida em feature — criar,
  alterar, evoluir, remover ou reverter — exige atualizar a documentação no MESMO
  passo. Nunca deixar `CLAUDE.md`, `docs/prd.md`, `docs/architecture.md`,
  `docs/stories/` ou README do módulo desatualizados em relação ao código. Feature
  mudou → doc muda junto, sempre.
- **CHANGELOG A CADA VERSÃO (INVIOLÁVEL)**: todo incremento de versão (nova `git tag
  vX.Y.Z`) exige uma entrada correspondente no TOPO de `web/app/lib/changelog.ts`
  (mais recente primeiro: `version/date/title/highlights[]`, pt-BR, voltado ao
  usuário). O gate `scripts/changelog-check.sh` **aborta o build** (`task web:build`,
  `task desktop:build`, `task deploy`) se a git tag mais recente não tiver entrada no
  changelog. A versão exibida no titlebar vem da git tag (`NEXT_PUBLIC_APP_VERSION`,
  injetada por `scripts/web-build.sh`/Dockerfile `ARG APP_VERSION`); clicar nela abre
  o `ChangelogModal`. Bumpou versão → adicione a entrada no MESMO passo.
  ⚠️ **O RÓTULO SAI DE `scripts/app-version.sh`, NÃO do `changelog-check.sh` (0.2.17)**:
  o gate segue devolvendo a versão da tag, mas quem decide o que aparece na tela é o
  `app-version.sh` — HEAD **exatamente** na tag **e** árvore limpa → `v<tag>` (release);
  qualquer outra coisa → `v<próxima patch>-dev.<sha>` (+ `.dirty` se houver alteração
  não commitada). Motivo: antes o número vinha da última tag independentemente do que
  o HEAD continha, então build local pós-release exibia o número idêntico ao de prod e —
  pior — um `web/out` velho exibia versão MENOR que a de prod sem sinal de staleness.
  Consequência prática: **`-dev` no número = não é produção**, e deploy fora da tag
  rotula a imagem como `-dev` de propósito. Ao mexer em versão, mexa no `app-version.sh`
  (ponto único), nunca reintroduza `git describe` solto no `web-build.sh`/Taskfile.
- **BRANCH DE TRABALHO = `develop` (INVIOLÁVEL, OBRIGATÓRIO)**: todo commit do dia a dia
  vai na `develop`. **`main` = produção** — só recebe o que vai pro ar, via **merge de
  `develop` → `main`** no momento do deploy (nunca commit direto na `main`). Nada de
  branches `feature/*` avulsas por padrão; se precisar isolar algo grande, a branch nasce
  de `develop` e volta pra `develop`. Sequência de release: commit na `develop` → doc +
  changelog + `git tag vX.Y.Z` → `git checkout main && git merge develop` → `task deploy`
  → voltar pra `develop`. Tags de versão ficam no commit que foi pra prod. Remote =
  `ab:bassi-projects/autodj.git` (`origin`).
- **DEPLOY EM PROD SÓ COM AUTORIZAÇÃO EXPLÍCITA DO USUÁRIO (INVIOLÁVEL, OBRIGATÓRIO)**:
  `task deploy` (Fly.io, app `autodj`) **nunca** roda por iniciativa própria — nem depois
  de "está tudo pronto", nem como último passo natural de uma tarefa, nem porque os testes
  passaram. É preciso um "pode subir / põe em prod / deploy" **explícito, daquela vez**.
  Autorização não é retroativa nem se estende ao próximo deploy. O mesmo vale pra tudo que
  altera produção: `fly secrets set`, `fly scale`, restart de máquina, mudança de DNS.
  Merge `develop` → `main` acontece no ato do deploy autorizado. Sem autorização: deixar
  pronto na `develop`, avisar que está pronto, e **parar aí**.
- **⚠️ O CI SÓ COMEÇOU A RODAR NA PUBLICAÇÃO (2026-07-28)** — o
  `.github/workflows/ci.yml` existia há muitas versões, mas o repositório vivia no
  GitLab, então **nenhuma execução tinha acontecido**. A primeira encontrou dois
  bugs só-de-Linux com ~45 versões de idade: `mktemp -t` (BSD-only) no
  `coverage-gate.sh`, e cobertura do `adapters/driving/ws` dependente de corrida
  de tempo (92% no Mac, 80% no runner). Ver `DEV-11` no known-issues.
  **Consequência prática**: portão que nunca executou é decoração, e cobertura que
  muda de máquina não mede nada — teste que depende de `time.Sleep`/tique para
  ALCANÇAR um caminho deve chamar o caminho direto.
- Timeout 30s em comandos; logging `2>&1 | tee /tmp/log.txt`; Taskfile primeiro.

## Princípio central — três motores de áudio

Uso **pessoal/caseiro** (não transmissão pública): saída é playout local. Spotify =
DRM (sem PCM). YouTube = embed IFrame (sem PCM). Local = PCM próprio. Bifurca o
domínio via porta `PlaybackEngine` + value object `Playable{SourceKind}` + matriz de
capacidade.

**Fila (agenda) ≠ bus de mix (PCM).** A `PlayQueue` é a **agenda** (ordem dos itens):
Spotify é **cidadão de 1ª classe** — entra na fila, vira now-playing e avança como
qualquer mídia. Só a **transição** difere: PCM entra com `crossfade`; Spotify com
`hard_cut`/`fade_tts` (`shared.DefaultTransition`) e toca **out-of-band** pelo Web
Playback SDK do browser, nunca no bus PCM.

Invariante **`spotify ⇒ mixable=false`** = áudio DRM **nunca entra no bus PCM/mix
local**: sem crossfade/beatmatch/waveform. Imposto em **2 camadas**: domínio
(`Track.Validate`) e banco (CHECK `spotify_never_mixable`). Marker/Beatgrid/waveform
só p/ fontes PCM. **Nunca** relaxar o invariante — mas **não** confundi-lo com a
fila: enfileirar/tocar Spotify (hard_cut) é permitido e esperado. A fila e o stub
engine (mirror do playout no browser) **aceitam** Spotify. (O antigo bus de broadcast
Liquidsoap, que era a 3ª camada de imposição, foi descontinuado.)

**Invariante gêmeo `youtube ⇒ mixable=false`** (v0.2.0) — mesma regra p/ o embed
YouTube (sem PCM, sem crossfade/beatmatch/waveform), imposto nas mesmas 2 camadas:
domínio (`Track.Validate` via `ErrYouTubeNeedsURI` + matriz de capacidade
`SourceYouTube`) e banco (CHECK `youtube_never_mixable`). YouTube também é cidadão de
1ª classe na fila (entra, vira now-playing, avança) e transiciona só por
`hard_cut`/`fade_tts`. `GET /library/tracks/{id}/audio` → **403** p/ youtube (não-PCM).

## YouTube (v0.2.0) — nova fonte, espelho fiel do Spotify

Fonte `source=youtube` replica a integração Spotify inteira (OAuth por estação,
busca/import, playout no browser) como terceiro motor lado a lado. **Divergência
estrutural única**: o IFrame Player API exige um elemento DOM **visível** no deck (ver
"iframe persistente" abaixo); Spotify SDK é device remoto sem elemento, local é
`<audio>` destacado.

- **⚠️ LOGIN GOOGLE NÃO É MAIS OBRIGATÓRIO (v0.2.17 — muda a decisão original).** Até a
  0.2.16 o painel INTEIRO ficava atrás do card "Conectar ao YouTube" e não dava pra fazer
  NADA sem OAuth (espelho fiel do Spotify). Hoje a aba tem **duas sub-abas** (`Sub =
  "buscar" | "colar"`, segmented control `opsub2` dentro do `YouTubePanel`):
  - **"Colar links" — SEM LOGIN** (`YouTubeLinksImport.tsx`): textarea grande onde o
    operador cola endereços separados por `;` (vírgula, quebra de linha, tab e espaço
    também servem) e manda o lote pra fila/deck de uma vez. Rota nova **`POST
    /api/v1/youtube/import-links`** (`{text}` → `[]trackDTO`).
  - **"Buscar" — DOIS motores chaveáveis, e o padrão NÃO pede login** (ver "Dois motores
    de busca" abaixo): `⚡ Sem login` (InnerTube, 0 unidade de cota) e `🔐 Minha conta`
    (Data API, exige OAuth). O hero "Conectar com Google" mora **dentro desta sub-aba**
    e só aparece no motor da Data API com a conta desconectada.
  **Por que o motor pago ainda pede login (não é inconsistência, é economia de cota):**
  `search.list` custa **~100 unidades** das ~10k/dia (≈100 buscas), enquanto resolver ids
  colados é `videos.list` = **1 unidade por lote (≤50 ids)** — 100× mais barato. Prod é
  público e multi-tenant por fingerprint: sem gate ali, um visitante queima a cota do dia
  de todo mundo em ~100 teclas. Então o OAuth fica como **escudo de cota do `search.list`**,
  e os caminhos baratos (InnerTube e paste) rodam livres. Gatear o paste seria fricção à toa.
  **TODO import roda deslogado — não só o paste.** `ImportLinks`, `ImportTracks` e
  `ImportPlaylist` passam `bestEffortToken()` (token se há conta, `""` se não) em vez de
  exigir `AccessToken`. O adapter da Data API **ignora o token** (`_ /*token*/` em
  `catalog.go`) — tudo autentica pela API KEY —, então o gate era decorativo e quebrava o
  motor grátis na prática: o operador buscava deslogado e levava **"Conta do YouTube não
  conectada"** ao arrastar o resultado pro deck (enqueue → `importYouTubeURI` →
  `ImportTracks`). Regressão coberta por `TestYouTube_Import{Tracks,Playlist}_semContaConectada`.
  **Sub-aba inicial**: `subPick = null` (ninguém escolheu) abre no que funciona agora —
  `status.freeSearch || status.connected` → "Buscar"; só cai em "Colar links" quando
  buscar é impossível (sem motor grátis E sem conta), já que colar só precisa da API key.
  Escolha manual trava. **Era `status.connected ? "buscar" : "colar"` até a v0.2.18**: com
  o motor grátis no ar isso escondia o chaveador atrás de dois cliques não-óbvios (o painel
  direito abre no Spotify, e o YouTube abria no paste) — passou a impressão de que a busca
  sem login não tinha subido pra prod, quando tinha.
  **Chip**: desconectado mostra `○ Sem conta (opcional)` (`.spot-chip.spot-chip-off`,
  tracejado/apagado — não é erro). O único early-return que restou é o guia de setup
  (`loaded && !configured`).
  **Parser espelhado nas 2 pontas**: `domain/youtube/youtube.go` (`ParseLinks`, `VideoID`,
  `MaxImportLinks = 50`) é a verdade; `web/app/lib/ytLinks.ts` é um espelho client-side
  usado **só** p/ a contagem viva "N link(s) detectado(s)". Aceita `watch?v=`, `youtu.be/`,
  `shorts/`, `embed/` e id cru (11 chars); dedupe preservando ordem; cap 50. Mexeu em um,
  mexa no outro. Import anexa ao FIM da fila (`pos=-1`) preservando a ordem colada;
  "Tocar o primeiro agora" só promove a primeira. **Pode entrar MENOS que o colado** —
  vídeo não-incorporável é descartado pelo catálogo, e a sub-aba reporta o descarte.
- **OAuth = Google auth-code COM client secret** (não PKCE público como o Spotify).
  `accounts.google.com/o/oauth2/v2/auth` + `oauth2.googleapis.com/token`; scopes
  `youtube.readonly openid email profile`, `access_type=offline` + `prompt=consent`;
  `channels.list?mine=true` → `ChannelTitle`. State-smuggling `<sid>~<state>` reusa o
  envelope provider-agnóstico do Spotify (`sessionFromState`, `tenants.go` intocado).
- **DOIS MOTORES DE BUSCA (v0.2.17), a mesma porta `outbound.YouTubeCatalog`.** Nada foi
  removido: a Data API continua inteira, ganhou uma irmã grátis e o operador chaveia.
  `domain/youtube` define `BackendFree`/`BackendAPI` + `NormalizeBackend` (vazio/lixo →
  `free`); a query string `?backend=` viaja da UI até o service.
  - **`free` — InnerTube (DEFAULT, 0 unidade de cota, sem conta)**: adapter
    `adapters/driven/youtube/innertube.go` sobre a lib **`github.com/raitonoberu/ytsearch`
    v0.2.0** (MIT, zero deps externas, tipada, sem API key) — mesmo protocolo que o site
    do YouTube usa. `app.YouTubeService.Search` **pula o gate de OAuth** nesse caminho.
    Armadilhas da lib, já tratadas no adapter: (a) `Next()` **não recebe
    `context.Context`** → cancelamento/timeout entram pelo campo `HTTPClient`; (b)
    `VideoItem.Duration` vem em **SEGUNDOS** → `×1000` p/ `DurationMs`; (c) construtores
    default `Language:"en"`/`Region:"US"` → setar `pt`/`BR`.
  - **`api` — YouTube Data API v3 (o de sempre, exige conta)**: `search.list`
    (type=video,playlist, **~100 unidades**) + `playlistItems.list` + `videos.list?part=
    contentDetails` p/ durações; parse ISO-8601 `PT#M#S` → ms; thumbnail
    `snippet.thumbnails.medium.url` → `ArtworkURL`. Quota ~10k/dia → debounce 300ms +
    limit 10. Usa a **API KEY**, NÃO o token OAuth — o token serve só p/ identidade/gating.
  - **Só o `Search()` troca de motor.** `Videos()` e `PlaylistItems()` do adapter InnerTube
    **delegam pro adapter da Data API** (1 unidade por lote de ≤50 ids), porque é de lá que
    saem duração exata e o flag `embeddable`. Se o parse do InnerTube falhar, cai no adapter
    Data API — a busca degrada, não quebra.
  - **Preço do grátis**: sem o pré-filtro `embeddable` do Google, um resultado do motor
    `free` pode ser descartado só na hora do import (o painel avisa). Duração vem da
    `videos.list` no import, então não perde precisão na fila.
  - **UI do chaveador** (`web/app/components/YouTubeBackendSwitch.tsx` + família `.ytbk` no
    `globals.css`): `role="radiogroup"`, pílula deslizante `.ytbk-glide` posicionada por
    `transform: translateX(idx*100%)` (mesma gramática do `.opsub2`), ícone escala 1.16 no
    ativo, `container-type: inline-size` esconde o `hint` abaixo de 300px (**container
    query**, não media query — o painel é uma coluna, não a tela) e opt-out obrigatório em
    `prefers-reduced-motion`. Só tokens semânticos, zero cor hardcoded. Trocar de motor com
    a busca preenchida **refaz a consulta** (`searchBackend` está nas deps do efeito).
  - **`status.freeSearch`** (`Status.FreeSearch = s.free != nil`) diz se o servidor tem o
    motor grátis fiado. Sem ele, o switcher **nem aparece** e o painel força `api`.
- **"Premium" não mapeia**: YouTube não expõe membership limpo e o playback não depende
  dela. Campo `Premium` fica só p/ paridade do chip (best-effort, provavelmente sempre
  false); **playout NUNCA gated nele**. Limitação conhecida documentada.
- **Playout no deck via IFrame Player API** (grátis, sem premium): quando a faixa ao
  vivo é YouTube, o slot de capa do deck (`.cover-deck`, **60×60**) vira o player —
  **mesmo tamanho, só chaveia o motor**, não redimensiona nada. Timers/progresso reais;
  avança sozinho no `ENDED` + fallback por duração.
- **Iframe persistente (INVIOLÁVEL — a divergência)**: UM `<div>` host do YT player mora
  alto na árvore de `page.tsx` e **nunca desmonta** (oculto quando ocioso). Quando a
  faixa ao vivo é YouTube, o nó é **movido imperativamente** (`appendChild`, **NÃO**
  React Portal) pra dentro do `<div data-yt-slot>` do deck ao vivo. `appendChild` MOVE o
  nó existente → iframe preservado, sem reload. Portal com alvo mutável
  destruiria/recriaria → recarregaria o vídeo (rejeitado). O ping-pong A/B do deck não
  recarrega o player. **ToS**: player tem que ficar visível (60×60 satisfaz, não 0px).
- **Seletor de motor (três vias, `page.tsx`)**: monta os 3 hooks sempre; prioridade
  **Spotify > YouTube > local** (catch-all): `playDeck = spotDeck.isSpotify ? spotDeck :
  youtDeck.isYouTube ? youtDeck : localDeck` (idem toggle/seek/fadeOut). Catch-all local
  apertado: `isLocal = np.source !== "spotify" && np.source !== "youtube"`.
- **Gating: NENHUMA credencial é requisito (v0.2.22).** A aba existe SEMPRE — buscar,
  colar link, importar (vídeo e playlist) e tocar funcionam com zero configuração, via
  InnerTube. Antes a API key era obrigatória só porque o import dependia da Data API;
  `innertube_meta.go` tirou essa dependência. O que cada credencial **acrescenta**:
  - **`AUTODJ_YOUTUBE_API_KEY`** → Data API **na frente** do import (`Videos`/
    `PlaylistItems` tentam ela primeiro): filtro `status.embeddable` do Google e playlist
    paginada inteira. Falhou (cota, rede)? cai no grátis em vez de propagar o erro.
  - **`AUTODJ_YOUTUBE_CLIENT_ID` + `SECRET`** → motor de busca `api` ("Minha conta") e
    conta ligada. Sem eles, connect/callback respondem **503** (`ErrOAuthUnavailable`),
    **não** 404/500, e o `status.oauth = false` faz a UI **esconder o chaveador** — antes
    ela oferecia um motor que só podia terminar em erro.
  - **Preço do modo sem key**, que a UI precisa dizer: sem o pré-filtro do Google, vídeo
    não-incorporável só é descartado na importação; e a playlist vem pela janela do painel
    do `next` (medido 19 de 20 — o que falta costuma ser privado/removido), sem paginação
    por continuation.
  Defaults:
  `AUTODJ_YOUTUBE_REDIRECT` = loopback `.../api/v1/youtube/callback`;
  `AUTODJ_YOUTUBE_RETURN` = `/?op=youtube`. **Secret via env/`pass` (nunca no código)**;
  prod = `fly secrets set AUTODJ_YOUTUBE_*` (token do `pass`, nunca `fly auth login`).
  Repos: `sqlite/postgres/memory youtube_account_repo.go` + `.YouTubeAccount()`; migração
  SQLite = rebuild da tabela `tracks` (CHECK novo, idempotente no open, padrão
  `ensureArtworkColumn`) + `youtube_uri` ALTER idempotente + `youtube_account` CREATE IF
  NOT EXISTS; Postgres = `migrations/017_youtube.sql`.
- **Drag**: rows YouTube usam dataTransfer `text/autodj-youtube-uri` (+origin), lido por
  `onDeckDrop`/`ProgramList`; `enqueueYouTubeSafe` (`importYouTubeURI` → enqueue).
- **Cota da busca esgotada → HTTP 429 (não 500) + humanização (v0.2.5)**: a busca
  (`search.list`, ~100 unidades) esgota a cota grátis (~10k/dia ≈ 100 buscas) e o
  upstream devolve **429** (ou **403** com `reason=quotaExceeded`/`rateLimitExceeded`).
  Antes o `catalog.go get()` devolvia erro cru → `writeYouTubeErr` caía no default →
  **500** e o painel mostrava "youtubeSearch: 500" inútil. Fix em 3 camadas: (a) domínio
  `domain/youtube/youtube.go` novo sentinel **`ErrQuotaExceeded`**; (b) adapter
  `adapters/driven/youtube/catalog.go` — `get()` embrulha status não-200 como
  `ErrQuotaExceeded` (`%w`) quando `isQuotaStatus(status, body)` (429, ou 403 com
  corpo contendo quota/ratelimit/dailylimit); (c) HTTP `httpapi/youtube.go`
  `writeYouTubeErr` mapeia `ErrQuotaExceeded → 429`. **Frontend** (`web/app/lib/errors.ts`,
  humanizador central): lê `{error}`+status e casa por palavra-chave depois por status
  → frase pt-BR ("Cota diária de busca do YouTube esgotada… dá pra IMPORTAR colando a
  URL do vídeo"). Todo `throw` de `api.ts` passa por `throwHuman` (erro amigável +
  `.status` preservado p/ `useSocial`). Reset da cota ~meia-noite Pacífico. Importar por
  URL e tocar **não** gastam cota — só a busca.
- **LIMITAÇÃO CONHECIDA — cota da busca YouTube (10k unidades/dia por projeto).** A
  YouTube Data API v3 dá **10.000 unidades/dia** por projeto Google Cloud, e cada
  `search.list` custa **~100 unidades** → **~100 buscas/dia**, reseta ~meia-noite
  Pacífico. Uso pesado (testar, buscar por tecla) esgota rápido → upstream 429 → a busca
  para até o reset. **Não é bug — é o teto grátis do Google.** Contorno, em ordem de ROI:
  0. **Chavear o motor da busca p/ `⚡ Sem login`** (v0.2.17, já é o DEFAULT): a via
     InnerTube não toca na Data API → **0 unidade**, cota intacta, e nem precisa de conta.
     Só volte pro motor `🔐 Minha conta` se quiser o filtro `embeddable` do próprio Google.
  1. **Importar por URL/ID em vez de buscar** (custo ~zero): `videos.list`/
     `playlistItems.list` custam **1 unidade** (100× mais barato que `search.list`). O
     AutoDJ já importa colando a URL sem gastar busca — cobre a maioria dos casos.
  2. **Múltiplos projetos Google Cloud** (workaround prático): cada projeto tem sua
     API key = 10k/dia cada; round-robin/fallback entre keys quando uma dá 429. Escala
     linear (3 projetos ≈ 300 buscas/dia). Cinza no ToS (Google desencoraja splitar
     cota), mas técnica comum. **Não implementado hoje** — `AUTODJ_YOUTUBE_API_KEY` é
     uma key só; virar lista + round-robin é a evolução se precisar.
  3. **Aumento oficial via Google** (grátis, mas burocrático e lento): Cloud Console →
     APIs & Services → YouTube Data API v3 → **Quotas** → "Queries per day" → Request
     increase → preenche o "YouTube API Services - Audit and Quota Extension Form".
     **Passa por audit manual** (semanas), exige compliance de ToS/branding. Pra uso
     caseiro raramente vale a pena / raramente aprovam volume alto.
  Mitigações já no app: debounce 300ms + `search` limit 10 + sem polling de busca.
  Cache de busca (query→resultado, TTL) ainda **não** implementado — evolução possível.
- **LIMITAÇÃO CONHECIDA — "Premium" do YouTube não mapeia** (já registrado no bloco
  YouTube acima): o YouTube não expõe flag limpa de membership e o playout não depende
  dela; `Premium` fica best-effort só p/ paridade do chip (provavelmente sempre false),
  **playout nunca gated nele**.
- **Troubleshooting — YouTube não toca / `Erro 153` ("Video player configuration error")**:
  sintoma = iframe cuea (`getPlayerState()===3` BUFFERING) mas nunca chega a PLAYING(1),
  `getDuration()===0`, `getVideoData().title===""` **para qualquer vídeo** (inclusive
  known-embeddable), e o player exibe "Watch video on YouTube / Error 153". **Causa raiz
  (pesquisada, não é palpite):** desde **dez/2025** o YouTube passou a **exigir que o embed
  se identifique via header HTTP `Referer`** — qualquer player embedado (inclusive a IFrame
  Player API) que não mande referer, ou cuja página mande `Referrer-Policy` restritiva
  (`no-referrer`/`same-origin`), estoura 153. Ref.: Simon Willison TIL
  (`til.simonwillison.net/youtube/fixing-153-embed`, 2025-12-01) + KBs (appuals, umich
  TeamDynamix). **Content-blocker/extensão que remove o referer é UM dos gatilhos, mas NÃO o
  único** — a mudança é do lado do YouTube. **Fix aplicado no AutoDJ (v0.2.1,
  `useYouTubePlayer.ts`):** (a) `host: "https://www.youtube-nocookie.com"` no `YT.Player`
  (workaround oficial documentado do 153); (b) `referrerpolicy="strict-origin-when-cross-origin"`
  setado explicitamente no `<iframe>` gerado pela API (no tick de criação e no `setTimeout 0`),
  garantindo o envio do Referer mesmo se a página herdar política mais restrita. A página prod
  **não** manda `Referrer-Policy` (default do browser = `strict-origin-when-cross-origin`, o
  recomendado). **Armadilha de teste:** abrir `youtube.com/embed/<id>` **direto numa aba**
  (navegação top-level) SEMPRE dá 153 agora — não há página-pai, logo zero referer; isso **não
  prova** bloqueio de perfil/extensão. Testar sempre **dentro do app** (embed com página-pai
  real). O fallback por duração do playout (`npDurMs + 1500`) escoa a fila mesmo com o embed
  quebrado (agenda não trava), mas **sem áudio** enquanto o 153 persistir. Se 153 voltar mesmo
  com nocookie+referrerpolicy: conferir `Referrer-Policy` da página (curl `-I`), testar
  `youtube-nocookie` no browser, e checar se alguma extensão está stripando o referer.
- **Troubleshooting — YouTube trava em 00:00 com "Ocorreu um erro. Tente novamente mais
  tarde" (ID de reprodução)** — sintoma DISTINTO do 153: o embed carrega os **metadados
  certos** (título + duração reais, ex. Rick Astley -03:34), mas a posição fica presa em
  00:00, o player nunca chega a PLAYING e mostra a overlay nativa "Ocorreu um erro...".
  `getDuration()>0` e `getVideoData().title!==""` (por isso NÃO é 153, que dá duration=0 +
  title vazio). **Causa raiz (pesquisada):** a IFrame Player API valida o handshake
  `postMessage` contra a origem declarada; sem o `origin` nos `playerVars`, o embed cai no
  wildcard `"*"` e desde ~2025 esses players passaram a falhar com essa overlay. **Fix
  aplicado (v0.2.2, `useYouTubePlayer.ts`):** `playerVars.origin =
  window.location.origin` + `enablejsapi: 1` (recomendação oficial da API) — o handshake do
  player casa com a página-pai e o vídeo toca. **Armadilha de teste (importante):** o Chrome
  **automatizado** (claude-in-chrome) NÃO decodifica YouTube nem em nível top-level —
  `youtube.com/watch` fora do app trava igual em 00:00 (spinner infinito, `vps=…:N`
  unstarted, formato AV1 `fmt=398` negociado, e requests `videoplayback` do CDN devolvendo
  **503** no IP de datacenter do egress da automação). Isso é limitação do **browser de
  teste** (decode AV1 desabilitado + CDN do YouTube barrando o IP da automação com 503),
  **não do app** — validar sempre no **browser real** do usuário, onde o mesmo build toca
  normalmente.
- **Troubleshooting — vídeo do YouTube "some" do deck ao dar play (pula na hora)** — a faixa
  entra no deck, some no instante do play e `Tocadas` incrementa, sem tocar. **Causa raiz:** o
  detector de bloqueio de embed (`useYouTubePlayer.ts` poll → `embed:<code>` → `useYouTubePlayout`
  3a → `POST /playback/next`) disparava **falso positivo** logo após `loadVideoById`: por uma
  fração de segundo `getVideoData().errorCode` devolve o código do **vídeo ANTERIOR** (quirk da
  IFrame API — metadados stale até o novo carregar). **Fix (v0.2.2, `useYouTubePlayer.ts`):**
  janela de graça — só marca `embed:<code>` se (a) o vídeo **nunca chegou a PLAYING**
  (`playedRef`); (b) está preso em -1/3/5; (c) o `errorCode` é de bloqueio (auth/150/101/100);
  (d) o `getVideoData().video_id` **casa** com o carregado (não é stale); (e) a condição
  persiste **≥3s** (`embedBadSinceRef`). Vídeo tocável chega a PLAYING em 1-2s → nunca pula;
  vídeo bloqueado real fica travado 3s → pula. Não afeta buffering lento de rede (vídeo em
  buffering não tem `errorCode` de bloqueio, só estado 3 → não conta strike).
- **Troubleshooting — vídeo do YouTube NÃO TOCA em browser real (trava em 00:00 / "Ocorreu um
  erro") — CAUSA-RAIZ DEFINITIVA PROVADA (v0.2.4):** o culpado era **reparent do iframe**. O
  `Deck.tsx` movia o host do player (`appendChild` p/ o slot de capa do deck ao vivo) a cada
  troca A/B. Pela spec HTML, **mover um `<iframe>` na árvore DOM (remover + reinserir) RECARREGA
  o iframe** — o document interno reinicializa. Isso **matava o handshake postMessage do
  YT.Player a cada swap** → o player nunca chegava a PLAYING (comprovado: reparent dispara um 2º
  evento `load`, `reloadedOnReparent:true`; um player nunca-movido tocava). As tentativas antigas
  (nocookie+referrerpolicy, origin/enablejsapi, janela de graça, e o palco 320×320 do v0.2.3) NÃO
  eram a cura — o palco 320×320 ficou só como margem de segurança de viewport, **não** é a
  causa-raiz. **Fix (`useYouTubeOverlay.ts` + `page.tsx`):** o host **nunca é reparentado** —
  fica fixo/parqueado na árvore (`page.tsx`, escondido). Quando faixa YouTube está no ar, o host
  é sobreposto **visualmente** ao slot `[data-yt-slot]` do deck ao vivo via `position:fixed` +
  `getBoundingClientRect` num loop `requestAnimationFrame`. Reposicionar por CSS **não recarrega**
  o iframe → o player sobrevive ao ping-pong A/B e toca (validado no browser real: estados
  `[-1,3,1]`, `finalState:1` PLAYING). **Regra:** NUNCA reparentar o iframe do YouTube (nem por
  `appendChild`, nem por portal React que troque o pai real) — só reposicionar por CSS/overlay.
- **Troubleshooting — deck PRETO / YouTube não toca SÓ na transição vindo do Spotify
  (v0.2.9, regressão da v0.2.8):** sintoma = Spotify→YouTube (partida automática, sem novo
  gesto do usuário) deixa o deck preto e o vídeo não toca; YouTube-first (▶ clicado) toca
  normal. **Causa raiz — tripla regressão introduzida na v0.2.8 (commit 9783086):** (1) o
  `onError` do IFrame passou a marcar `embed:<code>` p/ TODOS os códigos (era só cosmético na
  v0.2.4); (2) surgiu um campo `errored` no deck YouTube derivado de `error.startsWith("embed:")`;
  (3) `page.tsx` passou a **esconder o overlay** quando `errored` (`useYouTubeOverlay(ref,
  isYouTube && !errored)`). Esconder o overlay **parqueia o iframe fora do slot → deck preto**, e
  na partida cross-engine (sem gesto fresco) o vídeo demora mais a chegar a PLAYING, então um
  `onError` transitório (código 2/5) dispara antes do detector de bloqueio travar → `embed:` →
  `errored=true` → overlay some → preto, e o effect 3a ainda PULA a faixa. YouTube-first tem gesto
  imediato → PLAYING em 1-2s → detector trava primeiro, nunca regride. **Fix (restaura a semântica
  validada da v0.2.4):** (a) `useYouTubePlayer.ts onError` só marca `embed:<code>` p/ códigos
  **permanentemente mortos** — `dead = code===100 || code===101 || code===150` (removido/privado,
  incorporação desabilitada) — que NUNCA tocam nem YouTube-first, logo nunca são falso positivo
  cross-engine; códigos ambíguos 2/5 viram erro cosmético `yt:<code>` (sem pular, sem esconder); (b)
  removido o campo `errored` do `YouTubeDeck`; (c) `page.tsx` volta a `useYouTubeOverlay(ytHostRef,
  youtDeck.isYouTube)` — **o overlay NUNCA é escondido** (esconder = parquear = preto). Um 2/5
  genuinamente terminal é coberto pelo fallback por duração (effect 3b, `durMs+1500`) — a fila
  escoa sem travar. **Regra:** só `embed:` (→ pula na hora) p/ {100,101,150}; nunca esconder o
  overlay do YouTube.
- **⚠️ CAUSA-RAIZ DEFINITIVA do "clipe some ao arrastar pro deck" — ORIGEM IP (v0.2.10,
  medida 27/07/2026):** o YouTube **recusa incorporar vídeo monetizado** (VEVO/gravadora/
  oficial) quando o **host da origem da página é um IP literal** (`http://127.0.0.1:8099`).
  O player devolve `onError 150` + `getVideoData().errorCode === "auth"` e nunca sai de
  UNSTARTED — mesmo em vídeo cujo `status.embeddable` da Data API é `true`. O app então faz o
  que deve (`embed:150` → effect 3a → `POST /playback/next`) e a faixa **some do deck**.
  **Prova A/B** (mesmo vídeo `JGwWNGJdvx8`, mesmo `host: youtube-nocookie`, mesmos playerVars,
  mesmo Chrome, mesmo servidor, mesmo instante):

  | origem | `getPlayerState()` | `errorCode` | `onError` |
  |---|---|---|---|
  | `http://127.0.0.1:8099` | -1 (UNSTARTED) | `"auth"` | 150 |
  | `http://localhost:8099` | 3 (BUFFERING→carrega) | `null` | — |
  | `https://example.com` | 3 (BUFFERING→carrega) | `null` | — |

  Não é o `youtube-nocookie` (testado cookieful vs nocookie na origem IP: **os dois** dão 150),
  não é o esquema `http:` (localhost também é `http:`), não é o vídeo (`embeddable:true`).
  É o **host ser IP**. Vídeo livre de direitos (ex. Big Buck Bunny `aqz-KE-bpKQ`) toca até em
  IP — por isso o bug parecia aleatório ("só alguns vídeos somem"). **Prod não é afetado**
  (domínio real). **Fix (v0.2.10):** (a) `web/app/lib/ytOrigin.ts` (`isBareIpOrigin()`,
  `localhostEquivalentURL()`); (b) `YouTubeOriginWarning.tsx` no topo do `YouTubePanel` (os dois
  estados) explicando o bloqueio + link p/ localhost; (c) `useYouTubePlayer.ts onError` emite
  `console.warn` com a dica quando o código morto acontece em origem IP; (d) `scripts/
  serve-local.sh` e `serve-youtube-local.sh` anunciam `http://localhost:$PORT` e setam
  `AUTODJ_{SPOTIFY,YOUTUBE}_RETURN` **absolutos p/ localhost** — os `*_REDIRECT` de OAuth
  continuam em `127.0.0.1` (o Spotify **exige** loopback IP literal e é o registrado nos dois
  consoles); só o retorno pós-callback muda, senão o login terminaria de volta na origem IP.
  **Regra: em dev, abrir sempre `http://localhost:<porta>` — nunca `127.0.0.1`.**
  **Correção de um registro anterior:** o Chrome automatizado (claude-in-chrome) **decodifica
  YouTube sim** — Big Buck Bunny chega a PLAYING(1) dentro do app. O que ele não consegue é
  tocar **clipe musical monetizado** (fica em BUFFERING(3) eterno, `currentTime` 0, sem erro),
  inclusive fora do app. Logo: bloqueio/erro de embed **é** reproduzível na automação (foi
  assim que a origem IP foi isolada); a **reprodução de áudio de clipe VEVO** exige o browser
  real do usuário.

## Frontend — Playout no browser (três motores) e Transport

- **Avanço da fila A→B é dirigido pelo BROWSER**, um driver por motor, ambos
  espelhando a mesma agenda e chamando `POST /playback/next` no fim da faixa:
  - **Spotify** (`useSpotifyPlayout`): toca a faixa via Web Playback SDK (DRM,
    hard_cut, out-of-band); avança pelo evento `endedNonce`/`endedUri` do SDK **+
    fallback por duração** (`durMs+1500`) — o evento do SDK às vezes não chega
    (device desconectado após restart do server / sem Premium ativo) e a faixa
    ficava presa no deck sem avançar (bug #63). Guard `advancedFor` = 1 avanço/faixa.
  - **Local** (`useLocalPlayout`): toca via `<audio>` apontando pra
    `/library/tracks/{id}/audio` (Range/seek); avança no evento `ended`. **Fallback**:
    se o arquivo não carregar (sem PCM/permissão/ausente), um timer pela duração
    conhecida avança mesmo assim — a agenda nunca trava. Guard `advancedFor` = 1
    avanço/faixa (evento OU timer).
  - **PRÉ-CARGA no deck ocioso (v0.2.11)** — o deck ocioso deixa de ser só metadado:
    o áudio/vídeo da PRÓXIMA (`queue[1]`) já é carregado enquanto a atual toca, então
    a virada é instantânea (antes o YouTube só chamava `loadVideoById` ao entrar no ar
    → 1–3s de buffer; o `<audio>` local só recebia `src` na hora → espera de rede).
    **Dois motores em ping-pong por driver**, papel decidido por `liveIdx`:
    - **YouTube** (`useYouTubePlayout` sobre DUAS instâncias de `useYouTubePlayer`,
      hosts `ytHostRef`/`ytHostBRef` em `page.tsx`): o engatilhado faz `cue(uri)` =
      `mute()` + `cueVideoById` + **warm-play mudo** (`playVideo` em 300ms, `pauseVideo`
      + `seekTo(0)` em 2600ms). O warm-play é obrigatório: **`cueVideoById` sozinho só
      baixa thumbnail/metadata, não mídia** (docs oficiais); autoplay **mudo** é sempre
      permitido pela política do browser (o flag olhado é o `muted`, **não** `setVolume(0)`
      — por isso o hook usa `mute()/unMute()`, não volume). Na virada: `standby()` no que
      sai (pause+mute+limpa `currentUri`) e `goLive()` no engatilhado (`seekTo(0)` +
      `unMute()` + `playVideo()`), com **self-heal** — se em 1.8s não estiver PLAYING,
      cai no `loadVideoById` (caminho antigo). `liveIdxRef` é atualizado **antes** do
      `setLiveIdx` p/ o `fadeIn()` logo em seguida mirar a instância nova. Timers do warm
      são canceláveis (`clearWarm`) — swap antes do warm-pause nunca pausa o áudio no ar.
      O listener de gesto do usuário é **gated no player ao vivo** (`liveRef`), senão um
      clique dispararia a faixa engatilhada.
      **⚠️ ARMADILHA DO `liveIdx` OBSOLETO NO HANDOFF (bug 0.2.15) — a instância ociosa
      SÓ pode ser resolvida pela REF, nunca pelo `cued` derivado do render.** No handoff
      o effect 1 troca os papéis DENTRO do próprio effect: escreve `liveIdxRef.current`
      de forma síncrona, chama `setLiveIdx(nidx)` e dispara `other.goLive()`. O
      `setLiveIdx` só reflete no render SEGUINTE — logo `liveIdx` (e o `cued` que sai
      dele) fica velho pelo resto daquele flush. O effect 1b (pré-carga da próxima) tem
      `npUri` nas deps, que acabou de mudar, então roda no MESMO flush logo depois: lendo
      o `cued` velho ele mirava exatamente o player que tinha acabado de receber
      `goLive()` e matava o áudio recém-iniciado — `cue()` (mute + `cueVideoById` da
      OUTRA faixa) com próxima na fila, ou `standby()` (pause + mute) com fila vazia.
      Sintoma: virada YouTube→YouTube entra no deck e fica parada em 00:00, transporte
      lendo `RETOMAR`, e um PLAY ou refresh recupera (o `wantPlay` seguia `true`; o
      player só estava pausado). Fix: `playersRef.current[liveIdxRef.current === 0 ? 1 : 0]`
      dentro do corpo do effect, com `liveIdx` mantido nas deps p/ reexecutar depois que
      o swap commita. Só afetava YouTube→YouTube: a mixagem cross-motor tem guard
      `if (mixUri) return;` (0.2.12) e faixa local não passa por esse hook.
    - **Local** (`useLocalPlayout`): dois `new Audio()` com `preload="auto"`; o ocioso
      recebe `src = trackAudioURL(next)` e o browser baixa em background. Na virada, o
      handoff só troca papéis (sem request novo) e **semeia `realDur` da `duration` já
      conhecida** — o `loadedmetadata` do pré-carregado disparou antes dos listeners
      migrarem, então a barra não fica sem duração.
    - **Spotify NÃO pré-carrega** — o Web Playback SDK **não expõe API de preload**
      (nem cue, nem buffer). Transição envolvendo Spotify segue com o ~1s de
      transfer+play. Limitação da plataforma, documentada.
    - **Overlay por PAPEL, não por deck** (`useYouTubeOverlay(hostRef, role)`,
      `role: "live" | "cue" | null`): `"live"` mira `[data-yt-slot]` (deck no ar),
      `"cue"` mira `[data-yt-cue-slot]` (deck ocioso, `Deck.tsx` renderiza quando
      `DeckPair.nextRole.ytCued`), `null` = parqueado. **A invariante de nunca reparentar
      o iframe continua valendo p/ os DOIS hosts** (só CSS/`position:fixed`).
  - **Recurso IDÊNTICO nos dois motores (hexagonal)**: o mesmo comportamento
    (carregar/tocar/pausar/avançar por evento OU duração) vale p/ qualquer provider
    de áudio (Spotify/YouTube/Beatport/local). A regra mora na camada uniforme, não
    é caso especial por provider.
  - `page.tsx` escolhe o **deck ativo** (`spotDeck.isSpotify ? spot : local`) e
    passa deck/toggle/seek do motor que casa com o now-playing. `PlayoutDeck.live`
    (Spotify OU local) generaliza o antigo `isSpotify`: `DeckPair.liveActive` e o
    PAUSE leem `live` → posição real + pause funcionam nos dois motores.
- **RETOMADA PÓS-REFRESH — um registro só, os três motores (0.2.26,
  `web/app/lib/resume.ts`)**: F5 destrói `<audio>`, IFrame e device do SDK. O registro
  `{uri,pos,at,playing}` (localStorage `autodj.playout.v1`) é lido **na carga do
  módulo** (antes de qualquer motor escrever), aplicado **uma vez** e só se a **uri
  casar** com o now-playing e o registro estiver **fresco** (10 min). Cada motor faz
  só o seu gesto: `<audio>.currentTime` no `loadedmetadata`, `loadVideoById({videoId,
  startSeconds})` no YouTube, `play()+seek` no Spotify. Retomada **não** é faixa nova:
  entra a ganho pleno, **sem fade-in**.
  - **`salvarResume` IGNORA posição < 1s**: ao recarregar, o motor monta em `00:00` e o
    tick salvava **0 por cima** do valor bom antes do PLAY — a retomada se destruía
    sozinha. Guardar 0 também não serve: retomar em 0 = não retomar.
  - **Nenhum motor pode limpar o registro "quando não é a minha fonte"**: o
    `clearResume()` incondicional do tick do Spotify (e o do YouTube) apagava a posição
    de quem ESTAVA no ar. Registro de outra faixa é inofensivo — a uri não casa.
  - Antes: Spotify tinha resume com uma chave, YouTube com **outra**
    (`autodj.ytplayout.v1`) e o **local não tinha nenhuma**. Duas fontes de verdade
    para algo que só pode ter UMA faixa no ar.
  - **Autoplay — o que É e o que NÃO é possível (medido, 0.2.26).** O browser não deixa
    uma página recém-carregada emitir som sem interação, e **Cmd+R não é interação** (o
    gesto acontece no chrome do browser, não na página).
    - **`<audio>` NÃO tem contorno.** O truque conhecido "autoplay mudo é sempre
      permitido" vale pra `<video>`; num elemento de áudio o Chrome recusa igual —
      medido: `a.muted = true` + `play()` devolve **`NotAllowedError` também**. Web
      Audio não ajuda (`AudioContext` nasce `suspended` pelo mesmo motivo). Sobra
      re-tentar no 1º clique/tecla, **em silêncio** (`armResumeOnGesture`).
    - **IFrame do YouTube tem**: é `<video>`, então mudo toca. Se em 1,2s não sair de
      UNSTARTED/CUED, o player toca **mudo na posição certa** e o gesto devolve o som
      (`mutedByPolicyRef`).
    - **DETECÇÃO É POR PROGRESSO, NÃO POR EXCEÇÃO (`deck.blocked`, 0.2.26).** Medido:
      quando o Chrome barra o autoplay depois de um refresh, o `play()` **não
      rejeita** — o elemento fica `paused = false`, o evento `play` DISPARA, o
      carregamento é ADIADO (`readyState` preso em 0) e a promessa nunca resolve nem
      rejeita. Ou seja: nem `catch` nem `playing` servem de sinal — pior, `playing`
      fica `true` e mascara o problema. O sinal honesto é **a posição não andar**
      enquanto a estação diz que quer tocar (amostra a cada 2,5s nos motores local e
      YouTube; no Spotify, ausência de playback por 3s). Vale também pra buffer
      travado e arquivo que não decodifica — em todos, o operador precisa da mesma
      coisa.
    - **O deck mostra ▶, não ⏹, enquanto não sai som.** `deck.blocked` entra no
      `stopped` do deck e do transporte. Antes ficava ⏹: anunciava "no ar" sem áudio
      e, pior, o clique natural de quem quer som **parava a estação**. O ▶ também
      **força o play no motor** — com `wantPlay` já ligado, `setWant(true)` não muda
      estado e o botão pareceria morto; e esse clique é justamente o gesto que o
      browser exigia.
    - **Saída de verdade pro operador: INSTALAR O APP** (`web/app/manifest.ts`,
      `display: standalone`). O Chrome concede **permissão de autoplay a apps
      instalados** — aí o refresh volta tocando sozinho, sem clique. O mesmo vale
      quando o site acumula engagement (uso diário em produção): o Chrome passa a
      liberar por conta própria, e é por isso que o sintoma parece intermitente entre
      máquinas.
  - Barra avisando "clique para retomar" foi testada e **removida a pedido do
    operador** — aparecia todo refresh e virava ruído.
  - **Registro é POR FAIXA (mapa `uri → posição`), não um slot único.** Com um slot só,
    duas abas no mesmo endereço (ou o consumo de uma retomada, que apagava tudo)
    destruíam a posição uma da outra em silêncio. Cada escrita **relê** o armazenamento
    antes de gravar, pela mesma razão.
- **QUEDA DE INTERNET NÃO PODE TROCAR A FAIXA (0.2.27)** — três peças, todas
  medidas derrubando o servidor com música no ar:
  1. **Faixa engatilhada vira BLOB** (`web/app/lib/audioBuffer.ts`): a próxima é
     baixada inteira e guardada em memória, então existe do lado de cá mesmo com o
     servidor fora. A faixa NO AR segue em streaming de propósito — baixá-la antes
     de tocar atrasaria o início num link lento, e o browser já bufferiza à frente.
     ⚠️ **Guarda-se o Blob, não o object URL.** A 1ª versão guardava a URL pronta e
     a revogava ao expulsar do cache — URL revogada continua sendo string válida, e
     o `<audio>` só descobre ao tentar carregar: dava "Não foi possível carregar o
     áudio desta faixa" numa faixa íntegra. Blob é o dado; URL é ponteiro
     descartável, criado na hora do uso.
     **localStorage não serve pra isso** (foi a primeira ideia): guarda texto, teto
     de ~5 MB por origem e é síncrono — um MP3 vira base64 (+33%) e travaria a
     thread que desenha o deck. Duas faixas nem caberiam.
  2. **Sem enlace, ninguém avança a fila** (`lib/onAir.ts` `isServerLinked`,
     publicado pelo `useConnectivity`). O avanço sai de dois gatilhos: fim da faixa
     e um **timer de relógio de parede**. Numa queda o áudio pode ficar atrasado
     enquanto o relógio corre — sem guarda, o `POST /playback/next` sairia no exato
     instante em que a rede volta e **cortaria a música no meio**. O timer também
     não avança enquanto a posição estiver claramente ANTES do fim (parou ≠
     terminou): ele se reagenda.
  3. **Não recarrega se o áudio sobreviveu** (`isAudioAlive`): o reload da 0.2.21
     existe pra retomar áudio perdido; quando a faixa atravessou a queda tocando,
     recarregar seria cortar justamente o que sobreviveu. Aí o toast vira
     "Conexão de volta — o áudio não parou."
  Medido (queda de 20s): 03:57 → 04:26 tocando com o aviso vermelho na tela, fila
  andando só no fim natural da faixa, sem reload.
- **SOBREVIVER AO F5 SEM SERVIDOR (0.2.27)** — o app abre, mostra a estação e
  continua tocando com o servidor fora. Quatro peças, e cada uma cobre um buraco
  diferente (faltando qualquer uma, o conjunto não funciona):
  1. **Casca do app** — Service Worker (`web/public/sw.js`, registrado por
     `ServiceWorkerBoot`). Sem ele, um F5 offline nem carregava o HTML: a tela era a
     página de erro do Chrome, e guardar áudio não adiantava nada. Estratégia por
     tipo: documento **rede-primeiro** (deploy não pode ficar invisível — DEV-7),
     `/_next/static/*` **cache-primeiro** (nome com hash = imutável), `/audio`
     **cache-primeiro**, resto da API **nunca** (fila e now-playing são estado vivo).
  2. **Áudio** — Cache Storage (`lib/audioBuffer.ts`), a faixa no ar + as **3
     próximas**. Sobrevive ao reload (Blob não sobrevive) e o conjunto é
     **recalculado a cada mudança da fila**: reordenar solta o que saiu de vista e
     busca o que entrou. Download em SÉRIE, nunca em paralelo — o buffer existe pra
     proteger o ar, não pra disputar banda com ele.
  3. **Estado** — espelhos em localStorage (aqui é o container certo: JSON de poucos
     KB): último snapshot da fila (`useNowPlaying`), catálogo da tela
     (`useOperational`), lista da biblioteca (`useLibrary`) e último operador
     (`useAuth`). Sem eles o app abria vazio e — pior — decidia que "não há faixa
     local" e caía na aba **Spotify**, justamente a única que não funciona offline.
  4. **Fila anda offline** — `post("/playback/next")` sem servidor adianta o
     ESPELHO e anota a dívida (`autodj.pendingnext.v1`, persistida); quando o
     enlace volta, a dívida é paga chamando `next` a mesma quantidade de vezes.
     Sem isso o cabeçalho ficava preso na faixa que já tinha acabado.
  - **O que fica APAGADO sem enlace** (`useServerLinked`): Spotify, YouTube e
    "+ Adicionar" — são 100% online (busca, import e upload passam pela API).
    Reacendem sozinhos. **Músicas continua viva**: lê do espelho e toca do cache.
  - **Spotify e YouTube não foram afetados por nada disto**: o buffer só pega
    `source` local, o SW ignora outra origem (SDK/IFrame carregam de fora), e a
    sonda `isAudioAlive` só é registrada pelo motor local — com streaming no ar ela
    dá `false` e o reload da 0.2.21 acontece como sempre, que é o certo pra eles.
- **PLAY mestre (`wantPlay`) — gate ÚNICO de transporte, provider-agnóstico.** Um
  booleano em `page.tsx` (persistido em `localStorage autodj.wantplay.v1`, default
  **false** = "nada no ar"/parado) passado aos DOIS hooks (`useSpotifyPlayout(...,
  wantPlay)` e `useLocalPlayout(..., wantPlay)`). Regras:
  - `wantPlay=false` ⇒ os hooks **carregam a faixa da cabeça mas NÃO tocam** — fica
    engatilhada a 00:00 até darem PLAY. Timers de avanço e `endedNonce` também são
    ignorados (estação parada não escoa a fila).
  - **Só entra tocando ao arrastar/enfileirar se AUTO estiver On** (bug #64); AUTO
    Off ⇒ enfileira e **aguarda o operador dar PLAY**. `enqueueSafe`/`enqueueSpotifySafe`
    setam `wantPlay=true` só quando `autoOnRef.current` OU `playNow`.
  - **AUTO On ⇒ estação no ar** (efeito `autoOn → setWant(true)`); o piloto reabastece
    a fila e o transporte segue tocando (o "esvaziar" transitório entre refills NÃO
    para: guard `!hasHead && !autoOn`).
  - **Fila drenada com AUTO Off ⇒ `wantPlay=false`** = "nada no ar para", igual ao
    deck ocioso quando não toca nada; re-arma o gate p/ o próximo PLAY/drop.
    ⚠️ **Só depois do 1º snapshot do WS** (`snapshotSeen = !!snap`, 0.2.26): no boot
    `snap` é `null`, logo `hasHead` é `false`, e a regra disparava **antes de saber se
    havia fila** — todo refresh apagava o `wantPlay` persistido e a estação voltava
    parada ("dei refresh e parou"). Fila realmente vazia continua parando, porque aí o
    snapshot já chegou.
- **PLAY/STOP mestre = botão ▶/⏹ do DECK AO VIVO (topo)** — não mais na barra inferior
  (mudança 2026-07-26). O botão de controle do deck ativo (`DeckPair.nowRole.onCtl`) é o
  transporte mestre e **funciona SEMPRE que há cabeça carregada**: parada (`stopped =
  !wantPlay`) → **▶ PLAY** (`onPlayMaster → setWant(true)`); no ar → **⏹ STOP**
  (`onStopMaster`). Antes o botão do deck era `liveActive ? onToggle : undefined` +
  `ctlDisabled: !liveActive` — quando parado ficava **desabilitado** ("aperto e não
  toca"). Agora `onCtl: stopped ? (live ? onPlayMaster : undefined) : onStopMaster`. O
  toque no ▶ é também o **gesto de usuário** que destrava o autoplay do browser p/
  Spotify SDK / `<audio>` — por isso "AUTO On" sozinho não soava (autoplay policy), e o
  ▶ resolve. `page.tsx` já faz `if (autoOn) setWant(true)`, mas o áudio só arranca no 1º
  gesto.
- **Transport (barra inferior)** = FALAR / **INSTRUMENTAL** / RETROCEDER / PRÓXIMA /
  PAUSAR. **O LOOP está OCULTO desde a 0.2.33** (`SHOW_LOOP = false` no
  `Transport.tsx`) a pedido do operador — **a lógica continua inteira** no
  `useLocalPlayout`, e o FALAR do modo Rádio ainda arma e desarma o loop; só o botão
  sumiu. Apagar código para reescrever depois custaria mais que esconder o botão.
  O antigo `🚫🎤 VOZ` virou **`🎼 INSTRUMENTAL`** em tamanho cheio, igual aos
  vizinhos: com o cancelamento de centro aposentado, o botão não "tira a voz" — ele
  troca a fonte pelo arquivo instrumental, e o rótulo passou a dizer isso.
  **PLAY/STOP saíram daqui** (o deck faz o mestre). **LOOP deixou de ser "em breve" na
  0.2.28** — ver "Áudio das faixas locais" abaixo. Reforço server: **`LoadFromLibrary`
  é idempotente** — fila não-vazia → no-op, nunca `Clear()`+rebuild.
  - **RETROCEDER** (⏮, 0.1.17) = `disabled = busy || !hasPrev` (`hasPrev` =
    `history.length > 0`). Volta pra faixa ANTERIOR — a última das Tocadas — e religa
    ela ao ar. Fluxo (`onPrev` em `page.tsx`, simétrico ao `playCuedSafe`): **fade-out
    do que toca** (`await fadeOutRef.current()`, se o fade estiver ligado) → pega
    `historyRef.current[len-1]` (mirror p/ evitar closure velha), **sem `confirmPlayNow`**
    (ação direta, pedido do usuário — retrocede na hora mesmo cortando o ar),
    `enqueue(track_id, 1, false, origin)` → engatilha
    no slot cued → `POST /playback/next` promove ao ar → `setWant(true)`. A faixa que
    tocava vira "tocada"; a religada re-entra no histórico → dispara o badge de
    repetida (abaixo). Sem histórico → toast "Sem faixa anterior".
  - **PRÓXIMA** = `disabled = busy || !hasNext` (`hasNext` = existe `queue[1]`).
    Sem próxima na fila, avançar tocava faixa aleatória; agora fica desabilitado.
    Avanço manual junto com RETROCEDER e o ▶ do deck cued (abaixo). `onNext` faz
    **fade-out do que toca** antes do `POST /playback/next` (se o fade estiver ligado).
  - **Fade nas transições (0.1.18; liga/desliga + duração em ⚙ Configurações →
    "Modo Tocador → Fade nas transições", `web/app/lib/fade.ts`)** — RETROCEDER e
    PRÓXIMA fazem **fade-out + fade-in na duração escolhida** no lugar do corte seco
    (STOP continua corte seco de propósito).
    - **Gate único = `fadeEnabled()`** (todos os motores). O gating cross-engine da
      0.2.5/0.2.6 (`web/app/lib/transition.ts` com `markEngine`/`crossFadeInFor`/
      `crossFadeOutBetween`/`isSpotifyYouTubeCross`) foi **removido** junto com o
      crossfade preemptivo revertido — **esse arquivo não existe mais**, não tentar
      importar. O que é cross-engine hoje é só a MIXAGEM sobreposta (0.2.12, abaixo),
      que carrega o próprio `isCross(a,b)` local.
    - **Duração configurável (0.2.5)**: o store virou **ms** (`fade.ts`, localStorage
      `autodj.fade.v1`): `0`=desligado (corte seco), `3000`/`5000`/`7000`. Combo no
      `LayoutEditor` com 4 opções fixas (3s/5s/7s/Desligado) — **sem campo aberto pra
      digitar**. Compat legado: `"1"`→3000, `"0"`→0, ausente→3000 (default). Getters:
      `fadeMs()` (duração), `fadeEnabled()` = `fadeMs()>0`; setter `setFadeMs(ms)`;
      hook `useFadeMs()`. Client-side, uso caseiro — preferência de UI, não coluna no
      banco. Os hooks leem `fadeEnabled()` na hora e **rampam por `fadeMs()`** — nunca
      hardcodar duração (bug até 0.2.11: `FADE_MS = 3000` fixo nos dois hooks fazia o
      ⚙ em 5s/7s rampar 3s).
    Mecânica por motor (recurso IDÊNTICO, hexagonal):
    - **Local** (`useLocalPlayout`): ramp no `<audio>.volume` (passos de 50ms), alvo
      1.0. Faixa nova carrega em `volume=0` (`fadeInPending`) e sobe no play; `fadeOut`
      desce a 0 antes de trocar.
    - **Spotify** (`useSpotifyPlayout`): ramp via `player.setVolume` do SDK (passos de
      100ms, ganho local, sem rede), alvo `SPOT_TARGET=0.8` (espelha o volume de criação
      do device p/ não mudar a sonoridade). `fadeIn` arranca em 0 e sobe; `fadeOut`
      desce a 0.
    - **Fim NATURAL de faixa — MIXAGEM sobreposta (0.2.12; local entrou na 0.2.26,
      `web/app/hooks/useCrossEngineMix.ts`)**: até a 0.2.11 o fade na virada era
      **sequencial** — a que saía descia, a fila avançava, só então a entrante subia:
      dois fades encostados, buraco no meio, nunca sobreposição. Agora, `fadeMs()`
      ANTES do fim natural, o hook coordenador dispara **`incoming.startMix(nextUri)`
      e só depois `outgoing.fadeOut()`** — os dois provedores soam JUNTOS durante a
      janela do ⚙ (mixagem de DJ com motores diferentes).
      - **A FILA NÃO É ADIANTADA** — é o que separa isto da tentativa revertida da
        0.2.6: lá o `POST /playback/next` era antecipado e todo o resto (deck, takeover
        do Spotify, histórico) via a troca acontecer cedo. Aqui o **servidor não muda**:
        avança só no fim natural. A entrante toca "por fora" e o hook do motor dela a
        **ADOTA** quando o now-playing finalmente a alcança.
      - **Adoção/abort por `preStarted`** (ref em `useSpotifyPlayout`/`useYouTubePlayout`):
        `startMix(uri)` grava `preStarted = uri`; o efeito de play, ao ver o np virar essa
        uri, **não re-toca** (só limpa a ref e segue no volume já rampado). Efeito de abort:
        se o np virar OUTRA uri (skip manual no meio da janela), a pré-partida é descartada.
      - **Ordem importa**: entrante primeiro. Se `startMix` devolver `false` (device
        Spotify / iframe não pronto), **não** baixamos o volume do que está no ar — melhor
        sem mixagem que silêncio; `mixedFor` é zerado p/ nova tentativa.
      - **Spotify** `startMix`: `setVolume(0)` → `play(uri)` → `rampVol(SPOT_TARGET,
        fadeMs())`. Não há preload no Web Playback SDK (limitação da plataforma).
      - **YouTube** `startMix`: reusa o preload da 0.2.11 (instância ociosa já bufferizada)
        — `setVolume(0)`, vira `liveIdxRef` ANTES do `setLiveIdx`, `goLive()`, ramp. Sem
        preload, degrada p/ `cur.play(uri)`. `armMix(uri)` **congela o efeito de preload**
        (senão ele re-cueia a instância que acabou de entrar no ar) e torna o memo `roles`
        mix-aware: com np ainda Spotify, `r[liveIdx] = "cue"` — se os dois papéis ficassem
        `null` o iframe seria parqueado → **deck preto** (regressão da 0.2.9).
      - **LOCAL entrou na mixagem (0.2.26)** — antes `local` era excluído por regra
        ("qualquer coisa com local = corte seco"), o que deixava justamente o caso mais
        comum (acervo próprio, faixa local → faixa local) sem crossfade. Hoje
        `useLocalPlayout.startMix(uri)` põe a PRÓXIMA no `<audio>` OCIOSO — que a
        pré-carga da 0.2.11 já deixou bufferizado — em `volume 0` e rampa até 1,
        enquanto o do ar desce. Dois `<audio>` soando juntos = crossfade real.
        - **Dois timers de ramp** (`fadeTimer` p/ o do ar, `mixTimer` p/ o entrante):
          com um só, a segunda rampa cancelaria a primeira (o `clearInterval` é por
          timer) e um dos lados congelaria no volume em que estava.
        - **ADOÇÃO no handoff** (`preStarted`, espelha o mesmo mecanismo do Spotify/
          YouTube): quando o servidor finalmente promove a entrante, o effect de load
          **não pode** rodar o handoff normal — ele faz `currentTime = 0` e re-arma o
          `fadeInPending`, o que jogaria a música pro início e a faria cair a 0 e subir
          de novo NO MEIO. O branch de adoção só troca os papéis e não encosta no
          elemento que já toca.
        - **ABORTO**: now-playing virou outra coisa (pulo manual na janela, ou entrou
          Spotify/YouTube) ⇒ o elemento pré-iniciado é pausado/zerado, senão fica um
          áudio órfão por cima do que o operador escolheu.
        - `startMix` devolve **false** (e o coordenador NÃO baixa o volume do ar) sem
          pré-carga, com `readyState < 2` ou se a uri pedida não é mais a `queue[1]`.
      - **Gates**: `fadeMs() > 0` (⚙ desligado = corte seco), par sobreponível
        (`canMix`), `npUri !== nextUri` (mesma faixa nos
        dois decks nunca mixa), `dur > fadeMs() + 3000` (faixa curta demais não mixa),
        `remaining ≤ fadeMs()` e `remaining ≥ 600` (tarde demais), `mixedFor` = 1
        mixagem por faixa no ar.
      - **A TELA MOSTRA A MIXAGEM (0.2.13)**: na 0.2.12 só o áudio mixava — o deck da
        entrante ficava congelado em `00:00`, barra vazia e selo `CUE`, embora ela já
        estivesse tocando ("funcionou o crossfade mas não refletiu na tela"). Causa: na
        janela o servidor ainda diz que o `now_playing` é a que SAI, e nenhum hook de
        playout sabe a posição da entrante (`useYouTubePlayout` zera `livePos` com
        `!npIsYouTube`; `useSpotifyPlayout` gateia `live`/`posMs` em `npIsSpotify`).
        Fix: o coordenador exporta `CrossMixState { active, uri, elapsedMs, ms }` —
        `elapsedMs` é wall-clock (`Date.now() - startedAt` gravado no `startMix`, único
        relógio disponível), com ticker de 250ms (`UI_TICK_MS`) só enquanto a janela vive.
        Fiação: `page.tsx` → `PlayerScreen` → `ProgramList` → `DeckPair` → `Deck` (prop
        `mix`). No `DeckPair` a janela só conta se `mix.uri === queue[1].playable.uri`
        (a uri mora no `Playable`; `DeckInfo` do `nowNext` não tem `uri`) — mexeu na fila
        no meio, a tela sai do modo MIX na hora. Deck entrante passa a mostrar decorrido/
        restante reais e barra andando; os DOIS decks trocam o selo por `MIX ▲` (subindo)
        / `MIX ▼` (descendo) no lugar do `CUE`, com contorno âmbar (`.deck.mixing`) e
        barra âmbar na entrante (`.deck.mixing-in .deck-prog-fill`, CSS em `globals.css`,
        pulso com opt-out em `prefers-reduced-motion`).
      Retomada pós-refresh (resume) vai a volume pleno direto, sem fade (não é troca de
      faixa). PAUSAR/RETOMAR não fazem fade.
    - **Fim NATURAL não pode ser lido como takeover (0.2.7 — bug "acabou o Spotify e
      parou tudo")**: o Web Playback SDK dispara `player_state_changed(null)` quando o
      device do SDK deixa de ser o ativo — DOIS casos indistinguíveis pelo evento: (a)
      **takeover real** (outro aparelho da conta assumiu) e (b) **fim natural** (a faixa
      acabou, device ficou idle; o SDK às vezes emite o null com `prevState` ainda
      "tocando" ANTES do paused-em-0). Ler QUALQUER null-enquanto-tocando como takeover
      fazia `takeoverNonce++` → `onExternalTakeover` → `page` `setWant(false)` → o effect
      `!wantPlay` do `useYouTubePlayout` dava `ytStop()` na faixa entrante do YouTube → a
      estação parava e o crossfade de 7s nunca acontecia. Fix (`useSpotifyPlayer.ts`):
      `lastProgressRef {pos,dur,at,playing}` (atualizado no branch não-null a cada state);
      no branch null, `nearEnd = dur>0 && dur-(pos+elapsed) < 4000` (elapsed = wall-clock
      desde o último progresso, só se tocando). `takeoverNonce++` **só** com `wasPlaying &&
      !nearEnd` (takeover real, meio da faixa); `wasPlaying && nearEnd` (fim natural, <4s p/
      acabar) só marca `playing:false` SEM bumpar takeover → a fila avança pro próximo motor
      e a transição roda. Trade-off: takeover real nos últimos 4s da faixa fica suprimido
      (raro). **Guarda robusta (não depende de timing) em `page.tsx onSpotifyTakeover`**: o
      heurístico `nearEnd` sozinho não bastava porque na janela de fade o outro motor já
      está no ar (na 0.2.6 a fila era adiantada; hoje a mixagem da 0.2.12 põe o YouTube
      tocando por fora); quando o null atrasado do Spotify chega, matar wantPlay cortava
      o YouTube já no ar (a parada total reportada).
      A guarda ignora o takeover quando `snap.now_playing.source !== "spotify"` (já
      cruzamos de motor — Spotify só idlando) OU `snap.queue[1].playable.source !== "spotify"`
      (prestes a cruzar). Só é takeover de verdade com Spotify no ar E próxima também Spotify
      (ou fila vazia). Regra: **fim natural de faixa Spotify NUNCA pode disparar takeover** —
      só o null com Spotify ainda no ar e próxima Spotify é takeover.
    - **Wiring em `page.tsx`**: `playFadeOut = spotDeck.isSpotify ? spotFadeOut :
      localFadeOut` espelhado num `fadeOutRef` (evita closure velha); `onPrev`/`onNext`
      chamam `fadeOutRef.current()` no deck ATIVO antes de avançar.
  - **PAUSAR/RETOMAR** (rótulo do botão; antes "PAUSE") = habilitado só com deck `live` e `!stopped` (Spotify SDK ou
    local `<audio>`); é o toggle momentâneo do deck, independente do `wantPlay`.
    **PAUSE CONGELA o auto-avanço** (bug corrigido 2026-07-26): antes o fallback por
    duração (wall-clock) só olhava `wantPlay`, então pausar não parava o timer — ele
    disparava `/playback/next` mesmo pausado, a faixa avançava sozinha e o load da
    próxima re-tocava, "tirando" o pause. Ambos os hooks têm agora um estado
    **`userPaused`** (intent de segurar a faixa, distinto do `wantPlay`) que gateia o
    fallback timer E o auto-play; RETOMAR/troca de faixa reseta `userPaused=false`
    (faixa nova entra fresca, seguindo o `wantPlay`).
- **Deck cued (Player ocioso) ▶ toca a próxima CARREGADA agora** (`DeckPair.
  nextRole.onCtl = onPlayCued`, habilitado só com `hasNext`): clicar no ▶ do deck
  ocioso promove **exatamente a faixa engatilhada nele** para o ar. Como o backend
  mantém engine e mirror em lockstep (ver "Enqueue" abaixo), `mirror[1]` (a que a
  UI mostra cued) **é** `engine.items[0]` — logo `/playback/next` promove ela, não
  uma "próxima-da-fila" diferente. `playCuedSafe` (page.tsx): `confirmPlayNow` →
  `/playback/next` → `setWant(true)`. Estação no ar ⇒ confirma antes (corta o que
  toca). Reverte a decisão 0.1.5 (▶ cued desabilitado) — agora que engine e mirror
  não desincronizam, clicar toca a certa. Só há próxima via **PRÓXIMA** (transport),
  **▶ do deck cued** OU **fim natural** da faixa (driver de playout).
- **Decks FIXOS + abas Tocadas | Fila** (0.1.8): `ProgramList` = `.playout` (flex
  coluna) com os dois decks A/B **parados no topo** (`ul.rows.deck-fixed`, `flex:none`,
  nunca rolam) e, abaixo, `PlayoutTabs` — segmented control (classes `opsub2`) com
  duas abas roláveis:
  - **FILA** (default): cauda da fila (`queue.slice(2)`, pula Deck A/B) + eventos
    `below` + `EmptyQueue`. É o **drop target de append** (arrastar mídia da
    biblioteca cai aqui). Contador no rótulo = itens engatilhados.
  - **TOCADAS**: eventos `above` + histórico da sessão (`HistoryRow`), vazio →
    "Nada tocado ainda nesta sessão." **Visual idêntico à FILA** (0.1.17):
    `HistoryRow` renderiza com classe `.row.q` (mesmo grid/estilo da `QueueRow`) —
    "como se a faixa tivesse descido pra lá". Mesmas colunas (pos│grip│ico│check│capa│
    título│fonte│origem│trans│dur); grip é placeholder (não arrasta), coluna trans
    mostra `tocada`, e o carimbo de hora virou tooltip da linha (`title`). Removidos
    `.row.played`/`.r-when`.
    - **Badge de repetida (`.r-repeat`, 0.1.17)**: `PlayoutTabs` reduz o histórico num
      `playCounts` (Map `track_id → nº de vezes na sessão`) e passa `plays` à
      `HistoryRow`; o badge `🔁 N×` (tokens `--ac-warn-*`, laranja) só aparece quando
      `plays > 1` — ex.: depois de RETROCEDER religar a mesma faixa, ela reaparece nas
      Tocadas com a contagem total.
  Antes tudo morava numa lista única rolável (histórico → decks → fila) que fazia os
  decks "pularem" ao rolar e re-ancorava a cada snapshot do WS (2s); agora os decks
  ficam parados e só as listas rolam. Drop SOBRE um deck (adiantar/cortar,
  `onDeckDrop`) segue em `ProgramList`; append à fila mudou p/ a aba FILA.

## Capa + intérprete (cover/artist) — provider-agnóstico (hexagonal)

Player (decks A/B), fila e histórico mostram **capa + título + cantor/intérprete**,
layout estilo Spotify (título forte em cima, artista cinza embaixo). Deck = capa
grande (`.cover-deck` 60px, VirtualDJ-like); fila/histórico = miniatura menor (26px).
**Ocioso = "—"** no header e nos decks (`IDLE_NOW.full`/`nextRole.artist`), não
"nada no ar" — silêncio é estado neutro, não rótulo.

- **Endpoint uniforme `GET /api/v1/library/tracks/{id}/artwork`** resolve a capa
  **por trás da porta**, sem caso especial por provider:
  - provider com URL externa (Spotify album image / YouTube thumb) → **302 redirect**
    p/ a imagem (`Track.ArtworkURL`);
  - arquivo local → **ffmpeg extrai o embedded picture** (`-map 0:v:0 -f image2pipe`)
    e serve os bytes (`Cache-Control 1d`);
  - sem capa → **404** (`library.ErrNoArtwork`).
- `LibraryService.Artwork(id)` (porta inbound) decide 404 cedo (`ArtworkURL=="" &&
  Path==""`). O front lê **uma coisa só**: `trackArtworkURL(track_id)` (`web/app/lib/
  api.ts`) — deriva a URL do id, sem mudança de DTO. `TrackCover.tsx` renderiza
  `<img onError>` → **placeholder** (ladrilho ♪) em 404/sem id. A mesma peça vale p/
  spotify/youtube/beatport/local — regra na camada uniforme, não por provider.
- **Dado**: `library.Track.ArtworkURL` (URL externa da capa; vazio p/ local) +
  `spotify.CatalogItem.ArtworkURL` (album image, `albumImage()` pega a ~300px).
  `importItems`/`toPlayable` propagam. **Persistido** na coluna `artwork_url` dos
  repos de faixa (`sqlite/schema.sql` + `postgres/migrations/016_artwork_url.sql`;
  migração idempotente `ensureArtworkColumn` no SQLite p/ estações antigas). Sem essa
  coluna, o `Upsert` droppava a URL e a faixa Spotify reaberta pela fila (só tem
  `track_id`) caía em `/artwork` 404 → placeholder ♪ (bug prod 0.1.20). Regra: campo
  de exibição do `Track` que vem do provider **tem que persistir no repo**, senão só
  a lista de Mídias (que passa `src` direto) mostra — fila/deck/Tocadas 404. `playableDTO.artist`/`track_id` já fluem via
  WS/REST → `Playable.artist` no front alimenta a linha do intérprete (deck via
  `nowNext.info()` usa `p.artist` real, não split do título). ⚠️ **Dois DTOs
  separados**: `adapters/driving/httpapi/dto.go` (REST) e `adapters/driving/ws/dto.go`
  (WebSocket). A UI ao vivo (deck/fila/histórico) lê o **WS**; o `playableDTO` do WS
  **não serializava `Artist`** (só o REST) → intérprete sumia em deck/fila/histórico
  enquanto a busca Spotify (endpoint próprio) mostrava certo. Fix: campo `Artist
  json:"artist"` no `playableDTO` + preencher nos 3 pontos do `snapshot()`. Histórico
  (`usePlayHistory.stamp` faz `{...p}`) herda automático. Regra: **ao adicionar campo
  de exibição no Playable, atualizar OS DOIS DTOs**.
- **Abas separadas por fonte + import recarrega** (0.1.21): **Músicas = só locais,
  Spotify = só importadas — nunca juntar** (pedido do usuário). `OperacionalScreen`
  divide `tracks` por `source`: `localAll` (`source!=="spotify"`) e `spotAll`
  (`source==="spotify"`). Aba **Músicas** (e demais categorias de mídia) lista só
  `localMedia` (= `localAll` filtrado pela busca `q`); aba **Spotify** renderiza
  **só** o `SpotifyPanel` (conexão + busca/import) — a lista `OpMediaList` de `spotAll`
  (faixas importadas) está **oculta por ora (0.1.23, pedido do usuário)**; o import segue
  gravando no banco, só não renderiza. Importada nunca salta pra Músicas nem mistura.
  `media` (lista que os handlers indexados sel/markers/edit/delete/enqueue enxergam) =
  `cat==="spotify" ? spotAll : localMedia`, então o índice sempre casa com a aba ativa.
  O import (`SpotifyPanel` via `useSpotify.importItem`) e a lista (`useLibrary`, montado
  no `OperacionalScreen`) são **hooks separados**: `useLibrary` só recarrega no mount e
  nas próprias mutações (upload/update/remove/markers) — o import externo persiste no
  backend mas **não** avisa a lista → `tracks` do React fica velho e a faixa importada
  só aparecia após F5 (bug prod "importei 25 e não apareceu", só display; o DB tinha
  tudo). Fix: `SpotifyPanel` tem prop `onImported?: () => void`, disparada **após** o
  import landar (`handleImport`) e no enfileirar (`onEnqueueItem`, que também importa);
  `OperacionalScreen` liga `onImported={handleImported}` = `() => void reload()` do
  `useLibrary` (sem trocar de aba). Regra: **ação que grava faixa por fora do
  `useLibrary` tem que chamar `reload` ao terminar** — senão a lista não reflete.
  **Chip Músicas desabilitado sem faixa local** (0.1.21): `hasLocal = tracks.some(source
  !== "spotify")`. Sem nenhuma local, `OpCategoryBar` recebe `disabledKeys={new
  Set(["musicas"])}` → o chip fica esmaecido/não-clicável (`.opcat.off`, title "Adicione
  uma música local para habilitar"); e um efeito (`loaded && !hasLocal && mediaCat`) cai
  pra aba **Spotify**, que já tem conteúdo importável. `OpCategoryBar` ganhou props
  `disabledKeys?: Set<OpCategoryKey>` / `disabledTitle?` (chip `disabled` + `.off`).
  Adicionou 1 música local → `hasLocal` vira true → chip reabilita sozinho.
  **Funções "em breve" = VISÍVEIS e desabilitadas (0.2.14)**: `⚡ Instantâneas`
  entrou fixo no `disabledKeys` (title "Instantâneas — em breve") e o botão
  `🎙 Locutor com IA`, que já tinha `disabled` mas parecia clicável, ganhou estilo
  próprio (`.opscreen-mix:disabled`: opacity 0.42 + grayscale + `not-allowed` +
  hover neutro). Como o `disabledTitle` era único p/ todos os chips, o
  `OpCategoryBar` ganhou `disabledTitles?: Partial<Record<OpCategoryKey,string>>`
  (motivo POR chip, com fallback no `disabledTitle`). Regra: função não liberada
  **não some da tela** — o usuário quer ver que ela existe.
- **Selo "não-mixável" NÃO existe mais na UI (0.2.14)**: `SpotifyResults`,
  `YouTubeResults` e a linha "Mixável" do `TrackDetailModal` deixaram de mostrá-lo
  (`.spot-badge`/`.spot-badge.drm` removidos do `globals.css`). Mixabilidade é
  consequência determinística da plataforma (`spotify`/`youtube` ⇒ `mixable=false`,
  **invariante do domínio intacta**) — repetir isso em toda linha era ruído. Rodapés
  e toast de import dos painéis seguem explicando "só metadados / BPM-refrão-waveform
  ficam pros locais", sem o rótulo. **Mensagens de ERRO que citam não-mixável
  (`useDeck`, `useCartwall`) ficam** — ali explicam uma falha concreta.
- **Drag = só a capa flutua** (`web/app/lib/dragImage.ts` `dragCover(e)`): no
  `onDragStart` troca a imagem-fantasma nativa (linha inteira) por um clone da
  `.track-cover` da própria linha via `setDragImage`, com efeito (classe
  `.cover-drag-ghost`: `scale(1.7) rotate(-5deg)` + sombra + ring azul). Usado em
  `QueueRow` (reordenar fila) e `SpotifyResults` (arrastar busca p/ o playout). O
  clone fica offscreen e some no próximo `requestAnimationFrame` (snapshot já foi).
- **Capa na busca Spotify** (`SpotifyResults`): resultado de busca **não é faixa
  importada** (sem `track_id`/endpoint), então `TrackCover` aceita `src` (URL de
  capa crua) além de `trackId`, e ganha size `spot` (38px). `CatalogItem.artworkUrl`
  (backend já serializa `spotify.CatalogItem.ArtworkURL` → `albumImage()`) alimenta
  a `<img>`; 404/vazio → placeholder ♪.
- **AUTO LIGADO A CADA SESSÃO, no cliente (0.2.27)** — `useAutopilot` liga o piloto
  no boot quando o servidor responde com ele desligado, **a menos** que exista a
  preferência `autodj.auto.v1 = "off"`. E a preferência só é gravada pelo **clique no
  próprio chip AUTO**: o STOP mestre também desliga o piloto (senão ele religa a
  estação no ato), mas isso é ação de TRANSPORTE, não opinião sobre o AUTO — por isso
  `toggleForTransport()`, que não persiste nada. Sem essa separação, um STOP fazia a
  sessão seguinte abrir com AUTO OFF sem ninguém ter pedido. O gate de servidor
  abaixo continua valendo (é o padrão do boot); esta é a camada do operador.
- **Autopilot LIGADO por padrão** (é um AutoDJ): gate em `cmd/serverkit/serverkit.go`
  é opt-out — `AUTODJ_AUTOPILOT != "0"` ⇒ boot sobe com AUTO On (`SetEnabled(true)` +
  tick imediato). `AUTODJ_AUTOPILOT=0` desliga p/ operação 100% manual. `serve-local.sh`
  não seta a var, então local sobe ligado. Confirmar: `curl /api/v1/autopilot` →
  `{"enabled":true,...}`.
- **Timers do header (`Timers.tsx`)** — ligados a dado real (eram mockup hardcoded):
  ⏱ = decorrido da faixa (`posMs`), － = restante/countdown (`durMs-posMs`),
  ＋ = **reserva da fila** (autonomia até esvaziar, `queueMs-posMs`), RDS = indicador
  RadioText. Props: `PlayerScreen` (`posNow`/`durNow`/`queueTotalMs`) → `NowPlayingHeader`
  → `Timers`. `posMs` real só vem do Spotify SDK; fila local sem SDK ⇒ `--:--`.
- **Drop POR-DECK** (`DeckPair` → `Deck` com zona de drop própria; `onDeckDrop(e,
  deckIsLive)`): cada deck (A/B) é um alvo de drop separado. `deckIsLive = playing &&
  activeDeck === esse deck` — o deck que está TOCANDO agora. Regra do drop:
  - **No deck AO VIVO (cut)** → tocar já: mídia entra na cabeça (`pos=0`) com
    `playNow=true`; corta o que toca. Realce âmbar (`.deck-hot-cut`). Passa pelo
    `confirmPlayNow` (pergunta antes de cortar).
  - **No deck OCIOSO / nada tocando (cue)** → "coloca e espera": mídia vira a PRÓXIMA
    (`pos=1`, ou `pos=0` se nada no ar), `playNow=false`, **sem popup**, sem cortar.
    Realce verde (`.deck-hot-cue`).
  Fontes aceitas: mídia local (`text/autodj-track`), resultado Spotify
  (`text/autodj-spotify-uri`) **e** item da própria fila (`text/autodj-reorder` →
  adiantar). `stopPropagation` evita cair no drop da grade (que anexaria ao fim).
- **Adiantar item da fila arrastando no deck** (`onPromote(fromVisible, cut)` em
  `page.tsx` → `promoteSafe`): arrastar uma `QueueRow` da cauda POR CIMA de um deck.
  `fromVisible` = índice na cauda visível (`slice(2)`) → mirror `+2`. **cut** (deck ao
  vivo) → `move(mirror,1)` + `POST /playback/next` (corta o atual e promove); confirma
  antes. **!cut** (deck ocioso) → só `move(mirror,1)`: vira a próxima e espera. A
  reordenação DENTRO da cauda (soltar numa outra `QueueRow`) segue igual; o drop no
  deck é o único caminho que alcança os slots 0/1.
- **"Tocar agora" pede confirmação se há faixa no ar** (`confirmPlayNow` em
  `page.tsx`): todo caminho que corta o ar (menu `OpContextMenu`/`SpotifyResultMenu`
  com `playNow=true`, drop no deck ao vivo, ou promover item da fila no deck ao vivo)
  passa pelo guard. On-air real = `hasHead && wantPlay` (ref `onAirRef`); se ligado ⇒
  `confirmDialog` ("Trocar a faixa no ar?"). Cancelar aborta antes de
  enfileirar/importar/mover. Estação parada (nada no ar) ⇒ nunca pergunta, e todo
  drop vira "coloca e espera" (cue).

## Áudio das faixas locais — trim, ganho e vocal (0.2.28)

Três recursos, **uma passada de `ffmpeg`**, todos com liga/desliga em ⚙ Configurações →
"Áudio das faixas locais". (A 0.2.29 chegou a acrescentar um quarto sinal a esta mesma
passada — imagem estéreo, para o modo VOZ —, **removido na 0.2.32** junto com o
cancelamento de centro.) Valem **só para faixa local (PCM)**: Spotify e YouTube tocam
out-of-band, sem PCM deste lado — não há o que medir nem o que cortar, e o invariante
`spotify|youtube ⇒ !mixable` segue intocado.

- **O ponto de partida era pior que zero.** `mixing.Settings` já tinha `TrimMusicas`,
  `TrimVinhetas`, `Normalize` e `NormalizeVol` desde a Story 3.3 — com tela, rota e
  persistência — e **nenhuma linha de código os lia**. `library.Analysis.LoudnessLUFS`
  existia como campo e nunca era calculado (nem gravado: `Track` não tinha o campo). O
  marcador `intro` estava documentado como *"where vocals kick in"* e ninguém o
  preenchia. E o analisador (`audioanalyze.CLI`) shellava `aubio`/`keyfinder-cli`/
  `audiowaveform`, **nenhum dos três na imagem de produção** — logo, em prod ele era o
  `Noop` e nada era analisado. O trabalho foi ligar o que estava pendurado, usando o
  `ffmpeg` que já existe nos dois ambientes.

- **A passada única** (`adapters/driven/audioanalyze/cli.go`, const `playoutFilter`):
  `asplit=3` alimenta `silencedetect` (cues de trim), `ebur128` (loudness) e um par
  **mid/side** por janelas de 200ms (vocal). Três sinais, um decode. Medido: **3 min de
  áudio em ~1,3s**.
  - ⚠️ **`framelog=quiet` é obrigatório** no `ebur128`: sem ele são ~1800 linhas por
    faixa de 3min e o resumo se perde no meio. E o parser lê **só a linha do resumo** —
    as de quadro carregam o mesmo `I: … LUFS` e dariam o valor do primeiro 100ms
    (o silêncio da entrada, ≈ −70).
  - ⚠️ **`measure_perchannel=RMS_level`** corta o `astats` de ~30 métricas por janela
    para uma. Sem isso são ~60 mil linhas numa faixa comum.
  - **`ametadata=print:file=-` escreve na SAÍDA PADRÃO**, enquanto silencedetect e
    ebur128 logam na de ERRO. Dois canais, dois parsers, sem desembaraçar texto — é o
    que torna a passada única viável.
  - **O ffmpeg FECHA o silêncio final no EOF** (emite `silence_end` com o tempo total),
    então esperar um `silence_start` órfão não funciona. `parseSilence` decide por
    geometria: o silêncio que **termina no fim do arquivo** é o vazio final.

- **Detecção de vocal — a física.** Voz de estúdio é mixada no **centro**; instrumento é
  panorâmico. `mid` (L+R) reforça o central, `side` (L−R) o cancela ⇒ `mid − side` mede
  "quão centralizado". O `bandpass` em ~1,4kHz tira bumbo e baixo (também centrais) da
  conta. Medido com sinal sintético: instrumento hard-left dá **0 dB**, tom centralizado
  por cima dá **+15 dB**.
  - **Regra do operador, codificada: na dúvida NÃO marca.** Só marca com subida
    sustentada por **≥1,5s** acima da linha de base do intro, depois do `cueIn`, nunca
    antes de 1s. `VocalMs == 0` é resposta legítima, não dado faltando — vocal marcado
    errado faz a locução entrar em cima do canto.
  - **A linha de base é MEDIANA, não média**: um estouro no começo (contagem, efeito)
    deslocaria a média e esconderia a entrada real da voz.
  - **Mono não tem side ⇒ não marca vocal.** Canais vêm do `ffprobe`; trim e loudness
    seguem normais.

- **⚠️ O BPM NUNCA FUNCIONOU ATÉ A 0.2.33 — dois binários, dois formatos.**
  Medido no acervo do operador: **83 faixas, ZERO com BPM**, com a ferramenta
  instalada e sem erro nenhum. O motivo é uma incompatibilidade silenciosa:

  ```
  aubio tempo <arquivo>   → "110.31 bpm"          (UM número, já é o resultado)
  aubiotrack <arquivo>    → 1.665510 2.215442 …   (tempos de cada batida)
  ```

  O código chamava `aubio tempo` e mandava a saída para `bpmFromBeats`, que espera
  timestamps: com uma linha só ela não fecha dois deltas e devolve **0**. Hoje
  existe `parseBPM` (lê "N bpm", com faixa de sanidade 20–300 — o aubio devolve 0
  em trecho sem batida, e um "BPM 0" gravado pareceria medido) e **plano B no
  `aubiotrack`**, porque os dois ambientes divergem: o Homebrew instala `aubio`, e
  o pacote `aubio-tools` do Debian — o da imagem de deploy — traz `aubiotrack` sem
  garantia do unificado. `Available()` aceita qualquer um dos dois.
  **A imagem passou a instalar `aubio-tools`** (~2 MB); sem ele prod ficava com o
  acervo inteiro sem BPM, em silêncio. `keyfinder-cli` (tom musical) segue de
  fora: não existe no repositório do Debian e exigiria compilar, o que não se paga
  por um campo de exibição.

- **⚠️ O BPM NUNCA FUNCIONOU ATÉ A 0.2.33 — dois binários, dois formatos.**
  Medido no acervo do operador: **83 faixas, ZERO com BPM**, com a ferramenta
  instalada e sem erro nenhum. A incompatibilidade era silenciosa:

  ```
  aubio tempo <arquivo>   → "110.31 bpm"          (UM número, já é o resultado)
  aubiotrack <arquivo>    → 1.665510 2.215442 …   (tempos de cada batida)
  ```

  O código chamava `aubio tempo` e mandava a saída para `bpmFromBeats`, que espera
  timestamps: com uma linha só ela não fecha dois deltas e devolve **0**. Hoje há
  `parseBPM` (lê "N bpm", com faixa de sanidade 20–300 — o aubio devolve 0 em
  trecho sem batida, e um "BPM 0" gravado pareceria medido) e **plano B no
  `aubiotrack`**, porque os ambientes divergem: o Homebrew instala `aubio`, e o
  pacote `aubio-tools` do Debian — o da imagem de deploy — traz `aubiotrack` sem
  garantia do unificado. `Available()` aceita qualquer um dos dois.
  **A imagem passou a instalar `aubio-tools`** (~2 MB); sem ele prod ficaria com o
  acervo inteiro sem BPM, em silêncio. `keyfinder-cli` (tom) segue fora: não existe
  no repositório do Debian e exigiria compilar, o que não se paga por um campo de
  exibição. Backfill mede o acervo antigo sozinho (83/83 na verificação).

- **Parsers PUROS em `playout.go`, testados sem binário** (espelha o `bpmFromBeats`):
  `parseSilence`, `parseIntegratedLUFS`, `parseChannelRMS`, `detectVocalStart`, com
  fixtures em `testdata/` que são **saída REAL do ffmpeg**. Mudança de formato do
  upstream aparece como teste vermelho, não como campo zerado em silêncio na produção.

- **Cues viajam como MARCADORES, não colunas.** `Marker` ganhou `Auto bool` e
  `Track.ApplyAutoMarkers` substitui só o que é automático, **preservando o que o
  operador colocou à mão** (reusa `SetMarkers` — não duplica as regras de singleton,
  ordem e range). Motivo: `markers` já é JSON no banco (zero migração), já tem rota,
  editor e desenho, e assim a marca que o operador arrasta dirige o deck exatamente como
  a do analisador. Uma fonte de verdade, não duas. `start` = cue in, `end` = cue out,
  `intro` = vocal; acessores `Track.CueInMs()/CueOutMs()/VocalMs()`.
  Cue fora de `(0, duração]`, ou `end` antes de `start`, é **descartado sozinho** —
  arredondamento entre duas sondas do mesmo arquivo não pode derrubar BPM e loudness.

- **Só `loudness_lufs` virou coluna** (`ensureLoudnessColumn` no SQLite +
  `migrations/018_loudness.sql`): sem ela o `Upsert` perderia a medida e o `Backfill`
  re-analisaria a mesma faixa em **todo boot, para sempre**. E o `Backfill` passou a
  re-enfileirar quem tem `LoudnessLUFS == 0`, não só `!Analyzed()` — senão o recurso
  estrearia só para faixa nova, ou seja, não estrearia para quem já usa o app.

- **`NormalizeVol` passou a ter significado**: `alvo = -24 + (vol/100)·12`
  (`mixing.TargetLUFS`, espelhado em `web/app/lib/gain.ts` — mexeu num, mexa no outro).
  O default histórico **89 cai em ≈ −13,3 LUFS**, o padrão de streaming: escolhido
  justamente para que **nenhum dado precisasse migrar**.

- **As dicas chegam ao browser no `Playable`** (`cue_in_ms`, `cue_out_ms`, `vocal_ms`,
  `loudness_lufs`, `category`), preenchidas no ÚNICO ponto de construção (`toPlayable`)
  e serializadas nos **DOIS DTOs** (`ws/dto.go` e `httpapi/dto.go`; o do WS ganhou um
  `toPlayableDTO` único — montar o struct campo a campo em três lugares foi como o
  `artist` sumiu do deck na 0.1.20). Vão no `Playable` e **não** num fetch novo porque o
  snapshot já é espelhado em localStorage: as dicas funcionam **offline de graça**.

- **⚠️ GANHO NO `<audio>.volume`, NUNCA em Web Audio** (mas ver a exceção do modo VOZ
  na seção seguinte — a diferença é qual caminho de áudio fica refém do
  `AudioContext`). O caminho "certo" seria um
  `GainNode`, mas o `AudioContext` nasce `suspended` e exige gesto do usuário — com o
  histórico de autoplay barrado deste app (0.2.26/0.2.27), rotear o áudio por um nó que
  pode não destravar arrisca deixar a estação **muda até alguém clicar**. Regressão
  inaceitável num playout. O ganho entra nos **7 pontos** do `useLocalPlayout` onde o
  literal `1` significava "ganho pleno", via `gainRef` — exatamente o que o motor do
  Spotify já faz com `SPOT_TARGET = 0.8`.
  **Limite honesto**: `volume` não passa de 1, então sobra ~**+1,9 dB** de reforço.
  Atenuar é ilimitado; amplificar, não. Normalização desligada ⇒ ganho 1.0 = exatamente
  o comportamento anterior. Faixa sem medição ⇒ a referência (0.8).
  A **entrante da mixagem sobe até o ganho DELA** (`nextGainRef`), não até 1 — senão a
  normalização se perderia justo na virada, onde o salto mais incomoda.

- **`endMs` ≠ `durMs`, de propósito.** O deck publica os dois: `durMs` é a duração REAL
  (é o que a tela mostra e a barra mede) e `endMs` é o **fim efetivo** (menor quando o
  trim corta o vazio do fim). Quem coordena a mixagem usa `endMs` — com a duração crua o
  crossfade começaria **depois** do fim já cortado, e a virada viraria corte seco justo
  nas faixas com silêncio no fim.

- **Trim no playout**: `cueIn` entra pelo `pendingSeek` que já existia (aplicado no
  `loadedmetadata`), e a **retomada pós-refresh vence o trim** (quem recarregou quer
  voltar onde estava). `cueOut` encerra a faixa por checagem no tick + encurtamento do
  timer de fallback, ambos sob o guard `advancedFor` que já impedia avanço duplo.
  **Trim não reescreve arquivo** — é só de reprodução, sempre reversível.

- **LOOP saiu do "em breve"** (estava `disabled` com "até Épico 4"): com loop armado e
  vocal marcado, ao alcançar `vocalMs` a faixa volta ao **`cueIn`** — não ao 00:00, que
  jogaria o vazio da frente no meio da locução. **FALAR arma e desarma junto**; o botão
  LOOP continua clicável sozinho porque **FALAR só existe no modo Rádio**
  (`showTalk={isRadio}`) e o padrão do app é Tocador — sem ele, o recurso ficaria
  inalcançável no modo padrão. Com o loop armado o timer de relógio de parede **não
  avança**: não pode trocar a faixa por cima de quem está falando.

- **Marcas no deck** (`DeckMarks.tsx`): zonas de corte hachuradas, tick 🎤 no vocal,
  faixa do loop (pulsando quando armado, com opt-out em `prefers-reduced-motion`) e selo
  de ganho ("−3,2 dB") quando está atenuando. As marcas são **irmãs** da barra, dentro
  de um `.deck-viz` novo — a `.deck-prog` tem `overflow: hidden` (é o que arredonda o
  preenchimento) e cortaria o tick e o selo. `pointer-events: none` é inviolável: a
  barra é clicável para seek. As dicas vêm do `Playable` que o `DeckPair` já tem em mãos
  — **nenhum fetch novo**.

- **⚙ Configurações, não o modal de Mixagem.** Os interruptores foram para o
  `LayoutEditor` porque o `MixConfigModal` é gated no modo Rádio e o padrão do app é
  Tocador — na prática nem dava para abri-lo. Mesma configuração, mesma rota
  (`useMix`), então os dois ficam em sincronia. O alvo mostra o LUFS resultante ao lado
  do número ("89% ≈ −13,3 LUFS"): knob escondido é knob que mente.
  Trocar um interruptor vale **a partir da próxima faixa** — as dicas vivem em `ref`,
  fora das deps dos efeitos, para que a música no ar não seja reposicionada sob os pés
  do operador.

- **Repo de mix decodifica SOBRE o default** (`sqlite`/`postgres` `MixRepo.Get`): chave
  ausente no blob antigo = campo que ainda não existia ⇒ valor de fábrica, não zero.
  Sem isso, `DetectVocal` nasceria `false` em toda estação já existente, em silêncio.

## Modo VOZ — tocar a faixa local sem o vocal (0.2.29 · reescrito na 0.2.32)

Botão **`🚫🎤 VOZ`** no Transport, **entre LOOP e RETROCEDER** (mesma família: controle
ao vivo, um clique, sempre à mão). Ligado, a faixa local toca pelo **instrumental
separado por IA** no lugar do arquivo original; desligado, volta ao normal na hora.
Mais o selo **`INSTRUMENTAL`** no deck ao vivo. **Não é gravado** — F5 volta desligado,
igual ao LOOP. Vale **só para faixa local (PCM)**; `spotify|youtube ⇒ !mixable` intocado.

- **⚠️ O CANCELAMENTO DE CENTRO FOI APOSENTADO (0.2.32) — e o motivo é uma medição.**
  A 0.2.29 ligava este botão a um grafo de Web Audio (`side + lowpass(mid,200) +
  highpass(mid,9k)`). A tabela que justificava aquilo foi medida num **mix sintético**
  — senoides puras, vocal perfeitamente centrado —, ou seja, um caso construído para o
  filtro acertar. Medido depois numa **música real** (*Ace Of Base — All That She
  Wants*, trecho de 40s, usando o próprio Demucs como régua: "quanta voz sobra"):

  | caminho | voz restante | redução |
  |---|---|---|
  | original (referência) | −17,3 dB | — |
  | **cancelamento de centro** | **−19,3 dB** | **2,0 dB** |
  | **instrumental (IA)** | **−49,2 dB** | **31,9 dB** |

  Dois decibéis não é "tirar a voz" — é não fazer nada de audível. Manter os dois
  caminhos atrás do mesmo botão significava que o operador não tinha como saber qual
  dos dois estava atuando, e o selo `INSTRUMENTAL` mentia metade das vezes. **Decisão:
  o caminho de DSP saiu inteiro**, código e dado.

  **O que foi REMOVIDO (não deixar voltar por engano):** `web/app/lib/vocalCut.ts` e
  todo o grafo de Web Audio do `useLocalPlayout`; `Analysis.StereoSideDB`,
  `Track.StereoSideDB`, `MinStereoSideDB`, `Track.VocalRemovable()`;
  `sideEnergyDB`/`stereoFloorDB` do analisador; a coluna `stereo_side_db` dos dois
  repos; `Playable.VocalRemovable` e o `vocal_removable` dos **dois** DTOs.

  **Lição registrada, porque é a mesma de outras vezes**: um teste construído a partir
  da hipótese confirma a hipótese. O sintético provava que o filtro fazia o que a
  fórmula dizia — nunca que aquilo removia voz de uma música. Medida de qualidade de
  áudio precisa de material real.

- **A régua ficou UMA**: `Track.HasInstrumental()` — existe PCM deste lado e o arquivo
  separado já foi gerado? O `Playable` **não** carrega isso (é retrato do
  enfileiramento; ver a seção da IA abaixo): o hook de playout consulta o endpoint ao
  vivo. Como o modelo separa por CONTEÚDO, imagem estéreo saiu da conta — e faixa
  **mono** deixou de ser caso especial.

- **⚠️ WEB AUDIO VOLTOU AO PLAYOUT — só pela TRANSIÇÃO do VOZ** (`lib/filterSweep.ts`).
  Corrige o que ficou escrito aqui na primeira versão da 0.2.32 ("não é mais usado em
  lugar nenhum"): a varredura de filtro exige um nó de filtro, e `<audio>` só expõe
  ganho. As travas que tornam isto aceitável são as mesmas da 0.2.29, e não à toa é o
  caminho do MESMO botão:
  - o contexto nasce do **clique** no VOZ, **nunca** no carregamento da página — o
    cenário perigoso (autoplay barrado deixando a estação muda no boot) não existe
    por construção;
  - `attach` **só roteia com o contexto já em `running`**. Contexto suspenso +
    elemento roteado = estação MUDA, que é a regressão que não pode acontecer. Por
    isso o contexto é pedido no INÍCIO da troca (o `resume()` é assíncrono e tem os
    ms da carga do arquivo para subir), não na hora de filtrar — senão a primeira
    troca da sessão sairia sem efeito;
  - sem contexto utilizável, cai no **fade sequencial** e a troca acontece do mesmo
    jeito. O efeito nunca é condição pra a música continuar;
  - o filtro **descansa em 20 Hz** (abaixo do audível): elemento roteado e parado ali
    soa como elemento não roteado. `createMediaElementSource` é irreversível, então o
    cleanup devolve a frequência ao repouso — esquecê-lo no topo entregaria a próxima
    faixa sem grave, sem explicação.
  Fora daqui a regra da 0.2.28 continua valendo: **ganho no `<audio>.volume`**.

- **A TRANSIÇÃO — varredura de passa-alta com crossfade por baixo (afinada de ouvido).**
  Trocar a fonte é trocar de ARQUIVO, e trocar arquivo no elemento no ar **dá repique**:
  o browser descarta o buffer, baixa de novo, decodifica e busca a posição — o elemento
  PARA de produzir áudio por 80–300 ms. Nenhum ajuste de volume no mesmo elemento
  resolve isso. O que resolve tem três partes, e as três foram necessárias:
  1. **a fonte nova entra no elemento OCIOSO**, já tocando e já na posição (mesmo
     mecanismo do crossfade local↔local da 0.2.26) — assim nunca há silêncio;
  2. **varredura de filtro** 20 Hz → **4 kHz** → 20 Hz, 3 s por lado: o grave sai pela
     faixa que vai embora e volta pela que chega. Aos ouvidos é UMA varredura só
     atravessando a troca. É o mascaramento certo, e a razão é espectral — no topo o
     sinal perdeu quase toda a energia, então é onde uma descontinuidade é menos
     audível;
  3. **crossfade de 6 s por baixo**, exatamente a duração do gesto: a voz sai junto com
     o grave e o instrumental entra junto com ele de volta. Durações divergentes fariam
     o ouvido ler dois eventos (filtro e volume) em vez de um.
  **A sobreposição só é segura POR CAUSA do filtro**: cruzar em banda cheia somaria duas
  cópias da mesma base com milissegundos de defasagem (comb filtering, aquela lavagem
  metálica); no miolo do cruzamento os dois estão passa-alta em 4 kHz, sem o conteúdo
  correlacionado que produzia o efeito. Pela mesma razão a faixa que sai **fica no topo**
  pelo resto do gesto.
  ⚠️ **O ESTALO ERA DA PRÓPRIA RAMPA, não da troca de arquivo.** `rampEl` usava passo
  fixo de 50 ms: ótimo num fade de 3–7 s (60–140 passos), mas numa rampa de 140 ms dá
  **3 passos** — o volume salta 1,0 → 0,67 → 0,33 → 0, degraus de ~3,5 dB, e salto de
  ganho é descontinuidade, que É clique (zipper noise). Hoje o passo é **adaptativo**
  (~30 passos em rampas ≤600 ms, piso de 8 ms) e a curva é **cosseno**, não reta — a
  reta tem quina nas pontas, e quina também estala.
  O **buffer anti-queda é ignorado** com o modo ligado — o Blob guardado é sempre do
  original, e preferi-lo tocaria a versão COM voz.
  Constantes: `SWEEP_MS` (3000, por lado) em `useLocalPlayout.ts`; `TOP_HZ` (4000) e
  `REST_HZ` (20) em `lib/filterSweep.ts`.

- **O BOTÃO HABILITA SOZINHO quando a separação termina.** Uma consulta só por faixa não
  bastava: a separação leva minutos e costuma terminar com a faixa já no ar, então a
  resposta ficava congelada em `queued` e o botão só acordava na troca de faixa. O poll
  (10 s) existe **enquanto** o servidor diz que está trabalhando; `ready` e `failed`
  encerram. Sem trabalho pendente, zero requisição.

- **⚠️ `stemSrcRef = null` significa "NÃO SEI o que está carregado", e não pode virar
  troca.** Quem carrega o elemento é quem anota qual fonte entrou (carga fresca, handoff
  da pré-carga e adoção da mixagem — os três). Sem essa regra o bug era grave e ficou
  invisível até o acervo ter instrumentais: ao terminar a consulta, `hasStem` virava
  true e disparava a troca com `want = false` contra `null`; como `null !== false`, ele
  "trocava" o original PELO original — `src =` novo no elemento no ar, ou seja, reload no
  meio da música. O operador ouviu isso como corte seco a cada faixa.

- **UI**: o botão tem **dois motivos distintos** de desabilitação e o `title` diz qual
  ("não há faixa local no ar" ≠ "instrumental ainda não preparado", este último com o
  caminho: menu da faixa → 🎼 Preparar instrumental). O modo **não se desarma sozinho**
  quando entra uma faixa sem preparo — vale pra próxima que tiver —, **mas o botão
  continua clicável enquanto ligado**: aceso e desabilitado prenderia o operador no
  modo. O selo `INSTRUMENTAL` aparece **só quando o arquivo separado está de fato no
  ar**, e agora significa exatamente uma coisa.
  **O deck engatilhado nunca anuncia o selo**: o preparo da próxima não é consultado
  (o hook só pergunta pela faixa no ar), e chutar ali seria anunciar um corte que pode
  não existir.

## Separação de vocal por IA — o instrumental preparado (0.2.31)

É o **único** caminho do modo VOZ desde a 0.2.32 (o cancelamento de centro que
dividia o botão foi aposentado — 2,0 dB de redução em música real; ver a seção
acima). **Pré-render por faixa**, nunca botão ao vivo: o custo medido abaixo é o
que proíbe qualquer versão instantânea disso.

`Menu da faixa → 🎼 Preparar instrumental (IA)` → roda em segundo plano → o VOZ
passa a usar o arquivo pronto.

- **`demucs.cpp` (MIT), NÃO cgo — SHELL-OUT.** É C++17 + Eigen header-only, sem
  PyTorch, então cgo funcionaria; mas passaria a exigir toolchain C++ pra compilar
  o servidor Go **inclusive na imagem de deploy, onde o recurso nem roda**. Shell-out
  mantém o binário puro e o recurso opt-in.
- **OPT-IN E LOCAL.** `scripts/build-demucs.sh` clona, compila e baixa os pesos
  (`htdemucs` 4-source, **80 MB**) pra `~/.autodj/{bin,models}`. Sem eles,
  `StemSeparator.Available()` é false, **as rotas nem são registradas** e a UI
  esconde o item — mesma degradação graciosa do analisador sem `aubio`.
  Env: `AUTODJ_DEMUCS_BIN` / `AUTODJ_DEMUCS_MODEL`.
  ⚠️ O `CMakeLists` do upstream declara compat com CMake <3.5, removido no CMake
  4.x — daí o `-DCMAKE_POLICY_VERSION_MINIMUM=3.5` no script.
- **⚠️ PROD NÃO TEM — tentado na 0.2.35, REVERTIDO na 0.2.36 pela CONTA.**
  Funcionou tecnicamente: a imagem compilava o `demucs.cpp`, a VM foi escalada e uma
  separação chegou a rodar em produção. O que derrubou a ideia foi o preço.

  | medição (M1 Pro) | pico de RAM | tempo |
  |---|---|---|
  | 30 s de áudio, 2 threads | 5,87 GB | 53 s |
  | 30 s de áudio, 4 threads | **6,98 GB** | 47 s |
  | 150 s de áudio, 2 threads | **5,91 GB** | 281 s (1,9× o real) |

  Duas conclusões que continuam valendo: **a memória NÃO cresce com a duração** (é
  por segmento — 150 s custa o mesmo que 30 s) e **cresce com o número de threads**.
  Logo o mínimo viável é `performance-4x` (8 GB), e aí a fatura do Fly sai de
  **US$ 7 para ~US$ 125/mês** (17×) para rodar **mais devagar** que na máquina do
  operador — 8 a 15 min por faixa contra ~6 min.

  ⚠️ **NÃO BASTA ENCOLHER A VM DEIXANDO O BINÁRIO NA IMAGEM.** Com o `demucs`
  presente, `Available()` responde true, as rotas sobem, a UI oferece a ação — e o
  primeiro clique estoura a memória, matando a **máquina** (não só o recurso) e
  tirando a estação do ar. Por isso o estágio de build saiu junto com o `[[vm]]`:
  os dois voltam juntos ou não voltam. Ficaram **comentados**, não apagados, com o
  passo a passo no próprio `Dockerfile`.

  **Alternativas em aberto** (nenhuma implementada): preparar na máquina do operador
  e enviar o instrumental como faixa comum (custo zero, funciona hoje); escalar sob
  demanda, já que o Fly cobra por segundo (~US$ 0,17/h — um lote de 20 faixas sai
  por menos de US$ 1, ao custo de reiniciar a máquina); rodar o mesmo `demucs.cpp`
  **em WASM no browser** (é o que o [freemusicdemixer](https://github.com/sevagh/free-music-demixer)
  faz — servidor não gasta nada, ~81 MB de modelo por visitante); ou API paga por
  uso (~US$ 0,04–0,11/min, com o áudio saindo para terceiros).

- **CUSTO MEDIDO (M1 Pro, 6 threads): 45s de áudio em 64,8s** — ~1,4× o tempo
  real, ou ~6 min para uma música de 4. É a razão de nada disto encostar no
  caminho do playout.
- **UM TRABALHADOR SÓ**, e teto de **6 threads** no modelo. Ele já usa vários
  núcleos por dentro; dois trabalhos em paralelo não terminariam antes, só
  disputariam CPU com o playout, que roda ao mesmo tempo.
- **SOMA drums+bass+other, não subtrai o vocal do original.** Subtrair
  reintroduziria, com fase invertida, o resíduo que o modelo NÃO separou — o
  defeito sai do lugar errado e vira artefato audível. `normalize=0` no `amix` é
  obrigatório: com a normalização ligada o ffmpeg divide pelo nº de entradas e o
  instrumental sairia ~10 dB abaixo ("sumiu tudo" ≠ "sumiu a voz").
- **`?stem=instrumental` na rota de áudio que já existe**, não uma rota nova: o
  `<audio>` do deck já negocia Range/seek com ela, o Service Worker já a cacheia e
  o buffer anti-queda já a conhece. Sem o preparo, o servidor devolve o original
  em silêncio — nunca 404 no meio da música.
- **Coluna `instrumental_path`** (`ensureInstrumentalColumn` + `migrations/019`).
  Perder esse caminho num Upsert custaria MINUTOS de CPU por gravação.
- **⚠️ O ESTADO VEM DO ENDPOINT, NUNCA DO `Playable`.** O Playable é um retrato do
  instante do enfileiramento e a separação termina minutos depois — o retrato
  diria "não" para sempre. O teste ponta a ponta pegou exatamente isso
  (`instrumental_ready: false` com o arquivo pronto), e o campo foi **removido**
  do Playable pra não deixar a mentira latente. Ficou no `trackDTO` (lido fresco)
  e no `GET /library/tracks/{id}/instrumental`.
- **PREPARO AUTOMÁTICO (0.2.33)** — ⚙ Configurações → "Preparar instrumental
  automaticamente" (`mixing.Settings.AutoInstrumental`, **desligado por padrão**).
  Ligado, toda faixa local NOVA entra na fila da separação assim que chega ao
  acervo, sem o operador pedir uma a uma.
  - **Desligado por padrão não é rigor, é a conta**: ~1,4× o tempo real por faixa,
    num trabalhador só. Um envio de 80 músicas = **~5 h de CPU** moendo em segundo
    plano, disputando com o playout. Ligar é decisão consciente, e a UI diz isso
    embaixo do interruptor quando ele está marcado.
  - Entra pelo mesmo ponto do enfileiramento de análise (`enqueueAnalysis`), via
    porta própria **`StemQueue`** (só `Prepare`) + o `MixSettingsRepo` — mesma
    gramática do `AnalysisEnqueuer`/`CatalogWatcher`. Quem importa faixa não tem
    de saber de estado, fila ou erro da separação.
  - **Best-effort: falha NUNCA derruba o import.** Fila cheia (o canal tem teto de
    128) ou ferramenta ausente vira log, e a faixa segue tocando — trocar o
    essencial (a música entrou) pelo acessório (o instrumental) seria pior.
  - `Prepare` já é idempotente, então re-importar o mesmo arquivo não repete os
    minutos de CPU.
  - **Em produção não faz nada**: sem `demucs`, `stemSvc.Available()` é false e a
    porta nem é ligada. O interruptor existe, e não tem efeito lá.

- **`GET /library/instrumentals` — o que está em andamento AGORA, para a GRADE.**
  Uma requisição para a lista inteira, não uma por linha (o acervo tem dezenas).
  Devolve só `queued`/`running`: `ready` já viaja no `trackDTO` de cada faixa (o
  arquivo é a verdade e não muda sozinho), e `idle`/`failed` não desenham nada.
  **Mapa vazio é o sinal de parada do poll** do cliente (`useStemWork`, 4 s) — sem
  trabalho, zero requisição. Fica **dentro do mesmo gate** de disponibilidade: sem
  a ferramenta a rota não existe (404) e a UI não desenha barra, em vez de
  responder `{}` fingindo um recurso que não está lá.

- **A FILA também mostra BPM e SEM-VOZ (0.2.34)** — e ela NÃO é montada de
  `trackDTO`, o que forçou duas mudanças:
  - **BPM entrou no `Playable`** (e nos dois DTOs). Pode viajar ali porque é
    **estático**: uma vez medido não muda, então o retrato do enfileiramento
    continua verdade.
  - **`GET /library/instrumentals` passou a devolver também as PRONTAS.** A
    primeira versão devolvia só trabalho vivo, argumentando que `ready` já ia no
    `trackDTO` — verdade para a GRADE, falso para a FILA, que vem do `Playable` e
    não pode carregar o estado do preparo (ele termina minutos depois). As prontas
    saem do REPO, não da memória: o estado em memória se perde no restart, e o
    arquivo é a verdade.

- **⚠️ `useStemWork` É STORE ÚNICO DE MÓDULO, não estado por componente (0.2.34)** —
  e isto foi bug real. Com um `useState` por chamador, o clique em PROCESSAR no
  transporte acendia a barra LÁ e a grade continuava sem saber: a instância dela já
  tinha encerrado o ciclo (mapa vazio encerra o poll) e ninguém a acordava. Dois
  relógios contando a mesma coisa, um parado. Hoje é UM mapa, UM timer e N
  assinantes (`useSyncExternalStore`), com notificação só quando o conteúdo MUDA —
  a resposta costuma ser idêntica a cada 4 s, e re-renderizar a grade à toa pisca a
  tela.

- **CONFIRMAÇÃO DE APERTO, 3 s, em todos os botões do transporte (0.2.34).** O
  `:active` do CSS só dura enquanto o dedo está no botão; num playout o efeito da
  ação (a fila andou, o áudio trocou) nem sempre é visível na hora, e sem retorno o
  operador clica de novo achando que não pegou. É **anel expandindo**, não fundo
  cheio — fundo cheio é o que marca LIGADO no SEM-VOZ, e misturar as linguagens
  confundiria os estados. `pointer-events: none` é inviolável (não pode roubar o
  clique seguinte).
  ⚠️ **`.tbtn-pressed:disabled { opacity: 1 }`**: PRÓXIMA e RETROCEDER ficam `busy`
  logo após o clique — exatamente durante o efeito — e a regra genérica
  `.tbtn:disabled` o apagava justo no momento em que ele mais importa.
  ⚠️ **A respiração verde é só do `.tbtn-voice`**, não de todo `.tbtn.active`: o
  PAUSAR usa `.active` para dizer "pausado" (virando RETOMAR), e a regra genérica o
  punha a piscar junto — ligar o SEM-VOZ acendia dois botões animados, sugerindo
  que o RETOMAR estava fazendo algo.

- **A GRADE mostra três coisas (0.2.33)**: pílula verde **`🎼 INSTRUMENTAL`** nas
  faixas já preparadas (verde igual ao do botão ligado — repetir a cor encurta a
  associação; nasceu como emoji de 11px e o operador não achou na tela), **barra
  indeterminada abaixo do nome** enquanto separa, e a coluna **BPM** (só local).
  - A barra é **indeterminada de propósito**: o modelo até imprime `(N%)`, mas por
    thread, em segmentos diferentes — não vira um total confiável, e barra que
    finge saber a porcentagem mente justo quando trava.
  - **Fila ≠ rodando** têm desenho diferente (parada e apagada = "sua vez não
    chegou"): o worker é UM só, e sem essa distinção três faixas enfileiradas
    pareceriam três separações simultâneas.

- **O BOTÃO TEM TRÊS ESTADOS (0.2.34)**, e é isso que tira o beco sem saída:
  preparada → liga/desliga · sem preparo → **`✨ PROCESSAR SEM-VOZ (IA)`**, e o
  clique enfileira · processando → **barra no próprio botão**, sem clique. Antes,
  faixa sem preparo deixava o botão apagado e ponto: o operador via a função
  existir e não tinha como alcançá-la dali — a saída morava num menu de contexto
  em outra tela.

- **`PrepareAll` — ligar a opção vale pro ACERVO INTEIRO (0.2.34)**, não só pra
  faixa nova. Sem isso, marcar o interruptor com 83 músicas na biblioteca não
  produziria nada visível e o operador concluiria que não funciona. Dispara **só
  na transição desligado→ligado** (`MixService.Save` compara com o valor
  anterior): salvar o ⚙ de novo com a opção já marcada percorreria o acervo à toa
  — `Prepare` é idempotente, então não haveria trabalho repetido, mas haveria uma
  varredura por clique. Roda em goroutine (quem espera é uma requisição HTTP) e
  para quando a fila enche, registrando o que sobrou.

- **⚠️ O `pause` DA TROCA NÃO É UMA PAUSA DA ESTAÇÃO (0.2.34).** Na troca de fonte
  o elemento que SAI é pausado depois da rampa — e naquele instante ele ainda é o
  "ao vivo", então o evento chegava ao hook e marcava a estação como parada: o
  transporte trocava PAUSAR por **RETOMAR** no meio de uma música que seguia
  tocando pelo outro elemento. `swappingRef` (ref, não estado — isto acontece
  dentro de uma sequência assíncrona e um estado chegaria tarde) engole o evento
  só nessa janela, e é solto em TODOS os caminhos de saída, inclusive quando o
  browser recusa o `play()`: preso, ele engoliria uma pausa real depois.

- **O botão VOZ liga por UM caminho só**: instrumental pronto
  (`Track.HasInstrumental()`). Sem preparo o botão fica apagado com o caminho
  escrito no `title` — não existe mais um plano B que quase não faz nada.
- **O buffer anti-queda é IGNORADO com o modo ligado**: o Blob guardado é sempre
  do original, e preferi-lo tocaria a versão COM voz.
- **QUALIDADE MEDIDA EM MÚSICA REAL (0.2.32)**, com o próprio Demucs como régua
  ("quanta voz sobra em cada saída"), num trecho de 40s de *Ace Of Base — All That
  She Wants*: original **−17,3 dB**, cancelamento de centro **−19,3 dB** (queda de
  2,0), instrumental da IA **−49,2 dB** (queda de **31,9**). É esta medição que
  aposentou o DSP. ⚠️ O sintético anterior (senoides) NÃO servia: o Demucs foi
  treinado em música e não reconhece um seno de 1400 Hz como voz — as bandas caíam
  uniformemente ~3 dB, que é mudança de nível, não separação.

## Separação na nuvem — GPU por segundo (0.2.37)

Segundo adapter da MESMA porta `outbound.StemSeparator`. Local usa o `demucs.cpp`;
sem ele (ou com `AUTODJ_STEMSEP=cloud`), o AutoDJ aluga uma GPU no RunPod.

**MEDIDO ponta a ponta**: faixa de 45 s pronta em **78 s** contando subir o pod e
instalar o Demucs. No spike isolado, **~10 s de GPU** para 3 min de áudio (0,06× o
tempo real, contra 1,9× num M1 Pro). O acervo de 83 faixas sai por **menos de
US$ 0,10, uma vez** — a alternativa de VM dedicada custava **US$ 124/mês** (0.2.35,
revertida).

- **⚠️ O POD É UM SERVIÇO, NÃO UM JOB — é a decisão que simplifica tudo.** Como job
  (baixa a faixa, devolve por callback) seria preciso rota nova no AutoDJ, token de
  escopo para o pod se autenticar e mudança na porta. Como serviço, o AutoDJ manda
  os bytes e recebe os bytes na mesma requisição: **`SeparateVocals(path, outPath)`
  fica idêntica e nada novo é exposto na internet**. E o modelo carrega UMA vez por
  lote — num job por faixa, cada uma pagaria os ~3 min de subida, que custam mais
  que a separação em si.

- **⚠️ WATCHDOG NO AUTODJ, porque a auto-terminação NÃO é confiável.** Medido no
  spike: o pod terminou o trabalho, subiu o resultado e o RunPod **reiniciou o
  container**, que refez tudo. E não dá para detectar de dentro — a mutation
  `podTerminate` responde `{"data":{"podTerminate":null}}` **mesmo quando
  funciona**. Logo: quem lança é quem cobra a morte. `Shutdown` insiste e confirma
  **por consulta** (o pod sumiu da listagem), e loga aviso explícito se não
  conseguir. O relógio de ociosidade é do AutoDJ, não do pod: só ele sabe se ainda há fila, e é
  o operador quem paga a GPU parada. **2 min** (era 8): o pod fica quente DE
  PROPÓSITO entre faixas — subir e instalar o Demucs custa ~3 min pagos, e matá-lo
  a cada faixa faria um lote de 10 pagar 10 subidas —, mas 8 min de GPU vazia a
  ~US$ 0,27/h davam **~US$ 0,036 por janela ociosa**, gasto observado na prática.
  2 min ainda cobrem o intervalo entre faixas de um lote em sequência (~10 s por
  faixa) e cortam o desperdício para ~US$ 0,009. O tick do watchdog é de 20 s: com
  tick igual ao teto, um pod ocioso sobrevivia até o DOBRO do previsto.

- **⚠️ "OCIOSO" SIGNIFICA "NINGUÉM ESTÁ TRABALHANDO" — e errar isso matou todo pod
  novo por 8 versões (0.2.40→0.2.45).** O relógio de ociosidade nasceu sem
  inicialização: `lastUse` só era escrito pelo `touch()` do `SeparateVocals`, que
  roda DEPOIS do `waitReady`, então durante os ~3 min de instalação do Demucs ele
  ficava zerado e `time.Since(zero)` dava **2562047h** — o watchdog matava o pod
  recém-nascido aos 20 s. Inicializar no lançamento só mudou a forma: aí ele
  morria em **2m0s exatos** (o teto), porque o relógio corria durante a
  instalação. Hoje o **`waitReady` marca atividade a cada sondagem**, e quem
  limita essa fase é o `readyTimeout` (8 min).
  ⚠️ **Ficou latente por sorte de cronômetro**: com tique de 2 min, a faixa medida
  no lote real ficou pronta em 78 s e o `touch()` chegava antes do primeiro tique.
  Baixar o tique para 20 s — mudança que parece inócua — reposicionou a primeira
  observação para dentro da instalação. **Mexeu em tique/teto do watchdog: lance
  um pod de verdade antes de dar por feito.** Dois testes travam as duas formas.

- **⚠️ LISTA DE FALLBACK DE GPU É REQUISITO.** No spike, TRÊS modelos seguidos
  responderam `SUPPLY_CONSTRAINT`, e o mesmo aconteceu no primeiro lote real (a
  A5000 caiu, entrou a A4000 sozinha). Ordem: mais barata → mais cara, `SECURE` →
  `COMMUNITY`. Erro que **não** é de capacidade (chave inválida, cota) aborta na
  hora, em vez de varrer a lista escondendo a causa.

- **⚠️ `mkdir -p /workspace` no bootstrap.** Com `volumeInGb: 0` o diretório não
  existe; escrever ali falha, o comando morre e o RunPod reinicia o container em
  loop (o "uptime caindo de 10 s para 3 s" do spike).

- **O pod devolve WAV, e isso é contrato.** A primeira versão devolvia mp3 (menos
  bytes), mas o caminho de saída é `.instrumental.wav`, escolhido pelo
  `StemService` — a extensão passava a mentir, e os dois adapters da mesma porta
  produziam formatos diferentes.

- **Preço vem da API, nunca da tabela do site**: lá são valores de **Serverless**
  (US$ 0,69/h); pod on-demand sai por **US$ 0,16–0,25/h**. E **"NVIDIA L4" não
  existe** no secure cloud — o pod nem sobe.

- **A API de logs do RunPod não serve para orquestrar**:
  `rest.runpod.io/v1/pods/{id}/logs` responde erro de spec. Quem precisa de
  resultado usa canal próprio (aqui, a resposta HTTP do serviço).

- **Env**: `AUTODJ_RUNPOD_API_KEY` (sem ela o recurso some), `AUTODJ_STEMSEP=cloud`
  (força nuvem mesmo com binário local), `AUTODJ_RUNPOD_GPUS`,
  `AUTODJ_RUNPOD_IMAGE`, `AUTODJ_RUNPOD_MIN_BALANCE`. Chave sempre do `pass`
  (ver `scripts/secrets.example.sh`), nunca em arquivo.

- **Lote**: `task stems:cloud` estima custo e pede confirmação antes de enfileirar.

## Biblioteca — Pastas e envio em lote (0.2.26)

- **Pastas do operador** (`domain/library/folder.go`, 100% coberto): `NormalizeFolder`
  (trim + colapso de espaço, cap `MaxFolderName=40`), `SameFolder` (comparação
  case/espaço-insensível), `NormalizeFolders` (dedupe, cap `MaxFoldersPerTrack=12`).
  `Track.Folders []string` normaliza no `Validate()`. Persistência: coluna `folders`
  (JSON) em `tracks` + tabela `folders(name, name_lower PK, created_at)`; migração
  idempotente `ensureFoldersColumn`. ⚠️ **O rebuild da tabela `tracks`
  (`ensureSourceCheck`) tem que copiar `folders` no `INSERT…SELECT`** — esquecer isso
  apaga a organização inteira de estações antigas na primeira abertura.
- **Três tipos, e a diferença é regra de domínio, não enfeite de UI**: pastas do
  operador (cria/apaga/atribui); **automáticas de fonte** (`Spotify`/`YouTube`,
  derivadas do `source` — `SourceFolder`; atribuir faixa a elas devolve
  `ErrFolderStreaming`); e **"Sem pasta"** (virtual, junta as locais não organizadas —
  existe pra ninguém perder faixa de vista). `app/folders.go Folders()` devolve a
  UNIÃO (catálogo + em uso + automáticas) com contagem. **Excluir pasta NUNCA apaga
  faixa** — elas voltam pra "Sem pasta".
- **Rotas**: `GET/POST/DELETE /library/folders`, `PUT /library/tracks/{id}/folders`.
- **Dois caminhos pra atribuir pasta, e os dois são obrigatórios**: no **envio**
  (chips do lote, `UploadModal`) e na **edição** (`TrackFormModal`, bloco "Pastas" +
  "nova pasta (Enter)"). Só o envio não bastava: quem já tinha acervo dentro do app
  ficava com tudo preso em "Sem pasta", sem caminho pra organizar sem reenviar os
  arquivos. O editor grava por **rota própria** (`PUT .../folders`) e só quando mudou
  — o update de metadados não carrega pastas. Faixa de streaming **não mostra o
  bloco** (a pasta dela é derivada da fonte e o servidor recusa a troca).
- **UI**: `FolderBrowser.tsx` (chips com contagem; logo real do serviço via
  `BrandIcon` nas automáticas — emoji não diz de qual serviço se trata e muda de
  desenho por SO). **A aba Músicas abre DIRETO nele, em "Todas"** — as sub-abas
  `Favoritas/Últimas Inseridas/Manuais` foram **removidas** (`OpSubTabs.tsx` deletado;
  `content.MusicSubtabs` segue no domínio/API, sem consumidor). Dentro de uma pasta o
  universo passa a ser a biblioteca INTEIRA (é o que faz a pasta "Spotify" mostrar as
  do Spotify); em "Todas" vale o recorte por fonte de sempre.
- **Envio em LOTE** (`UploadModal.tsx`): N arquivos por vez, **concorrência 2**, dedupe
  por nome+tamanho, metadados (categoria/intérprete/álbum/pastas) por LOTE — formulário
  por arquivo em 40 itens seria pior que o problema. Progresso REAL via XHR
  (`uploadTrackProgress`; `fetch` não expõe progresso de upload). Uma falha não derruba
  o lote: cada arquivo tem seu estado e seu motivo.
  **Progresso: NADA de barrinha por linha (0.2.26)** — com 80+ arquivos, um traço por
  linha vira uma cerca de tracinhos verdes (foi o que o operador reprovou). Ficou:
  medidor do LOTE no topo (sticky, com brilho correndo só enquanto sobe — parado vs.
  travado tem que ser visível de longe), **anel cônico** no disco de status por arquivo
  e preenchimento do fundo da linha que **some ao terminar**. A lista **segue** o item
  em voo (`scrollIntoView({block:"nearest"})` no primeiro `enviando`). `container-type`
  no lote (a largura que manda é a do modal, não a da tela) e opt-out em
  `prefers-reduced-motion`.
- **Erro de upload passa pelo humanizador** (`humanizeRaw`, `lib/errors.ts`): o XHR não
  tem `Response`, então antes o texto cru do backend (`"exit status 1"` do ffprobe)
  aparecia na lista de arquivos. Agora vira "Arquivo de áudio inválido ou corrompido".

## Sem tela de login (0.2.26) e capa sem explosão de processos

- **A tela de login FOI REMOVIDA** (`LoginScreen.tsx` deletado; `useAuth` virou só
  sonda — `login`/`logout`/`can` saíram, e `can()` não gateava um único botão). Uso
  caseiro é single-user por decisão de produto: a estação é isolada por **browser**
  (fingerprint), não por conta. O formulário só aparecia por ACIDENTE — o segredo
  JWT de dev é regerado a cada boot, então um restart invalidava o token guardado, a
  sonda `/auth/me` ia com ele, levava 401 e caía no login. **Fix na sonda**: 401 ⇒
  limpa o token e **re-sonda SEM ele**; se passar, é single-user e o console abre.
- **O BACKEND de auth continua inteiro de propósito** (JWT, argon2id, RBAC,
  `AUTODJ_SINGLE_USER=0`): produção é uma **URL pública** — hoje qualquer visitante
  usa o servidor e o volume. Esse backend é a única alavanca se um dia for preciso
  fechar o acesso; apagar é fácil, refazer é projeto. No lugar do formulário ficou
  `ServerLoginRequired.tsx`, que só aparece se alguém subir em multiusuário.
- **Capa embutida: 1 ffmpeg POR FAIXA, não por requisição**
  (`httpapi/artworkcache.go`). O endpoint rodava um `ffmpeg` a cada request; com 84
  faixas locais e recargas seguidas, a tabela de processos do macOS estourou e a
  máquina parou de fazer fork (aconteceu em 27/07/2026, durante os testes).
  **Reusar o mesmo processo é impossível** — o `ffmpeg` CLI é one-shot por arquivo,
  não tem modo servidor. O que resolve é cortar o NÚMERO de execuções:
  - **cache positivo** (bytes em memória, orçamento 32 MB) e **negativo** — este é o
    que mais importa: acervo de MP3 baixado é quase todo **sem** capa, sai 404, e
    404 não fica no cache do browser, então repetia em toda carga, pra sempre;
  - **single-flight** (deck ao vivo + deck engatilhado + linha da lista pedem a mesma
    capa quase junto → uma extração);
  - **semáforo de 2** (a VM de prod é `shared-cpu-1x`);
  - chave = caminho + tamanho + mtime, então reeditar a tag invalida sozinho;
  - o 404 passa a ir com `Cache-Control: max-age=3600` (1h, não 1 dia: a faixa pode
    ganhar capa depois).
  Medido: 40 capas frias 3441ms → 430ms nas rodadas seguintes; pico de **2**
  processos com 40 pedidos paralelos (antes seriam 40).

## Frontend — Modo, Tema e Limpar

- **Modo do player (`web/app/lib/mode.ts`)**: toggle client-side `radio | tocador`,
  **default `tocador`** (0.1.8 — uso caseiro é tocador puro; `radio` só quando o
  operador liga; `read()`/`getServerSnapshot` retornam `tocador`, só `"radio"` explícito
  no localStorage muda) (localStorage `autodj.mode.v1`, `useSyncExternalStore`, carimba
  `<html data-mode>`).
  Escolhido no modal **Modo** (ex-"Layout", botão `⚙ Configurações` no header —
  renomeado de `🎛 Modo` em 0.1.17, `UserBadge`), abaixo do Tema. O mesmo modal
  (`LayoutEditor`) tem o fieldset **Modo Tocador → Fade nas transições** (0.1.18,
  liga/desliga, `@/lib/fade`) — ver "Fade nas transições" no Transport acima. **Tocador** = tocador puro de música: `OperacionalScreen` esconde as funções
  de estação (Relógios/Grade/Eventos/RDS/Line-in/ECAD/Comprovante/Pedidos/Site
  público/Redes sociais/Escala) e filtra os chips de categoria de estação, deixando
  só `PLAYER_CATS` (instantâneas/músicas/spotify/youtube) + Adicionar + **Locutor com IA**
  (0.2.6, ex-"AI DJ", segue desabilitado — visível-porém-off desde a 0.2.14, junto
  com o chip Instantâneas). **Mixagem/Tocadas (barra) e FALAR (Transport)
  só no modo Rádio** (0.2.6): `PlayerScreen` passa `showTalk={isRadio}` ao Transport e as
  toolbars Mixagem/Tocadas ficam gated por `isRadio` — no Tocador não aparecem. Faixas
  YouTube importadas nunca entram na aba **Músicas** (só locais/MP3). A tab **Internet**
  do `RightPanel` também é função de estação: some no Tocador (só Operacional). Não é
  coluna no banco — uso caseiro, é UI.
  - **Timers de estação (`Timers.tsx`: ⏱/＋/－/RDS) só no modo Rádio**: conceito de
    playout de estação. `PlayerScreen` lê `useMode()==="radio"` → `NowPlayingHeader
    showTimers`; no Tocador o header mostra só o relógio de parede.
  - **Título do painel e placeholder da busca mudam por modo**: aba do `RightPanel` =
    **🎛 Operacional** (rádio) / **🎧 Meu Som** (tocador). `OpSearch` placeholder: rádio
    = "Buscar música, vinheta, comercial, artista…"; tocador = "Buscar música, artista,
    álbum…" (sem vinheta/comercial, que são de estação).
  - **Relógio de parede (`ClockBlock`) para de piscar os `:` quando algo toca**: prop
    `steady` (= `hasNow` em `PlayerScreen`) → classe `.clock.steady b { animation:none }`.
    Ocioso pisca (segundos correndo); tocando fica fixo (menos ruído).
- **Versão + changelog no titlebar (`OperatorTitlebar`)**: ao lado do logo `▲ AutoDJ`,
  um crachá com a **versão resolvida por `scripts/app-version.sh`** (0.2.17;
  `web/app/lib/version.ts` → `APP_VERSION` = `NEXT_PUBLIC_APP_VERSION` injetado no
  build, fallback `v0.0.1`). Release (HEAD na tag + árvore limpa) mostra `v0.2.16`;
  qualquer build fora disso mostra `v0.2.17-dev.<sha>[.dirty]` — **`-dev` = não é
  produção**.
  Clicar no crachá (`.brand-ver`, vira `<button>`) abre o **`ChangelogModal`**
  (`.mix-modal.cl-modal`): linha do tempo das versões (`web/app/lib/changelog.ts`,
  mais recente destacada) com destaques por versão + link do autor
  `andrebassi.com.br` no rodapé. Ver regra CHANGELOG A CADA VERSÃO acima —
  `changelog.ts` é a fonte e o gate de build cobra entrada por git tag.
- **Popup de detalhes da faixa (`TrackDetailModal`)**: **clicar na capa EM QUALQUER
  lugar** — lista de Mídias (`OpMediaList`), **decks A/B, fila e Tocadas** (0.1.17) —
  abre `.mix-modal.td-modal` com capa ampliada (120px) + metadata (plataforma,
  intérprete, álbum, categoria, duração, BPM, tom Camelot, mixável, análise,
  marcadores, tocadas, id) + deep link "Abrir no Spotify" (`spotify:track:` →
  `open.spotify.com`). Puramente informativo, não toca em fila/broadcast; campos
  ausentes são omitidos. DTO REST (`dto.go`) ganhou `spotify_uri`/`play_count`;
  `OpMedia`/`toMedia` propagam.
  - **Provider-agnóstico via adapter único** (`web/app/lib/operational.ts`
    `playableToMedia(p: Playable): OpMedia`): converte qualquer `Playable`/`PlayedItem`
    da agenda em `OpMedia` p/ o modal. Honesto — campos só-de-catálogo
    (BPM/tom/mixável/análise/marcadores) ficam ausentes no playout e o modal os omite
    (não renderiza "—" de ruído). `OpMedia.artworkUrl` carrega a URL de capa direta
    (CDN Spotify) → o modal renderiza a capa sem depender do endpoint `/artwork`
    sessão-scoped.
  - **`TrackCover.onClick`** (opcional): clicar na capa dispara o callback
    (`stopPropagation` p/ não acionar a linha). `cursor:zoom-in` + hover só quando
    clicável. **Fluxo do callback**: `ProgramList` é o dono do estado (`useState<OpMedia
    | null>`), passa `onOpenDetail={setDetail}` p/ `DeckPair` (decks) e `PlayoutTabs`
    (fila+tocadas); cada `TrackCover` monta o `OpMedia` via `playableToMedia` do seu
    item e chama `onOpenDetail`. `page.tsx` **não** é tocado — `ProgramList` já embrulha
    os três contextos (deck/fila/tocadas). `DeckPair` passa `onCoverClick` via `RoleProps`
    (spread `{...aRole}`/`{...bRole}` → `Deck`); deck ao vivo usa `np`, deck cued usa
    `queue[1].playable`.
- **`RightPanel` tabs = Operacional | Internet** (a tab "DJ"/`DjDeckPair` foi
  removida da barra; deck A/B continua embutido no player à esquerda). `PanelMode`
  = `"operacional" | "internet"`. Sem subtítulo roxo "Operacional" duplicado.
- **Fila (`QueueRow`) = cor uniforme das "mais tocadas"** (`--surface-2`, não a
  listra colorida por `k-*`). O 🟢 Spotify / 🎵 Local já marca a fonte; regra em
  `.rows .row.q` vence a `k-music` por ordem no arquivo. Vale claro e escuro (token).
- **Numeração ABSOLUTA da sequência de execução** (mudança 2026-07-26): cada linha
  mostra sua posição REAL na sequência, não mais só 1 e 2 nos decks. Base de
  deslocamento **`posBase = history.length`** (nº de faixas já tocadas), computado em
  `ProgramList` e propagado. Fórmulas: **deck ao vivo = `posBase+1`**, **cued (Deck B) =
  `posBase+2`**, **cauda[i] = `posBase+3+i`**, **tocada[i] (aba Tocadas) = `i+1`**
  (histórico é mais-antigo-primeiro). `DeckPair.nowRole.pos = live ? posBase+1`,
  `nextRole.pos = hasNext ? posBase+2`; `PlayoutTabs` passa `pos={posBase+i+3}` à
  `QueueRow` e `pos={i+1}` à `HistoryRow` (que renderiza com `.row.q`, visual idêntico
  à `QueueRow`).
  Nos decks A/B a pílula (`.deck-pos`) fica **FORA do deck, no canto superior-DIREITO
  embutido** (`position:absolute; top:-11px; right:6px` no `.deck` que é
  `position:relative` — movida da esquerda p/ não ser cortada pela borda do painel; ring
  escuro fixo p/ recortar da borda do `.deckpair` slate). Gradiente da cor do deck
  (`.deck-a` verde, `.deck-b` azul). **A numeração deriva de índice + posBase, não do
  item** → tocar/reordenar/remover re-renumera sozinho, sem estado extra.
  Provider-agnóstico (Spotify/local iguais).
- **Reordenar a fila = ARRASTAR E SOLTAR (drag-and-drop) em `QueueRow`**: operador
  move a ordem de execução pegando pelo grip `⠿` (`.r-grip`, aparece no hover) e
  soltando na posição desejada; indicador de borda (`.drop-up`/`.drop-down`) mostra
  onde cai. As **setinhas ▲/▼ foram removidas** (davam 404 quando o server estava
  velho + UX ruim). HTML5 DnD com type próprio `text/autodj-reorder` p/ **não
  colidir** com o drop de mídia da biblioteca (`text/autodj-track` /
  `-spotify-uri`). Fluxo: `QueueRow.onReorder(from,to)` (índices VISÍVEIS) →
  `ProgramList` converte p/ mirror (+2) → `page.tsx` → `moveSafe` →
  `useNowPlaying().move` → **`POST /playback/move {from,to}`**. Só mexe na **cauda
  visível** (`queue.slice(2)`, mirror índices ≥2): decks A/B (0,1 = no-ar +
  engatilhada) são intocáveis. Domínio:
  `PlayQueue.MoveAt(from,to)` (clamp, `from==to` no-op, índice 0 = no-ar off-limits).
  App `PlaybackService.Move` espelha na fila e chama `engine.MoveAt(from-1,to-1)`
  (engine exclui o current → índice engine = mirror−1). Stub engine faz o move real
  no pending; o mirror é fonte de verdade da UI e o browser reflete a nova ordem.
  Reordenar **não** reavalia transição (regra de entrada já satisfeita no enqueue) —
  invariante `spotify ⇒ mixable=false` intocado.
- **Tema dark REAL (`web/app/globals.css`)**: as superfícies leem tokens semânticos
  `--surface/-2/-3`, `--txt/-2/-3`, `--bd/-2` e famílias de acento `--ac-{green,warn,
  danger,purple,blue}-{bg,tx}` — valor **light = literal original** (claro fica
  idêntico). Os blocos `:root[data-theme="escuro"]` e `@media (prefers-color-scheme:
  dark) :root[data-theme="auto"]` remapeiam TODOS esses tokens + os `--ant-*` dos
  modais para uma paleta slate → dark inverte a UI inteira. Nunca hardcodar `#fff`/
  cor de texto escura em superfície nova; usar os tokens. **Default do domínio = CLARO**
  (`operatorprofile.Default().Theme = ThemeLight`; fallback frontend `useApplyProfile`
  `?? "claro"`) — instalação limpa e pós-Limpar abrem no tema claro.
- **Limpar (header) = começar do zero de verdade**: `POST /playback/clear` (zera
  fila + para no-ar) **E** `POST /library/clear` (apaga a biblioteca de mídias inteira)
  **E** `spotifyDisconnect()` (desconecta integrações, idempotente; 404 sem client id
  → ignora) **E** `usePlayHistory().clear()` (histórico local `autodj.playhistory.v1`).
  Confirmação via `confirmDialog`. No fim faz `window.location.reload()` porque as
  instâncias de `useLibrary`/`useSpotify` vivem soltas em componentes separados (sem
  estado compartilhado) — o reload garante tudo vazio de uma vez. Backend:
  `LibraryService.ClearLibrary` (porta inbound) → `TrackRepo.DeleteAll` (memory/sqlite/
  postgres, idempotente) + best-effort `MediaStore.Remove` de cada arquivo.
- **Chips de categoria enxutos**: `Efeitos`/`Mais Tocadas`/`Executadas` foram
  **removidos** do topo (`OperationalCatalog().Categories` no domínio + `OpCategoryKey`
  + `PLAYER_CATS`) — não agregavam ação. Categoria de mídia `effect` (dto.go) segue
  existindo p/ classificar upload; só o chip do topo saiu.
- **Cartwall — barra de páginas só com +de 1 página**: `Cartwall` esconde `.cw-tabs`
  quando há uma única página (o chip `Instantâneas` do topo já indica onde se está —
  evita o "Instantâneas 1" redundante).
- **Toolbar Operacional (`.opscreen-head`) em tom azul** (antes roxo) p/ casar com a
  paleta azul do app: gradiente `#cddcf1→#b6cbe8`, borda `#8fb0d6`, texto/pills via
  `--ac-blue-bg`/`--ac-blue-tx`.
- **Popup Histórico & Mais Tocadas (`PlayLogPanel`)**: usa `.mix-modal.pl-modal` —
  retângulo grande em % do viewport (`92vw`/`90vh`, max 1280px), responsivo;
  titlebar/footer fixos, corpo rola. O `.mix-modal` genérico segue compacto. A
  coluna **"irradiado/não irradiado" foi removida da UI** (irrelevante p/ uso
  caseiro); os campos `broadcasted`/`ecad_relevant` continuam no servidor p/ reports.

## Fora de escopo (cortes do usuário)

Controle de Contratos (comercial/CRM/financeiro), Módulo Gerencial, Módulos Por
Setor. Comerciais existem **só como categoria de mídia**.

## Comandos

```bash
nix develop           # dev shell
task up               # Postgres (Nix, opcional); persistência local = SQLite por browser
task dev              # API Go :8095 (memory sem env)
task serve            # server + web/out :8099, Client ID Spotify baqueado (survive restart)
                      # DB persistente por fingerprint desde a v0.2.18 (era memory);
                      # AUTODJ_DB_PATH=memory task serve → efêmero (smoke em estado limpo)
task web:dev          # Next.js Player :3000 (playout no browser)
task test / task cover
task lib:scan FOLDER=/path
task playback:load
task deploy           # Fly.io (Docker remoto)
```

## Deploy produção (Fly.io) — `autodj.andrebassi.com.br`

App Fly **`autodj`** (org `personal`, region `gru`). Deploy: `task deploy` (roda o
gate de changelog + `fly deploy --build-arg APP_VERSION=v<git-tag>`). Token sempre do
o token do gerenciador de senhas (nunca `fly auth login`) — ver `scripts/secrets.example.sh`.

- **VM**: `cpu_kind=performance`, `cpus=4`, `memory=8gb` (`[[vm]]` no `fly.toml`),
  desde a v0.2.35. **O tamanho é ditado pela separação por IA, não pelo playout** —
  o servidor sozinho rodava folgado em `shared-cpu-1x`/1 GB. Ver a seção da
  separação para as medições que fixaram 8 GB e `AUTODJ_DEMUCS_THREADS=2`.
  (Configuração anterior, se um dia a separação sair de prod: `shared`, 1, `1gb`.)
  Broadcast server-side saiu — o server só orquestra fila/now-playing (SQLite leve),
  então 1GB shared basta. shared limita 2GB/CPU; subir p/ `2gb` se scan/ffmpeg de
  artwork apertar. (Antes era `performance-1x`/`4gb`, dimensionado pro Liquidsoap.)
- **⚠️ O CONTAINER NÃO RODA MAIS COMO ROOT (revisão de segurança)** — usuário `app`
  (uid 10001) — mas **`HOME` e o volume continuam em `/root`**, e isso é deliberado.
  ⚠️ **A v0.2.42 moveu o volume para `/home/app/.autodj` e QUEBROU o áudio em prod**:
  `Track.Path` é gravado ABSOLUTO no banco, então toda faixa existente seguia
  apontando para `/root/.autodj/media/...`; sem root, abrir isso falha já na
  TRAVESSIA de `/root` (modo 700) e o `http.ServeFile` responde **403** — biblioteca
  listava, nada tocava. Corrigido na 0.2.43 voltando a montagem. Mover de verdade
  exigiria reescrever o caminho em todos os bancos de todas as estações.
  O `deploy/entrypoint.sh` começa root só para `chmod o+x /root` (atravessar, não
  listar) e `chown` o volume (uma vez; ele testa o dono antes, porque `chown -R` a
  cada boot custaria tempo dentro do grace period do health check), e então larga
  privilégio com `setpriv`. **Falha aberta de propósito**:
  sem `setpriv`/`su` na imagem ele segue como root COM AVISO no log, em vez de abortar
  e tirar a estação do ar. Se esse aviso aparecer, o ganho de segurança não aconteceu.
- **Health check**: `GET /api/v1/health` (rota real do chi router — `/api/v1/*`).
  ⚠️ NÃO usar `/api/health` (404 → check 0/1 → proxy `[PR01] no known healthy
  instances` → 503). Bug corrigido 2026-07-26.
- **Volume SQLite `autodj_data`** (10GB, gru) montado em **`/root/.autodj`** (`[mounts]`).
  Cobre TUDO da persistência local: `library.db` (tenant default), `sessions/<fp>.db`
  (1 por browser/fingerprint) e `media/<tenant>/` (uploads, isolados desde a 0.2.16 —
  o tenant default segue na raiz do `media/`). `HOME=/root` no runtime → o volume
  cai exatamente onde o código grava (`os.UserHomeDir()`). Sobrevive deploy/restart.
  **Não** usa Postgres (sem `DATABASE_URL`) — uso caseiro, 1 máquina.
- **Domínio**: CNAME `autodj` → `autodj.fly.dev` na zona `andrebassi.com.br`
  (Cloudflare; zone id e token no gerenciador de senhas do autor),
  **DNS-only (grey)** p/ o Fly emitir o cert LE. Cert via `fly certs add
  autodj.andrebassi.com.br`. IPs: v4 compartilhado da Fly + v6 dedicado (`fly ips list`).
- **`auto_stop_machines="off"` + `min_machines_running=1`**: 24/7, nunca idle-stop.
  **Custo** ≈ shared-cpu-1x@1GB rodando o mês inteiro (~US$4/mês) + volume 10GB
  (~US$1.50/mês) + v6 dedicado grátis + v4 shared grátis. Sem Postgres = sem custo de
  DB. **Total ~US$5–6/mês.** Ligar `auto_stop_machines` derruba mais ainda (quase zero
  em horas paradas) mas atrasa o 1º request após ociosidade (~2–5s cold start) —
  mantido off por ser uso caseiro constante.
- **Spotify em prod**: Client ID PKCE **setado como secret** (`AUTODJ_SPOTIFY_CLIENT_ID`,
  de `scripts/spotify-client-id.txt`) → `/spotify/status` responde 200, integração
  ativa. Rotar/atualizar: `fly secrets set AUTODJ_SPOTIFY_CLIENT_ID=<id> -a autodj`
  (secret sobrevive restart/deploy).
- **Redirect URI do OAuth (`AUTODJ_SPOTIFY_REDIRECT`)**: em dev o default é
  `http://127.0.0.1:8095/api/v1/spotify/callback` (só a máquina local). **Em prod TEM
  que setar** o env, senão o `connect` emite o loopback e o browser volta pra localhost
  (bug corrigido 2026-07-26):
  `fly secrets set AUTODJ_SPOTIFY_REDIRECT="https://autodj.andrebassi.com.br/api/v1/spotify/callback" -a autodj`.
  A **mesma URI exata** precisa estar cadastrada no Spotify Developer Dashboard →
  app → Redirect URIs (manter também a de dev), senão `INVALID_CLIENT: Invalid
  redirect URI`.

## Layout

`domain/{shared,library,playback}` (zero deps) · `ports/{inbound,outbound}` ·
`app/` · `adapters/driven/{library,ffprobe,audioanalyze,memory,sqlite,postgres,stubengine,spotify,youtube}` ·
`adapters/driving/{httpapi,ws}` · `cmd/{server,migrate}` · `web/` ·
`migrations/` · `flake.nix` · `deploy/` (remoto).

Playout: **roda no browser** (3 motores: Spotify Web Playback SDK / YouTube IFrame
Player API / `<audio>` local). O IFrame YouTube é a única divergência estrutural
(exige elemento DOM visível no deck 60×60, movido via `appendChild`).
O servidor é só o cérebro de fila/scheduling — mantém a agenda (`PlayQueue`) e o
now-playing via `stubengine` (mirror do playout), sem bus de mix. O broadcast por
servidor (Liquidsoap/Icecast, `.liq`, socket Unix, stream) foi **descontinuado**.

## Wiring (`cmd/server`)

Repo (precedência): `DATABASE_URL` → Postgres (remoto/escala, compartilhado); senão
**SQLite por browser** — 1 arquivo por fingerprint em `~/.autodj/sessions/<fp>.db`
(`modernc.org/sqlite`, sem daemon — sobrevive restart), resolvido pelo `tenantMux`
(ver "Estação por browser"). Tenant `default` (curl/testes/sem fingerprint) usa o
`~/.autodj/library.db` legado (sobe VAZIO — sem demo). `AUTODJ_DB_PATH=memory` → repo efêmero por tenant
(dev/teste). Engine: **sempre `stubengine`** (fila/now-playing; playout roda no
browser). `AUTODJ_LIB_ROOT` scan no boot (opcional). `WEB_DIR` serve `web/out`.
`PORT` default 8095.

**Cache do bundle estático (v0.2.18)**: `staticUI` envelopa os dois ramos (disco e
`StaticFS` do desktop) em `httpapi.StaticCacheControl` —
`/_next/static/*` (nome com hash de conteúdo) → `public, max-age=31536000, immutable`;
todo o resto, **`index.html` incluso** → `no-cache` (guarda mas revalida; 304 é barato).
**Por quê**: `http.FileServer` manda só `Last-Modified`, e sem `Cache-Control` o browser
cacheia o HTML por **heurística** (~10% da idade do arquivo). Como é o HTML que aponta pros
nomes hasheados, um HTML velho continua pedindo o chunk velho — deploy fica invisível mesmo
com servidor e assets novos no ar. Versionar por `?v=` NÃO resolve: a query string mora
dentro do HTML, então HTML velho carrega `?v=` velho; e a URL do documento (o domínio raiz)
não é versionável. Só header alcança o documento de entrada. Lógica pura e testada em
`adapters/driving/httpapi/staticcache.go` (o gate pula `cmd/*`).

**Toasts com Ant Design + queda de rede (v0.2.21)** — `antd` **6.5.2** entrou como dep
(React 19 nativo, sem patch de compat). Custo medido: **+35 kB** no First Load (185 → 220 kB)
usando `Alert`; com `message` seriam +54 kB.
**Por que `Alert` e não `message`/`notification`**: os dois renderizam pílula centrada em
portal próprio, com posição própria — brigariam com a âncora pedida (barra larga presa ao
rodapé da coluna esquerda). `Alert` traz o mesmo sistema visual sem impor layout.
**Posição**: `ToastHost` mora **dentro do `PlayerScreen`**, irmão do `<Transport>`, num
`.transport-wrap { position: relative }`; o host usa `bottom: 100%` → cola ACIMA do transporte,
largura inteira da coluna, **sobrepondo** o conteúdo (empurrar encolheria a fila a cada
mensagem — layout shift durante operação ao vivo é pior que a mensagem). Pilha em
`column-reverse` (mais recente perto do olho) e `pointer-events: none`, com `auto` só nos
Alerts, pra **nunca** roubar clique do transporte.
**ARMADILHA de classe — antd v6 ≠ v5**: o wrapper é `.ant-alert-section` e o texto é
`.ant-alert-title` (na v5 eram `.ant-alert-content` / `.ant-alert-message`). Mirar nas classes
da v5 **não gera erro** — só não centraliza, e o CSS morto passa despercebido. Centralizar exige
`justify-content: center` no root **e** `flex: 0 1 auto` no `.ant-alert-section` (o default é
`flex: 1 1 0%`, que estica e empurra o texto pra esquerda). O `X` de fechar vai
`position: absolute`, senão entra na conta da centralização e desloca o texto.
**Visual**: fundo **sólido** + texto branco (não o Alert pastel padrão) porque a barra fica no
rodapé, longe de onde o olho está; cores da paleta antd. TTLs longos (erro 14s, aviso 11s,
info 8s, sucesso 7s) pelo mesmo motivo.
**Toast fixo** (`toast.sticky(kind, texto, key)`, ttl 0): para **estado**, não evento — some só
por `dismissToast(key)`. Repetir a mesma `key` substitui em vez de empilhar (reconexões
sucessivas do WS não podem virar pilha de avisos idênticos). Fixo não é fechável: sumiria sem o
problema ter sido resolvido.
**`useConnectivity(wsConnected)`** — sinal principal é o **WebSocket**, não `navigator.onLine`:
o app é 100% online contra ESTE servidor, e `onLine` dá `true` em portal cativo, Wi-Fi sem saída
e servidor fora do ar. `onLine === false` entra só como gatilho rápido (evita esperar o timeout
do socket). Queda só vira aviso depois de **2,5s** (`GRACE_MS`) — o WS reconecta a cada 1,5s e
blip de troca de rede não pode acender alarme. Na volta, **reload** após 1,5s (o operador pediu:
em queda longa o token do Spotify expira, o IFrame do YouTube fica órfão e a fila diverge do
servidor; recarregar é o caminho curto pro áudio voltar) — **mas só se a queda passou do
GRACE_MS**, porque reload corta o que estiver tocando. `everConnected` evita alarme no boot
antes da primeira conexão.
**Duas armadilhas de efeito, as duas achadas SIMULANDO — nenhuma aparece no compilador:**
(a) os listeners `online`/`offline` precisam re-rodar o efeito **inteiro** (`netTick` no state
+ nas deps), não só acender o aviso: com o WS vivo e só a internet oscilando, o toast acendia e
**nunca apagava**, porque a dispensa mora no corpo do efeito e ele só reagia a `wsConnected`.
(b) o timer do reload **não pode viver no escopo do efeito** — o `clearTimeout` do cleanup roda a
cada re-execução (e o `netTick` garante várias) e cancelava o reload agendado; mora num `ref`, e
só o `graceTimer` é limpo no cleanup (ele é reagendado descontando o tempo já decorrido). Antes
do `netTick` o bug (b) ficava escondido: o efeito re-rodava uma única vez e o reload passava por
sorte de timing.
**Como simular sem derrubar rede de verdade** (foi assim que os dois vieram à tona):
`Object.defineProperty(navigator,'onLine',{get:()=>false})` + `dispatchEvent(new Event('offline'))`,
e o inverso pra voltar. Pra confirmar o reload, gravar `Date.now()` em `sessionStorage` antes —
ele sobrevive ao reload, e `performance.now()` sozinho não distingue "não recarregou" de
"recarregou faz tempo" (foi assim que eu li errado o resultado na primeira vez).

**Checagem de versão em runtime (v0.2.20)** — fecha o limite que o cache não alcança
(aba já aberta nunca refaz a requisição do HTML). `httpapi.Version` virou **var** e o
Dockerfile injeta a tag: `-ldflags "-X autodj/adapters/driving/httpapi.Version=$APP_VERSION"`,
a MESMA `APP_VERSION` que vira `NEXT_PUBLIC_APP_VERSION` no bundle — servidor e bundle saem
do mesmo build, então divergir = servidor trocado embaixo da aba. `GET /api/v1/health` já
publicava o campo `version`; agora ele vale alguma coisa (era `const "0.1.0"`).
`useVersionCheck` consulta a cada 5 min **e a cada volta de foco** (é quando o operador
repara), compara via `shouldOfferReload` (`lib/version.ts`, lógica pura — o `web/` não tem
test runner) e acende o `UpdateBadge` no titlebar.
**⚠️ RECARREGA SOZINHO DESDE A 0.2.35 — MAS SÓ EM SILÊNCIO (0.2.41).** A regra (a) abaixo
foi revertida a pedido do operador (crachá de clique ficava dias sem ser notado), e a 0.2.41
recolocou o limite no lugar certo: **com som no ar o reload ESPERA**. Retomar de onde parou
(0.2.26) não é o mesmo que não interromper — são segundos de silêncio no meio da faixa, e o
crossfade da virada, a janela de mixagem e uma locução em curso não voltam do localStorage.
Detectou versão nova tocando ⇒ avisa na hora (o operador precisa saber) e ARMA o reload, que
dispara no primeiro silêncio. Estação parada ⇒ contagem de 5 s e reload, como na 0.2.35.
Adiar ≠ desistir: enquanto espera, um intervalo curto (`SILENCE_POLL_MS` 3 s) vigia o playout
— o heartbeat de 5 min desperdiçaria a janela se a estação parasse logo depois do deploy.
**"Tocando" vem de `isPlayingNow` (`lib/onAir.ts`), publicado por `page.tsx`, que cobre os
TRÊS motores.** ⚠️ **`isAudioAlive` NÃO serve aqui** — ela é registrada só pelo motor local
(de propósito: quem a lê é o reload por volta de rede, e com streaming no ar recarregar é o
certo), então lê-la aqui recarregaria por cima de uma faixa do Spotify. Duas sondas, duas
perguntas. `blocked` conta como parado: a estação quer tocar e não sai som, é a hora em que
recarregar não custa nada. O timer do reload mora em **ref** — `clearTimeout` no cleanup do
efeito cancelaria o reload já agendado (mesmo bug do reload por queda de rede, v0.2.21).
Validado no browser: tocando 00:39→00:44 sem reload; faixa acabou ⇒ contagem ⇒
`navigation.type === "reload"`.
**Regras que não podem regredir**: (a) ~~**nunca** recarregar sozinho~~ → **nunca recarregar
por cima de som no ar**; o badge segue como caminho de quem quer atualizar AGORA, confirmando
e dizendo o custo; (b) build sem ldflags reporta
`"dev"` e a checagem se **desliga** — fora do Docker o bundle e o binário são compilados em
momentos diferentes e divergiriam à toa; (c) comparação é por **igualdade**, não "maior que":
rollback deliberado (versão menor) também precisa oferecer reload; (d) uma vez detectado o
aviso não some sozinho (rolling deploy com 2 máquinas faria ele piscar). Armadilha: `apiBase()`
devolve só a ORIGEM — o `/api/v1` é do chamador (custou um teste e2e batendo em `/health`).

**Spotify Client ID (`AUTODJ_SPOTIFY_CLIENT_ID`, PKCE público, sem secret)**: some
no restart se passado só por env. `scripts/serve-local.sh` (task `serve`) baqueia o
valor de `scripts/spotify-client-id.txt` (gitignored) → sobrevive restart. Sem client
id, as rotas `/spotify/*` dão 404 e o front mostra o guia de setup; com ele,
`/spotify/status` responde 200 e o painel troca p/ "Conectar conta".

**YouTube local (`task serve` habilita junto do Spotify — end-to-end)**: o
`serve-local.sh` puxa as 3 vars do `pass` em runtime (caminhos em
`scripts/secrets.local.sh`, gitignorado; modelo em `secrets.example.sh`) → `AUTODJ_YOUTUBE_CLIENT_ID/_CLIENT_SECRET/_API_KEY`.
Precedência env > pass. **Desde a 0.2.17 o corte é por variável, não tudo-ou-nada**: sem
a **API key** ⇒ `/youtube/*` 404 (aba some, guia de setup); com a key mas sem client
id/secret ⇒ aba sobe, sub-aba "Colar links" funciona 100%, e só `connect`/`callback`
respondem **503**. **Secret nunca vai pra arquivo/binário** (só env, code-flow no servidor).
Redirect local emitido = `http://127.0.0.1:8099/api/v1/youtube/callback` (loopback,
Google aceita sem HTTPS) — **tem que estar cadastrado no OAuth client do Google Cloud**
+ o e-mail em **Test users** (app não-verificado só deixa test users logar). Com as 3
vars, `/youtube/status` responde 200 e o painel troca p/ "Conectar com Google". Prod
(Fly) usa `fly secrets set AUTODJ_YOUTUBE_*` + redirect
`https://autodj.andrebassi.com.br/api/v1/youtube/callback` cadastrado no MESMO client.

## Estação por browser (fingerprint → SQLite isolado)

Cada browser = **estação isolada** (biblioteca, fila, engine/now-playing, cartwall,
grade, marcadores, playlog — nada compartilhado). Chave = `visitorId` do
**FingerprintJS** (OSS, client-side), cacheado em `localStorage` (`autodj:sid`).

- **Backend dispatcher** (`cmd/serverkit/tenants.go`, `tenantMux`): `serverkit.Build`
  não monta mais uma stack única — o corpo antigo virou `buildTenant(ctx, dbPath)` e o
  dispatcher o chama **lazy + cacheado por fingerprint** (canal `ready` serializa
  cold-start do mesmo id; ids distintos sobem em paralelo). Toda stack sobe **VAZIA**
  (sem `seedDemo` — modo demo removido). **Zero mudança nos handlers** — os `rt.*` do
  router ficam intactos.
- **DB por tenant**: `~/.autodj/sessions/<fp>.db` (`AUTODJ_SESSIONS_DIR` override); o
  tenant `default` (curl/testes/primeira pintura, sem fingerprint) fica no legado
  `~/.autodj/library.db`. `AUTODJ_DB_PATH=memory` → efêmero por tenant, e aí o
  `sessions/` nem é criado. `id` saneado a `[a-z0-9]` (sem path traversal), cap 64.
  **`AUTODJ_DB_PATH` é MODO, não caminho** — `dbPathFor(id)` deriva o arquivo do id;
  a variável só escolhe entre arquivo e RAM. `serve-local.sh` forçava `memory` até a
  **v0.2.18**: como o script mata o server antigo a cada execução, todo restart pra pegar
  código novo apagava biblioteca, fila e conta ligada — e do lado do operador isso
  aparecia como "dou refresh e some tudo", já que o refresh era só quando ele percebia.
- **MÍDIA por tenant (0.2.16)**: `~/.autodj/media/<tenant>/` (`AUTODJ_MEDIA_DIR` =
  raiz). Até a 0.2.15 o multi-tenant existia só na METADE do estado — banco por
  fingerprint, mas `filestore.New(...)` global. Como `Save` sanitiza pro basename e
  colisão **sobrescreve in place**, e o id é `idFor(path) = sha1(path)`, duas estações
  enviando `intro.mp3` sobrescreviam o áudio uma da outra **e caíam no mesmo id**;
  `Remove` (guarda só contra sair do `Base`) deixava uma apagar arquivo que a outra
  referenciava. Hoje `filestore.Store` tem **DUAS fronteiras**: `Base` = dir de
  ESCRITA (`<root>/<tenant>`), `Root` = raiz do media = fronteira do delete;
  construtor `NewFor(base, tenant)` (tenant re-saneado no adapter, `[^a-z0-9]` —
  defensivo, vira componente de path). `buildTenant(ctx, tenantID, dbPath)` mapeia
  **`defaultTenant → ""` ⇒ layout legado** (Base = Root = raiz), então curl/testes/
  primeiro paint seguem gravando onde sempre gravaram.
  **`Remove` aceita DUAS coisas e só elas**: (a) sob o próprio `Base`; (b) **filho
  DIRETO da raiz** — acervo pré-isolamento, referenciado por path absoluto nas linhas
  antigas; sem essa exceção, apagar faixa velha deixaria órfão eterno. Subdiretório de
  OUTRO tenant e path fora da raiz: ignorados (o teste `TestRemoveOtherTenantIgnored`
  trava isso).
  **Zero migração, por construção**: nada é movido/renomeado, `Path` absoluto já
  gravado continua resolvendo, nenhum `sha1(path)` muda ⇒ nenhuma referência (fila,
  play_log, cartwall, grade) precisa ser reescrita. Só upload NOVO vai pro subdir.
  ⚠️ Se um dia mover os arquivos legados pra dentro de um subdir, **o id muda** —
  aí vira migração de dados de verdade.
- **ÁUDIO NÃO VAI PRO SQLITE — decisão registrada (0.2.16).** BLOB no banco foi
  avaliado e recusado. 4 dos 5 consumidores de `Track.Path` são **processos externos
  que exigem caminho**: `ffprobe` (metadados no upload, `library_service.go:150`),
  `ffmpeg` (capa embutida APIC, `router.go:artwork`), analyzer (BPM/refrão/waveform,
  `analysis_service.go:107`), engine de playout (`playback_service.go:229`). O quinto é
  `http.ServeFile`, que entrega **Range/seek, `Accept-Ranges`, `Content-Length` e
  modtime de graça** pro `<audio>` do deck — com blob isso é reimplementado na mão.
  Driver é `modernc.org/sqlite` (Go puro): **não expõe I/O incremental de blob via
  `database/sql`**, então `SELECT data` materializa a faixa INTEIRA em RAM (VM de
  **1 GB shared**, com deck ao vivo + pré-carga do ocioso + cartwall). BLOB em SQLite
  só ganha do filesystem **abaixo de ~100 KB**; MP3 3–10 MB e FLAC 30–50 MB ficam
  30–500× acima. Some WAL inflado, `VACUUM` exigindo 2× o tamanho do banco e delete
  que não devolve espaço. **Onde blob FAZ sentido**: waveform peaks já calculados,
  thumbnails, vinhetas TTS curtas — coisa pequena e sem processo externo.
- **Transporte do id**: header `X-AutoDJ-Session` no REST, `?sid=` no handshake WS
  (browser não põe header custom no upgrade WS). Sem id (curl/testes) → tenant
  **`default`** compartilhado no `~/.autodj/library.db` legado.
- **Frontend** (`web/app/lib/session.ts`): computa o `visitorId`, cacheia, e instala
  um wrapper global de `window.fetch` que injeta o header **só em `/api/*`** (Spotify
  SDK / CDN de fora NÃO recebem header → não quebra CORS). `<SessionBoot>` (em
  `layout.tsx`) resolve o id antes de renderizar filhos; `wsURL()` anexa `?sid=`.
  A popup **Modo do Player** (`LayoutEditor`) mostra o ID (`.mix-sid`, click-to-copy).
- **TODA estação sobe VAZIA (sem modo demo, v0.2.5)**: o modo demo foi **removido** —
  não existe mais `AUTODJ_DEMO`, nem seed de catálogo/clocks/grid/events, nem tenant
  `default` pré-populado. Todo tenant (default OU fingerprint real) = sem faixas e sem
  program-preview (`/ui/program` = `{above:[],below:[]}`). O operador constrói a estação
  do zero (scan de pastas, import Spotify/YouTube). Motivo: o tenant `default` semeado
  gerava confusão (browser caindo no `default` via bug de fingerprint via a biblioteca
  demo em vez da estação isolada). Os dados de seed (`DemoTracks`, `schedule.Default*`)
  seguem no domínio (testados) mas **nenhum wiring os chama**.
- **OAuth do Spotify roteado ao tenant certo** (senão "no account connected"): o
  callback `GET /spotify/callback` é **navegação top-level** (redirect do Spotify) — sem
  header/`?sid=` → cairia no `default`, longe de onde o `connect` (fetch) guardou o
  `pending` PKCE. Fix: o fingerprint viaja **dentro do `state`** do OAuth. `spotifyConnect`
  (`api.ts`) reescreve `state` → `<sid>~<state>` antes de navegar; o dispatcher
  (`sessionFromState` em `tenants.go`) extrai o `<sid>`, roteia, e **restaura o `state`
  cru** antes do handler validar. Delimiter `~` seguro (`state` é base64url, nunca tem
  `~`). Ao mexer em qualquer coisa de sessão/OAuth: preservar esse envelope.
- **Capa via `<img>` também precisa de `?sid=`**: `<img src>` não passa pelo wrapper de
  `fetch`, então `trackArtworkURL` (`api.ts`) anexa `?sid=` (`sidQuery()`). Sem isso a
  capa cai no tenant `default` (sem a faixa Spotify do browser) → 404 → placeholder `♪`.
  Regra geral: **todo asset carregado fora de `fetch`** (`<img>`, WS, navegação OAuth)
  leva o fingerprint na query/state, nunca no header.
