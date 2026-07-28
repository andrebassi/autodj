package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"autodj/domain/library"
)

// FolderCatalog guarda o CATÁLOGO de pastas criadas pelo operador.
//
// Por que uma tabela só de nomes, se a pasta já vive no JSON da faixa: pasta
// recém-criada ainda não tem faixa dentro. Sem este catálogo, "criar pasta"
// sumiria no primeiro reload — o operador criaria "rock", sairia da tela e não
// acharia mais. A listagem final é a UNIÃO deste catálogo com as pastas em uso
// pelas faixas (ver LibraryService.Folders).
type FolderCatalog struct {
	db *sql.DB
}

// Folders returns a FolderCatalog backed by the same database as this TrackRepo.
func (r *TrackRepo) Folders() *FolderCatalog { return &FolderCatalog{db: r.db} }

// List devolve os nomes do catálogo em ordem alfabética (ignorando caixa).
func (r *FolderCatalog) List(ctx context.Context) ([]string, error) {
	rs, err := r.db.QueryContext(ctx, `SELECT name FROM folders ORDER BY lower(name)`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list folders: %w", err)
	}
	defer rs.Close()
	var out []string
	for rs.Next() {
		var n string
		if err := rs.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rs.Err()
}

// Create grava a pasta. Idempotente: recriar a mesma (ignorando caixa) não
// duplica nem estoura — o operador não deve levar erro por clicar duas vezes.
func (r *FolderCatalog) Create(ctx context.Context, name string) error {
	n, err := library.NormalizeFolder(name)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO folders (name, name_lower) VALUES (?, lower(?))
		 ON CONFLICT(name_lower) DO NOTHING`, n, n)
	if err != nil {
		return fmt.Errorf("sqlite: create folder: %w", err)
	}
	return nil
}

// Delete remove a pasta do catálogo. NÃO mexe nas faixas — decisão do produto:
// excluir pasta não apaga música. Quem tira a pasta das faixas é o serviço.
// Idempotente: apagar pasta inexistente não é erro.
func (r *FolderCatalog) Delete(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM folders WHERE name_lower = lower(?)`, name)
	if err != nil {
		return fmt.Errorf("sqlite: delete folder: %w", err)
	}
	return nil
}
