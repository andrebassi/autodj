/* espelho client-side do `domain/youtube.ParseLinks` (Go) — usado SÓ para a dica
   "N link(s) detectado(s)" enquanto o operador digita/cola. Quem importa de verdade
   é o backend (POST /api/v1/youtube/import-links), que reparseia e é a fonte de
   verdade; aqui é feedback imediato, sem round-trip.

   Ao mexer nas regras, mexer nos DOIS lados (Go + este arquivo) — divergir faria a
   contagem mentir sobre o que vai ser importado. */

/* MAX_LINKS espelha youtube.MaxImportLinks: o Data API aceita no máximo 50 ids por
   videos.list e o serviço manda uma chamada só. */
export const MAX_LINKS = 50;

/* parseYouTubeLinks extrai ids de vídeo do texto colado. Separadores: ";" (o
   formato documentado), vírgula, quebra de linha, tab e espaço. Duplicata colapsa
   preservando a ordem da primeira aparição; o resultado é limitado a MAX_LINKS. */
export function parseYouTubeLinks(raw: string): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const field of raw.split(/[;,\n\r\t ]+/)) {
    const id = youtubeVideoID(field);
    if (id === "" || seen.has(id)) continue;
    seen.add(id);
    out.push(id);
    if (out.length === MAX_LINKS) break;
  }
  return out;
}

/* youtubeVideoID reduz uma entrada a um id puro, ou "" quando não carrega nenhum.
   Aceita watch?v=<id>, youtu.be/<id>, /shorts/, /embed/, /live/, /v/ e o id cru.
   Link só de playlist (sem "v=") devolve "" — playlist tem rota própria. */
export function youtubeVideoID(entry: string): string {
  const s = entry.trim();
  if (s === "") return "";
  // sem separador de URL → já é (ou deveria ser) um id cru
  if (!s.includes("/") && !s.includes("?") && !s.includes("&")) return validID(s);

  let u: URL;
  try {
    // pastes costumam vir sem esquema; URL() exige um pra separar host de path
    u = new URL(s.includes("://") ? s : "https://" + s);
  } catch {
    return "";
  }
  if (!youtubeHost(u.hostname)) return "";

  const v = u.searchParams.get("v");
  if (v) return validID(v);

  const parts = u.pathname.replace(/^\/+|\/+$/g, "").split("/");
  const last = parts[parts.length - 1] ?? "";
  // youtu.be/<id> põe o id direto no path; as formas longas prefixam um segmento
  if (parts.length === 1 && u.hostname.toLowerCase() === "youtu.be") return validID(last);
  const prev = parts.length >= 2 ? parts[parts.length - 2] : "";
  if (prev === "shorts" || prev === "embed" || prev === "live" || prev === "v") {
    return validID(last);
  }
  return "";
}

/* youtubeHost aceita só os domínios do YouTube — link solto de outro serviço no
   meio do paste nunca vira id bogus. */
function youtubeHost(host: string): boolean {
  const h = host.toLowerCase().replace(/^www\./, "").replace(/^m\./, "");
  return (
    h === "youtube.com" ||
    h === "youtu.be" ||
    h === "music.youtube.com" ||
    h === "youtube-nocookie.com"
  );
}

/* validID aceita o alfabeto de 11 chars que o YouTube usa; qualquer outra coisa
   (palavra solta, paste truncado) é rejeitada em vez de virar lixo na Data API. */
function validID(s: string): string {
  return /^[A-Za-z0-9_-]{11}$/.test(s) ? s : "";
}
