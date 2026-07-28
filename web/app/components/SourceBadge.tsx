"use client";

/* Selo de origem do deck: logo da marca + wordmark, informando em que motor a
   faixa toca (Spotify ou YouTube). Só aparece p/ esses dois — faixa local não
   tem marca. Elegante e discreto: logo oficial (SVG inline) + nome ao lado.
   Um componente por arquivo. */

function SpotifyLogo() {
  return (
    <svg viewBox="0 0 168 168" width="14" height="14" aria-hidden="true">
      <circle cx="84" cy="84" r="84" fill="#1ED760" />
      <path
        fill="#000"
        d="M122.6 120.4c-1.6 2.6-5 3.4-7.6 1.8-20.8-12.7-47-15.6-77.8-8.6-3 .7-5.9-1.2-6.6-4.1-.7-3 1.2-5.9 4.1-6.6 33.7-7.7 62.7-4.4 86 9.9 2.6 1.6 3.5 5 1.9 7.6zm10.3-22.9c-2 3.2-6.2 4.3-9.4 2.3-23.8-14.6-60-18.9-88.1-10.4-3.6 1.1-7.4-1-8.5-4.6-1.1-3.6 1-7.4 4.6-8.5 32.1-9.7 72-4.9 99.4 11.9 3.1 2 4.2 6.2 2 9.3zm.9-23.9C105.3 56.7 60.1 55 33.9 63c-4.3 1.3-8.9-1.1-10.2-5.5-1.3-4.3 1.1-8.9 5.5-10.2 30.1-9.1 80-7.3 112.9 12.2 3.9 2.3 5.2 7.3 2.9 11.2-2.3 3.9-7.4 5.2-11.2 2.9z"
      />
    </svg>
  );
}

function YouTubeLogo() {
  return (
    <svg viewBox="0 0 28 20" width="18" height="13" aria-hidden="true">
      <path
        fill="#FF0000"
        d="M27.4 3.1c-.3-1.2-1.3-2.2-2.5-2.5C22.7 0 14 0 14 0S5.3 0 3.1.6C1.9.9.9 1.9.6 3.1.1 5.3.1 10 .1 10s0 4.7.5 6.9c.3 1.2 1.3 2.2 2.5 2.5C5.3 20 14 20 14 20s8.7 0 10.9-.6c1.2-.3 2.2-1.3 2.5-2.5.5-2.2.5-6.9.5-6.9s0-4.7-.5-6.9z"
      />
      <path fill="#fff" d="M11.2 14.3l7.3-4.3-7.3-4.3z" />
    </svg>
  );
}

function LocalLogo() {
  return (
    <svg viewBox="0 0 24 24" width="13" height="13" aria-hidden="true">
      <path
        fill="currentColor"
        d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3h-6zm-2 16a2 2 0 1 1 0-4 2 2 0 0 1 0 4z"
      />
    </svg>
  );
}

const BRANDS: Record<string, { logo: () => React.ReactElement; label: string; cls: string }> = {
  spotify: { logo: SpotifyLogo, label: "Spotify", cls: "src-spotify" },
  youtube: { logo: YouTubeLogo, label: "YouTube", cls: "src-youtube" },
  local: { logo: LocalLogo, label: "Local", cls: "src-local" },
};

/* `source` desconhecido/vazio → trata como local (arquivo PCM). Passe
   `hideLocal` p/ omitir o selo em faixas locais (contexto que só quer marca).
   `variant="deck"` → selo compacto/glassy p/ o fundo escuro do deck (discreto,
   não estica na coluna flex, com animação de entrada). */
export function SourceBadge({
  source,
  hideLocal = false,
  variant,
}: {
  source?: string;
  hideLocal?: boolean;
  variant?: "deck";
}) {
  const key = source === "spotify" || source === "youtube" ? source : "local";
  if (key === "local" && hideLocal) return null;
  const brand = BRANDS[key];
  const Logo = brand.logo;
  const cls =
    "src-badge " + brand.cls + (variant === "deck" ? " src-badge-deck" : "");
  return (
    <span className={cls} title={`Origem: ${brand.label}`}>
      <Logo />
      <span className="src-badge-txt">{brand.label}</span>
    </span>
  );
}
