# AutoDJ — Erros conhecidos e aprendizados

Catálogo consolidado de **sintoma → causa-raiz → fix → como reconhecer de novo**. É o
primeiro lugar a olhar quando algo "não toca", "some do deck", "trava em 00:00" ou
"funciona local e não em prod" (ou o inverso).

Regra de manutenção: **todo bug com causa-raiz provada entra aqui** no mesmo passo do fix,
junto com a entrada em `web/app/lib/changelog.ts` e a linha no Change Log de
`docs/architecture.md` (regra DOC SEMPRE ATUALIZADA). Detalhe de implementação fica no
`CLAUDE.md`; aqui fica o **diagnóstico operacional**.

Legenda de estado: ✅ corrigido · ⚠️ limitação aceita (não tem fix) · 🧪 armadilha de teste.

---

## Índice rápido — sintoma → seção

| Sintoma | Seção |
|---|---|
| Clipe do YouTube **some ao arrastar pro deck** / pula sozinho | [YT-1](#yt-1--origem-ip-bloqueia-embed-de-clipe-monetizado-) (dev) e [YT-6](#yt-6--falso-positivo-do-detector-de-bloqueio-errorcode-stale-) |
| Deck **preto** na transição Spotify→YouTube | [YT-5](#yt-5--esconder-o-overlay-parqueia-o-iframe--deck-preto-) |
| YouTube **trava em 00:00** com metadados certos | [YT-4](#yt-4--handshake-postmessage-sem-origin--trava-em-0000-) |
| YouTube **Erro 153** / "Video player configuration error" | [YT-3](#yt-3--erro-153-referer-obrigatório-desde-dez2025-) |
| YouTube nunca toca, mesmo com tudo certo (A/B ping-pong) | [YT-2](#yt-2--reparent-do-iframe-recarrega-o-player-) |
| Busca do YouTube devolve erro / para de funcionar no meio do dia | [YT-7](#yt-7--cota-da-data-api-v3-100-buscasdia-) |
| Capa da faixa YouTube 404 / placeholder ♪ | [YT-8](#yt-8--artwork-do-youtube-sem-artwork_url-) |
| Chip "Premium" do YouTube sempre false | [YT-9](#yt-9--premium-do-youtube-não-mapeia-) |
| Acha no "Sem login" mas **arrastar pede conta** | [YT-10](#yt-10--buscava-deslogado-mas-arrastar-pro-deck-pedia-conta-) |
| Dá pra rodar YouTube **sem API key nenhuma**? | [YT-11](#yt-11--youtube-100-sem-api-key-o-que-dá-e-o-que-não-dá-) |
| Deck diz **no ar** mas transporte diz **RETOMAR**, relógio congelado | [PLY-1](#ply-1--troca-de-fonte-deixa-o-playout-pela-metade-) |
| **Mixagem de 7 s parou de acontecer** (some o crossfade) | [PLY-1](#ply-1--troca-de-fonte-deixa-o-playout-pela-metade-) e [PLY-2](#ply-2--pause-agendado-passa-pelo-guarda-e-marca-a-estação-como-parada-) |
| Com **SEM-VOZ ligado** a mixagem nunca acontece | [PLY-2](#ply-2--pause-agendado-passa-pelo-guarda-e-marca-a-estação-como-parada-) |
| **Pod ligado sem processar nada** / conta do RunPod subindo | [GPU-1](#gpu-1--pod-fica-ligado-de-propósito-após-a-última-faixa-) |
| **Separação na nuvem nunca termina** (fica em "processando" para sempre) | [GPU-5](#gpu-5--o-pod-morria-recém-nascido-o-relógio-de-ociosidade-em-duas-formas-) |
| **CI reprova só no Linux**, passa no Mac | [DEV-11](#dev-11--o-ci-nunca-tinha-rodado-e-escondia-dois-bugs-só-de-linux-) |
| **Estação abre VAZIA depois de um deploy** (dados intactos no volume) | [DEP-1](#dep-1--mover-o-home-do-container-invalida-todo-caminho-já-gravado-) |
| Pod **reinicia em laço** e refaz o trabalho | [GPU-2](#gpu-2--pod-reinicia-em-laço-mkdir--p-workspace-e-auto-terminação-não-confiável-) |
| Separação falha com "**no longer any instances available**" | [GPU-3](#gpu-3--supply_constraint-capacidade-de-gpu-some-e-volta-) |
| Faixa **já preparada** exibida como "⏳ PROCESSANDO" | [PLY-3](#ply-3--endpoint-ampliado-sem-varrer-quem-consome-) |
| Feature nova **não aparece** (mas textos novos aparecem) | [DEV-5](#dev-5--feature-nova-não-aparece-binário-velho-servindo-bundle-novo-) |
| "**Dou refresh e some tudo**" no dev local | [DEV-6](#dev-6--dou-refresh-e-some-tudo-task-serve-era-efêmero-) |
| Deploy subiu mas a tela continua **antiga** | [DEV-7](#dev-7--deploy-invisível-html-sem-cache-control-) |
| **MP3 local** fica parado em 00:00, sem erro | [LOC-1](#loc-1--mp3-local-fica-parado-em-0000-sem-erro-) |
| **Dei refresh e a faixa recomeçou / parou** | [PLY-1](#ply-1--refresh-parava-a-estação-e-a-faixa-voltava-pro-0000-) |
| `curl` vê a faixa e o **browser não** (ou o inverso) | [DEV-8](#dev-8--curl-e-browser-caem-em-tenants-diferentes-) |
| `fetch` novo devolve **404 silencioso** | [DEV-9](#dev-9--apibase-devolve-só-a-origem-não-o-prefixo-da-api-) |
| `task cover` verde mas `go test ./...` vermelho | [DEV-10](#dev-10--o-gate-de-cobertura-dizia-passed-sobre-pacote-que-não-compilava-) |
| Spotify acaba a faixa e **para tudo** | [SP-1](#sp-1--player_state_changednull-fim-natural-lido-como-takeover-) |
| Mixagem **muda** e a fila **esvazia sozinha** | [SP-4](#sp-4--spotify-aceita-todo-comando-e-não-toca-a-fila-esvazia-sozinha-) |
| `oauth state mismatch` no connect (YouTube ou Spotify) | [SP-2](#sp-2--oauth-state-mismatch-) |
| Login do Spotify recusa o redirect | [SP-3](#sp-3--spotify-exige-loopback-ip-literal-no-redirect-) |
| Build aborta reclamando de changelog | [DEV-2](#dev-2--gate-do-changelog-aborta-o-build--é-o-comportamento-desejado) |
| Faixa YouTube/Spotify recusada pelo banco (CHECK) | [DEV-3](#dev-3--check-do-sqlite-não-altera-in-place-) |
| Vídeo fica em BUFFERING eterno no browser de automação | [DEV-4](#dev-4--browser-de-automação-não-toca-clipe-monetizado-) |

---

## YouTube — embed e playout

### YT-1 — Origem IP bloqueia embed de clipe monetizado ✅
**Versão:** v0.2.10 (medido em 27/07/2026) · **Só afeta dev.**

**Sintoma:** arrasta um clipe musical (VEVO/gravadora/canal oficial) pro deck, ele
**some no ato** e a fila avança. Vídeo livre de direitos (ex. Big Buck Bunny
`aqz-KE-bpKQ`) toca normal — por isso o bug parece **aleatório** ("só alguns vídeos somem").

**Causa-raiz:** o YouTube **recusa incorporar vídeo monetizado quando o host da origem da
página é um IP literal** (`http://127.0.0.1:8099`). O player devolve `onError 150` +
`getVideoData().errorCode === "auth"` e nunca sai de UNSTARTED — **mesmo com
`status.embeddable === true`** na Data API. O app então faz o correto (`embed:150` →
`useYouTubePlayout` effect 3a → `POST /playback/next`) e a faixa desaparece.

**Prova A/B** — mesmo vídeo (`JGwWNGJdvx8`), mesmo `host: youtube-nocookie`, mesmos
playerVars, mesmo Chrome, mesmo servidor, mesmo instante:

| origem | `getPlayerState()` | `errorCode` | `onError` |
|---|---|---|---|
| `http://127.0.0.1:8099` | -1 (UNSTARTED) | `"auth"` | **150** |
| `http://localhost:8099` | 3 (BUFFERING → carrega) | `null` | — |
| `https://example.com` | 3 (BUFFERING → carrega) | `null` | — |

Descartado por medição: **não** é o `youtube-nocookie` (cookieful também falha em IP);
**não** é o esquema `http:` (localhost também é http); **não** é o vídeo. É o **host ser IP**.

**Fix (v0.2.10)** — a lógica de skip está certa e não mudou; o que faltava era não falhar
em silêncio:
- `web/app/lib/ytOrigin.ts` — `isBareIpOrigin()` (IPv4 literal ou `[IPv6]`),
  `localhostEquivalentURL()`, `currentOrigin()`.
- `web/app/components/YouTubeOriginWarning.tsx` — banner no topo do `YouTubePanel` (estado
  conectado **e** desconectado), com link pra URL equivalente em `localhost`. Fora de origem
  IP renderiza `null`.
- `useYouTubePlayer.ts` `onError` — `console.warn` com a dica quando código morto ocorre em
  origem IP.
- `scripts/serve-local.sh` / `serve-youtube-local.sh` — anunciam `http://localhost:$PORT` e
  passam `AUTODJ_{SPOTIFY,YOUTUBE}_RETURN` **absolutos p/ localhost**.

**Regra:** em dev, abrir **sempre `http://localhost:<porta>`, nunca `127.0.0.1`**.
**Prod nunca foi afetado** (domínio real).

**Pegadinha do fix:** os `*_REDIRECT` de OAuth **continuam em `127.0.0.1`** — ver [SP-3](#sp-3--spotify-exige-loopback-ip-literal-no-redirect-).
Só o **retorno pós-callback** virou localhost; o state-smuggling `<sid>~<state>` é agnóstico
de origem, então trocar de origem no meio do fluxo não quebra o login.

**Como reconhecer de novo:** no console do browser,
`window.YT.get('widget2').getVideoData().errorCode === "auth"` com
`getPlayerState() === -1` e a barra de endereço num IP.

---

### YT-2 — Reparent do iframe recarrega o player ✅
**Versão:** v0.2.4.

**Sintoma:** vídeo nunca chega a PLAYING em browser real; trava em 00:00 / mostra "Ocorreu
um erro. Tente novamente mais tarde". Piora a cada troca A/B de deck.

**Causa-raiz:** o `Deck.tsx` movia o host do player (`appendChild` pro slot de capa do deck
ao vivo). Pela spec HTML, **mover um `<iframe>` na árvore DOM (remover + reinserir)
RECARREGA o iframe** — o document interno reinicializa e o handshake `postMessage` do
`YT.Player` morre a cada swap. Comprovado: reparent dispara um 2º evento `load`
(`reloadedOnReparent:true`); player nunca-movido tocava.

**Fix:** `useYouTubeOverlay.ts` + host parqueado em `page.tsx`. O host **nunca é
reparentado** — é sobreposto **visualmente** ao `[data-yt-slot]` do deck ao vivo via
`position:fixed` + `getBoundingClientRect` num loop `requestAnimationFrame`. Reposicionar
por CSS não recarrega o iframe.

**Regra INVIOLÁVEL:** nunca reparentar o iframe do YouTube — nem por `appendChild`, nem por
portal React que troque o pai real. Só reposicionar por CSS/overlay.

**Aprendizado meta:** o fix anterior (palco 320×320, v0.2.3) tinha causa-raiz **errada**; o
palco ficou só como margem de segurança de viewport (a IFrame API recusa player <200×200px,
então renderizar grande + `transform:scale` continua correto).

---

### YT-3 — Erro 153 (Referer obrigatório desde dez/2025) ✅
**Versão:** v0.2.1.

**Sintoma:** iframe cuea (`getPlayerState()===3`) mas nunca vira PLAYING; `getDuration()===0`
e `getVideoData().title===""` **para qualquer vídeo**, inclusive known-embeddable; overlay
"Watch video on YouTube / Error 153".

**Causa-raiz:** desde dez/2025 o YouTube **exige que o embed se identifique via header HTTP
`Referer`**. Página com `Referrer-Policy` restritiva (`no-referrer`/`same-origin`) — ou
extensão que stripa referer — estoura 153. É mudança do lado do YouTube, content-blocker é
só **um** dos gatilhos.

**Fix:** `host: "https://www.youtube-nocookie.com"` no `YT.Player` (workaround oficial) +
`referrerpolicy="strict-origin-when-cross-origin"` setado explicitamente no `<iframe>`
gerado pela API.

**🧪 Armadilha de teste:** abrir `youtube.com/embed/<id>` **direto numa aba** (navegação
top-level) SEMPRE dá 153 — não há página-pai, logo zero referer. Isso **não prova** bloqueio
de perfil/extensão. Testar sempre **dentro do app**.

**Distinguir de YT-4:** 153 = `duration === 0` **e** `title === ""`. YT-4 = metadados certos.

---

### YT-4 — Handshake postMessage sem `origin` → trava em 00:00 ✅
**Versão:** v0.2.2.

**Sintoma:** embed carrega **metadados certos** (título + duração reais) mas a posição fica
presa em 00:00 e aparece a overlay nativa "Ocorreu um erro. Tente novamente mais tarde".

**Causa-raiz:** a IFrame Player API valida o handshake `postMessage` contra a origem
declarada; sem `origin` nos `playerVars` o embed cai no wildcard `"*"` e desde ~2025 esses
players falham.

**Fix:** `playerVars.origin = window.location.origin` + `enablejsapi: 1`.

---

### YT-5 — Esconder o overlay parqueia o iframe → deck preto ✅
**Versão:** v0.2.9 (regressão introduzida na v0.2.8, commit `9783086`).

**Sintoma:** Spotify→YouTube em **partida automática** (sem novo gesto do usuário) deixa o
deck **preto** e não toca. YouTube-first (▶ clicado) toca normal.

**Causa-raiz — tripla regressão que aterrissou junta:** (1) `onError` passou a marcar
`embed:<code>` p/ **todos** os códigos; (2) surgiu o campo `errored` no deck YouTube; (3)
`page.tsx` **escondia o overlay** quando `errored`. Esconder o overlay = **parquear o iframe
fora do slot = deck preto**. Como a partida cross-engine não tem gesto fresco, o vídeo demora
mais a chegar a PLAYING → um `onError` transitório (2/5) dispara antes do detector de bloqueio
travar → `embed:` → overlay some → preto + faixa pulada.

**Fix:** `onError` só marca `embed:<code>` p/ códigos **permanentemente mortos**
(`100`, `101`, `150`); códigos ambíguos `2`/`5` viram erro cosmético `yt:<code>`. Campo
`errored` removido; overlay volta a `useYouTubeOverlay(ytHostRef, youtDeck.isYouTube)`.

**Regras:** só `embed:` (→ pula na hora) para `{100,101,150}`; **nunca esconder o overlay do
YouTube**. Um 2/5 genuinamente terminal é coberto pelo fallback por duração (`durMs+1500`).

**Tabela de códigos `onError` da IFrame API — semântica adotada:**

| código | significado | tratamento no AutoDJ |
|---|---|---|
| 2 | parâmetro inválido | cosmético `yt:2` — não pula, não esconde |
| 5 | erro do player HTML5 | cosmético `yt:5` — idem |
| 100 | vídeo removido/inexistente | `embed:100` → pula |
| 101 | dono desabilitou incorporação | `embed:101` → pula |
| 150 | idem 101 (variante) — **e o que a origem IP dispara** | `embed:150` → pula |

---

### YT-6 — Falso positivo do detector de bloqueio (`errorCode` stale) ✅
**Versão:** v0.2.2.

**Sintoma:** a faixa entra no deck, some no instante do play e `Tocadas` incrementa, sem tocar.

**Causa-raiz:** logo após `loadVideoById`, por uma fração de segundo
`getVideoData().errorCode` devolve o código do **vídeo ANTERIOR** (metadados stale — quirk da
IFrame API).

**Fix — janela de graça.** Só marca `embed:<code>` se **todas**: (a) o vídeo nunca chegou a
PLAYING (`playedRef`); (b) está preso em `-1`/`3`/`5`; (c) `errorCode` é de bloqueio
(`auth`/150/101/100); (d) o `video_id` **casa** com o carregado (não é stale); (e) a condição
persiste **≥3s**. Vídeo tocável chega a PLAYING em 1-2s → nunca pula. Buffering lento não tem
`errorCode` de bloqueio → não conta strike.

---

### YT-7 — Cota da Data API v3 (~100 buscas/dia) ⚠️
**Versão:** v0.2.8 (erro humanizado) · **Limitação do teto grátis do Google, não é bug.**

**Sintoma:** busca do YouTube para de funcionar no meio do dia. Antes da v0.2.8 aparecia
`⚠️ youtubeSearch: 500`; hoje devolve **HTTP 429** com frase em pt-BR.

**Números:** 10.000 unidades/dia por projeto Google Cloud; `search.list` custa ~100 →
**~100 buscas/dia**, reset ~meia-noite (Pacífico).

**Dois eixos independentes (não confundir):**
- **Busca** depende da **API key** (`AUTODJ_YOUTUBE_API_KEY`) — **não** do login OAuth.
- **Login OAuth** só **gateia a aba** (escolha de UX = espelho fiel do Spotify). Playout e
  **importar por URL/ID** funcionam sem login e sem key.

**Como aumentar (ordem de ROI):** (1) importar por URL/ID — `videos.list` custa **1 unidade**,
100× mais barato; (2) múltiplos projetos + round-robin de keys (**não implementado** —
`AUTODJ_YOUTUBE_API_KEY` é uma só); (3) audit form oficial do Google (semanas, raramente
vale p/ uso caseiro).

**Mitigações já no app:** debounce 300ms, `search` limit 10, sem polling. Cache de busca
(query→resultado, TTL) **não** implementado.

**Detalhe de arquitetura do fix:** `ErrQuotaExceeded` no domínio → `catalog.go` embrulha
(`%w`) o status upstream (429, ou 403 com `quotaExceeded`/`rateLimitExceeded`/
`dailyLimitExceeded` no corpo) → `writeYouTubeErr` traduz p/ HTTP 429. `errors.Is` cruza a
fronteira hexagonal.

---

### YT-8 — Artwork do YouTube sem `artwork_url` ✅
**Versão:** v0.2.8.

**Sintoma:** faixa YouTube importada antes (ou arrastada por URI crua) aparecia com
placeholder ♪ / 404 na capa.

**Causa-raiz:** `artwork_url` vazio no banco + a guarda de curto-circuito de
`LibraryService.Artwork()` não considerava `YouTubeURI` (track YouTube não tem `Path`) →
morria com `ErrNoArtwork` antes de chegar ao handler.

**Fix:** guarda passa a incluir `YouTubeURI==""`; o handler deriva a miniatura pública
`https://i.ytimg.com/vi/<videoID>/mqdefault.jpg` (sem API key) via `youtubeVideoID`, que
aceita `youtube:video:<id>` e id cru (playlist/desconhecido → 404).

---

### YT-9 — "Premium" do YouTube não mapeia ⚠️
**Versão:** v0.2.0.

O YouTube não expõe flag limpa de membership e o playout (IFrame) não depende dela. `Premium`
no domínio/status é **best-effort só p/ paridade do chip** (provavelmente sempre false);
**playout nunca é gated em Premium**.

---

### YT-10 — Buscava deslogado, mas arrastar pro deck pedia conta ✅
**Versão:** v0.2.17 (motor grátis) → corrigido na mesma v0.2.17.

**Sintoma:** com o motor `⚡ Sem login`, a busca funcionava sem conta e trazia resultados;
arrastar um resultado pro deck devolvia **"Conta do YouTube não conectada. Clique em
'Conectar ao YouTube' primeiro."** Busca livre inútil na prática — acha e não usa.

**Causa-raiz:** arrastar não é enqueue puro. O caminho é
`enqueueYouTubeSafe → importYouTubeURI → POST /youtube/import → ImportTracks`, e
`ImportTracks`/`ImportPlaylist` ainda chamavam `s.AccessToken(ctx)` antes do catálogo,
falhando com `ErrNoAccount` (HTTP 409). Herança de quando toda a aba exigia login.

**O gate era decorativo.** `adapters/driven/youtube/catalog.go` **ignora o token** —
`Search`, `Videos` e `PlaylistItems` recebem `_ /*token*/` e autenticam pela **API KEY**.
E o custo não pedia gate: `videos.list`/`playlistItems.list` = **1 unidade por lote**,
contra ~100 do `search.list` (o único que o OAuth precisa proteger).

**Fix:** `bestEffortToken(ctx)` — devolve o access token quando há conta e `""` quando não
há (ou quando o refresh falha). Falha de OAuth deixa de derrubar import que não depende de
OAuth. Conta ligada segue repassando o token (paridade de assinatura). `ImportLinks` (v0.2.17,
paste) já era assim.

**Como reconhecer de novo:** qualquer 409 num caminho que só resolve metadados. Pergunta de
triagem: *este endpoint usa o token pra alguma coisa?* Se o adapter descarta o argumento, o
gate é decorativo.

**Regressão:** `TestYouTube_ImportTracks_semContaConectada`,
`TestYouTube_ImportPlaylist_semContaConectada`, `TestYouTube_ImportTracks_comContaRepassaToken`.

---

### YT-11 — YouTube 100% sem API key: o que dá e o que não dá ✅
**Medido em 27/07/2026, de IP residencial. IMPLEMENTADO na v0.2.22** —
`adapters/driven/youtube/innertube_meta.go`.

> **Resolvido, com ressalvas.** A pesquisa abaixo virou código: `player` resolve id avulso
> e `next` resolve playlist (o `browse`, que falhou nas 3 primeiras tentativas, foi
> abandonado — `next` é outro endpoint e funciona). A aba agora sobe sem credencial
> nenhuma. Com API key configurada, a Data API continua **na frente** no import (filtro
> `embeddable` + playlist paginada) e o grátis vira fallback quando ela falha.
> **Ressalvas que continuam valendo**: `playabilityStatus` do servidor segue imprestável
> (ver abaixo — é ignorado no código, com teste travando isso); playlist grande pode vir
> incompleta (medido 19/20).

**Pergunta:** a busca grátis (InnerTube) dispensa key e conta. Dá pra dispensar a
`AUTODJ_YOUTUBE_API_KEY` de vez e fazer a aba inteira funcionar sem credencial nenhuma?

**Por que hoje não dá:** todo import passa por `Videos()`/`PlaylistItems()`, que delegam à
Data API. Sem a key, o operador acharia a música e não conseguiria importar nem tocar — por
isso a aba inteira 404 sem key. **A key ficou obrigatória; o que virou opcional foi o login.**

**O que foi medido:**

| Caminho | Sem key | Como |
|---|---|---|
| Busca | ✅ funciona | `raitonoberu/ytsearch` (InnerTube), já em produção |
| Metadados de id avulso (colar link) | ✅ funciona | `POST /youtubei/v1/player`, client `WEB` |
| Expandir playlist | ❌ pelo `browse` | `POST /youtubei/v1/browse` c/ `browseId: "VL"+id` → resolve o título, volta **sem itens** |
| Expandir playlist | ✅ pelo `next` | `POST /youtubei/v1/next` c/ `{playlistId}` → `playlistPanelVideoRenderer` com id, título e `lengthText` |

**A lição do `browse` vs `next`**: as 3 primeiras tentativas insistiram no MESMO endpoint
e concluí "não dá". Endpoint diferente para o mesmo dado resolveu na primeira tentativa.
Antes de declarar impossível, vale trocar de porta de entrada — não repetir a mesma.

**Prova do `player`** (`videoId=dGFxdmuDA4A`), por client:

| clientName | `videoDetails` | `lengthSeconds` | `playabilityStatus` |
|---|---|---|---|
| `WEB` | ✅ título + autor | `451` | `UNPLAYABLE` |
| `MWEB` | ✅ título + autor | `451` | `UNPLAYABLE` |
| `ANDROID` | ✗ ausente | — | ausente |
| `IOS` | ✗ ausente | — | ausente |
| `TVHTML5_SIMPLY_EMBEDDED_PLAYER` | ✗ ausente | — | `ERROR` |

**Armadilha central: `playabilityStatus` medido do SERVIDOR é imprestável.** Voltou
`UNPLAYABLE` para um vídeo que toca normalmente no browser — é bot-detection (falta
PO-token/sessão), não bloqueio de incorporação. Quem decide se toca é o **IFrame no browser
do operador** (IP residencial, sessão real). Usar esse campo como filtro rejeitaria
praticamente tudo. Os `videoDetails` vêm corretos **apesar** do status.

**Playlist:** `browse` responde (playlist inexistente → `404 NOT_FOUND`, real → resolve o
título, ex. `"Select Lectures"`), mas o corpo volta **sem nenhum item de vídeo**: zero
`playlistVideoRenderer`, só moldura (`buttonRenderer`, `hotkeyDialogSectionRenderer`…).
Ou os itens vêm por continuation, ou o YouTube serve casca para requisição sem sessão de
browser. Três tentativas, abandonado — daqui pra frente é engenharia reversa de continuation
token, sem garantia de sobreviver à próxima mudança de layout.

**Fato que muda o desenho, se alguém retomar:** o resultado da busca **já traz duração**
(`VideoItem.Duration`, em segundos) além de título/canal/thumbnail — e o app **descarta isso**,
re-resolvendo o mesmo vídeo pela Data API no `ImportTracks`. O caminho busca→fila não precisa
de key nenhuma; só precisa parar de jogar fora o que já tem na mão. A key só é mesmo
necessária para **id sem contexto de busca**: colar link e expandir playlist.

**Risco de produção a considerar antes de tentar:** prod roda em **datacenter (Fly)**, onde o
YouTube aperta bem mais que num IP residencial. A busca grátis já provou funcionar de lá
(verificado em prod), mas `player`/`browse` são superfícies diferentes e não-oficiais. Se for
implementar, o desenho seguro é **inverter o arranjo atual**: InnerTube primeiro, Data API como
fallback quando a key existir.

**Cuidado com a duração:** `importItems` **descarta item com duração 0**
(`TestYouTube_Import_skipsPlaylistAndZeroDuration`). Uma fonte sem duração — oEmbed, por
exemplo, que é oficial e sem key mas só dá título/autor/thumbnail — faria **todo** import
falhar silenciosamente.

---

## Spotify

### SP-1 — `player_state_changed(null)`: fim natural lido como takeover ✅
**Versão:** v0.2.7.

**Sintoma:** faixa A no Spotify, B no YouTube; A termina sozinha e o player **limpa os dois
decks e para tudo** — sem áudio e sem o crossfade configurado.

**Causa-raiz:** o Web Playback SDK dispara `player_state_changed(null)` quando o device do SDK
deixa de ser o ativo — e isso acontece em **dois casos indistinguíveis pelo evento**:
(a) takeover real por outro aparelho; (b) **fim natural** (device ficou idle; o null às vezes
chega com `prevState` ainda "tocando"). Tratar todo null como takeover levava a
`takeoverNonce++` → `setWant(false)` → `ytStop()` na faixa entrante → parada total.

**Fix em 2 camadas:**
1. `useSpotifyPlayer.ts` — `lastProgressRef{pos,dur,at,playing}`; no branch null computa
   `nearEnd = dur>0 && dur-(pos+elapsed) < 4000`. `takeoverNonce++` só quando
   `wasPlaying && !nearEnd`. Trade-off aceito: takeover genuíno nos **últimos 4s** fica
   suprimido (raro).
2. `page.tsx onSpotifyTakeover` — guarda que **independe de timing**: ignora o takeover quando
   `snap.now_playing.source !== "spotify"` (já cruzamos de motor) ou quando a próxima da fila
   não é Spotify (prestes a cruzar). Necessária porque com fade de 7s o crossfade preemptivo
   avança a fila **antes** do fim natural, e o null atrasado do Spotify chegava depois,
   cortando o YouTube já no ar.

---

### SP-2 — `oauth state mismatch` ✅
**Versão:** v0.2.7 · Afeta **YouTube e Spotify** (mesmo padrão de código).

**Sintoma:** connect falha com `youtube: oauth state mismatch` (ou o equivalente Spotify).

**Duas causas-raiz distintas, corrigidas juntas:**
1. **Pending de OAuth era slot único.** Um 2º `Begin()` (retry, duplo-clique, aba de login
   antiga) **sobrescrevia** o pending; completar a primeira autorização falhava. Fix: `pending`
   virou `map[string]*PendingAuth` keyed by state, com poda de expirados e cap
   `maxPendingAuth=16`.
2. **Guard que rejeitava o tenant `default`.** `sessionFromState` tinha
   `if sid == defaultTenant { return "", "", false }`; quando o browser não tinha FingerprintJS
   resolvido, `sessionId()` devolvia `"default"`, o envelope virava `default~<state>` e a peel
   era rejeitada → `Complete()` recebia o envelope colado. Fix: guard removido — o envelope é
   **sempre** peelado quando há `~`.

**Aprendizado:** o state-smuggling `<sid>~<state>` é provider-agnóstico **e** origin-agnóstico
— por isso [YT-1](#yt-1--origem-ip-bloqueia-embed-de-clipe-monetizado-) pôde mover o retorno
pós-callback p/ localhost sem tocar no fluxo de OAuth.

---

### SP-4 — Spotify aceita todo comando e não toca; a fila esvazia sozinha ✅
**Diagnosticado e mitigado em 27/07/2026 (v0.2.24), medido em produção.**

**Sintoma do operador:** "a mixagem entre YouTube e Spotify ficou muda, e depois some
tudo". Parecia bug de crossfade.

**Não era o crossfade.** Medido no ar: o YouTube faz o fade-out correto (volume
100→87→57→27→4→0) e o deck do Spotify entra em `MIX ▲` na hora certa. O Spotify é que
nunca começa — **e falha igual fora de mixagem**, com uma faixa Spotify sozinha.

**O que o SDK responde** (instrumentando `Spotify.Player.prototype` em runtime):

| Chamada | Resultado |
|---|---|
| `getCurrentState()` | `{paused: true, pos: 0, track: "Another Life"}` — device conectado, faixa **carregada** |
| `togglePlay()` | resolve **sem erro** · nada acontece |
| `activateElement()` | resolve **OK** · nada acontece (não era política de autoplay) |
| `resume()` | resolve **OK** · nada acontece |
| `GET api.spotify.com/v1/me` | **HTTP 403, corpo vazio** |

**Causa fora do app:** o Spotify passou a recusar o token (403 no `/me`). O SDK aceita
tudo em silêncio — nenhum erro, nenhum evento — e simplesmente não produz áudio.

**O estrago era nosso, e foi corrigido:**
1. **Falha silenciosa.** Sem áudio e sem aviso. Agora, faixa que não sai de `pos: 0`
   depois de `STALL_MS` (9s) dispara `onStalled` → a estação **para** e o operador recebe
   um toast.
2. **Fila drenada.** O fallback por duração (3b) avançava passado `duração + 1,5s` mesmo
   sem a faixa ter tocado — "acabou" e "nunca começou" eram a mesma coisa. Cada faixa que
   não tocava era pulada, esvaziando a programação. Agora 3b só avança se a faixa
   **realmente progrediu** (`progressed`, posição > 1,5s).
3. **Selo "Premium" desatualizado.** `Product` só é reconsultado quando o token é renovado
   (`postToken` → `fetchProduct`), então o painel podia exibir "Conectado · Premium" com a
   sessão já recusada. Novo `POST /api/v1/spotify/revalidate` força a renovação **no momento
   da falha** e devolve o estado real; falha de renovação → **409**, e a UI diz "reconecte
   a conta".

**Como reconhecer de novo:** faixa Spotify parada em `00:00` com o transporte em
`▶RETOMAR`, sem erro no console. Confirme com `getCurrentState()` no player do SDK: se
vier `paused:true, pos:0` com o track certo, o device está vivo e o problema é a sessão —
não o app. `GET /v1/me` com 403 fecha o diagnóstico.

**Lição:** um SDK que resolve todas as promessas **não** é um SDK que funciona. Nenhuma
chamada rejeitou; o único sinal honesto foi a posição nunca sair de zero.

---

### SP-3 — Spotify exige loopback IP literal no redirect ⚠️
**Restrição do provider, não tem fix.**

O Spotify **exige `127.0.0.1` literal** como redirect URI de loopback (não aceita `localhost`);
o Google aceita os dois. Por isso, mesmo depois de [YT-1](#yt-1--origem-ip-bloqueia-embed-de-clipe-monetizado-):

- `AUTODJ_SPOTIFY_REDIRECT` / `AUTODJ_YOUTUBE_REDIRECT` → **continuam em `127.0.0.1`** (é o
  registrado nos dois consoles).
- `AUTODJ_SPOTIFY_RETURN` / `AUTODJ_YOUTUBE_RETURN` → **absolutos p/ `http://localhost:<porta>`**,
  senão o login termina de volta na origem IP e o YouTube volta a bloquear o embed.

Ver comentário em `cmd/serverkit/serverkit.go` e os dois scripts de serve.

---

## Playout local — troca de fonte e mixagem

### PLY-1 — Troca de fonte deixa o playout pela metade ✅

**Sintoma.** O deck ao vivo mostra ⏹ (no ar), mas o transporte mostra **RETOMAR**;
o relógio congela e a **mixagem de 7 s não acontece**. O áudio pode até continuar
saindo.

**Causa raiz.** A troca do SEM-VOZ (original ⇄ instrumental) faz uma varredura de
filtro de ~6 s usando o elemento `<audio>` OCIOSO. Se algo interrompe no meio — e o
caso mais fácil é **a faixa acabar** —, o efeito é abortado, `finishSwap()` não roda
e os papéis ficam pela metade: o elemento que o app considera "ao vivo" está em
**volume 0** (a rampa já desceu) enquanto quem toca é o outro. Como o coordenador da
mixagem tem gate literal em `playing`, o crossfade nem tenta.

**Fix (0.2.39).** Três regras em `useLocalPlayout`:
1. a troca **não começa** se falta menos que o gesto inteiro (~7,5 s) — ali ela é
   dispensável, porque a PRÓXIMA faixa já carrega na fonte certa;
2. interrupção **REVERTE** (volume de quem está no ar restaurado, o outro pausado,
   filtro ao repouso) — reverter é mais seguro que completar;
3. **`startMix` recusa enquanto há troca em curso**: os dois usam o elemento
   ocioso, e melhor virada seca que dois donos do mesmo `<audio>`.

**Como reconhecer.** Deck e transporte discordando é a assinatura: um diz no ar, o
outro diz pausado. Se o relógio não anda mas o som continua, é este.

### PLY-2 — `pause` agendado passa pelo guarda e marca a estação como parada ✅

**Sintoma.** Com o **SEM-VOZ ligado**, a mixagem não acontece em faixa NENHUMA (com
o modo desligado, funciona). Transporte fica em RETOMAR o tempo todo, mesmo com o
áudio tocando.

**Causa raiz — ordem de eventos assíncronos.** `a.pause()` **não emite o evento na
hora**: o browser o AGENDA. A sequência real era:

```
a.pause()               → evento 'pause' AGENDADO
finishSwap()            → swappingRef = false        ← guarda liberado
…evento 'pause' chega   → guarda já false → setPlaying(false)   ✗
```

O guarda caía ANTES de chegar o evento que ele existia para engolir. E como
`playing` só muda por evento — e o elemento que assume já estava tocando enquanto
era o ocioso, portanto **não emite `play` novo** — nada trazia o estado de volta até
a faixa virar.

Só aparece com o modo ligado porque **sem ele nenhuma faixa passa pela troca; com
ele, todas passam**.

**Fix (0.2.39).** `finishSwap` **AFIRMA** o estado a partir de quem assumiu
(`setPlaying(!el.paused)`) em vez de esperar um evento que nunca vem, e o guarda só
cai no **próximo ciclo** (`setTimeout 0`), de modo que o evento atrasado ainda é
engolido.

**Medido.** Antes: **107 de 111** amostras com o transporte em RETOMAR e **ZERO**
em mixagem. Depois: **0** em RETOMAR e **15** amostras com os dois decks em
`mixing-out`/`mixing-in`.

**Lição transversal.** Guarda booleano contra evento de mídia é frágil: o evento
chega DEPOIS. Ou o guarda cai no próximo ciclo, ou o estado é afirmado em vez de
aguardado — de preferência os dois.

### PLY-3 — Endpoint ampliado sem varrer quem consome ✅

**Sintoma.** Faixa com o SEM-VOZ **pronto** aparecia como "⏳ PROCESSANDO", com o
botão desabilitado, e a barra de progresso girava para sempre.

**Causa raiz.** `GET /library/instrumentals` passou a devolver também `ready` (a
FILA precisa disso, porque é montada do `Playable` e não pode carregar estado que
termina minutos depois). Os consumidores anteriores — `PlayerScreen` e
`OpMediaList` — tratavam **qualquer** valor no mapa como "trabalho em andamento".

**Fix (0.2.38).** Ambos filtram por `queued`/`running`. É a mesma classe do bug dos
**dois DTOs** já registrada no `CLAUDE.md`: ampliar o significado de um contrato sem
varrer quem consome.

**Como reconhecer.** Estado que deveria ser final aparecendo como "em andamento".

## Separação na nuvem (RunPod)

### GPU-1 — Pod fica ligado de propósito após a última faixa ✅

**Sintoma.** Existe pod ativo no RunPod e o saldo cai, mesmo sem ninguém processar
nada naquele instante.

**Não é vazamento — é desenho.** Subir o pod e instalar o Demucs custa **~3 minutos
pagos**; matá-lo após cada faixa faria um lote de 10 pagar 10 subidas em vez de uma.
Por isso ele fica quente esperando a próxima. O preço é GPU parada nesse intervalo.

**Números.** A ~US$ 0,27/h: com os 8 min originais, **~US$ 0,036** por janela
ociosa; com os **2 min** atuais (0.2.40), **~US$ 0,009**. O tick do watchdog é de
**20 s** — quando ele era igual ao teto, um pod ocioso sobrevivia até o **dobro** do
previsto.

**Como reconhecer que é isto e não outra coisa.** Consulte
`GET /library/instrumentals`: se não houver nada em `queued`/`running`, o pod está
só esperando o desligamento. Processar em sequência é muito mais barato que
espaçado — cada lote novo paga a subida de novo.

**Matar na mão** (quando quiser cortar antes):
```bash
KEY=$(pass show "$PASS_RUNPOD_API_KEY")   # ver scripts/secrets.example.sh
curl -sS -X POST https://api.runpod.io/graphql -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"query":"mutation { podTerminate(input:{podId:\"<POD_ID>\"}) }"}'
```
⚠️ A resposta é `{"data":{"podTerminate":null}}` **mesmo quando funciona** — confira
por consulta (`myself { pods { id } }`), nunca pela resposta.

### GPU-2 — Pod reinicia em laço; `mkdir -p /workspace` e auto-terminação não confiável ✅

**Sintoma.** O pod sobe, o `uptime` reinicia do zero repetidamente (10 s → 3 s), e o
trabalho é refeito — às vezes gerando resultado duplicado.

**Duas causas distintas, ambas medidas:**

1. **`/workspace` não existe.** Com `volumeInGb: 0` o diretório não é criado;
   escrever ali falha, o comando de partida morre e o RunPod **reinicia o
   container**. O bootstrap precisa de `mkdir -p /workspace` na primeira linha.
2. **A auto-terminação do pod não é garantida.** Mesmo com `RUNPOD_API_KEY` +
   `RUNPOD_POD_ID` no container, houve caso do pod terminar o trabalho, subir o
   resultado e ser **reiniciado**, refazendo tudo. E não dá para detectar de dentro,
   porque a mutation responde `null` em sucesso e em falha.

**Fix.** O `mkdir` no bootstrap **e** um watchdog do lado do AutoDJ: quem lançou
acompanha, insiste e só considera encerrado quando a API diz que o pod sumiu da
listagem. O relógio do pod é folga de segurança, não a autoridade.

### GPU-3 — `SUPPLY_CONSTRAINT`: capacidade de GPU some e volta ✅

**Sintoma.** Lançar o pod falha com *"There are no longer any instances available
with the requested specifications"*.

**Causa raiz.** Capacidade de nuvem é volátil. No spike, **três modelos seguidos**
recusaram — incluindo o primeiro da lista; no primeiro lote real, a A5000 caiu e a
A4000 entrou. Prender o recurso a um modelo o faz falhar por motivo que não tem nada
a ver com o AutoDJ.

**Fix.** Lista de fallback (`AUTODJ_RUNPOD_GPUS`), da mais barata à mais cara, e
`SECURE` → `COMMUNITY`. Erro que **não** é de capacidade (chave inválida, cota)
aborta na hora, em vez de varrer a lista escondendo a causa.

**Armadilha de preço.** A tabela do site mostra valores de **Serverless**
(US$ 0,69/h); pod on-demand sai por **US$ 0,16–0,27/h**. E **"NVIDIA L4" não existe**
no secure cloud — consulte `gpuTypes` pela API antes de fixar um nome.

### GPU-4 — A API de logs do pod não existe ⚠️

`rest.runpod.io/v1/pods/{id}/logs` responde **erro de spec**. Quem depender dela
para orquestrar fica cego — foi assim que o primeiro spike falhou sem diagnóstico.
O resultado tem de voltar por um canal próprio (aqui, a resposta HTTP do serviço que
roda no pod).

⚠️ O `Logs()` do `finetuning-platform` usa essa rota e provavelmente está quebrado
pelo mesmo motivo.

## Ambiente de dev / build

### GPU-5 — O pod morria recém-nascido: o relógio de ociosidade, em duas formas ✅

**Sintoma.** Toda separação na nuvem fica em `running` e nunca termina; ao final,
`runpod: pod não ficou pronto em 8m0s`. Nenhuma faixa é preparada.

**As duas formas do MESMO erro conceitual**, achadas em sequência no primeiro pod
real desde a 0.2.40:

```
20:39:13 stem: pod hg1gwl7qms1zy2 no ar
20:39:33 stem: pod ocioso há 2562047h47m16.854775807s — desligando   ← forma 1
...
20:46:06 stem: pod q7a3fl4s9jt9rj no ar
20:48:07 stem: pod ocioso há 2m0s — desligando                        ← forma 2
```

**Forma 1 — `2562047h47m…` é `MaxInt64` nanossegundos**, ou seja `time.Since()`
sobre um `time.Time` **zerado**: `lastUse` só era escrito pelo `touch()` do
`SeparateVocals`, que roda DEPOIS do `waitReady`. Durante os ~3 min de instalação
do Demucs o campo seguia zerado e o watchdog lia ociosidade infinita.

**Forma 2 — corrigida a inicialização, o pod passou a morrer em `2m0s` exatos**,
o teto: o relógio agora partia do lançamento, mas continuava correndo durante a
instalação, que leva mais que o teto.

**A raiz é a definição.** "Ocioso" tem de significar **ninguém está trabalhando**.
Enquanto sondamos o `/health`, alguém está: o pod instala e nós esperamos. Quem
limita essa fase é o `readyTimeout` (8 min), não o relógio de ociosidade. Hoje o
`waitReady` marca atividade a cada sondagem.

**⚠️ Por que ficou escondido por 8 versões.** Sorte de cronômetro: com tique de
2 min, a faixa do lote medido ficou pronta em **78 s** — o `touch()` chegava antes
do primeiro tique. Baixar o tique para 20 s na 0.2.40 (sem lançar um pod para
verificar) fez a primeira checagem cair no meio da instalação, e aí **nenhum pod
sobrevivia**. Mudança de tique parece inócua e não é: ela reposiciona a primeira
observação.

**Como reconhecer.** Qualquer duração absurda em log de ociosidade (`2562047h`) é
tempo zero, não um relógio maluco. Duração **exatamente igual ao teto** é relógio
correndo durante uma fase que não deveria contar.

### DEP-1 — Mover o HOME do container invalida todo caminho já gravado ✅

**Sintoma.** Depois de um deploy, a biblioteca **lista** as faixas normalmente
(título, BPM, SEM-VOZ pronto) e **nenhuma toca**: `GET .../audio` responde
**403 Forbidden**.

**Causa raiz.** `Track.Path` é gravado **absoluto** no banco. Ao trocar o container
para usuário sem privilégio, o volume foi movido de `/root/.autodj` para
`/home/app/.autodj` — e cada faixa existente seguia apontando para o caminho
antigo. Sem root, abrir aquilo falha já na **travessia** de `/root` (modo `700`),
antes de descobrir que o diretório mudou.

**O 403 é a pista precisa**: o corpo era `403 Forbidden` em **texto puro**, não o
JSON do tratador de erros do router — ou seja, veio do próprio `http.ServeFile`,
cujo `toHTTPError` mapeia `fs.ErrPermission` para exatamente essa string. Erro de
arquivo ausente daria 404; permissão dá 403.

**Fix.** O volume voltou para `/root/.autodj` e `HOME` para `/root`; o ganho de
segurança (processo sem root) fica de pé com `chmod o+x /root` — atravessar, não
listar. Mover de verdade exigiria reescrever o caminho em TODOS os bancos de
TODAS as estações, o que é migração de dado de usuário.

**Regra**: usuário do container e ponto de montagem **mudam juntos ou não mudam**.

### DEV-1 — Abrir o app em IP quebra o YouTube 🧪
Consequência prática de [YT-1](#yt-1--origem-ip-bloqueia-embed-de-clipe-monetizado-):
**sempre `http://localhost:<porta>`**. Os scripts `serve-local.sh` / `serve-youtube-local.sh`
já imprimem a URL certa; o app mostra banner vermelho se detectar origem IP.

### DEV-2 — Gate do changelog aborta o build ✅ (é o comportamento desejado)
`scripts/changelog-check.sh` roda antes de `task web:build` / `task deploy` e **aborta** se a
git tag de versão mais recente (`v[0-9]*`) não tiver entrada correspondente em
`web/app/lib/changelog.ts`. O changelog **pode** estar à frente da tag (documentando trabalho
ainda não taggeado) — o gate só exige que a **versão taggeada** exista no arquivo. Sem git ou
sem tag, o gate é pulado.

### DEV-3 — CHECK do SQLite não altera in-place ✅
Adicionar valor novo a `source_valid` (ex. `'youtube'`) exige **rebuild da tabela `tracks`**
(create nova com o CHECK novo → copy → drop → rename), idempotente no open do repo — padrão do
`ensureArtworkColumn`. Coluna nova vai por `ALTER` idempotente; tabela nova por
`CREATE IF NOT EXISTS`. Postgres altera CHECK direto (migração numerada).

### DEV-4 — Browser de automação não toca clipe monetizado 🧪
**Correção de um registro anterior** (que dizia "o Chrome automatizado não decodifica YouTube"):
ele **decodifica sim** — Big Buck Bunny chega a PLAYING(1) dentro do app. O que ele não
consegue é tocar **clipe musical monetizado**: fica em BUFFERING(3) eterno, `currentTime` 0,
**sem erro**, inclusive fora do app.

**Consequência prática:** erro/bloqueio de embed **é** reproduzível na automação (foi assim que
[YT-1](#yt-1--origem-ip-bloqueia-embed-de-clipe-monetizado-) foi isolado, comparando origens);
mas **confirmar que o áudio toca exige o browser real do usuário**.

---

### DEV-5 — Feature nova não aparece: binário velho servindo bundle novo 🧪
**Medido em 27/07/2026.**

**Sintoma:** implementa uma feature, `task web:build` passa, abre o app — e o elemento novo
não está lá. Textos novos do mesmo commit **aparecem** (o que confunde: parece deploy parcial).

**Causa-raiz:** `task web:build` regenera `web/out`, mas **não recompila o Go**. O servidor
antigo (de horas atrás) segue no ar servindo o bundle novo. Se a feature depende de campo novo
na API, o front recebe `undefined` e degrada em silêncio.

**Caso real:** `GET /api/v1/youtube/status` devolvia `{"connected":false,"premium":false}` —
**sem `freeSearch`**. O `YouTubePanel` faz `status.freeSearch ? … : "api"`, então escondia o
chaveador e caía no motor da conta, exibindo o hero de login. Diagnóstico em uma linha:

```bash
curl -s http://localhost:8099/api/v1/youtube/status   # campo novo está aí?
ps -o pid,lstart,command -p $(pgrep -f /tmp/autodj-server)  # binário é de quando?
```

**Fix:** `task serve` (o script recompila e mata o antigo). **Como reconhecer:** campo novo
ausente no JSON + horário de start do processo anterior ao commit.

**Variante irmã:** aba aberta com JS antigo em memória. Aí a API já responde certo e o DOM
continua velho — nenhuma requisição acontece. Confirma-se pelo chunk carregado:
`performance.getEntriesByType('resource')` filtrando `app/page`.

---

### DEV-6 — "Dou refresh e some tudo": `task serve` era efêmero ✅
**Versão:** corrigido na v0.2.19.

**Sintoma:** operador relata que **recarregar a página apaga a biblioteca, a fila e a conta
ligada**.

**Causa-raiz — não é o refresh.** `scripts/serve-local.sh` forçava `AUTODJ_DB_PATH=memory`
(repo em RAM). Como o script **mata o server antigo a cada execução**, todo restart para pegar
código novo zerava tudo. O refresh era só o momento em que a perda ficava visível — a causa
tinha acontecido antes.

**Prova A/B** (mesmo servidor, mesmo tenant):

| ação | resultado |
|---|---|
| importa faixa → **recarrega a página** | faixa continua lá ✅ |
| importa faixa → **reinicia o servidor** (modo `memory`) | biblioteca vazia ❌ |
| importa faixa → **reinicia o servidor** (modo arquivo) | faixa continua lá ✅ |

**Fix:** `DB="${AUTODJ_DB_PATH:-}"` (era `:-memory`) → cai no default do serverkit: SQLite por
fingerprint. Opt-out preservado: `AUTODJ_DB_PATH=memory task serve` para smoke em estado limpo.

**`AUTODJ_DB_PATH` é MODO, não caminho** — confusão fácil. `dbPathFor(id)` deriva o arquivo do
id; a variável só escolhe entre arquivo e RAM:

```go
if t.mem              { return "memory" }
if id == defaultTenant { return defaultDBPath() }   // ~/.autodj/library.db
return filepath.Join(t.base, id+".db")              // ~/.autodj/sessions/<fp>.db
```

Em `memory` o diretório `sessions/` **nem é criado** — sinal rápido de qual modo está rodando.

---

### DEV-7 — Deploy invisível: HTML sem `Cache-Control` ✅
**Versão:** corrigido na v0.2.18.

**Sintoma:** deploy sobe, servidor e assets novos no ar, e o browser continua mostrando a UI
antiga até um hard-refresh (`Cmd+Shift+R`).

**Causa-raiz:** `http.FileServer` do Go manda só `Last-Modified`, **sem `Cache-Control`**.
Sem ele o browser aplica **cache heurístico** (~10% da idade do arquivo). Como é o `index.html`
que aponta para os nomes hasheados dos chunks, um HTML velho continua pedindo o **chunk velho** —
servidor novo, bundle novo, tela velha.

**Por que `?version=x` NÃO resolve** (pergunta que aparece sempre): a query string mora **dentro
do HTML**. HTML velho carrega `?v=` velho, exatamente como carrega o nome hasheado velho — mesma
falha. E a URL do documento (o domínio raiz, que o usuário digita) **não é versionável**. Só
header alcança o documento de entrada. Os nomes hasheados que o Next já emite
(`page-a999bd5f03d9f95d.js`) *são* o `?version=`, derivados do conteúdo em vez de digitados —
sem risco de esquecer de incrementar e sem proxy que ignore query string na chave de cache.

**Fix:** `httpapi.StaticCacheControl` carimba por caminho —
`/_next/static/*` (nome com hash) → `public, max-age=31536000, immutable`; todo o resto,
**`index.html` incluso** → `no-cache` (guarda mas revalida; 304 é barato). Fica em `httpapi`,
não em `serverkit`, porque o gate de cobertura pula `cmd/*`.

**Fica mais rápido, não mais lento:** hoje cada asset paga um condicional por reload; com
`immutable` ele não pergunta nunca.

**Limite que sobra:** aba já aberta nunca refaz a requisição do HTML — nenhuma política de cache
cobre isso, porque não há requisição. Fechado por outro caminho na v0.2.20 (checagem de versão
em runtime, ver `CLAUDE.md`).

---

### LOC-1 — MP3 local fica parado em 00:00, sem erro ✅
**Corrigido na v0.2.26. Medido em produção.**

**Sintoma:** faixa local entra no deck com título, capa e duração certos, e ao dar PLAY
fica em `00:00`. Transporte volta pra `▶ RETOMAR`. **Nenhuma mensagem.**

**Causa-raiz — a mesma do artwork, mas o áudio ficou pra trás.** `<audio src>` **não
carrega header customizado** (só o `fetch` é embrulhado com `X-AutoDJ-Session`), então
`/library/tracks/{id}/audio` chegava ao tenant **`default`** — que não tem a faixa desta
estação — e respondia **404**. O elemento falhava, `play()` rejeitava, e o
`.catch(() => setPlaying(false))` **engolia em silêncio**.

`trackArtworkURL` já resolvia isso com `?sid=` (e tinha comentário explicando!);
`trackAudioURL` não. O servidor sempre aceitou os dois caminhos —
`sessionID()` lê o header **e** `?sid=`.

**Prova** (mesmo id, mesmo instante, cache furado com `?cb=`):

| Requisição | Status |
|---|---|
| `/audio?cb=…` (sem sessão) | **404** |
| `/audio?sid=…&cb=…` | **200** |
| `new Audio('/audio?sid=…')` | `canplay`, duração 200s |

**Descartado por medição**: caminho no volume (`/root/.autodj/media/<tenant>/arquivo.mp3`,
2.508.219 bytes, batendo com o `Content-Range`), permissão e formato.

**🧪 ARMADILHA que quase escondeu o bug:** o cache HTTP do browser **não distingue
header**. Um `fetch` anterior (com sessão) deixa a resposta no cache, e aí o `<audio>`
carrega e o bug some. Ao depurar qualquer coisa envolvendo `<audio>`/`<img>` e tenant,
**sempre furar o cache** com um parâmetro único — senão o teste mente.

**Fix:** `?sid=` no `trackAudioURL` + o `catch` passou a avisar (formato não suportado,
falha de rede, ou genérico), em vez de deixar a faixa muda.

**Regra que fica:** qualquer URL consumida por **elemento HTML** (`<audio>`, `<img>`,
`<video>`, `<source>`) precisa de `?sid=` — header só funciona no `fetch`.

---

### DEV-8 — `curl` e browser caem em tenants diferentes 🧪
**Medido em 27/07/2026.**

**Sintoma:** você importa por `curl`, confirma que a faixa está na biblioteca, e o operador jura
que não vê nada. Os dois estão certos.

**Causa-raiz:** isolamento por fingerprint. O browser manda `X-AutoDJ-Session: <fp>` (o
`autodj:sid` do `localStorage`, 32 chars) e cai em `~/.autodj/sessions/<fp>.db`; `curl` sem
header cai no tenant `default` → `~/.autodj/library.db`. **Bancos diferentes, mesmo servidor.**

**Como depurar direito** — repetir a leitura nos dois lados:

```bash
curl -s localhost:8099/api/v1/library/tracks                          # tenant default
curl -s -H "X-AutoDJ-Session: <fp>" localhost:8099/api/v1/library/tracks
```

No browser, cuidado extra: o app **embrulha `window.fetch`** para injetar o header, então um
`fetch('/api/…')` colado no console **não** mede o tenant `default` — usa `XMLHttpRequest` quando
quiser controlar o header explicitamente.

---

### DEV-9 — `apiBase()` devolve só a ORIGEM, não o prefixo da API 🧪
**Medido em 27/07/2026.**

`apiBase()` retorna `""` (same-origin) ou `http://localhost:8095` (dev na :3000). O `/api/v1`
é **responsabilidade do chamador**: `fetch(apiBase() + "/api/v1/library/categories")`.

Escrever `` fetch(`${apiBase()}/health`) `` compila, roda, devolve **404 em silêncio** e a
feature simplesmente não acontece. Custou um teste end-to-end no `useVersionCheck`. Achado só
porque a verificação foi feita **no browser**, não no compilador.

---

### PLY-1 — refresh parava a estação e a faixa voltava pro 00:00 ✅
**Corrigido na v0.2.26. Medido no browser, nos três motores.**

**Sintoma:** com música tocando, dar F5 trazia a estação **parada** e a faixa **do começo**.
Relatado primeiro no motor local e depois no YouTube ("dei um refresh no YouTube e parou tbm").

**São DUAS causas independentes** — e é por isso que consertar uma só não resolvia:

**(a) O transporte era desligado no boot.** `page.tsx` tem a regra "fila esvaziou ⇒ para o
transporte" (`if (!hasHead && !autoOn) setWant(false)`). No primeiro render o `snap` do WS
ainda é `null`, então `hasHead` é `false` — e com AUTO Off a regra disparava **antes de o
servidor dizer o que havia na fila**, apagando o `wantPlay` que estava persistido em
`localStorage`. Todo refresh parava a estação. Fix: a regra só vale **depois do 1º snapshot**
(`snapshotSeen = !!snap`).

**(b) A retomada de posição existia só pro Spotify — e se auto-destruía.**
- **Local**: nunca salvou posição. Simples assim.
- **YouTube**: salvava (chave própria `autodj.ytplayout.v1`), mas restaurava com
  `loadVideoById(id)` **e um `seekTo` 700 ms depois** — o seek chegava com o vídeo ainda em
  UNSTARTED/BUFFERING, estado em que a IFrame API **descarta** o pedido. Fix: a posição vai
  no próprio load (`loadVideoById({videoId, startSeconds})`).
- **Spotify**: funcionava, mas o `clearResume()` incondicional do seu tick (rodava sempre que
  o now-playing NÃO era Spotify) apagaria o registro dos outros motores assim que a chave
  virou única.
- **Armadilha que só aparece testando**: ao recarregar, o motor monta com a faixa em `00:00`
  e o tick de posição salvava **0 por cima** do valor bom, antes de o operador dar PLAY —
  a retomada se destruía sozinha. Fix: `salvarResume` **ignora posição < 1s** (guardar 0 não
  serve pra nada: retomar em 0 é igual a não retomar).

**Hoje**: um registro só (`web/app/lib/resume.ts`) pros três motores, lido na carga do módulo
(antes de qualquer motor escrever), aplicado uma única vez e só se **a uri casar** e o
registro for **fresco** (10 min).

**(c) Autoplay: `Cmd+R` NÃO é gesto na página.** O atalho acontece no chrome do browser, então
o documento novo nasce sem ativação e `play()` é recusado (`NotAllowedError`) — a posição
voltava certa e a faixa ficava **parada**. Chrome às vezes libera (Media Engagement Index alto
para o site), o que faz o sintoma parecer intermitente e enganar o diagnóstico.
**(e) O bloqueio não se anuncia — e o deck mentia.** Medido: com o autoplay barrado o
`play()` **não rejeita**; o elemento fica `paused = false`, o evento `play` dispara, o
carregamento é adiado (`readyState` 0) e a promessa nunca se resolve. Detector por `catch`
não pega, e `playing` fica `true` — chegou a **mascarar** o estado numa primeira tentativa de
fix (`blocked && !playing` nunca era verdade). Hoje a detecção é **por progresso**: posição
parada por ~2,5s com a estação querendo tocar ⇒ `deck.blocked`. O deck passa a mostrar **▶**
em vez de ⏹ (que anunciava "no ar" sem som e, no clique natural de quem quer áudio, PARAVA a
estação), e esse ▶ força o play — `setWant(true)` sozinho não muda nada quando `wantPlay` já
é true.

**(f) Login aparecendo em modo single-user** (mesma sessão, causa vizinha): o segredo JWT de
dev é regerado a cada boot do servidor, então todo restart invalida o token guardado. A sonda
`/auth/me` ia **com o token velho**, levava 401, limpava o token e **não sondava de novo** —
caindo na tela de login que em single-user nem deveria existir. Fix: após o 401, limpar e
**re-sondar sem token**; se passar, é single-user e o console abre direto.

**O que foi TESTADO e NÃO funciona** (registrado pra ninguém tentar de novo): o truque
"autoplay mudo é sempre permitido" **não vale pra `<audio>`** — medido, `a.muted = true` +
`play()` devolve **`NotAllowedError` igual**. Ele vale pra `<video>`, por isso o IFrame do
YouTube consegue (e usa: mudo na posição certa, som no primeiro gesto). Web Audio não ajuda —
`AudioContext` nasce `suspended` pelo mesmo motivo.

**Fix possível**: re-tentar no primeiro clique/tecla, **em silêncio**. Uma barra "clique para
retomar o áudio" chegou a existir e foi **removida a pedido do operador**: aparecia a cada
refresh e virava ruído.

**Saída de verdade**: **instalar o app** (manifest PWA, `display: standalone`). O Chrome
concede **permissão de autoplay a apps instalados** — aí o refresh volta tocando sem clique
nenhum. O mesmo acontece por engagement acumulado no site (uso diário em produção), o que faz
o sintoma parecer intermitente entre máquinas e enganar o diagnóstico.

**(d) Um slot só de registro era disputado.** Duas abas no mesmo endereço compartilham
`localStorage`: a aba B, tocando outra faixa, sobrescrevia a posição da aba A a cada segundo —
e consumir uma retomada apagava o arquivo inteiro. Hoje é um **mapa `uri → posição`** (cap 8),
com releitura antes de cada escrita.

⚠️ **Ao testar depois de um build**: o PRIMEIRO refresh ainda roda o bundle ANTIGO (ele só
busca o HTML/JS novo nesse carregamento). O comportamento novo aparece do SEGUNDO refresh em
diante — dá pra perder um tempo bom achando que o fix não funcionou.

**Como reconhecer:** deck volta em `00:00` com transporte em `▶ RETOMAR` depois de um F5.
Conferir `localStorage.autodj.wantplay.v1` (0 = caiu na causa (a)) e
`localStorage.autodj.playout.v1` (pos 0 = caiu na armadilha do save-zero).

---

### DEV-10 — o gate de cobertura dizia "passed" sobre pacote que NÃO COMPILAVA 🧪
**Medido em 27/07/2026.**

`scripts/coverage-gate.sh` lia `go test ./... -cover 2>/dev/null` num process substitution e
casava linha a linha por `coverage:`. Pacote cujo **teste não compila** (ou falha) escreve no
**stderr** — descartado — e **não emite linha `coverage:` nenhuma**. Resultado: o pacote
simplesmente **sumia da lista** e o gate imprimia `coverage gate passed`. O status do
`go test` também não era visto (dentro de process substitution o `set -e`/`pipefail` não
alcança).

Aconteceu de verdade: a porta `inbound.LibraryService` ganhou os métodos de pastas
(`Folders`/`CreateFolder`/`DeleteFolder`/`SetTrackFolders`), o `fakeLib` do
`adapters/driving/httpapi` ficou para trás e **todo o pacote httpapi parou de compilar** —
com o gate verde. Só apareceu ao rodar `go test ./...` na mão.

**Fix**: rodar `go test ./... -cover` ANTES, com saída em arquivo (`>"$OUT" 2>&1`) e status
verificado (`if ! go test …; then` → aborta mostrando as linhas de erro); o laço passa a ler o
arquivo. **Como reconhecer**: gate verde + `go test ./...` vermelho, ou um pacote que você
sabe que existe não aparecer na listagem do gate.

**Regra**: silêncio não é aprovação. Toda checagem que decide por presença de linha precisa
verificar o **status** do comando que produziu as linhas.

---

### DEV-11 — O CI nunca tinha rodado, e escondia dois bugs só-de-Linux ✅

**Sintoma.** Ao publicar o repositório no GitHub, o CI reprovou duas vezes seguidas
em coisas que passavam na máquina do autor.

**Causa comum.** O `.github/workflows/ci.yml` existia há muitas versões, mas o
repositório vivia no GitLab — **o workflow nunca havia sido executado uma única
vez**. A primeira execução da história foi a da publicação, e ela encontrou dois
defeitos que existiam há ~45 versões:

1. **`mktemp -t autodj-cover` é BSD-only.** No macOS `-t` é prefixo; no GNU
   coreutils `-t` exige o template com `XXXXXX` e responde
   `mktemp: too few X's in template`. O `coverage-gate.sh` só tinha rodado no Mac.
   Fix: template explícito (`"${TMPDIR:-/tmp}/autodj-cover.XXXXXX"`), que serve aos
   dois.

2. **Cobertura dependente de corrida de tempo.** `adapters/driving/ws` media
   **92,0% no Mac e 80,0% no runner** — mesmo código, mesmo commit. O único teste
   que exercitava o `broadcast` esperava um tique de 20 ms e lia o que chegasse;
   o quanto ele alcançava dependia do escalonador. Fix: chamar os caminhos
   diretamente (serviço falhando, conexão morta que precisa ser descartada,
   requisição não-WebSocket) — 96,0%, estável inclusive com `GOMAXPROCS=1`.

**A lição que vale além destes dois.** Portão que nunca executou não é portão, é
decoração — e cobertura que muda de máquina não mede nada. Ao adotar CI num
projeto que já existe, esperar que as primeiras execuções **reprovem**: elas estão
medindo o que ninguém mediu antes.

## Aprendizados transversais (o que se repetiu)

1. **Sintoma intermitente = variável escondida.** "Só alguns vídeos somem" era origem IP +
   monetização; "só quando vem do Spotify" era gesto do usuário vs partida automática. Antes de
   teorizar, achar a variável e medir **A/B com tudo o mais idêntico** (mesmo vídeo, mesmo host,
   mesmo instante).
2. **Falha silenciosa custa mais que a falha.** O skip de faixa bloqueada estava **correto**;
   o defeito era não dizer por quê. Fix de vários bugs = manter a lógica e **explicar em voz alta**
   (banner + `console.warn`).
3. **Fix que "resolveu" sem causa-raiz provada volta.** O palco 320×320 (v0.2.3) parecia curar
   e não era a cura ([YT-2](#yt-2--reparent-do-iframe-recarrega-o-player-)). Sem prova
   instrumentada (evento `load` duplicado, tabela A/B), tratar como hipótese.
4. **Regressão em trio.** A v0.2.8 quebrou o playout com **três** mudanças que só juntas
   produziam o preto ([YT-5](#yt-5--esconder-o-overlay-parqueia-o-iframe--deck-preto-)).
   Ao reverter, restaurar a **semântica inteira** validada, não uma peça.
5. **Providers impõem regras contraditórias.** Spotify quer `127.0.0.1`, YouTube odeia IP —
   a solução foi separar **redirect** (provider) de **retorno** (browser), ver
   [SP-3](#sp-3--spotify-exige-loopback-ip-literal-no-redirect-).
6. **Ambiente de teste mente.** Automação não toca VEVO ([DEV-4](#dev-4--browser-de-automação-não-toca-clipe-monetizado-));
   embed top-level sempre dá 153 ([YT-3](#yt-3--erro-153-referer-obrigatório-desde-dez2025-)).
   Saber o que o ambiente **consegue** provar antes de concluir.
7. **"Sumiu depois que eu fiz X" quase nunca é X.** Refresh não apagava nada — quem apagava era
   o restart do servidor em modo `memory` ([DEV-6](#dev-6--dou-refresh-e-some-tudo-task-serve-era-efêmero-)),
   e o refresh era só quando a perda ficava visível. Mesma armadilha em
   [DEV-5](#dev-5--feature-nova-não-aparece-binário-velho-servindo-bundle-novo-) e
   [DEV-7](#dev-7--deploy-invisível-html-sem-cache-control-): a causa é anterior ao gesto
   relatado. **Reproduzir isolando o gesto** (fiz só o refresh — sumiu?) antes de aceitar a
   narrativa.
8. **`go build` verde não prova nada de integração.** URL errada
   ([DEV-9](#dev-9--apibase-devolve-só-a-origem-não-o-prefixo-da-api-)), campo faltando no JSON
   ([DEV-5](#dev-5--feature-nova-não-aparece-binário-velho-servindo-bundle-novo-)) e header
   ausente ([DEV-7](#dev-7--deploy-invisível-html-sem-cache-control-)) compilam perfeitamente e
   falham **em silêncio**. Todo fix desta série só apareceu ao medir o artefato de verdade:
   `curl -I` no header, o JSON servido, o chunk carregado no browser.
9. **Status vindo do provider medido do SERVIDOR não descreve o browser do usuário.**
   `playabilityStatus: UNPLAYABLE` num vídeo que toca normal
   ([YT-11](#yt-11--youtube-100-sem-api-key-o-que-dá-e-o-que-não-dá-)) e
   `status.embeddable: true` num que não toca ([YT-1](#yt-1--origem-ip-bloqueia-embed-de-clipe-monetizado-))
   são o mesmo erro em direções opostas: IP, sessão e origem do servidor não são os do operador.
10. **Gate herdado vira gate decorativo.** O login protegia a busca cara; quando o caminho barato
    apareceu, o gate continuou no lugar sem proteger nada
    ([YT-10](#yt-10--buscava-deslogado-mas-arrastar-pro-deck-pedia-conta-)). Pergunta de triagem:
    *este endpoint usa a credencial pra alguma coisa?* Se o adapter descarta o argumento, é
    fricção pura.
