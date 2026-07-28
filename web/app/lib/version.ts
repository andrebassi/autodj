/* Versão do app exibida no titlebar. Vem da git tag no build via
   NEXT_PUBLIC_APP_VERSION (injetada por scripts/serve-local.sh e pelo build:
   `NEXT_PUBLIC_APP_VERSION=$(git describe --tags --abbrev=0)`). Fallback = v0.0.1
   quando não há tag / build fora do git. NEXT_PUBLIC_* é inlinado no bundle. */
export const APP_VERSION: string =
  process.env.NEXT_PUBLIC_APP_VERSION?.trim() || "v0.0.1";

/* Versão que o SERVIDOR reporta quando o binário foi compilado sem ldflags (build
   local: task serve, go run). Sinaliza "não dá pra comparar". */
export const SERVER_VERSION_DEV = "dev";

/* shouldOfferReload decide se a aba está rodando um bundle mais velho que o servidor.

   Contexto: `Cache-Control: no-cache` (v0.2.18) garante que um LOAD novo pegue a
   versão nova, mas aba que ficou aberta nunca refaz a requisição do documento —
   nenhuma política de cache alcança isso. Daí a checagem em runtime.

   Comparação é por IGUALDADE, não por "maior que": servidor e bundle saem do mesmo
   build (mesma APP_VERSION → ldflags no Go + NEXT_PUBLIC no Next), então qualquer
   divergência já significa que o servidor foi trocado embaixo da aba. Tentar ordenar
   semver seria pior: um rollback deliberado (deploy de versão MENOR) precisa oferecer
   reload igual, senão a aba fica presa numa versão que não existe mais no servidor.

   Devolve false quando não dá pra concluir nada:
   - servidor em "dev" (sem ldflags) → bundle e binário são compilados em momentos
     diferentes no local e divergiriam à toa;
   - resposta vazia/ilegível → rede instável não vira pedido de reload;
   - bundle no fallback v0.0.1 (build fora do git) → não há o que comparar. */
export function shouldOfferReload(
  running: string | null | undefined,
  server: string | null | undefined,
): boolean {
  const a = running?.trim();
  const b = server?.trim();
  if (!a || !b) return false;
  if (b === SERVER_VERSION_DEV) return false;
  if (a === "v0.0.1") return false;
  return normalizeVersion(a) !== normalizeVersion(b);
}

/* O Go carrega a tag crua ("v0.2.19"); o health legado respondia sem "v" ("0.1.0").
   Normalizar o prefixo evita um falso "tem versão nova" por causa de uma letra. */
function normalizeVersion(v: string): string {
  return v.startsWith("v") ? v.slice(1) : v;
}
