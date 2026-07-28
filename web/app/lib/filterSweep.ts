/* Varredura de passa-alta para cobrir a troca original ⇄ instrumental (modo VOZ).

   É o gesto de DJ: o grave vai saindo até sobrar só o corpo agudo da música, a
   emenda acontece ali, e o grave volta. Não é enfeite — é a escolha certa para
   MASCARAR uma troca, e o motivo é espectral: no topo da varredura o sinal perdeu
   quase toda a energia (grave e médio-grave concentram a maior parte dela), então
   é o instante em que uma descontinuidade é menos audível. Cortar no ponto mais
   fino do espectro é o oposto de cortar no meio de um acorde cheio.

   Por que isto e não as tentativas anteriores:
     - fade puro: some e volta, lê como "mexeram no volume";
     - eco: cauda longa, e o operador quis eco puro, não reverb;
     - whoosh sintetizado: som ESTRANHO à música, colado por cima.
   A varredura age SOBRE a própria música — não há elemento estranho, é a faixa
   que se afina e volta.

   ⚠️ AQUI O `<audio>` É ROTEADO POR WEB AUDIO, o que a 0.2.32 tinha eliminado do
   playout. Não existe alternativa: filtrar exige um nó de filtro, e `<audio>` só
   expõe ganho. As travas que tornam isto aceitável:

     - o contexto nasce do CLIQUE no VOZ, nunca no carregamento da página — o
       cenário perigoso (autoplay barrado deixando a estação muda no boot) não
       existe por construção;
     - `attach` só roteia com o contexto JÁ em `running` (quem chama dá o await no
       resume antes). Contexto suspenso + elemento roteado = estação MUDA, e essa
       é exatamente a regressão que não pode acontecer;
     - o filtro descansa em 20 Hz, que é praticamente transparente: um elemento
       roteado e parado ali soa como um elemento não roteado.

   ⚠️ `createMediaElementSource` é IRREVERSÍVEL — uma vez roteado, o elemento toca
   através do contexto pelo resto da sessão. Daí o WeakMap (rotear duas vezes lança
   InvalidStateError) e a regra de sempre devolver a frequência ao repouso.
*/

/** Repouso do filtro. 20 Hz está abaixo do audível: o passa-alta ali não tira nada
 *  que se ouça — é o "desligado" sem precisar de caminho paralelo. */
export const REST_HZ = 20;
/** Topo da varredura — subiu em etapas junto com a duração (1400 → 2000 → 4000),
 *  afinado de ouvido pelo operador.
 *
 *  Em 4 kHz sobra só o chiado dos pratos e o ar da mixagem: a música quase desaparece
 *  no topo. É o extremo da sensação de "a batida acabando", e é também onde a emenda
 *  fica mais escondida (quase não há energia pra descontinuar). O preço é que o topo
 *  soa mais como corte que como filtro — se ficar agressivo demais, 2500–3000 é o
 *  meio-termo. */
export const TOP_HZ = 4000;

export type SweepChain = { hp: BiquadFilterNode; ctx: AudioContext };

const chains = new WeakMap<HTMLAudioElement, SweepChain>();

/* attach roteia o elemento (uma vez só) e devolve a cadeia.

   null = não deu; quem chama tem de seguir sem efeito. NUNCA rotear com o contexto
   fora de `running`: o áudio passaria a sair por um contexto parado, ou seja,
   silêncio no ar. */
export function attach(el: HTMLAudioElement, ctx: AudioContext): SweepChain | null {
  const cached = chains.get(el);
  if (cached) return cached;
  if (ctx.state !== "running") return null;
  try {
    const source = ctx.createMediaElementSource(el);
    const hp = ctx.createBiquadFilter();
    hp.type = "highpass";
    // Q baixo = ressonância nenhuma no corte. O default (1) faria um pico na
    // frequência de corte, e um pico varrendo soa como assobio, não como filtro.
    hp.Q.value = 0.7;
    hp.frequency.value = REST_HZ;
    source.connect(hp).connect(ctx.destination);
    const chain: SweepChain = { hp, ctx };
    chains.set(el, chain);
    return chain;
  } catch {
    return null;
  }
}

/* rampa EXPONENCIAL de frequência — nunca linear.

   Altura é percebida em oitavas: uma rampa linear de 20 Hz a 1400 Hz gastaria a
   quase totalidade do tempo acima de 1 kHz, e o efeito soaria como um salto seguido
   de nada. A exponencial distribui o movimento por oitava, que é como o ouvido lê.
   (E `exponentialRampToValueAtTime` não aceita zero — outra razão pro repouso ser
   20 Hz e não 0.) */
function glide(chain: SweepChain, to: number, ms: number): void {
  const { hp, ctx } = chain;
  const t = ctx.currentTime;
  hp.frequency.cancelScheduledValues(t);
  hp.frequency.setValueAtTime(Math.max(REST_HZ, hp.frequency.value), t);
  hp.frequency.exponentialRampToValueAtTime(Math.max(REST_HZ, to), t + ms / 1000);
}

/** tira o grave: repouso → topo. */
export function sweepUp(chain: SweepChain, ms: number): Promise<void> {
  glide(chain, TOP_HZ, ms);
  return new Promise((r) => window.setTimeout(r, ms));
}

/** devolve o grave: topo → repouso. */
export function sweepDown(chain: SweepChain, ms: number): Promise<void> {
  glide(chain, REST_HZ, ms);
  return new Promise((r) => window.setTimeout(r, ms));
}

/** estaciona no topo sem varrer — usado no elemento que ENTRA, que precisa nascer
 *  já filtrado pra a varredura de volta ser contínua aos ouvidos. */
export function parkTop(chain: SweepChain): void {
  const { hp, ctx } = chain;
  hp.frequency.cancelScheduledValues(ctx.currentTime);
  hp.frequency.setValueAtTime(TOP_HZ, ctx.currentTime);
}

/** devolve ao repouso na marra (caminho de erro/cancelamento): elemento roteado e
 *  esquecido no topo continuaria sem grave na próxima faixa. */
export function reset(chain: SweepChain): void {
  const { hp, ctx } = chain;
  hp.frequency.cancelScheduledValues(ctx.currentTime);
  hp.frequency.setValueAtTime(REST_HZ, ctx.currentTime);
}
