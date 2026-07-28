/* Program-row TYPE + row styling map. The program row DATA now comes from the
   API (GET /api/v1/ui/program) via the useProgram hook — nothing hardcoded. */

export type RichRow =
  | { t: "music"; label: string; dur: string }
  | { t: "vinheta"; label: string; dur: string } // vinheta/media programada (verde)
  | { t: "bloco"; label: string; dur: string } // cabeçalho bloco comercial (lilás)
  | { t: "abert"; label: string; dur: string } // abertura de bloco (lilás)
  | { t: "locucao"; text: string }
  | { t: "comercial"; label: string; dur: string } // spot comercial (lilás)
  | { t: "horacerta"; label: string; dur: string }
  | { t: "temperatura"; label: string; dur: string }
  | { t: "vinhetaL"; label: string; dur: string }; // vinheta dentro do bloco (lilás)

/* icon + css class per row kind (pure presentation) */
export const ROW_META: Record<RichRow["t"], { ico: string; cls: string }> = {
  music: { ico: "♪", cls: "rr-music" },
  vinheta: { ico: "★", cls: "rr-vinhetaG" },
  bloco: { ico: "🧑‍💼", cls: "rr-bloco" },
  abert: { ico: "★", cls: "rr-abert" },
  locucao: { ico: "🎙", cls: "rr-locucao" },
  comercial: { ico: "👍", cls: "rr-comercial" },
  horacerta: { ico: "🕐", cls: "rr-horacerta" },
  temperatura: { ico: "❗", cls: "rr-temperatura" },
  vinhetaL: { ico: "★", cls: "rr-vinhetaL" },
};
