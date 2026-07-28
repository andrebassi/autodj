"use client";

import { APP_VERSION } from "@/lib/version";

/* Apresentação do AutoDJ — a PRIMEIRA coisa que o painel mostra ao abrir.

   Antes o painel abria no card "Conectar ao Spotify": quem chegava agora via um
   login de serviço externo antes de saber o que este app é. Esta tela responde
   isso em três segundos e sai da frente num clique.

   Regras que ela respeita, e que não são enfeite:
   • **Só tokens semânticos** — o tema escuro herda de graça, sem uma cor
     hardcoded (ver o bloco `.about-*` no globals.css).
   • **Container query, não media query** — o painel é uma COLUNA redimensionável
     (`PanelDivider`), então quem manda na quebra é a largura dele, não a da tela.
   • **Opt-out em `prefers-reduced-motion`** — a entrada escalonada e o equalizador
     somem inteiros; o conteúdo continua legível e no lugar.
   • **Nada de promessa que o app não cumpre**: cada cartão descreve função que
     existe hoje, com o limite dito quando há (o modo VOZ pede preparo por faixa).

   O atraso da entrada de cada peça vem da variável `--i` (índice), aplicada por
   `style` — CSS puro, sem biblioteca de animação e sem JS por quadro. */

type Feature = {
  ico: string;
  title: string;
  desc: string;
  /* família de acento do cartão — só nomes de token, nunca cor literal */
  tone: "green" | "blue" | "purple" | "warn";
};

const FEATURES: Feature[] = [
  {
    ico: "🎵",
    title: "Seu acervo, tocando aqui",
    desc: "Envie suas músicas e o navegador toca. Fica tudo guardado: fila, pastas e o que já tocou.",
    tone: "blue",
  },
  {
    ico: "⚡",
    title: "AUTO toca sozinho",
    desc: "O piloto mantém a fila cheia e o som rolando sem ninguém olhando. Desligue quando quiser assumir.",
    tone: "warn",
  },
  {
    ico: "🎚",
    title: "Mixagem de verdade",
    desc: "Na virada as duas músicas soam JUNTAS pelo tempo que você escolher — crossfade, não corte seco.",
    tone: "purple",
  },
  {
    ico: "🚫🎤",
    title: "Tire o vocal",
    desc: "Prepare a música uma vez e ela toca sem a voz, pra você cantar por cima.",
    tone: "green",
  },
  {
    ico: "🟢",
    title: "Spotify e YouTube",
    desc: "Busque e importe dos dois. O YouTube funciona sem login; o Spotify pede conta só para tocar.",
    tone: "green",
  },
  {
    ico: "📶",
    title: "A queda não para a música",
    desc: "As próximas faixas ficam guardadas no seu computador. Sem internet, o som continua tocando.",
    tone: "blue",
  },
];

export function AboutAutoDJ({
  hasLocal = false,
  onStart,
  onAdd,
}: {
  /* já existe faixa local? decide qual é o próximo passo natural do operador */
  hasLocal?: boolean;
  onStart?: () => void; // ir para a biblioteca
  onAdd?: () => void; // abrir o envio de músicas
}) {
  return (
    <div className="about" data-panel="sobre">
      <header className="about-hero">
        {/* equalizador: 5 barras com durações diferentes — o desencontro é o que
            faz parecer música em vez de metrônomo. */}
        <span className="about-eq" aria-hidden>
          {[0, 1, 2, 3, 4].map((i) => (
            <i key={i} style={{ ["--i" as string]: i }} />
          ))}
        </span>
        <h2 className="about-title">
          <span className="about-mark" aria-hidden>
            ▲
          </span>
          AutoDJ
        </h2>
        {/* NÃO é "rádio" (pedido do operador): o padrão do app é o modo Tocador —
            o acervo da pessoa tocando sozinho, com mixagem de DJ. Estação/rádio é
            um modo opcional, não o que este app é por padrão. */}
        <p className="about-tag">Suas músicas tocando sozinhas, com mixagem de DJ.</p>
        <span className="about-ver">{APP_VERSION}</span>
      </header>

      <ul className="about-grid">
        {FEATURES.map((f, i) => (
          <li
            key={f.title}
            className={"about-card about-" + f.tone}
            style={{ ["--i" as string]: i }}
          >
            <span className="about-ico" aria-hidden>
              {f.ico}
            </span>
            <div className="about-card-txt">
              <strong className="about-card-title">{f.title}</strong>
              <span className="about-card-desc">{f.desc}</span>
            </div>
          </li>
        ))}
      </ul>

      <footer className="about-foot" style={{ ["--i" as string]: FEATURES.length }}>
        {/* O próximo passo depende do estado real da estação: sem nenhuma faixa
            local, mandar pra biblioteca vazia seria um beco — o botão principal
            vira "enviar música". */}
        {hasLocal ? (
          <button className="about-cta" onClick={onStart}>
            Ver minhas músicas <span aria-hidden>→</span>
          </button>
        ) : (
          <button className="about-cta" onClick={onAdd}>
            <span aria-hidden>＋</span> Adicionar suas músicas
          </button>
        )}
      </footer>
    </div>
  );
}
