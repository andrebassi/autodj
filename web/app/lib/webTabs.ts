/* Web side-panel tab TYPES. The tab DATA now comes from the API
   (GET /api/v1/ui/webtabs) via the useWebTabs hook — no hardcoded list here.

   No RadioPro original, este painel é um WEBVIEW nativo (app Windows) que ignora
   X-Frame-Options. Na build web usamos <iframe>: só carrega sites que permitem
   framing; WhatsApp Web / X / Facebook bloqueiam framing e só abrem no webview
   desktop (Wails, Épico 9). Por isso cada aba carrega `framed`. */

export type { SubTab, WebTab } from "@/lib/uiContent";
export type { WebTab as TabDef } from "@/lib/uiContent";
