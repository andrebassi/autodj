package httpapi

import (
	"net/http"
	"strings"
)

// Cabeçalhos de cache do bundle estático. O Next exporta os assets com hash de
// conteúdo no nome (page-a999bd5f.js, 9ff388a7.css), então eles são imutáveis por
// construção: mudou 1 byte, muda o nome. O documento de entrada (index.html) NÃO
// tem hash — é ele que aponta pros nomes hasheados.
//
// Sem Cache-Control o Go (http.FileServer) manda só Last-Modified, e aí o browser
// aplica cache HEURÍSTICO (~10% da idade do arquivo). Foi isso que segurou a UI
// velha em produção depois de um deploy: o HTML cacheado continuava apontando pro
// chunk antigo, mesmo com servidor e assets novos no ar.
//
// O par correto é assimétrico:
//   - asset hasheado → immutable, um ano: zero revalidação, nem 304.
//   - qualquer outra coisa → no-cache: guarda, mas revalida sempre (304 é barato).
//
// "no-cache" NÃO é "não guarde" (isso é no-store) — é "guarde e pergunte antes de
// usar". O HTML é pequeno; o custo é um condicional por load, e em troca deploy
// nunca mais fica invisível.
const (
	// CacheImmutable vale só para caminhos com hash de conteúdo no nome.
	CacheImmutable = "public, max-age=31536000, immutable"
	// CacheRevalidate vale para o HTML e qualquer asset sem hash no nome.
	CacheRevalidate = "no-cache"
)

// hashedPrefixes lista os diretórios cujos nomes de arquivo carregam hash de
// conteúdo. Só o que está aqui pode ser marcado immutable — errar pra esse lado
// deixa o usuário preso num asset velho por um ano, sem escapatória.
var hashedPrefixes = []string{"/_next/static/"}

// CacheControlFor decide o Cache-Control de um caminho do bundle estático.
// Caminho com hash de conteúdo → immutable; qualquer outro (HTML incluso) →
// revalidação obrigatória.
func CacheControlFor(path string) string {
	for _, p := range hashedPrefixes {
		if strings.HasPrefix(path, p) {
			return CacheImmutable
		}
	}
	return CacheRevalidate
}

// StaticCacheControl envelopa o handler de arquivos estáticos carimbando o
// Cache-Control certo por caminho. Não sobrescreve um valor que o handler interno
// já tenha definido.
func StaticCacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if w.Header().Get("Cache-Control") == "" {
			w.Header().Set("Cache-Control", CacheControlFor(r.URL.Path))
		}
		next.ServeHTTP(w, r)
	})
}
