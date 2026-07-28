"use client";

import { useState } from "react";

/* Aviso de versão nova no servidor, ao lado do número da versão no titlebar.

   Deliberadamente NÃO é modal e NÃO recarrega sozinho: isto é playout de rádio, e
   reload derruba o áudio no ar. Um modal no meio de uma transmissão seria pior que
   a versão velha. Então: pílula discreta, dispensável, e o clique abre uma confirmação
   curta que diz o custo antes de recarregar.

   Quem recarrega sozinho é o `useVersionCheck` — mas só em SILÊNCIO (0.2.41). Com
   música no ar ele apenas avisa e espera; este crachá é o caminho de quem quer
   atualizar AGORA, aceitando o corte, e por isso a confirmação diz o custo.

   `serverVersion` entra no title (tooltip) em vez do rótulo — o rótulo precisa caber
   no titlebar em tela estreita. */
export function UpdateBadge({ serverVersion }: { serverVersion: string | null }) {
  const [confirming, setConfirming] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  if (dismissed) return null;

  if (confirming) {
    return (
      <span className="upd upd-confirm" role="alertdialog" aria-label="Recarregar para atualizar">
        <span className="upd-msg">Recarregar interrompe o áudio no ar.</span>
        <button className="upd-go" onClick={() => window.location.reload()}>
          Recarregar
        </button>
        <button
          className="upd-later"
          onClick={() => {
            setConfirming(false);
            setDismissed(true);
          }}
        >
          Agora não
        </button>
      </span>
    );
  }

  return (
    <button
      className="upd"
      onClick={() => setConfirming(true)}
      title={
        serverVersion
          ? `O servidor já está na ${serverVersion}. Esta aba está rodando uma versão anterior — recarregue quando puder.`
          : "Há uma versão mais nova no servidor. Recarregue quando puder."
      }
    >
      <span className="upd-dot" aria-hidden="true" />
      Nova versão
    </button>
  );
}
