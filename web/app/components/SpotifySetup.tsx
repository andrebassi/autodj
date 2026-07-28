"use client";

import { useEffect, useState } from "react";
import { BrandIcon } from "@/components/BrandIcon";

/* guia de setup do Spotify — mostrado quando a integração não está configurada
   (o servidor subiu sem AUTODJ_SPOTIFY_CLIENT_ID, então as rotas /spotify 404).
   Conectar exige um app do Spotify Developer (PKCE, client público, sem secret no
   binário): o usuário cria o app, registra a Redirect URI abaixo e sobe o servidor
   com o Client ID. Depois disso o painel troca pro card de conexão. */
export function SpotifySetup() {
  const [redirect, setRedirect] = useState("http://localhost:8099/api/v1/spotify/callback");
  useEffect(() => {
    setRedirect(window.location.origin + "/api/v1/spotify/callback");
  }, []);

  const copy = (v: string) => void navigator.clipboard?.writeText(v);

  return (
    <div className="spot">
      <div className="spot-setup">
        <div className="spot-setup-head">
          <span className="spot-hero-logo">
            <BrandIcon name="spotify" size={44} />
          </span>
          <div>
            <h3 className="spot-hero-title">Integração não configurada</h3>
            <p className="spot-hero-sub">
              Conectar ao Spotify precisa de um app do Spotify Developer (gratuito).
              Três passos e você já busca o catálogo aqui.
            </p>
          </div>
        </div>

        <ol className="spot-steps">
          <li>
            <b>Crie um app</b> em{" "}
            <a
              href="https://developer.spotify.com/dashboard"
              target="_blank"
              rel="noreferrer"
            >
              developer.spotify.com/dashboard
            </a>
            .
          </li>
          <li>
            <b>Redirect URI</b> — registre exatamente esta no app:
            <div className="spot-code">
              <code>{redirect}</code>
              <button onClick={() => copy(redirect)}>copiar</button>
            </div>
          </li>
          <li>
            <b>Suba o servidor com o Client ID</b> do app:
            <div className="spot-code">
              <code>AUTODJ_SPOTIFY_CLIENT_ID=&lt;seu-client-id&gt;</code>
              <button onClick={() => copy("AUTODJ_SPOTIFY_CLIENT_ID=")}>copiar</button>
            </div>
          </li>
        </ol>

        <p className="spot-hero-note">
          Client público (PKCE) — nenhum secret fica no binário. Ao reiniciar com
          o Client ID, este painel troca para o botão <b>Conectar conta</b>.
        </p>
      </div>
    </div>
  );
}
