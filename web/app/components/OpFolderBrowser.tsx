import type { OpFolder } from "@/lib/uiContent";

/* navegador de pastas (INTERNACIONAIS / NACIONAIS) — sistema de pastas, sem cadastro */
export function OpFolderBrowser({
  folders,
  selected,
  onPick,
}: {
  folders: OpFolder[];
  selected: string;
  onPick: (name: string) => void;
}) {
  return (
    <div className="opfolders">
      {folders.map((f) => (
        <button
          key={f.name}
          className={"opfolder" + (selected === f.name ? " on" : "")}
          onClick={() => onPick(f.name)}
          title={`${f.count} mídias`}
        >
          <span className="opfolder-ico">{f.ico}</span>
          <span className="opfolder-name">{f.name}</span>
        </button>
      ))}
    </div>
  );
}
