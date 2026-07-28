"use client";

import { useEffect, useState } from "react";
import { BrandIcon } from "@/components/BrandIcon";

/* guia de setup do YouTube — mostrado quando a integração não está configurada (o
   servidor subiu sem AUTODJ_YOUTUBE_CLIENT_ID/CLIENT_SECRET/API_KEY, então as rotas
   /youtube 404). Diferente do Spotify (PKCE público, sem secret), o YouTube usa OAuth
   do Google com CLIENT SECRET (auth-code flow) + uma API KEY própria pra busca
   (YouTube Data API v3). O usuário cria um projeto no Google Cloud, habilita a Data
   API v3, monta a tela de consentimento (test users), gera as credenciais e sobe o
   servidor com as 3 vars. Depois disso o painel troca pro card de conexão. */
export function YouTubeSetup() {
  const [redirect, setRedirect] = useState("http://localhost:8099/api/v1/youtube/callback");
  useEffect(() => {
    setRedirect(window.location.origin + "/api/v1/youtube/callback");
  }, []);

  const copy = (v: string) => void navigator.clipboard?.writeText(v);

  return (
    <div className="spot">
      <div className="spot-setup">
        <div className="spot-setup-head">
          <span className="spot-hero-logo">
            <BrandIcon name="youtube" size={44} />
          </span>
          <div>
            <h3 className="spot-hero-title">Integração não configurada</h3>
            <p className="spot-hero-sub">
              Conectar ao YouTube precisa de um projeto no Google Cloud (gratuito) com
              a YouTube Data API v3 habilitada. Alguns passos e você já busca o
              catálogo aqui — e o player toca no navegador, sem Premium.
            </p>
          </div>
        </div>

        <ol className="spot-steps">
          <li>
            <b>Crie um projeto</b> e habilite a <b>YouTube Data API v3</b> em{" "}
            <a
              href="https://console.cloud.google.com/apis/library/youtube.googleapis.com"
              target="_blank"
              rel="noreferrer"
            >
              console.cloud.google.com
            </a>
            .
          </li>
          <li>
            <b>Tela de consentimento OAuth</b> — configure como <i>External</i> e
            adicione seu e-mail em <b>Test users</b> (enquanto o app não for
            verificado, só test users conectam).
          </li>
          <li>
            <b>Credenciais</b> → crie um <b>OAuth client ID</b> (tipo <i>Web
            application</i>) e uma <b>API key</b>.
          </li>
          <li>
            <b>Redirect URI</b> — registre exatamente esta no OAuth client:
            <div className="spot-code">
              <code>{redirect}</code>
              <button onClick={() => copy(redirect)}>copiar</button>
            </div>
          </li>
          <li>
            <b>Suba o servidor com as 3 variáveis</b> (client id/secret + API key):
            <div className="spot-code">
              <code>
                AUTODJ_YOUTUBE_CLIENT_ID=&lt;client-id&gt;
                {"\n"}AUTODJ_YOUTUBE_CLIENT_SECRET=&lt;client-secret&gt;
                {"\n"}AUTODJ_YOUTUBE_API_KEY=&lt;api-key&gt;
              </code>
              <button
                onClick={() =>
                  copy(
                    "AUTODJ_YOUTUBE_CLIENT_ID=\nAUTODJ_YOUTUBE_CLIENT_SECRET=\nAUTODJ_YOUTUBE_API_KEY=",
                  )
                }
              >
                copiar
              </button>
            </div>
          </li>
        </ol>

        <p className="spot-hero-note">
          O <b>client secret</b> fica só no servidor (auth-code flow), nunca no
          binário do navegador. A <b>API key</b> serve pra busca (Data API v3, cota
          ~100 buscas/dia). Ao reiniciar com as 3 vars, este painel troca para o botão{" "}
          <b>Conectar com Google</b>.
        </p>
      </div>
    </div>
  );
}
