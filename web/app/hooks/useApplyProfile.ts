"use client";

import { useEffect } from "react";
import type { ProfileDTO } from "@/lib/api";

/* useApplyProfile projects the operator's saved layout onto the live shell
   (Story 7.4). Theme and density become data-attributes on <html> (CSS in
   globals.css swaps the palette / tightens spacing); panel visibility toggles the
   matching [data-panel] slots on/off. It runs whenever the profile changes and
   restores the defaults on logout (profile === null), so nothing leaks between
   sessions. Order is persisted in the profile but visual reordering across the
   separate panes is out of scope here — hide + theme + density are the applied
   effects. */
export function useApplyProfile(profile: ProfileDTO | null): void {
  useEffect(() => {
    const root = document.documentElement;
    const theme = profile?.theme ?? "claro"; // padrão CLARO (casa com Default do backend)
    const density = profile?.density ?? "confortavel";
    root.dataset.theme = theme;
    root.dataset.density = density;
    // touch mode + monitor count (Story 9.2): CSS under [data-touch="on"] enlarges
    // hit targets; data-screens lets the layout hint the console it spreads wide.
    root.dataset.touch = profile?.touch ? "on" : "off";
    root.dataset.screens = String(profile?.screens ?? 1);

    // panel visibility: default every known slot to shown, then hide the ones the
    // profile turned off. A slot is a display:contents wrapper, so hiding it drops
    // the pane without disturbing the surrounding grid. Each slot also carries its
    // assigned monitor as data-screen so the console can style panels by monitor
    // when two are configured (9.2).
    const hidden = new Set(
      (profile?.panels ?? []).filter((p) => !p.visible).map((p) => p.key),
    );
    const screenOf = new Map(
      (profile?.panels ?? []).map((p) => [p.key, p.screen ?? 0]),
    );
    for (const el of Array.from(
      document.querySelectorAll<HTMLElement>("[data-panel]"),
    )) {
      const key = el.dataset.panel ?? "";
      el.style.display = hidden.has(key) ? "none" : "";
      el.dataset.screen = String(screenOf.get(key) ?? 0);
    }

    return () => {
      // on unmount / before re-apply, clear the theme + density stamps.
      delete root.dataset.theme;
      delete root.dataset.density;
      delete root.dataset.touch;
      delete root.dataset.screens;
    };
  }, [profile]);
}
