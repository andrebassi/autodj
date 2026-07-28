package ws

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"autodj/domain/playback"
	"autodj/domain/shared"
)

type fakePlay struct {
	np  playback.Playable
	q   []playback.QueueItem
	err error
}

func (f fakePlay) LoadFromLibrary(context.Context) (int, error) { return 0, nil }
func (f fakePlay) Next(context.Context) error                   { return nil }
func (f fakePlay) Enqueue(context.Context, string, int, bool, string) error { return nil }
func (f fakePlay) NowPlaying(context.Context) (playback.Playable, []playback.QueueItem, error) {
	return f.np, f.q, f.err
}
func (f fakePlay) Queue(context.Context) []playback.QueueItem { return f.q }
func (f fakePlay) Dequeue(context.Context, int) error         { return nil }
func (f fakePlay) Move(context.Context, int, int) error       { return nil }
func (f fakePlay) Clear(context.Context) error                { return nil }

func TestHubSendsSnapshotOnConnect(t *testing.T) {
	play := fakePlay{
		np: playback.Playable{TrackID: "1", Title: "Live", Source: shared.SourceLocal, Duration: time.Minute},
		q: []playback.QueueItem{{
			Playable:   playback.Playable{TrackID: "2", Title: "Next", Source: shared.SourceLocal},
			Transition: shared.TransitionCrossfade,
		}},
	}
	hub := NewHub(play, 20*time.Millisecond)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go hub.Run(ctx)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// immediate snapshot on connect
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var msg nowPlayingMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "nowplaying" || msg.NowPlaying.Title != "Live" {
		t.Fatalf("bad snapshot: %+v", msg)
	}
	if len(msg.Queue) != 1 || msg.Queue[0].Transition != "crossfade" {
		t.Fatalf("bad queue: %+v", msg.Queue)
	}

	// periodic broadcast tick delivers another
	_, data2, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data2), "Live") {
		t.Fatalf("periodic broadcast missing: %s", data2)
	}
}

func TestHubDefaultInterval(t *testing.T) {
	h := NewHub(fakePlay{}, 0)
	if h.interval != 2*time.Second {
		t.Fatalf("default interval should be 2s, got %v", h.interval)
	}
}

func TestHubRunStopsOnContext(t *testing.T) {
	hub := NewHub(fakePlay{}, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { hub.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

/*
⚠️ ESTES TESTES EXISTEM PORQUE A COBERTURA DEPENDIA DE CORRIDA DE TEMPO.

O teste de ponta a ponta acima espera um tique de 20 ms para exercitar o
`broadcast`, e o que ele alcança depende do escalonador: no Mac do autor dava
92,0% e no runner do CI, 80,0% — abaixo do portão de 90%, com o MESMO código.
Cobertura que muda de máquina não mede nada.

Os casos abaixo chamam os caminhos diretamente, sem esperar relógio: erro do
serviço, conexão morta (que tem de ser descartada) e requisição que não é
WebSocket. São exatamente os ramos que o teste temporizado alcançava por acaso.
*/

// NowPlaying falhando: nada é enviado e ninguém quebra.
func TestBroadcastComServicoFalhando(t *testing.T) {
	h := NewHub(fakePlay{err: context.DeadlineExceeded}, time.Second)
	h.broadcast(context.Background()) // não pode entrar em pânico nem escrever
	if len(h.conns) != 0 {
		t.Fatal("não deveria haver conexão")
	}
}

// Conexão morta tem de ser DESCARTADA do hub — senão o mapa cresce para sempre
// e cada broadcast tenta escrever em soquete fechado.
func TestBroadcastDescartaConexaoMorta(t *testing.T) {
	play := fakePlay{np: playback.Playable{TrackID: "1", Title: "Live", Source: shared.SourceLocal}}
	hub := NewHub(play, time.Hour) // tique longo: quem dispara é o teste, não o relógio
	srv := httptest.NewServer(hub)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err != nil { // consome o snapshot imediato
		t.Fatal(err)
	}

	// espera o servidor registrar a conexão
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		n := len(hub.conns)
		hub.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	_ = c.Close(websocket.StatusNormalClosure, "")
	// com o cliente fora, o write falha e a conexão tem de sair do hub
	for i := 0; i < 50; i++ {
		hub.broadcast(context.Background())
		hub.mu.Lock()
		n := len(hub.conns)
		hub.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("conexão morta não foi descartada do hub")
}

// Requisição HTTP comum (sem upgrade) não pode virar conexão nem derrubar nada.
func TestServeHTTPRecusaQuemNaoEWebSocket(t *testing.T) {
	hub := NewHub(fakePlay{}, time.Hour)
	rec := httptest.NewRecorder()
	hub.ServeHTTP(rec, httptest.NewRequest("GET", "/ws", nil))
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.conns) != 0 {
		t.Fatal("requisição não-WebSocket não pode registrar conexão")
	}
}
