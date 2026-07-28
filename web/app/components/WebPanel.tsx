"use client";

import { useState } from "react";
import { useWebTabs } from "@/hooks/useWebTabs";
import { WebFrame } from "@/components/WebFrame";

/* headless embedded-browser content (tabs + social subtabs + frame).
   Chrome (título/✕) fica no RightPanel — este só renderiza o miolo. */
export function WebPanel() {
  const { tabs } = useWebTabs();
  const [tabName, setTabName] = useState("Sociais");
  const [social, setSocial] = useState("WhatsApp");
  const tab = tabs.find((t) => t.name === tabName) ?? tabs[0];
  const sub = tab?.subtabs?.find((s) => s.name === social);
  const active = sub ?? tab;

  return (
    <div className="webpanel-body">
      <nav className="webpanel-tabs">
        {tabs.map((t) => (
          <button
            key={t.name}
            className={"ptab" + (t.name === tabName ? " active" : "")}
            onClick={() => setTabName(t.name)}
          >
            <span className="ptab-ico">{t.icon}</span>
            <span className="ptab-lbl">{t.name}</span>
          </button>
        ))}
      </nav>

      {tab?.subtabs && (
        <div className="social-tabs">
          {tab.subtabs.map((s) => (
            <button
              key={s.name}
              className={"stab s-" + s.brand + (s.name === social ? " active" : "")}
              onClick={() => setSocial(s.name)}
            >
              {s.name}
            </button>
          ))}
        </div>
      )}

      {active && <WebFrame url={active.url} framed={active.framed} />}
    </div>
  );
}
