"use client";

/* "O áudio está mesmo saindo agora?" — sonda registrada pelo motor de playout e
   lida por quem precisa decidir sem ter acesso ao elemento.

   Existe por um caso concreto: quando a internet volta, o app RECARREGA pra
   retomar o áudio (v0.2.21). Só que faixa local bufferizada (`audioBuffer.ts`)
   atravessa a queda TOCANDO — e aí o reload cortaria justamente o que sobreviveu.
   Sem uma sonda, o hook de conectividade não teria como saber a diferença: ele vê
   socket, não som.

   Fica em módulo (e não em contexto React) porque o consumidor é um efeito que
   roda fora da árvore de render e não pode depender de ordem de montagem. */

type Probe = () => boolean;

let probe: Probe | null = null;

/** O motor de playout registra como responder "estou tocando?". */
export function setAudioAliveProbe(fn: Probe | null): void {
  probe = fn;
}

/** true se há som saindo AGORA. Sem sonda registrada, assume que não. */
export function isAudioAlive(): boolean {
  try {
    return !!probe?.();
  } catch {
    return false;
  }
}

/* "A ESTAÇÃO ESTÁ TOCANDO AGORA?" — publicado por `page.tsx`, que é quem escolhe o
   motor ao vivo entre os três, e lido por quem precisa decidir se pode interromper.

   ⚠️ NÃO CONFUNDIR COM `isAudioAlive` acima, e a diferença é o motivo de existirem
   duas sondas em vez de uma. Aquela responde "o elemento <audio> LOCAL está mesmo
   produzindo som?" e é registrada SÓ pelo motor local, de propósito: quem a lê é o
   reload por volta de rede, e com Spotify/YouTube no ar o reload é o certo (o token
   expira, o IFrame fica órfão). Esta aqui responde "há mídia tocando em QUALQUER
   motor?" — é a pergunta de quem quer evitar cortar a música do operador.
   Ler a errada aqui recarregaria a página no meio de uma faixa do Spotify.

   Vale o mesmo raciocínio do enlace: fica em módulo porque o consumidor
   (`useVersionCheck`, montado noutro ramo da árvore) roda num efeito e não pode
   depender de ordem de montagem. Default `false` = "não sei, ninguém publicou";
   quem lê trata isso como "pode interromper", que é o comportamento de sempre. */
let playingNow = false;

export function setPlayingNow(v: boolean): void {
  playingNow = v;
}

export function isPlayingNow(): boolean {
  return playingNow;
}

/* ENLACE COM O SERVIDOR (o WebSocket) — publicado pelo hook de conectividade e
   lido pelos motores de playout.

   Serve pra uma regra dura: **queda de internet não pode trocar a faixa**. Os
   motores avançam a fila chamando `POST /playback/next`, e quem dispara é (a) o
   fim da faixa e (b) um timer de segurança pela duração. Durante uma queda o áudio
   pode ficar atrasado (buffer secou) enquanto o relógio segue correndo — aí o timer
   acha que a faixa acabou e, no instante em que a rede volta, o avanço sai e o
   operador perde a música no meio. Com este sinal os motores seguram o avanço
   enquanto não há enlace.

   Default `true`: sem ninguém publicando (testes, telas isoladas) o comportamento
   é o de sempre. */
let linked = true;
const listeners = new Set<() => void>();

export function setServerLinked(v: boolean): void {
  if (linked === v) return;
  linked = v;
  for (const l of listeners) l();
}

export function isServerLinked(): boolean {
  return linked;
}

/* Assinatura pra UI. A tela também precisa saber do enlace — sem servidor, buscar
   no Spotify/YouTube e enviar arquivo só podem falhar, e oferecer um botão que
   nunca funciona é pior que mostrá-lo apagado. Formato de `useSyncExternalStore`
   pra não duplicar estado nem re-renderizar a árvore inteira. */
export function subscribeServerLinked(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

/** snapshot p/ SSR: no servidor não há enlace a consultar, e assumir "ligado"
 *  mantém o HTML gerado igual ao primeiro render do cliente online. */
export function serverLinkedServerSnapshot(): boolean {
  return true;
}
