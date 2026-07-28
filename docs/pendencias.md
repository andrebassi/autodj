# Pendências e limites conhecidos

O que **não** está de ponta a ponta no AutoDJ, com o estado honesto de cada item.
Complementa dois vizinhos, e a divisão importa:

- [`known-issues.md`](known-issues.md) — bugs com **causa-raiz provada** e fix aplicado.
- [`../CLAUDE.md`](../CLAUDE.md) — como o que **existe** funciona.
- **este arquivo** — o que falta, o que é parcial e o que é impossível por construção.

Legenda: 🔴 pedido e não feito · 🟡 feito pela metade (funciona com limite) ·
⚫ impossível/fora de escopo por decisão ou plataforma.

Última revisão: **2026-07-28** (v0.2.29 na `develop`; **produção em v0.2.26**).

---

## 🔴 Pedido pelo operador e ainda NÃO implementado

*(nada em aberto no momento — trim, ganho constante e entrada do vocal saíram na 0.2.28;
remover o vocal, na 0.2.29)*

---

## 🟡 Funciona, mas com limite conhecido

### Waveform do deck e do editor ainda é SINTÉTICA
`adapters/driven/waveform/placeholder.go` gera picos determinísticos por id — inclusive
em produção. A onda "anda" com a posição real, mas o desenho **não é o áudio**.

O caminho está pronto e barato: a mesma passada de `ffmpeg` da 0.2.28 já produz RMS por
janela de 200ms (é dela que sai a detecção de vocal). Trocar o placeholder por picos
reais é aproveitar um número que já está sendo calculado e jogado fora. **Não foi feito
porque o operador não pediu** — é o próximo passo óbvio, não uma dívida escondida.

### Ganho por faixa só ATENUA de verdade (~+1,9 dB de reforço)
`<audio>.volume` não passa de 1 e a referência com normalização ligada é 0.8, então
sobra pouco mais de um decibel e meio para levantar. Faixa gravada muito abaixo do alvo
continua mais baixa que as outras; abaixar não tem limite.

**Consequência prática, medida (2026-07-28)** — o teto de reforço é
`20·log10(1/0.8) = 1,94 dB`, então **só normaliza faixa com loudness ≥ (alvo − 1,94)**;
mais baixa que isso, o ganho grampeia em 1.0 e ela sai no nível dela mesma. No alvo
padrão (89 ≈ −13,3 LUFS) a janela é **≥ −15,3 LUFS**: cobre master moderno (−6 a −14),
**não** cobre rip antigo de −18/−20.

O **botão "Nível alvo" é a saída** dentro do limite atual — baixar o alvo traz mais do
acervo para a faixa onde a atenuação (ilimitada) faz o trabalho. Medido com quatro
arquivos entre −12,6 e −18,9 LUFS:

| alvo (⚙) | LUFS alvo | saída percebida |
|---|---|---|
| 89 (padrão) | −13,3 | −15,3 / −16,5 / −18,9 / −18,9 — **só a mais alta normalizou** |
| 33 | −20,0 | −22,0 / −22,0 / −22,0 / −22,0 — **todas iguais** |

Ou seja: acervo de master moderno funciona no padrão; acervo misto pede alvo mais
baixo. O ⚙ mostra o LUFS resultante ao lado do número justamente para isso.

Resolver exige **Web Audio** (`GainNode`), e aí esbarra no motivo pelo qual ele não foi
usado: o `AudioContext` nasce `suspended` e exige gesto do usuário. Com o histórico de
autoplay barrado deste app (0.2.26/0.2.27), rotear o áudio por um nó que pode não
destravar arrisca deixar a estação **muda até alguém clicar** — regressão pior que o
limite atual. Ver `web/app/lib/gain.ts`.

### O modo VOZ exige PREPARO — e o cancelamento de centro foi aposentado

O `🚫🎤 VOZ` (0.2.29) nasceu como **cancelamento de centro** (subtrai um canal do
outro, apagando o que está no meio da imagem estéreo). Na **0.2.32 esse caminho saiu
inteiro**, porque a medição em música real mostrou que ele não entrega o que o botão
promete.

Medido com o próprio Demucs como régua ("quanta voz sobra em cada saída"), num trecho
de 40s de *Ace Of Base — All That She Wants*:

| caminho | voz restante | redução |
|---|---|---|
| original (referência) | −17,3 dB | — |
| cancelamento de centro | −19,3 dB | **2,0 dB** |
| instrumental (IA) | −49,2 dB | **31,9 dB** |

Os −24,3 dB que estavam registrados aqui vinham de um **mix sintético** com o vocal
perfeitamente centrado — um caso construído para o filtro acertar. Em música real, voz
duplicada/alargada, com reverb e efeitos, não é um ponto no centro, e o filtro quase
não a alcança.

O que sobra, e é o limite honesto de hoje:

- **VOZ só liga em faixa PREPARADA.** Sem o instrumental gerado, o botão fica apagado
  com o caminho no `title` (menu da faixa → 🎼 Preparar instrumental). Não existe mais
  um plano B instantâneo — e é melhor assim que um efeito de 2 dB fingindo ser um.
- **Preparar leva minutos** (~6 min para uma música de 4; ver o bloco da IA abaixo), e
  a ferramenta é **opt-in e local** — prod não tem.
- **Mono deixou de ser caso especial**: o modelo separa por conteúdo.
- **Não reescreve o arquivo** — o instrumental é um arquivo novo, ao lado, e desligar o
  botão devolve o original na hora.

### Separação por IA — o que foi pesquisado (e a correção de 28/07)

⚠️ **A justificativa anterior registrada aqui estava ERRADA em dois fatos.** Dizia que
`onnxruntime-web` exige COOP/COEP (o que quebraria YouTube + Spotify) e que o modelo
teria ~172 MB. Pesquisado na fonte:

- **Só o WASM MULTI-THREAD precisa de `crossOriginIsolated`.** Single-thread roda sem
  header nenhum; o backend **WebGPU não tem esse requisito** documentado
  ([ONNX Runtime Web](https://onnxruntime.ai/docs/tutorials/web/env-flags-and-session-options.html)).
  E o isolamento é **por documento**, não por origem — uma página isolada à parte não
  afetaria o player.
- **O peso é 81 MB** (`htdemucs` em ggml; 6-source: 53 MB).
- **`demucs.cpp`** é C++17 + Eigen header-only, **MIT**, sem PyTorch — então "não existe
  implementação Go" era verdade irrelevante: cgo chama direto.

**O impedimento real é VELOCIDADE.** Benchmark do autor (4 min de música, Ryzen 5950X de
16 núcleos): **10m23s** single-thread, **4m9s** com 4 threads — ~2,5× mais lento que
tempo real num desktop parrudo. Prod é `shared-cpu-1x` com 1 GB.

**Logo: IA não pode ser um botão ao vivo.** É **pré-render por faixa**, feito uma vez e
guardado — outra promessa ao operador ("prepare esta música" ≠ "tire a voz agora"). É
também o único caminho que remove voz de verdade (ver a tabela acima), e desde a 0.2.32
é o único que o botão VOZ oferece.

Qualidade, pra calibrar: HTDemucs ≈ **9,0 dB** de SDR no vocal, Mel-Band RoFormer um
pouco acima, MDX-Net ≈ 8,5 dB e mais rápido. Tempo real de verdade ainda é pesquisa
([RT-STT](https://arxiv.org/abs/2511.13146), 383K parâmetros, 23 ms de latência, mas
**5,56 dB** e **sem pesos públicos**). Rota paga: Moises/Music.ai ≈ US$ 0,04–0,10/min,
LALAL.AI ≈ US$ 0,067–0,11/min.

**Status: IMPLEMENTADO na 0.2.31** — como pré-render por faixa
(`Menu da faixa → 🎼 Preparar instrumental (IA)`), via `demucs.cpp` (MIT) rodando
como processo à parte. Ver `docs/stories/10.5.separacao-por-ia.md`.

Três limites que continuam de pé:

- **Só na máquina do operador.** É opt-in (`scripts/build-demucs.sh`) e **não está
  em produção**: a VM é `shared-cpu-1x` com 1 GB e a separação leva ~1,4× o tempo
  real. Sem a ferramenta, a função some da UI.
- **Leva minutos.** Medido no M1 Pro com 6 threads: 45s de áudio em **64,8s**, ou
  ~6 min para uma música de 4. Nunca será um botão ao vivo — é preparo.
- **⚠️ A QUALIDADE NÃO FOI MEDIDA COM MÚSICA REAL.** O encanamento está provado
  ponta a ponta (pedido → separação → arquivo → deck tocando), o RESULTADO não: os
  arquivos de teste são senoides puras, e o Demucs foi treinado em música — ele não
  reconhece um seno de 1400 Hz como voz. As bandas caíram uniformemente ~3 dB, que
  é mudança de nível, não separação. **Isto é pendência real**, não formalidade.

### Trim não reescreve o arquivo
O corte de silêncio é **só de reprodução** (cue in/out), nunca destrutivo. É uma
escolha, não uma limitação: desligar o interruptor devolve a faixa inteira na próxima
música, sem reprocessar nada.

### Offline: o estoque é de 3 faixas
`BUFFERED_AHEAD = 3` (`web/app/lib/audioBuffer.ts`). Passada essa janela a fila
**continua andando** (avanço no espelho), mas cai numa faixa que não está no cache e
o áudio não toca — aparece "Esta faixa não está no cache local". Aumentar é mudar
uma constante; guardar a biblioteca inteira (medido: **83 faixas, 253 MB**) exige
opção explícita com barra de progresso e teto, senão vira download surpresa em rede
móvel.

### Dívida de avanços offline não valida o outro lado
`autodj.pendingnext.v1` conta quantos `next` aconteceram offline e, na volta, chama
`next` a mesma quantidade de vezes. Assume que a fila do servidor **não mudou** no
intervalo — verdade no uso caseiro (uma aba), falso se outra aba/dispositivo mexeu na
mesma estação. Não há reconciliação por id de faixa.

### Cache de áudio em disco não é limpo por ninguém
`clearAudioCache()` e `keepOnly()` existem em `lib/audioBuffer.ts` e **não estão
fiados no botão Limpar** — o Cache Storage cresce até o teto do browser (Chrome dá
~60% do disco livre por origem, então na prática não trava, mas fica lixo). Falta
chamar no `clearAll` do `page.tsx` e, idealmente, um LRU por faixa tocada.

### Service Worker: versão da casca é bump manual
`SHELL_REV` em `web/public/sw.js` é constante escrita à mão. A atualização de app
funciona sozinha (documento em rede-primeiro + bundles com hash + limpeza da casca no
`activate`), mas **mudança de estratégia** do próprio SW depende de lembrar de
incrementar. Injetar a versão do build no arquivo resolveria.

### Primeira visita offline não existe
Óbvio, mas registrado pra não virar bug report: o Service Worker só serve o que já
passou por ele uma vez. Um browser que **nunca** abriu o app online não tem casca em
cache.

### Artwork: cache só em memória
`adapters/driving/httpapi/artworkcache.go` guarda as capas extraídas em RAM (32 MB,
com cache negativo). **Reiniciar o servidor esvazia** — a primeira pintura da lista
volta a rodar ffmpeg (medido: 40 capas frias ≈ 3,4s). Um cache em disco ao lado do
media store eliminaria isso de vez.

### STOP mestre com AUTO ligado — verificar na mão
O caminho está no código e é curto (`onStopMaster` → `toggleForTransport(false)`, que
NÃO grava preferência), e a preferência ficou intacta em todas as tentativas. Mas eu
**não consegui reproduzir o STOP no browser automatizado**: o clique real erra o
botão (a página tem 2560px e o screenshot 1568 — escala 0.613 num alvo pequeno) e o
clique sintético não vale como gesto, então o deck volta a ▶ pelo detector de
autoplay bloqueado. Vale um teste manual: com AUTO ligado, apertar ⏹ deve parar a
estação e apagar o AUTO — e a sessão seguinte deve voltar com AUTO ligado.

### Sem testes automatizados no frontend
`web/package.json` **não tem test runner**. Toda a lógica nova de playout (retomada,
buffer offline, sonda de áudio, Service Worker, mixagem) é validada por **medição no
browser** — que pegou coisas que teste unitário não pegaria (autoplay que não rejeita,
URL de blob revogada), mas não protege contra regressão. As partes puras
(`lib/version.ts`, `lib/resume.ts`, `lib/ytLinks.ts`) são testáveis e ainda não têm
teste.

### Cobertura: `cmd/*` e `postgres` fora do gate
Por decisão (integration-tier). O gate cobre domínio 100% e adapters/app ≥90%.

### Erro de hidratação do React (#418) no console
Aparece no boot (`Minified React error #418`). É **real e não resolvido**; investigado
durante a caça a bugs de áudio e descartado como causa daqueles. Não tem efeito
observado na operação, mas é ruído que esconde erro de verdade no console.

### Pastas: sem renomear
Criar, excluir e atribuir existem. **Renomear não** — o jeito atual é criar a nova,
mover as faixas e excluir a antiga.

---

### Caminhos absolutos no banco impedem mover o diretório de dados

`Track.Path` é gravado **absoluto** (`/root/.autodj/media/<estação>/arquivo.mp3`),
então o diretório de dados não pode mudar de lugar sem reescrever o caminho em
TODOS os bancos de TODAS as estações. Isso já **quebrou produção uma vez**: ao
trocar o container para usuário sem privilégio (0.2.42), o volume foi movido junto
e nenhuma música tocava — a biblioteca listava e o áudio dava 403 (ver `DEP-1`).

Hoje o container roda sem root **com os dados ainda em `/root/.autodj`**, resolvido
com `chmod o+x /root`. Funciona e é seguro; é só feio.

**Por que não foi feito:** é migração de dado de usuário, sem nenhum ganho
funcional — ninguém percebe a diferença. E o `id` da faixa é `sha1(caminho)`, então
qualquer varredura futura do mesmo arquivo num caminho novo geraria id diferente e
duplicaria a faixa. Entra sozinho, com backup do volume antes, ou não entra.

### Separação na nuvem: sem teto de gasto automático

`task stems:cloud` **estima** o custo e pede confirmação antes do lote, mas não há
corte automático no meio (AC 6 da [story 10.6](stories/10.6.separacao-na-nuvem.md)).
O freio existente é o **watchdog de ociosidade** — **2 min** sem faixa nova derruba
o pod (era 8; ver `CLAUDE.md` para a conta), com a morte confirmada por consulta —,
validado em produção, mas ele exige o pod estar PARADO. Um lote que entre em laço de retentativa não tem freio.

⚠️ **Produção passou a poder gastar sozinha** (v0.2.38): com `AUTODJ_RUNPOD_API_KEY`
configurada, qualquer clique em processar aluga GPU. Medido: **US$ 0,035** por um
lote de 3 faixas, com o pod derrubado sozinho ao fim.

### Mixagem entre motores diferentes: não medida depois da 0.2.39

As correções de `PLY-1`/`PLY-2` foram validadas em **local↔local**, com o SEM-VOZ
ligado e desligado. As combinações com Spotify e YouTube **não passam por esse
código** (só o local usa o elemento ocioso), mas isso é raciocínio sobre o código,
não medição. Se aparecer estranheza numa virada entre motores diferentes, medir
aquele caminho antes de concluir.

### Qualidade da separação: medida uma vez, em uma música

Os **−31,9 dB** de redução de voz vêm de UM trecho de UMA faixa (0.2.32). Vale como
ordem de grandeza, não como média do acervo — vocal duplicado, coro e gravação
antiga podem se comportar diferente.

## ⚫ Impossível ou fora de escopo por decisão

### Mixagem spotify↔spotify
Uma conta Spotify tem **um** playback ativo: pôr a próxima no ar mata a atual. Não é
limitação nossa.

### Mixagem youtube↔youtube
As duas instâncias do IFrame existem, mas o `startMix` do motor dá `standby()` na que
está no ar (assume que a faixa de saída é de outro motor) e o `fadeOut` mira sempre a
"ao vivo" — que depois do swap já é a entrante. Exigiria **ramp por instância**, como
o motor local ganhou. Enquanto isso, esse par fica em corte seco.

### Offline não ajuda Spotify nem YouTube
São 100% online por natureza (DRM e embed). Sem servidor/rede eles não tocam, e o
reload da volta continua valendo pra eles — é o que recupera token expirado e IFrame
órfão. Por isso os chips ficam apagados sem enlace.

### Autoplay com som sem interação
Política do browser. Medido: em `<audio>` nem o truque do "autoplay mudo" funciona
(`NotAllowedError` igual). A saída legítima é **instalar o app** (manifest PWA já
existe) — o Chrome concede autoplay a apps instalados — ou o engagement acumulado do
site. Ver `known-issues.md` PLY-1.

### Áudio em BLOB no SQLite
Avaliado e recusado (ver `CLAUDE.md`): 4 dos 5 consumidores de `Track.Path` são
processos externos que exigem caminho de arquivo, e o driver Go puro materializaria a
faixa inteira em RAM.

### Multiusuário sem interface
O backend de auth (JWT, argon2id, RBAC, `AUTODJ_SINGLE_USER=0`) está inteiro e é
mantido de propósito — produção é URL pública e ele é a única alavanca se um dia for
preciso fechar o acesso. A **UI** de login foi removida: com `AUTODJ_SINGLE_USER=0` o
app mostra `ServerLoginRequired` e não tem como autenticar.

---

## Estado de release

| | versão | onde |
|---|---|---|
| Produção | **v0.2.26** | `autodj.andrebassi.com.br`, tag `stable` |
| `develop` | v0.2.27 (não deployado) | offline-first, mixagem local, deck ▶ quando bloqueado |

Deploy só com autorização explícita (regra inviolável do `CLAUDE.md`), e todo deploy
move a tag `stable`.
