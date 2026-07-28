import type { OpCategoryKey } from "@/lib/operational";
import type { OpCategory } from "@/lib/uiContent";
import { BrandIcon, isBrand } from "@/components/BrandIcon";

/* barra de categorias no topo do Operacional — pills inline compactas */
export function OpCategoryBar({
  categories,
  active,
  onPick,
  disabledKeys,
  disabledTitle,
  disabledTitles,
}: {
  categories: OpCategory[];
  active: OpCategoryKey;
  onPick: (k: OpCategoryKey) => void;
  // chips desabilitados (ex.: Músicas sem faixa local): não clicáveis, esmaecidos
  disabledKeys?: Set<OpCategoryKey>;
  disabledTitle?: string;
  // motivo POR chip — cada aba desligada tem a sua razão (sem faixa local vs
  // função ainda não liberada). Cai no `disabledTitle` quando a chave não estiver
  // no mapa.
  disabledTitles?: Partial<Record<OpCategoryKey, string>>;
}) {
  return (
    <div className="opcat-bar">
      {categories.map((c) => {
        const off = disabledKeys?.has(c.key as OpCategoryKey) ?? false;
        return (
          <button
            key={c.key}
            className={"opcat" + (active === c.key ? " on" : "") + (off ? " off" : "")}
            onClick={() => !off && onPick(c.key)}
            disabled={off}
            title={off ? disabledTitles?.[c.key as OpCategoryKey] ?? disabledTitle : undefined}
          >
            {/* fonte com marca própria mostra o logo; o resto segue com o emoji.
                `decorative`: o nome do serviço já vem no rótulo ao lado. */}
            <span className="opcat-ico">
              {isBrand(c.key) ? <BrandIcon name={c.key} size={15} decorative /> : c.ico}
            </span>
            <span className="opcat-lbl">{c.label}</span>
          </button>
        );
      })}
    </div>
  );
}
