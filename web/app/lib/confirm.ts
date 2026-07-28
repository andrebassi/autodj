/* Confirm dialog no estilo Ant (ícone ⚠ + título + corpo + Cancelar/OK à
   direita), substituindo `window.confirm`. Store módulo-nível pub/sub +
   API baseada em Promise: `if (await confirmDialog({ ... })) { ... }`.
   O `ConfirmHost` (montado uma vez em page.tsx) renderiza o pedido corrente. */

export interface ConfirmOpts {
  title: string;
  body?: string;
  /** Lista itemizada (⚠ escopo destrutivo) exibida abaixo do corpo. */
  bullets?: string[];
  okText?: string;
  cancelText?: string;
  danger?: boolean;
}

export interface ConfirmRequest extends ConfirmOpts {
  id: number;
  resolve: (v: boolean) => void;
}

let current: ConfirmRequest | null = null;
let seq = 0;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

export function getConfirm(): ConfirmRequest | null {
  return current;
}

export function getServerConfirm(): ConfirmRequest | null {
  return null;
}

export function subscribeConfirm(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

/** Abre o diálogo e resolve true (OK) / false (Cancelar). Um pedido novo
    enquanto outro está aberto cancela o anterior (resolve false). */
export function confirmDialog(opts: ConfirmOpts): Promise<boolean> {
  if (current) current.resolve(false);
  return new Promise<boolean>((resolve) => {
    current = { ...opts, id: ++seq, resolve };
    emit();
  });
}

/** Fecha o diálogo corrente com o veredito e resolve a Promise pendente. */
export function resolveConfirm(v: boolean): void {
  const req = current;
  current = null;
  emit();
  if (req) req.resolve(v);
}
