package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCacheControlFor(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		// Só o que tem hash de conteúdo no nome pode ser immutable.
		{"chunk js hasheado", "/_next/static/chunks/app/page-a999bd5f03d9f95d.js", CacheImmutable},
		{"css hasheado", "/_next/static/css/9ff388a777fa0f6f.css", CacheImmutable},
		{"media do next", "/_next/static/media/font-abc123.woff2", CacheImmutable},

		// O documento de entrada é quem aponta pros nomes hasheados: nunca immutable,
		// senão um deploy fica invisível até o cache expirar.
		{"raiz", "/", CacheRevalidate},
		{"index explicito", "/index.html", CacheRevalidate},
		{"rota exportada", "/changelog.html", CacheRevalidate},

		// Assets sem hash no nome: favicon é sobrescrito no lugar, então precisa revalidar.
		{"favicon", "/favicon.ico", CacheRevalidate},
		{"manifest", "/manifest.json", CacheRevalidate},

		// _next fora de /static/ não é hasheado.
		{"_next nao-static", "/_next/data/foo.json", CacheRevalidate},

		// Prefixo parecido não pode passar por hasheado.
		{"quase _next_static", "/_next/staticky/x.js", CacheRevalidate},
		{"static na raiz", "/static/x.js", CacheRevalidate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CacheControlFor(tc.path); got != tc.want {
				t.Fatalf("CacheControlFor(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestStaticCacheControl_StampsByPath(t *testing.T) {
	h := StaticCacheControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tc := range []struct{ path, want string }{
		{"/_next/static/chunks/app/page-abc123.js", CacheImmutable},
		{"/", CacheRevalidate},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Fatalf("path %q: Cache-Control = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// O middleware não pode atropelar um Cache-Control que o handler interno já
// tenha decidido (ex.: um endpoint que serve arquivo com política própria).
func TestStaticCacheControl_naoSobrescreveExistente(t *testing.T) {
	h := StaticCacheControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=60")
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=60" {
		t.Fatalf("Cache-Control = %q, want o valor do handler interno", got)
	}
}

// O handler interno tem que continuar sendo chamado (o middleware não pode virar
// um curto-circuito que serve header sem corpo).
func TestStaticCacheControl_DelegatesToHandler(t *testing.T) {
	called := false
	h := StaticCacheControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("conteudo"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if !called {
		t.Fatal("handler interno não foi chamado")
	}
	if rec.Body.String() != "conteudo" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
