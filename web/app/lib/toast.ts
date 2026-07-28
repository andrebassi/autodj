/* Toast global no estilo Ant `message`/`notification`: store módulo-nível com
   pub/sub (sem context React). Componentes chamam `toast.success(...)`; o
   `ToastHost` (montado uma vez em page.tsx) escuta e renderiza. Feedback
   flutuante para ações — salvar, enfileirar, excluir, upload. */

export type ToastKind = "success" | "error" | "info" | "warning";

export interface ToastItem {
  id: number;
  kind: ToastKind;
  text: string;
  /** Identidade lógica: repetir a key substitui em vez de empilhar. */
  key?: string;
  /** Fixo (ttl 0) — só sai por dismissToast/closeToast. */
  sticky?: boolean;
}

let items: ToastItem[] = [];
const EMPTY: ToastItem[] = [];
let seq = 0;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

/** Snapshot estável entre emissões (requisito do useSyncExternalStore). */
export function getToasts(): ToastItem[] {
  return items;
}

export function getServerToasts(): ToastItem[] {
  return EMPTY;
}

export function subscribeToasts(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

/** Empilha um toast; some sozinho após `ttl` ms. `ttl = 0` → FIXO (não expira),
 *  para condição que continua verdadeira enquanto ninguém a resolve — o caso é
 *  "sem conexão": some sozinho seria mentira, porque a rede continua caída. Toast
 *  fixo só sai por `dismissToast(key)`. */
export function pushToast(kind: ToastKind, text: string, ttl = 7000, key?: string): number {
  // Rede caída já tem a faixa fixa explicando ("Sem internet — áudio e fila
  // parados"). Cada ação que falhar por causa disso empilharia mais um erro em
  // cima, dizendo a mesma coisa com outras palavras — ruído no pior momento. O
  // aviso fixo (key "net") fica; os erros transitórios são suprimidos enquanto
  // durar a queda. Sucesso/aviso/info passam normalmente.
  if (kind === "error" && !key && items.some((t) => t.key === "net" && t.sticky)) {
    return 0;
  }
  const id = ++seq;
  // mesma `key` substitui em vez de empilhar: reconexões sucessivas do WS não
  // podem virar uma pilha de "sem conexão" idênticos na tela do operador.
  const base = key ? items.filter((t) => t.key !== key) : items;
  items = [...base, { id, kind, text, key, sticky: ttl === 0 }];
  emit();
  if (ttl > 0) {
    setTimeout(() => {
      items = items.filter((t) => t.id !== id);
      emit();
    }, ttl);
  }
  return id;
}

/** Remove um toast pela `key` (usado pelos fixos). Idempotente. */
export function dismissToast(key: string): void {
  const next = items.filter((t) => t.key !== key);
  if (next.length === items.length) return;
  items = next;
  emit();
}

/** Remove pelo id — o botão de fechar do Alert. */
export function closeToast(id: number): void {
  items = items.filter((t) => t.id !== id);
  emit();
}

/* TTLs generosos de propósito: o operador está olhando o deck/fila, não o rodapé —
   mensagem que some em 3s passa despercebida no meio de uma operação ao vivo. Erro
   dura bem mais que sucesso porque exige ação; sucesso é só confirmação.
   PISO: nenhum toast abaixo de 5s (vale também pro aviso da volta de rede, que expira
   por reload e não por ttl — ver RELOAD_DELAY_MS em useConnectivity). */
export const toast = {
  success: (t: string) => pushToast("success", t, 7000),
  error: (t: string) => pushToast("error", t, 14000),
  info: (t: string) => pushToast("info", t, 8000),
  warning: (t: string) => pushToast("warning", t, 11000),
  /** Fixo até `dismissToast(key)`. Para estado, não para evento. */
  sticky: (kind: ToastKind, t: string, key: string) => pushToast(kind, t, 0, key),
};
