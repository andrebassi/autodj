/* Humanizador central de erros de API (Épico 6 — melhora de retorno).

   PROBLEMA que resolve: as chamadas jogavam `throw new Error("op: 500")` cru — o
   painel mostrava "⚠️ youtubeSearch: 500", inútil pro usuário. Agora todo erro de
   rede vira uma frase PT-BR curta que diz O QUE deu e O QUE fazer.

   Provider-agnóstico (hexagonal): a regra mora aqui, não espalhada por chamada.
   O backend SEMPRE devolve `{"error": "..."}` (nossos handlers), então lemos o
   corpo + o status HTTP e casamos: primeiro por palavra-chave no corpo (mais
   específico), depois por status (genérico). `provider` só afina o texto. */

export type HumanError = {
  status: number; // status HTTP (0 = falha de rede antes de resposta)
  message: string; // frase PT-BR pronta pro usuário
  raw: string; // corpo {error} cru do backend (telemetria/debug)
};

export type Provider = "spotify" | "youtube" | "";

/* lê o corpo {error} do backend sem consumir o Response original (clone), tolera
   corpo não-JSON (retorna ""). */
async function readBackendError(res: Response): Promise<string> {
  try {
    const body = (await res.clone().json()) as { error?: string };
    return (body.error ?? "").trim();
  } catch {
    return "";
  }
}

/* gatilhos por palavra-chave no corpo do erro — o caso mais específico vence.
   Retorna null quando nada casa (cai no genérico por status). */
function byKeyword(raw: string, provider: Provider): string | null {
  const b = raw.toLowerCase();

  // Cota diária da API esgotada (YouTube Data API v3 / rate limit de provider).
  if (
    /quota/.test(b) ||
    /ratelimitexceeded/.test(b) ||
    /dailylimitexceeded/.test(b) ||
    /data api daily quota/.test(b)
  ) {
    if (provider === "youtube")
      return "Cota diária de busca do YouTube esgotada (a API grátis permite ~100 buscas/dia). Reseta de madrugada (meia-noite no Pacífico). Enquanto isso dá pra IMPORTAR colando a URL do vídeo — a busca é que precisa da cota, tocar e importar por link não.";
    return "Limite diário da API atingido. Tente novamente mais tarde.";
  }

  // OAuth / conta não conectada / token expirado.
  if (/no account connected/.test(b))
    return provider === "youtube"
      ? "Conta do YouTube não conectada. Clique em “Conectar ao YouTube” primeiro."
      : provider === "spotify"
        ? "Conta do Spotify não conectada. Clique em “Conectar conta” primeiro."
        : "Conta não conectada. Conecte a integração primeiro.";
  if (/state mismatch/.test(b))
    return "A volta do login não bateu (proteção CSRF). Refaça a conexão.";
  if (/auth expired|pending auth/.test(b))
    return "O login demorou demais e expirou. Comece a conexão de novo.";
  if (/token|expired|invalid_grant|refresh/.test(b))
    return "A sessão da integração expirou. Reconecte a conta.";

  // Upload que o ffprobe recusou. O erro chega como o status de saída cru do
  // processo ("exit status 1"/"exit status 127") — texto de terminal no meio de uma
  // lista de arquivos, que não diz nem qual é o problema nem o que fazer.
  if (/exit status|ffprobe|ffmpeg/.test(b))
    return /127|not found|executable/.test(b)
      ? "O servidor não encontrou o ffmpeg/ffprobe para ler o arquivo."
      : "Arquivo de áudio inválido ou corrompido — o servidor não conseguiu lê-lo.";

  // Gateway do catálogo não configurado (feature meio-ligada no servidor).
  if (/catalog gateway unavailable|catalog unavailable/.test(b))
    return provider === "youtube"
      ? "Busca do YouTube indisponível no servidor (falta a API key). Contate o operador."
      : "Busca indisponível no servidor. Contate o operador.";

  // Playlists editoriais do Spotify no modo Dev.
  if (provider === "spotify" && /status 404/.test(b))
    return "Playlist não acessível (playlists editoriais/algorítmicas do Spotify são bloqueadas no modo Dev). Use uma playlist sua.";
  if (provider === "spotify" && /status 403/.test(b))
    return "Acesso negado pelo Spotify (limite do modo Dev).";

  return null;
}

/* frase genérica por status HTTP quando nenhuma palavra-chave casou. */
function byStatus(status: number, provider: Provider): string {
  const who =
    provider === "youtube" ? "do YouTube" : provider === "spotify" ? "do Spotify" : "";
  switch (true) {
    case status === 0:
      return "Sem conexão com o servidor. Verifique a rede e tente de novo.";
    case status === 400:
      return "Requisição inválida. Confira os dados e tente de novo.";
    case status === 401:
      return `Não autorizado ${who}. Reconecte a conta.`.replace("  ", " ").trim();
    case status === 403:
      return `Acesso negado ${who}.`.replace("  ", " ").trim();
    case status === 404:
      return "Não encontrado.";
    case status === 409:
      return "Conflito com o estado atual. Recarregue e tente de novo.";
    case status === 422:
      return "Dados não aceitos pelo servidor.";
    case status === 429:
      return "Muitas requisições. Espere um pouco e tente de novo.";
    case status === 503:
      return "Serviço temporariamente indisponível. Tente em instantes.";
    case status >= 500:
      return "Erro no servidor. Tente novamente em instantes.";
    default:
      return `Falha na operação (código ${status}).`;
  }
}

/* humaniza um Response de erro (res.ok === false). Lê o corpo do backend, casa
   por palavra-chave depois por status, e devolve {status, message, raw}. */
export async function humanizeResponse(
  res: Response,
  provider: Provider = "",
): Promise<HumanError> {
  const raw = await readBackendError(res);
  const message = byKeyword(raw, provider) ?? byStatus(res.status, provider);
  return { status: res.status, message, raw };
}

/* humaniza um corpo de erro JÁ LIDO. Existe porque o upload usa XHR (é o único
   jeito de ter progresso real) e não tem `Response` pra passar — sem isto o texto
   cru do backend ("exit status 1") ia direto pra lista de arquivos. */
export function humanizeRaw(raw: string, status: number, provider: Provider = ""): string {
  return byKeyword((raw ?? "").trim(), provider) ?? byStatus(status, provider);
}

/* humaniza uma exceção já lançada (catch dos hooks). Erro nosso já vem com texto
   amigável (a `.message` do HTTPError/Error criado por humanizeResponse) → repassa.
   TypeError "Failed to fetch" (rede caiu antes da resposta) → frase de rede. */
export function humanizeThrown(e: unknown, fallback = "Falha inesperada."): string {
  if (isNetworkError(e)) {
    // distingue os dois casos, porque a ação do operador é diferente: sem rede ele
    // espera a internet voltar; com rede, o servidor do AutoDJ é que está fora.
    return navigator?.onLine === false
      ? "Sem internet — a ação não foi enviada. Ela volta a funcionar quando a rede voltar."
      : "O servidor do AutoDJ não respondeu. A ação não foi enviada.";
  }
  if (e instanceof Error && e.message) return e.message;
  return fallback;
}

/* isNetworkError reconhece a falha de rede do fetch, que chega como TypeError com
   texto CRU e diferente por navegador — "Failed to fetch" (Chrome), "NetworkError
   when attempting to fetch resource" (Firefox), "Load failed" (Safari). Sem isto,
   esse texto em inglês vaza direto pro toast do operador. */
export function isNetworkError(e: unknown): boolean {
  return (
    e instanceof TypeError &&
    /failed to fetch|networkerror|load failed|network request failed/i.test(e.message)
  );
}
