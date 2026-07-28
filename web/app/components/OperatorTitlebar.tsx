"use client";
import type { ReactNode } from "react";
import { useState } from "react";
import { AutopilotToggle } from "@/components/AutopilotToggle";
import { ChangelogModal } from "@/components/ChangelogModal";
import { UpdateBadge } from "@/components/UpdateBadge";
import { useVersionCheck } from "@/hooks/useVersionCheck";
import { APP_VERSION } from "@/lib/version";

/* top window chrome — AutoDJ brand + piloto automático + crachá do operador +
   toggle do painel (☰). `badge` é o UserBadge (identidade + sair), injetado pela
   page só quando há sessão. Clicar na versão abre o changelog (ChangelogModal).
   Ao lado da versão mora o UpdateBadge: aparece só quando o servidor reporta versão
   diferente da que esta aba carregou (deploy com a aba aberta — ver useVersionCheck). */
export function OperatorTitlebar({
  onMenu,
  menuActive,
  badge,
}: {
  onMenu?: () => void;
  menuActive?: boolean;
  badge?: ReactNode;
}) {
  const [changelog, setChangelog] = useState(false);
  const { updateAvailable, serverVersion } = useVersionCheck();
  return (
    <header className="op-titlebar">
      <div className="brand">
        <span className="brand-mark">▲</span>
        <span className="brand-name">AutoDJ</span>
        <button
          className="brand-ver"
          title="Ver novidades desta versão"
          onClick={() => setChangelog(true)}
        >
          {APP_VERSION}
        </button>
        {updateAvailable && <UpdateBadge serverVersion={serverVersion} />}
      </div>
      {changelog && <ChangelogModal onClose={() => setChangelog(false)} />}
      <div className="op-actions">
        <AutopilotToggle />
        {badge}
      </div>
      <button
        className={"op-menu" + (menuActive ? " on" : "")}
        onClick={onMenu}
        title="Abrir/fechar painel"
        aria-label="Abrir/fechar painel"
        aria-pressed={menuActive}
      >
        ≡
      </button>
    </header>
  );
}
