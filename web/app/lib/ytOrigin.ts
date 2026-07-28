/* ytOrigin — guarda de origem para o embed do YouTube (v0.2.10).

   FATO MEDIDO (27/07/2026, mesmo vídeo, mesmo host youtube-nocookie, mesmo
   playerVars, lado a lado no mesmo Chrome):

     origem http://127.0.0.1:8099  → onError 150 + getVideoData().errorCode "auth"
                                     (o player nunca sai de UNSTARTED)
     origem http://localhost:8099  → sem erro, vídeo carrega e toca
     origem https://<domínio>      → sem erro

   Ou seja: o YouTube recusa incorporar vídeo MONETIZADO (VEVO/gravadora/oficial)
   quando o host da origem é um IP literal. Vídeos livres (ex.: Big Buck Bunny)
   tocam em qualquer origem, o que fazia o bug parecer "só alguns vídeos somem".

   Consequência no app: onError 150 → status.error "embed:150" → o playout PULA a
   faixa (effect 3a) → a faixa "some do deck" logo depois de arrastar. O pulo em si
   está correto (150 é incorporação desabilitada); errado era o diagnóstico — em
   127.0.0.1 TODO clipe musical vira "não incorporável".

   Prod (autodj.andrebassi.com.br) usa domínio real → não é afetado. Isto é um
   problema EXCLUSIVO do dev local, e a correção é abrir o app em localhost. */

// IPv4 literal (1.2.3.4) ou IPv6 entre colchetes ([::1]) — o que o YouTube trata
// como origem "sem domínio" e bloqueia p/ conteúdo monetizado.
const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/;

/** A origem atual (ou a passada) é um IP literal? */
export function isBareIpOrigin(hostname?: string): boolean {
  const h =
    hostname ?? (typeof window !== "undefined" ? window.location.hostname : "");
  if (!h) return false;
  return IPV4.test(h) || h.startsWith("[");
}

/** Mesma URL servida em localhost — o destino sugerido ao usuário. Null fora do browser. */
export function localhostEquivalentURL(): string | null {
  if (typeof window === "undefined") return null;
  const { protocol, port, pathname, search, hash } = window.location;
  return `${protocol}//localhost${port ? ":" + port : ""}${pathname}${search}${hash}`;
}

/** Origem só com esquema+host+porta (p/ mensagens curtas). Vazio fora do browser. */
export function currentOrigin(): string {
  return typeof window !== "undefined" ? window.location.origin : "";
}
