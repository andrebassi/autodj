"use client";

/* RETOMADA PÓS-REFRESH — um registro só, para os TRÊS motores (v0.2.26).

   O problema: dar F5 reiniciava a música do 00:00. Playout roda no browser, então
   recarregar a página destrói o `<audio>`, o IFrame do YouTube e o device do SDK do
   Spotify. Quem estava no ar voltava do zero — e num tocador de rádio isso é o
   equivalente a alguém tropeçar no cabo.

   Existia resume, mas só o Spotify tinha (as funções moravam DENTRO do
   `useSpotifyPlayout`). Local e YouTube não guardavam nada. Recurso que vale pra um
   provider vale pra todos: a regra mora aqui, na camada uniforme, e cada motor só
   faz o seu gesto de posicionar (`currentTime` / `seekTo` / `startSeconds`).

   UM registro só porque só existe UMA faixa no ar. É também o que evita um motor
   apagar o registro do outro — a `clearResume()` incondicional do Spotify fazia
   exatamente isso: rodava sempre que o now-playing NÃO era Spotify e limpava o que
   o motor local teria acabado de salvar.

   Aplicar exige TRÊS condições, e cada uma existe por um caso concreto:
   - **a uri tem de casar** com o now-playing — senão retomaria a posição de uma
     faixa na outra;
   - **frescor** (`MAX_IDADE_MS`): voltar depois do almoço e cair no meio da música
     não é retomada, é confusão. Refresh leva segundos;
   - **consumo único**: vale pra 1ª carga depois do boot. Sem isso, a mesma faixa
     entrando de novo mais tarde (repeat, re-enfileirada) pularia pro meio. */

const KEY = "autodj.playout.v1";

type Rec = { uri: string; pos: number; at: number; playing: boolean };

/* ⚠️ REGISTRO POR FAIXA, não um slot único (correção 0.2.26).

   A primeira versão guardava UM registro: `{uri,pos,...}`. Parece suficiente — só
   existe uma faixa no ar — mas na prática o slot era disputado e a retomada
   sumia sem deixar rastro:

   - **duas abas no mesmo endereço** (situação normal: o operador abre uma segunda
     aba, ou deixa o app aberto no notebook e no desktop do mesmo browser)
     compartilham `localStorage`. A aba B, tocando outra faixa, sobrescrevia a
     posição da aba A a cada segundo. A refresh na aba A não achava mais a uri dela;
   - **consumir apagava TUDO**: `resumeParaFaixa` chamava `limparResume()`, então
     retomar uma faixa jogava fora a posição guardada de qualquer outra.

   Com um mapa `uri → registro` cada motor procura a SUA faixa, e ninguém pisa no
   registro do vizinho. O cap existe só pra não crescer sem limite. */
const MAX_ENTRIES = 8;

/** Janela em que uma retomada ainda faz sentido. Refresh leva segundos; 10 min
 *  cobre também reinício do servidor, sem alcançar "voltei horas depois". */
const MAX_AGE_MS = 10 * 60 * 1000;

/** Teto do avanço por tempo decorrido. A faixa NÃO tocou durante o recarregamento,
 *  mas a agenda é wall-clock, então descontamos o tempo do refresh pra não repetir
 *  áudio. Só que somar o intervalo INTEIRO seria errado quando a aba passou muito
 *  tempo fechada — nesse caso o registro está velho e o certo é retomar onde parou,
 *  não projetar minutos de música que ninguém ouviu. */
const MAX_CATCHUP_MS = 15 * 1000;

/** Margem do fim: retomar a 1s do fim só faria a fila avançar na hora. */
const END_MARGIN_MS = 3000;

function readAll(): Rec[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return [];
    const dados = JSON.parse(raw) as unknown;
    // aceita o formato ANTIGO (registro único) pra não perder a posição de quem
    // atualizou o app no meio de uma faixa.
    const lista: Rec[] = Array.isArray(dados) ? (dados as Rec[]) : [dados as Rec];
    return lista.filter(
      (r) => r && typeof r.uri === "string" && r.uri !== "" && typeof r.pos === "number",
    );
  } catch {
    return [];
  }
}

/* Lidos na CARGA DO MÓDULO, antes de qualquer motor montar e começar a salvar por
   cima. Se cada hook lesse no seu mount, o primeiro a escrever apagaria a posição
   que os outros ainda não tinham lido. */
const AT_BOOT = readAll();
const consumed = new Set<string>();

function pendingFor(uri: string): number {
  if (!uri || consumed.has(uri)) return 0;
  const r = AT_BOOT.find((x) => x.uri === uri);
  if (!r || typeof r.at !== "number") return 0;
  const age = Date.now() - r.at;
  if (age < 0 || age > MAX_AGE_MS) return 0;
  const catchUp = r.playing ? Math.min(age, MAX_CATCHUP_MS) : 0;
  return Math.max(0, r.pos + catchUp);
}

/** Posição (ms) pra retomar `uri`, ou 0. Consome só ESTA uri: a 2ª chamada pra ela
 *  devolve 0, e as outras faixas seguem com o registro delas intacto.
 *  `durMs` (quando conhecido) evita retomar em cima do fim da faixa. */
export function resumeFor(uri: string, durMs = 0): number {
  const pos = pendingFor(uri);
  if (pos <= 0) return 0;
  consumed.add(uri);
  if (durMs > 0 && pos > durMs - END_MARGIN_MS) return 0;
  return pos;
}

/** true se ainda HÁ uma retomada pendente pra esta uri (não consome). Serve pros
 *  motores decidirem, ANTES de carregar, que a entrada é retomada e não faixa nova
 *  — retomada vai a volume pleno, sem fade-in. */
export function hasResume(uri: string): boolean {
  return pendingFor(uri) > 0;
}

/** Persiste a posição do que está no ar. Chamar à vontade: escreve no máximo 1x/s
 *  (o custo é `JSON.stringify` + localStorage síncrono a cada `timeupdate`, que no
 *  `<audio>` dispara ~4x/s). O preço do throttle é retomar até 1s atrás — melhor
 *  que pular 1s à frente. */
let lastSec = -1;
let lastUri = "";
export function saveResume(uri: string, posMs: number, playing: boolean): void {
  if (typeof window === "undefined" || !uri) return;
  /* POSIÇÃO ~ZERO NÃO SOBRESCREVE. Sem isto a retomada se destrói sozinha: ao
     recarregar, o motor monta com a faixa em 00:00 (autoplay bloqueado, ou estação
     parada esperando PLAY) e o seu tick de posição salvava `0` POR CIMA do valor
     bom — o operador então dava PLAY e a música começava do início, mesmo com o
     registro tendo sido salvo direitinho antes do refresh.
     Guardar 0 também não serve pra nada: retomar em 0 é igual a não retomar. */
  if (posMs < 1000) return;
  const sec = Math.floor(posMs / 1000);
  if (sec === lastSec && uri === lastUri) return;
  lastSec = sec;
  lastUri = uri;
  try {
    // relê a cada escrita: OUTRA ABA pode ter gravado no meio, e ler-modificar-
    // escrever por cima de um estado velho é justamente o que fazia a posição
    // sumir. Custa um parse por segundo.
    const lista = readAll().filter((r) => r.uri !== uri);
    lista.unshift({ uri, pos: posMs, at: Date.now(), playing });
    window.localStorage.setItem(KEY, JSON.stringify(lista.slice(0, MAX_ENTRIES)));
  } catch {
    /* quota/modo privado — retomada é conforto, nunca pode quebrar o playout */
  }
}

/** Esquece a posição de UMA faixa (ou de todas, sem argumento). Use ao PARAR de
 *  propósito: aí a próxima carga começa do zero, que é o que o operador espera
 *  depois de parar a estação. */
export function clearResume(uri?: string): void {
  if (typeof window === "undefined") return;
  lastSec = -1;
  lastUri = "";
  try {
    if (!uri) {
      window.localStorage.removeItem(KEY);
      return;
    }
    const lista = readAll().filter((r) => r.uri !== uri);
    window.localStorage.setItem(KEY, JSON.stringify(lista));
  } catch {
    /* ignora */
  }
}
