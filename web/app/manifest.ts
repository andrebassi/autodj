import type { MetadataRoute } from "next";

/* Manifest de app instalável (PWA).

   NÃO é enfeite: o Chrome concede **permissão de autoplay** a aplicativos
   INSTALADOS. É a única saída legítima pro caso "dei Cmd+R e o áudio não voltou
   sozinho" — num site comum o browser exige interação com a página nova, e o
   atalho de recarregar acontece no chrome do browser, não na página. Medido
   (0.2.26): nem o truque do autoplay mudo salva `<audio>`, ele é recusado igual.

   Instalado (ou com engagement alto no site, o que acontece com uso diário), o
   playout volta a tocar sozinho depois de um refresh — sem clique nenhum.

   `display: standalone` é o que torna a instalação possível; `start_url: "/"`
   mantém a estação. Sem ícones próprios por ora: o Chrome usa o favicon, e
   inventar ícone aqui seria trabalho de marca, não de playout. */
// `output: export` (bundle estático servido pelo Go) exige a rota declarada como
// estática — sem isto o build falha com "force-static not configured".
export const dynamic = "force-static";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "AutoDJ — Player",
    short_name: "AutoDJ",
    description: "Rádio e DJ no navegador: fila, decks e playout ao vivo",
    start_url: "/",
    display: "standalone",
    orientation: "landscape",
    background_color: "#0f172a",
    theme_color: "#0f172a",
    lang: "pt-BR",
  };
}
