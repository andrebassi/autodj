package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/mixing"
	"autodj/domain/shared"
)

/*
PREPARO AUTOMÁTICO DE INSTRUMENTAL — o interruptor manda, e o import nunca cai.

Três coisas que precisam continuar valendo, e cada uma já foi um jeito de quebrar
isto em produção:

  1. desligado (o padrão) NÃO enfileira nada — senão quem só enviou músicas
     descobriria a máquina moendo horas sem ter pedido;
  2. ligado enfileira a faixa nova;
  3. falha da fila NÃO derruba o upload. A faixa entrou no acervo e toca; o
     instrumental é extra, e trocar o essencial pelo acessório seria pior.
*/
type fakeStemQueue struct {
	ids  []string
	fail error
}

func (f *fakeStemQueue) Prepare(_ context.Context, id string) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	f.ids = append(f.ids, id)
	return "queued", nil
}

// repo de configuração só pra este teste (o `fakeMixRepo` do mix_service_test é
// outro, com estado de escrita — reusar acoplaria dois testes sem necessidade).
type autoMixRepo struct{ on bool }

func (f *autoMixRepo) Get(context.Context) (mixing.Settings, error) {
	s := mixing.Default()
	s.AutoInstrumental = f.on
	return s, nil
}
func (f *autoMixRepo) Save(context.Context, mixing.Settings) error { return nil }

func TestAutoInstrumental(t *testing.T) {
	newSvc := func(on bool, q *fakeStemQueue) (*LibraryService, *memory.TrackRepo) {
		repo := memory.New()
		svc := NewLibraryService(repo, nil, nil, nil)
		svc.SetStemQueue(q, &autoMixRepo{on: on})
		return svc, repo
	}
	track := func() *library.Track {
		tr, err := library.NewTrack("t1", shared.SourceLocal, "Música", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		tr.Path = "/m/a.mp3"
		return tr
	}

	t.Run("desligado não enfileira", func(t *testing.T) {
		q := &fakeStemQueue{}
		svc, _ := newSvc(false, q)
		svc.enqueueAnalysis(track())
		if len(q.ids) != 0 {
			t.Fatalf("não devia enfileirar com o interruptor desligado: %v", q.ids)
		}
	})

	t.Run("ligado enfileira a faixa nova", func(t *testing.T) {
		q := &fakeStemQueue{}
		svc, _ := newSvc(true, q)
		svc.enqueueAnalysis(track())
		if len(q.ids) != 1 || q.ids[0] != "t1" {
			t.Fatalf("esperava t1 na fila, veio %v", q.ids)
		}
	})

	t.Run("fila cheia não derruba o import", func(t *testing.T) {
		q := &fakeStemQueue{fail: errors.New("fila cheia")}
		svc, _ := newSvc(true, q)
		svc.enqueueAnalysis(track()) // não pode entrar em pânico nem propagar
	})

	t.Run("streaming não separa", func(t *testing.T) {
		q := &fakeStemQueue{}
		svc, _ := newSvc(true, q)
		sp, err := library.NewTrack("s1", shared.SourceSpotify, "DRM", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		sp.SpotifyURI = "spotify:track:x"
		svc.enqueueAnalysis(sp)
		if len(q.ids) != 0 {
			t.Fatalf("faixa sem PCM não tem o que separar: %v", q.ids)
		}
	})

	t.Run("já preparada não repete os minutos de CPU", func(t *testing.T) {
		q := &fakeStemQueue{}
		svc, _ := newSvc(true, q)
		tr := track()
		tr.InstrumentalPath = "/m/a.instrumental.wav"
		svc.enqueueAnalysis(tr)
		if len(q.ids) != 0 {
			t.Fatalf("faixa com instrumental pronto não volta pra fila: %v", q.ids)
		}
	})
}
