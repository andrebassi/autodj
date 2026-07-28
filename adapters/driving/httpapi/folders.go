package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"autodj/domain/library"
)

// folderDTO é uma pasta na listagem. `auto` marca as derivadas de fonte
// (Spotify/YouTube) e a virtual "Sem pasta": a UI não deve oferecer renomear nem
// excluir essas — não existem pra serem editadas.
type folderDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Auto  bool   `json:"auto"`
}

// getFolders lista as pastas com contagem.
// GET /api/v1/library/folders → [{name,count,auto}]
func (rt *Router) getFolders(w http.ResponseWriter, r *http.Request) {
	fs, err := rt.lib.Folders(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]folderDTO, 0, len(fs))
	for _, f := range fs {
		out = append(out, folderDTO{Name: f.Name, Count: f.Count, Auto: f.Auto})
	}
	writeJSON(w, http.StatusOK, out)
}

// postFolder cria uma pasta vazia. Idempotente — recriar não é erro.
// POST /api/v1/library/folders {name} → 204
func (rt *Router) postFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	if err := rt.lib.CreateFolder(r.Context(), body.Name); err != nil {
		writeFolderErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteFolder remove a pasta do catálogo e das faixas. NUNCA apaga faixa.
// DELETE /api/v1/library/folders/{name} → 204
func (rt *Router) deleteFolder(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := rt.lib.DeleteFolder(r.Context(), name); err != nil {
		writeFolderErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// putTrackFolders troca as pastas de uma faixa.
// PUT /api/v1/library/tracks/{id}/folders {folders:[...]} → trackDTO
func (rt *Router) putTrackFolders(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Folders []string `json:"folders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	t, err := rt.lib.SetTrackFolders(r.Context(), chi.URLParam(r, "id"), body.Folders)
	if err != nil {
		writeFolderErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTrackDTO(t))
}

// writeFolderErr traduz os erros de pasta em status. Nome inválido e faixa de
// streaming são condição do PEDIDO (4xx), não falha do servidor: devolver 500
// faria o operador achar que o AutoDJ quebrou quando ele só digitou um nome vazio.
func writeFolderErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrFolderEmpty),
		errors.Is(err, library.ErrFolderTooLong),
		errors.Is(err, library.ErrTooManyFolders):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, library.ErrFolderStreaming):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "faixa do Spotify/YouTube fica na pasta do próprio serviço",
		})
	case errors.Is(err, library.ErrTrackNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		writeErr(w, err)
	}
}
