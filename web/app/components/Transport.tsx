"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/* bottom transport bar — FALAR/LOOP/RETROCEDER/PRÓXIMA/PAUSAR.
   Uso caseiro: PLAY/STOP mestre saíram daqui — agora é o botão ▶/⏹ do DECK AO VIVO
   (topo) que arranca/tira a estação do ar, e ele funciona SEMPRE (parado → ▶ PLAY;
   no ar → ⏹ STOP). Isto resolve o "aperto e não toca": o gesto no deck destrava o
   autoplay do browser p/ Spotify/local. Aqui embaixo fica só FALAR/LOOP/PRÓXIMA e o
   PAUSE (que pausa o deck ativo sem tirar do ar).

   LOOP (0.2.28) deixou de ser "em breve": segura a BASE INSTRUMENTAL repetindo o
   trecho antes da entrada do vocal, que é o encaixe de vinheta/locução. Só liga com
   faixa local no ar E vocal detectado — sem a marca não há o que segurar. FALAR arma
   e desarma o LOOP junto; o botão continua clicável sozinho porque FALAR só existe no
   modo Rádio e o padrão do app é Tocador.

   VOZ (0.2.29) mora AO LADO DO LOOP porque é da mesma família: controle ao vivo, um
   clique, sempre à mão. Enquanto ligado, a faixa local toca pelo INSTRUMENTAL
   separado por IA no lugar do arquivo original; desligou, volta ao normal na hora.
   Não é gravado: F5 volta desligado, igual ao LOOP. Desabilitado quando não há faixa
   local no ar OU quando o instrumental daquela faixa ainda não foi preparado — e o
   `title` diz qual dos dois é, porque "apagado sem motivo" vira chamado.

   ⚠️ 0.2.32: o cancelamento de centro que ligava este botão foi APOSENTADO. Medido
   numa música real, ele tirava 2,0 dB de voz (o instrumental da IA tira 31,9) —
   um botão que promete tirar a voz e não tira engana mais do que ajuda. */
/* LOOP escondido a pedido do operador — a lógica continua viva (ver o comentário
   no lugar do botão). Trocar para true devolve o botão sem mais nada. */
const SHOW_LOOP = false;

const SOON = "Breve — Épico 4 (autopilot/transport completo)";
const LOOP_OFF = "LOOP precisa de faixa local no ar com a entrada do vocal detectada";
// Dois motivos distintos p/ o VOZ estar apagado, e o operador precisa saber qual:
// "não tem faixa minha no ar" é temporário; "falta preparar" tem ação clara.
const VOICE_NO_TRACK = "SEM-VOZ precisa de uma música local no ar (Spotify e YouTube tocam por fora)";
const VOICE_NO_STEM =
  "Esta faixa ainda não foi processada — clique para separar a voz com IA";

/*
usePressFlash marca um botão como "recém-apertado" por alguns segundos.

Por que JS e não `:active`: o `:active` do CSS só dura ENQUANTO o dedo está no
botão — solta e some. O que o operador pediu é confirmação que SOBREVIVE ao
clique, porque num playout o efeito da ação (a fila andou, o áudio trocou) nem
sempre é visível na hora, e sem retorno ele clica de novo achando que não pegou.

Um `id` por botão, e não um booleano global: apertar PRÓXIMA e logo depois PAUSAR
tem de acender o segundo e apagar o primeiro, não os dois. O timer é reiniciado a
cada aperto (ref única) — dois cliques rápidos no mesmo botão renovam os 3s em vez
de cortar o efeito pela metade. */
const PRESS_MS = 3000;

function usePressFlash(): [string | null, (id: string) => void] {
  const [pressed, setPressed] = useState<string | null>(null);
  const timer = useRef(0);
  const flash = useCallback((id: string) => {
    window.clearTimeout(timer.current);
    setPressed(id);
    timer.current = window.setTimeout(() => setPressed(null), PRESS_MS);
  }, []);
  // sem isto, desmontar no meio do efeito deixaria um setState em componente morto
  useEffect(() => () => window.clearTimeout(timer.current), []);
  return [pressed, flash];
}

export function Transport({
  busy,
  hasNext,
  hasPrev,
  announcing,
  showTalk = true,
  onNext,
  onPrev,
  onTalk,
  onPause,
  canPause = false,
  paused = false,
  stopped = false,
  loopArmed = false,
  canLoop = false,
  onLoop,
  vocalCut = false,
  canVocalCut = false,
  hasLocalLive = false,
  onVocalCut,
  stemBusy = null,
  onPrepareStem,
}: {
  busy: boolean;
  hasNext: boolean; // há próxima real na fila (queue[1]) p/ avançar
  hasPrev: boolean; // há faixa no histórico p/ retroceder (religar a anterior)
  announcing: boolean;
  showTalk?: boolean; // FALAR (locução ao vivo) é função de estação → só modo Rádio
  onNext: () => void;
  onPrev: () => void; // volta p/ a música anterior (última das Tocadas), religa ao ar
  onTalk: () => void;
  onPause?: () => void; // pausa/retoma o deck ativo (Spotify SDK ou local <audio>)
  canPause?: boolean;
  paused?: boolean;
  stopped?: boolean; // estação parada — PAUSE fica inativo (nada no ar p/ pausar)
  loopArmed?: boolean; // base instrumental em repetição agora
  canLoop?: boolean; // faixa local no ar com vocal marcado
  onLoop?: () => void;
  vocalCut?: boolean; // modo VOZ ligado (faixa local toca o instrumental)
  canVocalCut?: boolean; // faixa local no ar E com instrumental já preparado
  // faixa local no ar (mesmo sem instrumental) — separa os dois motivos de o
  // botão estar apagado, pra explicar o certo em vez de um genérico.
  hasLocalLive?: boolean;
  onVocalCut?: () => void;
  /* separação em andamento PARA A FAIXA NO AR — vira barra de progresso no
     próprio botão. null = nada em andamento. */
  stemBusy?: "queued" | "running" | null;
  /* pede a separação da faixa no ar. Ausente = não dá pra pedir daqui
     (ferramenta ausente, ou faixa de streaming). */
  onPrepareStem?: () => void;
}) {
  const [pressed, flash] = usePressFlash();
  /* embrulha o onClick de cada botão: acende o efeito e chama a ação. Botão sem
     ação (desabilitado) não acende — piscar sem fazer nada seria mentira. */
  const press = (id: string, fn?: () => void) =>
    fn
      ? () => {
          flash(id);
          fn();
        }
      : undefined;
  const flashCls = (id: string) => (pressed === id ? " tbtn-pressed" : "");
  return (
    <div className="transport">
      {showTalk && (
        <button
          className={"tbtn small" + (announcing ? " active" : "") + flashCls("talk")}
          onClick={press("talk", onTalk)}
        >
          <span className="tico">🎙</span>
          <span>FALAR</span>
        </button>
      )}
      {/* LOOP OCULTO POR ORA (pedido do operador). Não foi removido: a lógica de
          repetir a base até a entrada do vocal continua inteira no
          `useLocalPlayout` (o `loopArmed` segue chegando aqui), e o FALAR do modo
          Rádio ainda arma e desarma o loop. Apagar o código para reescrever depois
          custaria mais que esconder o botão. Para trazer de volta: `SHOW_LOOP`. */}
      {SHOW_LOOP && (
        <button
          className={
            "tbtn small" + (loopArmed ? " active" : "") + (canLoop ? "" : " soon") + flashCls("loop")
          }
          disabled={!canLoop || !onLoop}
          onClick={press("loop", onLoop)}
          title={
            canLoop
              ? loopArmed
                ? "Soltar a base — segue para o vocal"
                : "Repetir a base instrumental até a entrada do vocal"
              : LOOP_OFF
          }
        >
          <span className="tico">🔁</span>
          <span>LOOP</span>
        </button>
      )}
      {/* SEM-VOZ — TRÊS estados no mesmo botão, e é isso que tira o beco sem saída.

          Antes, faixa sem preparo deixava o botão apagado e pronto: o operador via
          a função existir e não tinha como alcançá-la dali — a saída morava num
          menu de contexto em outra tela. Agora o próprio botão é o caminho.

            preparada     → liga/desliga (equalizador quando ligado)
            sem preparo   → PREPARAR, e o clique enfileira a separação
            preparando    → barra de progresso NO BOTÃO, sem clique

          O modo continua sem se desarmar sozinho ao entrar faixa não preparada
          (ele vale pra próxima que estiver), e por isso o botão segue clicável
          enquanto ligado: aceso e travado prenderia o operador no modo. */}
      {(() => {
        const busy = !!stemBusy;
        const toggling = canVocalCut || vocalCut;
        // PREPARAR só aparece quando não há o que ligar E dá pra pedir daqui.
        const preparing = !toggling && !busy && !!onPrepareStem && hasLocalLive;
        return (
          <button
            className={
              "tbtn tbtn-voice" +
              (vocalCut ? " active" : "") +
              (busy ? " tbtn-busy" : "") +
              (toggling || preparing || busy ? "" : " soon") +
              flashCls("voice")
            }
            disabled={busy || (!toggling && !preparing)}
            aria-pressed={toggling ? vocalCut : undefined}
            onClick={press(
              "voice",
              toggling ? onVocalCut : preparing ? onPrepareStem : undefined,
            )}
            title={
              busy
                ? stemBusy === "queued"
                  ? "Na fila para separar a voz desta faixa"
                  : "Separando a voz desta faixa — leva alguns minutos"
                : preparing
                  ? "Separar a voz desta faixa com IA (alguns minutos, roda em segundo plano)"
                  : vocalCut
                    ? canVocalCut
                      ? "Devolver a voz — a música volta completa na hora"
                      : "SEM-VOZ ligado, mas esta faixa ainda não foi preparada — vale para a próxima que estiver. Clique para desligar."
                    : canVocalCut
                      ? "Tocar sem a voz (vale para as próximas faixas preparadas também)"
                      : hasLocalLive
                        ? VOICE_NO_STEM
                        : VOICE_NO_TRACK
            }
          >
            {/* LIGADO vira um equalizador TOCANDO, não um ícone parado.

                Um botão que só troca de cor obriga a comparar com o vizinho pra
                saber se está ligado. Movimento não: o olho pega de longe, sem
                comparação. E barras dançando dizem a coisa certa — está saindo som
                assim AGORA —, que um símbolo estático nunca diz.

                Marcado como decorativo (`aria-hidden`): leitor de tela recebe o
                estado pelo `aria-pressed`, não pela animação. */}
            {vocalCut && !busy ? (
              <span className="tico tbtn-eq" aria-hidden>
                <i />
                <i />
                <i />
                <i />
              </span>
            ) : (
              <span className="tico">{busy ? "⏳" : preparing ? "✨" : "🎼"}</span>
            )}
            <span>
              {busy
                ? stemBusy === "queued"
                  ? "NA FILA"
                  : "PROCESSANDO"
                : preparing
                  ? "PROCESSAR SEM-VOZ (IA)"
                  : "SEM-VOZ"}
            </span>
            {/* Barra INDETERMINADA no rodapé do próprio botão — mesma gramática da
                grade. Indeterminada porque o modelo reporta progresso por thread,
                em segmentos distintos: não vira um total confiável, e barra que
                finge saber a porcentagem mente quando trava. */}
            {busy && (
              <span
                className={"tbtn-prep" + (stemBusy === "queued" ? " queued" : "")}
                aria-hidden
              >
                <i />
              </span>
            )}
          </button>
        );
      })()}
      {/* RETROCEDER — volta p/ a música anterior (última das Tocadas), religando-a
          ao ar. Sem histórico → desabilitado. Simétrico ao PRÓXIMA. */}
      <button
        className={"tbtn" + flashCls("prev")}
        disabled={busy || !hasPrev}
        onClick={press("prev", onPrev)}
        title={hasPrev ? "Voltar p/ a música anterior" : "Sem faixa anterior"}
      >
        <span className="tico">⏮</span>
        <span>RETROCEDER</span>
      </button>
      {/* PRÓXIMA só quando existe próxima real na fila — sem fila, avançar tocava
          faixa aleatória; agora fica desabilitado. */}
      <button
        className={"tbtn" + flashCls("next")}
        disabled={busy || !hasNext}
        onClick={press("next", onNext)}
        title={hasNext ? "Pular p/ a próxima" : "Fila vazia"}
      >
        <span className="tico">⏭</span>
        <span>PRÓXIMA</span>
      </button>
      {canPause && !stopped ? (
        <button
          className={"tbtn" + (paused ? " active" : "") + flashCls("pause")}
          onClick={press("pause", onPause)}
          title="Pausar/retomar"
        >
          <span className="tico">{paused ? "▶" : "⏸"}</span>
          <span>{paused ? "RETOMAR" : "PAUSAR"}</span>
        </button>
      ) : (
        <button className="tbtn soon" disabled title={SOON}>
          <span className="tico">⏸</span>
          <span>PAUSAR</span>
        </button>
      )}
    </div>
  );
}
