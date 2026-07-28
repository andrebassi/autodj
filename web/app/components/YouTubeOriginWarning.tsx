"use client";

import { isBareIpOrigin, localhostEquivalentURL, currentOrigin } from "@/lib/ytOrigin";

/* Aviso de origem IP (v0.2.10). O YouTube recusa incorporar clipe monetizado
   (VEVO/gravadora) quando a página está num IP literal — o player devolve
   onError 150 / errorCode "auth" e o playout pula a faixa, que "some do deck"
   logo após o arraste. A MESMA URL em localhost toca. Ver lib/ytOrigin.ts p/ a
   medição. Só aparece em dev com origem IP; em domínio real não renderiza nada. */
export function YouTubeOriginWarning() {
  if (!isBareIpOrigin()) return null;
  const url = localhostEquivalentURL();
  return (
    <div className="spot-warn spot-err">
      ⚠️ Você abriu o AutoDJ em <b>{currentOrigin()}</b> (IP). O YouTube só
      incorpora clipes musicais (VEVO/gravadora) a partir de um domínio — em IP
      eles falham com erro 150 e a faixa é pulada, sumindo do deck. Abra em{" "}
      {url ? (
        <a href={url}>
          <b>localhost</b>
        </a>
      ) : (
        <b>localhost</b>
      )}{" "}
      que toca normal. Em produção (domínio real) não acontece.
    </div>
  );
}
