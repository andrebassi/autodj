"use client";

import { useMemo, useState } from "react";
import { MAX_LINKS, parseYouTubeLinks } from "@/lib/ytLinks";

/* sub-aba "Colar links" do painel YouTube — o caminho SEM login. O operador cola
   endereços de vídeo separados por ";" (vírgula e quebra de linha também valem) e
   manda pra fila/deck de uma vez.

   Por que não exige conta conectada: o login do Google é um escudo de cota da BUSCA
   (`search.list` ≈ 100 unidades das ~10k/dia). Resolver ids colados usa
   `videos.list` = 1 unidade por lote — 100× mais barato — então gatear isso atrás de
   um OAuth seria fricção à toa.

   `onImport` devolve quantas faixas entraram: pode ser MENOS que o colado (vídeo
   não-incorporável é descartado pelo catálogo) ou 0 (nada reconhecido / erro). */
export function YouTubeLinksImport({
  onImport,
}: {
  // importa o texto colado e (quando o pai é a tela operacional) enfileira as
  // faixas. Resolve com o nº de faixas que entraram; rejeita em falha de rede/API.
  onImport: (text: string, playNow: boolean) => Promise<number>;
}) {
  const [text, setText] = useState("");
  const [playNow, setPlayNow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [errMsg, setErrMsg] = useState<string | null>(null);

  // contagem só p/ feedback imediato — quem parseia de verdade é o backend.
  const ids = useMemo(() => parseYouTubeLinks(text), [text]);
  const canImport = ids.length > 0 && !busy;

  const run = async () => {
    if (!canImport) return;
    setBusy(true);
    setOkMsg(null);
    setErrMsg(null);
    try {
      const n = await onImport(text, playNow);
      if (n === 0) {
        setErrMsg("Nenhum vídeo importado. Confira os endereços colados.");
        return;
      }
      setText(""); // lote aceito → limpa o campo p/ o próximo
      const dropped = ids.length - n;
      setOkMsg(
        `✓ ${n} vídeo${n === 1 ? "" : "s"} na fila.` +
          (dropped > 0
            ? ` ${dropped} descartado${dropped === 1 ? "" : "s"} (não permite incorporação).`
            : ""),
      );
    } catch (e) {
      setErrMsg(e instanceof Error ? e.message : "não foi possível importar os links");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="yt-paste">
      <label className="yt-paste-lbl" htmlFor="yt-paste-ta">
        Cole os endereços dos vídeos, separados por <b>;</b>
      </label>
      <textarea
        id="yt-paste-ta"
        className="yt-paste-ta"
        value={text}
        onChange={(e) => setText(e.target.value)}
        spellCheck={false}
        placeholder={
          "https://www.youtube.com/watch?v=dQw4w9WgXcQ;\n" +
          "https://youtu.be/9bZkp7q19f0;\n" +
          "https://www.youtube.com/shorts/kJQP7kiw5Fk"
        }
      />

      <div className="yt-paste-bar">
        <span className="yt-paste-count">
          {ids.length === 0
            ? "Nenhum link reconhecido ainda."
            : `${ids.length} link${ids.length === 1 ? "" : "s"} detectado${ids.length === 1 ? "" : "s"}` +
              (ids.length === MAX_LINKS ? ` (máximo por lote)` : "")}
        </span>
        <label className="yt-paste-chk">
          <input
            type="checkbox"
            checked={playNow}
            onChange={(e) => setPlayNow(e.target.checked)}
          />
          Tocar o primeiro agora
        </label>
        <button className="yt-paste-btn" onClick={() => void run()} disabled={!canImport}>
          {busy ? "Importando…" : "Adicionar à fila"}
        </button>
      </div>

      {errMsg && <div className="spot-warn spot-err">⚠️ {errMsg}</div>}
      {okMsg && <div className="spot-warn spot-ok">{okMsg}</div>}

      <div className="spot-note">
        🎧 Sem login: colar link usa <b>1 unidade</b> de cota por lote (a busca custa
        ~100). Aceita <b>watch</b>, <b>youtu.be</b>, <b>shorts</b> e <b>embed</b> — até{" "}
        {MAX_LINKS} vídeos por vez. Importa só <b>metadados</b>; o áudio toca no deck
        pelo player do YouTube.
      </div>
    </div>
  );
}
