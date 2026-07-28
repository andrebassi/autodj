/* Logos das fontes de música, como SVG INLINE.

   Por que inline e não <img>: o app é export estático servido pelo próprio Go, e
   qualquer arquivo externo vira mais uma requisição (e mais um caminho pra 404 no
   deploy). SVG no bundle escala sem borrar em qualquer tamanho e herda cor quando
   preciso.

   Antes eram os emoji 🟢 e 🔴 — dois círculos coloridos que não diziam de qual
   serviço se tratava, e que ainda mudam de desenho conforme o sistema operacional.

   Marcas são de Spotify AB e Google LLC; aqui aparecem só pra identificar de onde a
   faixa vem, que é o uso nominativo previsto pelas duas marcas. Cores oficiais.

   `title` deixa o SVG acessível (leitor de tela lê o nome do serviço); quando o
   rótulo textual já está do lado, quem chama passa `decorative` e o ícone some da
   árvore de acessibilidade em vez de duplicar a leitura. */

const BRAND = {
  spotify: {
    color: "#1DB954",
    label: "Spotify",
    path: "M12 0C5.4 0 0 5.4 0 12s5.4 12 12 12 12-5.4 12-12S18.66 0 12 0zm5.521 17.34c-.24.359-.66.48-1.021.24-2.82-1.74-6.36-2.101-10.561-1.141-.418.122-.779-.24-.899-.6-.12-.421.24-.781.6-.901 4.561-1.021 8.52-.6 11.64 1.32.42.18.479.659.241 1.082zm1.44-3.3c-.301.42-.841.6-1.262.3-3.239-1.98-8.159-2.58-11.939-1.38-.479.12-1.02-.12-1.14-.6-.12-.48.12-1.021.6-1.141C9.6 9.9 15 10.56 18.72 12.84c.361.181.54.78.241 1.2zm.12-3.36C15.24 8.4 8.82 8.16 5.16 9.301c-.6.179-1.2-.181-1.38-.721-.18-.601.18-1.2.72-1.381 4.26-1.26 11.28-1.02 15.72 1.621.539.3.719 1.02.419 1.56-.299.421-1.02.599-1.559.3z",
  },
  youtube: {
    color: "#FF0000",
    label: "YouTube",
    path: "M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z",
  },
} as const;

export type BrandName = keyof typeof BRAND;

/** isBrand diz se uma chave de categoria tem logo próprio. */
export function isBrand(k: string): k is BrandName {
  return k === "spotify" || k === "youtube";
}

export function BrandIcon({
  name,
  size = 16,
  decorative = false,
}: {
  name: BrandName;
  size?: number;
  /** true quando o nome do serviço já está escrito ao lado (evita leitura dupla) */
  decorative?: boolean;
}) {
  const b = BRAND[name];
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill={b.color}
      role={decorative ? undefined : "img"}
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : b.label}
      className="brand-ico"
    >
      {!decorative && <title>{b.label}</title>}
      <path d={b.path} />
    </svg>
  );
}
