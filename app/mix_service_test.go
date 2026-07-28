package app

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/mixing"
	"sync"
	"time"
)

// fakeMixRepo is a single-row in-test MixSettingsRepo.
type fakeMixRepo struct {
	s       mixing.Settings
	saveErr error
	getErr  error
	saved   bool
}

func (r *fakeMixRepo) Get(context.Context) (mixing.Settings, error) {
	if r.getErr != nil {
		return mixing.Settings{}, r.getErr
	}
	return r.s, nil
}

func (r *fakeMixRepo) Save(_ context.Context, s mixing.Settings) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.s = s
	r.saved = true
	return nil
}

func TestMixGet(t *testing.T) {
	repo := &fakeMixRepo{s: mixing.Default()}
	svc := NewMixService(repo)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != mixing.Default() {
		t.Fatalf("Get = %+v, want default", got)
	}
}

func TestMixSaveValid(t *testing.T) {
	repo := &fakeMixRepo{s: mixing.Default()}
	svc := NewMixService(repo)
	in := mixing.Default()
	in.MusicaOut = 3500
	got, err := svc.Save(context.Background(), in)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !repo.saved || got.MusicaOut != 3500 {
		t.Fatalf("Save not persisted: saved=%v got=%+v", repo.saved, got)
	}
}

func TestMixSaveInvalidNotPersisted(t *testing.T) {
	repo := &fakeMixRepo{s: mixing.Default()}
	svc := NewMixService(repo)
	in := mixing.Default()
	in.NormalizeVol = 500 // out of range
	_, err := svc.Save(context.Background(), in)
	if !errors.Is(err, mixing.ErrVolPercent) {
		t.Fatalf("err = %v, want ErrVolPercent", err)
	}
	if repo.saved {
		t.Fatal("invalid settings must not be persisted")
	}
}

func TestMixSaveRepoError(t *testing.T) {
	repo := &fakeMixRepo{s: mixing.Default(), saveErr: errors.New("boom")}
	svc := NewMixService(repo)
	if _, err := svc.Save(context.Background(), mixing.Default()); err == nil {
		t.Fatal("expected repo save error")
	}
}

func TestMixGetError(t *testing.T) {
	repo := &fakeMixRepo{getErr: errors.New("boom")}
	svc := NewMixService(repo)
	if _, err := svc.Get(context.Background()); err == nil {
		t.Fatal("expected repo get error")
	}
}

/*
LIGAR o preparo automático vale para o acervo INTEIRO — e só na TRANSIÇÃO.

Sem o lote, marcar a opção com a biblioteca cheia não produziria nada visível (o
gatilho automático só pega faixa nova) e o operador concluiria que não funciona.
E sem a checagem de transição, qualquer salvamento do ⚙ com a opção já marcada —
mexer no fade, no volume, em qualquer campo — varreria o acervo de novo à toa.
*/
type fakeStemBatch struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeStemBatch) PrepareAll(context.Context) int {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return 0
}
func (f *fakeStemBatch) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestSaveDisparaLoteSoNaTransicao(t *testing.T) {
	ctx := context.Background()
	repo := &fakeMixRepo{s: mixing.Default()}
	batch := &fakeStemBatch{}
	svc := NewMixService(repo)
	svc.SetStemBatch(batch)

	on := mixing.Default()
	on.AutoInstrumental = true

	// desligado → ligado: dispara.
	if _, err := svc.Save(ctx, on); err != nil {
		t.Fatalf("Save: %v", err)
	}
	waitFor(t, func() bool { return batch.count() == 1 }, "lote não disparou ao ligar")

	// já ligado, salvando de novo (outro campo mudou): NÃO dispara.
	on.NormalizeVol = 80
	if _, err := svc.Save(ctx, on); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// desligando também não dispara.
	off := mixing.Default()
	off.AutoInstrumental = false
	if _, err := svc.Save(ctx, off); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if n := batch.count(); n != 1 {
		t.Fatalf("lote devia ter rodado UMA vez (só na transição), rodou %d", n)
	}
}

// waitFor espera uma condição — o lote roda em goroutine (quem chama Save é uma
// requisição HTTP e não pode segurar dezenas de faixas).
func waitFor(t *testing.T, ok func() bool, msg string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}
