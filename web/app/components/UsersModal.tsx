"use client";

import { useState } from "react";
import { useUsers } from "@/hooks/useUsers";
import { ROLE_LABELS } from "@/lib/api";

const ROLES = ["admin", "operador", "locutor"] as const;

/* admin-only account manager (Story 7.3): list + create + change role/password +
   remove operators. Gated by users.manage — opened only from the badge menu when
   the current user holds it, and the backend re-checks (a 403 shows a notice). The
   permission matrix lives in the domain; this UI only edits who exists and their
   role. Reuses the mix-* modal styling. */
export function UsersModal({
  currentUserId,
  onClose,
}: {
  currentUserId: string;
  onClose: () => void;
}) {
  const { users, loaded, forbidden, busy, error, add, edit, remove } =
    useUsers(true);

  const [name, setName] = useState("");
  const [role, setRole] = useState<string>("operador");
  const [password, setPassword] = useState("");

  const onAdd = async () => {
    if (!name.trim() || !password) return;
    if (await add(name.trim(), role, password)) {
      setName("");
      setPassword("");
      setRole("operador");
    }
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" style={{ maxWidth: 560 }} onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Usuários & Acessos</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && forbidden && (
            <fieldset className="mix-group">
              <legend>Sem permissão</legend>
              <p className="mix-lbl">
                Gerência de usuários exige o papel Administrador.
              </p>
            </fieldset>
          )}

          {loaded && !forbidden && (
            <>
              <fieldset className="mix-group">
                <legend>Contas ({users.length})</legend>
                <table className="user-table">
                  <thead>
                    <tr>
                      <th>Usuário</th>
                      <th>Papel</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((u) => (
                      <tr key={u.id}>
                        <td className="mono">{u.name}</td>
                        <td>
                          <select
                            className="mix-num"
                            value={u.role}
                            disabled={busy}
                            onChange={(e) => void edit(u.id, e.target.value, "")}
                          >
                            {ROLES.map((r) => (
                              <option key={r} value={r}>
                                {ROLE_LABELS[r]}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td>
                          <button
                            className="mix-close"
                            disabled={busy || u.id === currentUserId}
                            title={
                              u.id === currentUserId
                                ? "Não é possível remover a própria conta"
                                : "Remover"
                            }
                            onClick={() => void remove(u.id)}
                          >
                            ✕
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </fieldset>

              <fieldset className="mix-group">
                <legend>Nova Conta</legend>
                <div className="mix-row">
                  <span className="mix-lbl">Usuário</span>
                  <input
                    className="mix-num"
                    type="text"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                  <span className="mix-lbl">Papel</span>
                  <select
                    className="mix-num"
                    value={role}
                    onChange={(e) => setRole(e.target.value)}
                  >
                    {ROLES.map((r) => (
                      <option key={r} value={r}>
                        {ROLE_LABELS[r]}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="mix-row">
                  <span className="mix-lbl">Senha</span>
                  <input
                    className="mix-num"
                    type="password"
                    autoComplete="new-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                  <button
                    className="mix-save"
                    onClick={onAdd}
                    disabled={busy || !name.trim() || !password}
                  >
                    Adicionar +
                  </button>
                </div>
              </fieldset>
            </>
          )}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
