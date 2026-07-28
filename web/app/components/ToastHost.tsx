"use client";

import { useSyncExternalStore } from "react";
import { Alert } from "antd";
import {
  subscribeToasts,
  getToasts,
  getServerToasts,
  closeToast,
} from "@/lib/toast";

/* Pilha de toasts, renderizada com `Alert` do Ant Design.
 *
 * Posição: **rodapé da coluna esquerda, logo acima do transporte**, ocupando a
 * largura inteira da coluna. Montado dentro do `PlayerScreen` (irmão do
 * `<Transport>`) e posicionado por `bottom: 100%` sobre o conteúdo — sobrepõe em
 * vez de empurrar. Empurrar encolheria a fila a cada mensagem, e layout pulando
 * no meio de uma operação ao vivo é pior que a mensagem.
 *
 * Por que `Alert` e não o `message`/`notification` do antd: os dois renderizam
 * pílula centrada em portal próprio, com posição própria — brigariam com a âncora
 * pedida (barra larga, presa ao rodapé desta coluna). `Alert` é o mesmo sistema
 * visual (ícones, cores, raio, tipografia) sem impor layout.
 *
 * A pilha inteira é `pointer-events: none` (não pode roubar clique do transporte);
 * cada Alert reativa o ponteiro só pra permitir fechar. */
export function ToastHost() {
  const items = useSyncExternalStore(subscribeToasts, getToasts, getServerToasts);
  if (items.length === 0) return null;
  return (
    <div className="toast-host" role="status" aria-live="polite">
      {items.map((t) => (
        <Alert
          key={t.id}
          className={"toast-alert" + (t.sticky ? " toast-alert-sticky" : "")}
          type={t.kind}
          // title = texto inteiro no hover, caso a barra corte com reticências
          message={<span title={t.text}>{t.text}</span>}
          showIcon
          // fixo (sem conexão) não é fechável: sumiria sem o problema ter sido
          // resolvido, e o operador acharia que voltou.
          closable={!t.sticky}
          onClose={() => closeToast(t.id)}
          banner
        />
      ))}
    </div>
  );
}
