/* formatting + RadioPro-style category classifier / calendar labels */

export type Kind = "music" | "vinheta" | "commercial" | "hora" | "programete";

export function classify(title: string): Kind {
  const t = (title || "").toLowerCase();
  if (/vinheta|abert|jingle/.test(t)) return "vinheta";
  if (/coment|comercial|patroc|of\.|oferec/.test(t)) return "commercial";
  if (/hora certa|hora\b|temperatura|tempo/.test(t)) return "hora";
  if (/programete|informe|not[ií]cia/.test(t)) return "programete";
  return "music";
}

export const KIND_ICON: Record<Kind, string> = {
  music: "♪",
  vinheta: "★",
  commercial: "📢",
  hora: "🕐",
  programete: "🎙",
};

export function fmtDur(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`;
}

export const WEEKDAYS = [
  "domingo",
  "segunda-feira",
  "terça-feira",
  "quarta-feira",
  "quinta-feira",
  "sexta-feira",
  "sábado",
];

export const MONTHS = [
  "janeiro",
  "fevereiro",
  "março",
  "abril",
  "maio",
  "junho",
  "julho",
  "agosto",
  "setembro",
  "outubro",
  "novembro",
  "dezembro",
];
