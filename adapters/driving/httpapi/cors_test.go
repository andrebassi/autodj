package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

/*
⚠️ ERA `Access-Control-Allow-Origin: *`, e num app que roda na máquina de quem usa
isso significa: qualquer site aberto noutra aba fala com o AutoDJ em localhost e LÊ
a resposta (biblioteca, fila, e `POST /library/clear`). Estes testes travam a
correção — e travam também o que NÃO pode quebrar junto: `task web:dev` serve a
página em :3000 contra a API em :8095, de propósito.
*/
func TestCORSPermiteDevEBloqueiaSiteQualquer(t *testing.T) {
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	casos := []struct {
		nome    string
		origin  string
		host    string
		liberar bool
	}{
		{"web:dev (:3000 -> :8095)", "http://localhost:3000", "localhost:8095", true},
		{"loopback por IP", "http://127.0.0.1:3000", "localhost:8095", true},
		{"mesma origem", "https://autodj.exemplo", "autodj.exemplo", true},
		{"SITE QUALQUER", "https://malicioso.example", "localhost:8099", false},
		{"sósia do host", "https://autodj.exemplo.evil.com", "autodj.exemplo", false},
	}
	for _, c := range casos {
		req := httptest.NewRequest(http.MethodGet, "http://"+c.host+"/api/v1/health", nil)
		req.Host = c.host
		req.Header.Set("Origin", c.origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header().Get("Access-Control-Allow-Origin")
		if c.liberar && got != c.origin {
			t.Errorf("%s: devia liberar %q, veio %q", c.nome, c.origin, got)
		}
		if !c.liberar && got != "" {
			t.Errorf("%s: NÃO devia liberar, veio %q", c.nome, got)
		}
	}
}

// `*` não pode voltar por descuido: com curinga o navegador libera para todo site.
func TestCORSNuncaDevolveCuringa(t *testing.T) {
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8099/api/v1/health", nil)
	req.Host = "localhost:8099"
	req.Header.Set("Origin", "https://qualquer.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("curinga voltou")
	}
}

// O cabeçalho que escolhe a ESTAÇÃO só pode ser aceito de origem liberada.
func TestCORSLiberaCabecalhoDeSessaoSoParaOrigemConhecida(t *testing.T) {
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "http://localhost:8095/api/v1/library/tracks", nil)
	req.Host = "localhost:8095"
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if h := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(h, "X-AutoDJ-Session") {
		t.Fatalf("web:dev precisa poder mandar X-AutoDJ-Session, veio %q", h)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight devia responder 204, veio %d", rec.Code)
	}
}

/*
⚠️ O `Referrer-Policy` é o teste que mais importa aqui, e não é sobre segurança —
é sobre o YouTube TOCAR. Desde dez/2025 o embed exige que a página-mãe mande
`Referer`; com `no-referrer` ou `same-origin` o player estoura erro 153 e o vídeo
não sai do lugar. Alguém "endurecendo" este cabeçalho no futuro derruba um dos
três motores de áudio, e é isso que este teste trava.
*/
func TestCabecalhosDeSeguranca(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8099/api/v1/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy tem de ser strict-origin-when-cross-origin (senão o YouTube dá erro 153): %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("faltou nosniff: %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("faltou X-Frame-Options: %q", got)
	}
	// HSTS em http:// prenderia o navegador a HTTPS num host que não o serve.
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS não pode sair em http simples: %q", got)
	}
}

// Atrás do proxy do Fly o TLS termina antes; o sinal é o X-Forwarded-Proto.
func TestHSTSSaiSomenteEmHTTPS(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "http://autodj.exemplo/api/v1/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Fatal("HSTS devia sair quando a requisição chegou por HTTPS")
	}
}
