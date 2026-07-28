"use client";

import type { CSSProperties } from "react";
import type { YouTubeBackend } from "@/lib/api";

type Opt = {
  key: YouTubeBackend;
  ico: string;
  label: string;
  hint: string;
  title: string;
};

/* as duas vias convivem — o operador chaveia, nada é removido:

   - "free"  → InnerTube (mesmo protocolo do site do YouTube, via lib Go
               raitonoberu/ytsearch). Sem login, sem API key, 0 unidade de cota.
   - "api"   → YouTube Data API v3. Exige conta conectada (o login é ESCUDO DE COTA)
               e cada busca custa ~100 das ~10k unidades/dia. Em compensação vem com
               duração exata e filtro `embeddable` do próprio Google.

   Este componente é só o controle; quem guarda o estado é o YouTubePanel. */
const OPTS: Opt[] = [
  {
    key: "free",
    ico: "⚡",
    label: "Sem login",
    hint: "cota livre",
    title:
      "Busca direta no YouTube (InnerTube): não pede conta e não gasta a cota diária da API.",
  },
  {
    key: "api",
    ico: "🔐",
    label: "Minha conta",
    hint: "~100 buscas/dia",
    title:
      "Busca pela YouTube Data API: pede conta conectada e consome ~100 das ~10k unidades/dia.",
  },
];

export function YouTubeBackendSwitch({
  value,
  onChange,
  connected,
}: {
  value: YouTubeBackend;
  onChange: (b: YouTubeBackend) => void;
  // conta conectada? só decora a via "api" (aviso de login pendente) — a escolha
  // continua livre; desconectado, o painel mostra o hero de conexão.
  connected: boolean;
}) {
  const idx = value === "api" ? 1 : 0;
  const glide: CSSProperties = {
    width: "calc((100% - 8px) / 2)",
    transform: `translateX(${idx * 100}%)`,
  };

  return (
    <div className="ytbk" role="radiogroup" aria-label="Motor da busca">
      <span className="ytbk-glide" style={glide} aria-hidden />
      {OPTS.map((o) => {
        const on = o.key === value;
        const pending = o.key === "api" && !connected;
        return (
          <button
            key={o.key}
            type="button"
            role="radio"
            aria-checked={on}
            title={o.title}
            className={"ytbk-opt" + (on ? " on" : "")}
            onClick={() => onChange(o.key)}
          >
            <span className="ytbk-ico">{o.ico}</span>
            <span className="ytbk-txt">
              <span className="ytbk-lbl">{o.label}</span>
              <span className="ytbk-hint">
                {pending ? "pede login" : o.hint}
              </span>
            </span>
          </button>
        );
      })}
    </div>
  );
}
