"use client";

import { trackAudioURL } from "@/lib/api";

/* ÁUDIO LOCAL EM BUFFER — sobreviver a queda de internet.

   O pedido do operador foi "guardar as duas faixas do deck pra aguentar a queda e
   continuar de onde parou". A ideia está certa; o **localStorage não serve**:
   guarda texto (um MP3 vira base64, +33%), tem teto de ~5 MB por origem e é
   SÍNCRONO — escrever 5 MB ali trava a thread que desenha o deck. Duas faixas nem
   caberiam.

   O que serve é baixar a faixa inteira como **Blob** e apontar o `<audio>` pra um
   object URL. A partir daí o áudio mora na memória do browser: o servidor pode
   sumir que a música continua, e o seek funciona igual (blob é buffer completo,
   não stream).

   Onde vale a pena, e onde NÃO vale:
   - **Faixa ENGATILHADA (a próxima)**: vale sempre. Ela tem a faixa inteira de
     tempo pra baixar e é a que morre numa queda — nunca foi pedida ao servidor.
   - **Faixa NO AR**: continua em streaming. Trocar pra blob adiaria o início até
     o arquivo inteiro chegar (num link lento, silêncio no ar). O browser já
     bufferiza à frente enquanto toca, então ela aguenta a queda por conta própria.

   Teto de memória: 2 faixas por vez (a no ar e a próxima). MP3 de rádio tem 3–10
   MB; manter mais que isso é ocupar RAM à toa num app que já roda IFrame e SDK.
*/

/** quantas faixas ficam em memória: a do ar + as 3 próximas.
 *  3 porque cobre ~12 min de queda (faixa média de 3–4 min) por ~10 MB — o custo
 *  some perto do que a aba já usa, e passar disso vira download competindo com o
 *  áudio que está tocando (o browser abre 6 conexões por host). */
const MAX_BUFFERED = 4;

/** Quantas faixas ADIANTE ficam guardadas (fora a que está no ar). Exportado
 *  porque a mensagem de "sem servidor" cita esse número — derivar daqui impede a
 *  frase de mentir se a profundidade mudar. */
export const BUFFERED_AHEAD = MAX_BUFFERED - 1;

/** nome do cache em DISCO. Diferente da memória, ele sobrevive ao F5 — é o que
 *  evita rebaixar as mesmas faixas a cada recarga da página. */
const CACHE_NAME = "autodj-audio-v1";

/** teto por arquivo. FLAC de 50 MB não entra em RAM — segue em streaming. */
const MAX_BYTES = 24 * 1024 * 1024;

/* GUARDAMOS O BLOB, NÃO A URL — e a URL é criada na hora do uso.

   A primeira versão guardava o object URL pronto e o revogava ao expulsar a faixa
   do cache. Deu ruim na prática: uma URL revogada continua sendo uma string
   perfeitamente válida, e o `<audio>` só descobre que ela morreu ao tentar
   carregar — o operador via "Não foi possível carregar o áudio desta faixa" numa
   faixa que estava ali, íntegra, no servidor. Blob é o dado; URL é um ponteiro
   descartável. */
type Entry = { trackId: string; blob: Blob; url: string | null };

const buffered: Entry[] = [];
const inFlight = new Map<string, Promise<Blob | null>>();

function evictBeyondLimit(keep: string[]): void {
  for (let i = buffered.length - 1; i >= 0 && buffered.length > MAX_BUFFERED; i--) {
    const e = buffered[i];
    if (keep.includes(e.trackId)) continue;
    if (e.url) URL.revokeObjectURL(e.url);
    buffered.splice(i, 1); // o Blob some com a entrada; o GC recolhe
  }
}

/** URL tocável desta faixa, ou "" se ela não está bufferizada. Não baixa nada.
 *  Cria a URL no momento do uso e reaproveita a mesma enquanto a entrada viver. */
export function bufferedURL(trackId: string): string {
  const e = buffered.find((x) => x.trackId === trackId);
  if (!e) return "";
  if (!e.url) e.url = URL.createObjectURL(e.blob);
  return e.url;
}

/** Baixa a faixa inteira em background e guarda como object URL.
 *  Idempotente e single-flight: chamar a cada snapshot do WS é seguro.
 *  Falha (rede/404/arquivo grande) NÃO é erro do playout — apenas não bufferiza,
 *  e o `<audio>` segue no streaming de sempre. */
export async function bufferTrack(trackId: string, keepAlso: string[] = []): Promise<string | null> {
  if (!trackId || typeof window === "undefined") return null;
  if (buffered.some((e) => e.trackId === trackId)) return bufferedURL(trackId);
  const running = inFlight.get(trackId);
  if (running) {
    const blob = await running;
    return blob ? bufferedURL(trackId) : null;
  }

  const task = (async (): Promise<Blob | null> => {
    try {
      const url = trackAudioURL(trackId);

      /* DISCO PRIMEIRO. O Cache Storage sobrevive ao F5 (Blob não) e à falta de
         internet, então uma faixa já baixada nunca é rebaixada — e continua
         disponível se o servidor sumir entre uma sessão e outra. */
      const cached = await fromCache(url);
      if (cached) {
        buffered.push({ trackId, blob: cached, url: null });
        evictBeyondLimit([trackId, ...keepAlso]);
        return cached;
      }

      // `fetch` passa pelo wrapper de sessão (header X-AutoDJ-Session), então a
      // faixa vem do tenant certo — o mesmo motivo pelo qual a URL do <audio>
      // carrega `?sid=`.
      const res = await fetch(url);
      if (!res.ok) return null;
      const len = Number(res.headers.get("content-length") ?? "0");
      if (len > MAX_BYTES) return null;
      void toCache(url, res.clone()); // guarda em disco pra próxima sessão
      const blob = await res.blob();
      if (blob.size === 0 || blob.size > MAX_BYTES) return null;
      buffered.push({ trackId, blob, url: null });
      evictBeyondLimit([trackId, ...keepAlso]);
      return blob;
    } catch {
      return null; // offline/abortado: segue streaming, sem barulho
    } finally {
      inFlight.delete(trackId);
    }
  })();

  inFlight.set(trackId, task);
  const blob = await task;
  return blob ? bufferedURL(trackId) : null;
}

/** Libera tudo que não estiver nesta lista (troca de estação / Limpar). */
export function keepOnly(trackIds: string[]): void {
  for (let i = buffered.length - 1; i >= 0; i--) {
    if (trackIds.includes(buffered[i].trackId)) continue;
    const e = buffered[i];
    if (e.url) URL.revokeObjectURL(e.url);
    buffered.splice(i, 1);
  }
}

/* ---- disco (Cache Storage) ------------------------------------------------
   Por que Cache Storage e não IndexedDB: a chave natural aqui é a URL, o valor é
   uma Response, e é isso que a API guarda nativamente — sem serializar nada. E é
   a MESMA prateleira que o Service Worker enxerga, então o áudio já baixado
   também serve pra uma sessão aberta offline. */

async function fromCache(url: string): Promise<Blob | null> {
  try {
    if (typeof caches === "undefined") return null;
    const c = await caches.open(CACHE_NAME);
    const hit = await c.match(url);
    if (!hit || !hit.ok) return null;
    const blob = await hit.blob();
    return blob.size > 0 ? blob : null;
  } catch {
    return null; // modo privado / cota: segue pela rede, sem barulho
  }
}

async function toCache(url: string, res: Response): Promise<void> {
  try {
    if (typeof caches === "undefined") return;
    const c = await caches.open(CACHE_NAME);
    await c.put(url, res);
  } catch {
    /* cota estourada ou navegação privada — o playout não depende disto */
  }
}

/** Apaga o áudio guardado em disco (usado pelo Limpar). */
export async function clearAudioCache(): Promise<void> {
  try {
    if (typeof caches !== "undefined") await caches.delete(CACHE_NAME);
  } catch {
    /* ignora */
  }
}

/** Só para diagnóstico/teste: o que está em memória agora. */
export function bufferedStats(): { count: number; bytes: number; ids: string[] } {
  return {
    count: buffered.length,
    bytes: buffered.reduce((s, e) => s + e.blob.size, 0),
    ids: buffered.map((e) => e.trackId),
  };
}
