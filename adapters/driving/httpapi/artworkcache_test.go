package httpapi

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

/* O que estes testes travam (o bug que derrubou a máquina em 27/07/2026):

   - extrair NO MÁXIMO uma vez por arquivo, inclusive quando a faixa NÃO tem capa
     — o acervo típico é quase todo sem capa, então é o caso negativo que decide
     se o custo cai ou não;
   - pedidos simultâneos da MESMA capa (deck ao vivo + deck engatilhado + linha da
     lista) compartilham UMA extração;
   - nunca passar do teto de processos simultâneos.

   O extrator é injetado: depender do ffmpeg real tornaria o teste ambiental e
   ainda por cima lento. */

func fakeCache(extract func(ctx context.Context, path string) ([]byte, error)) *artworkCache {
	c := newArtworkCache()
	c.extract = extract
	return c
}

func TestArtworkCache_ExtractsOncePerFile(t *testing.T) {
	var calls int32
	c := fakeCache(func(context.Context, string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("capa"), nil
	})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		got, err := c.Get(ctx, "/m/a.mp3")
		if err != nil || string(got) != "capa" {
			t.Fatalf("Get #%d: %q err=%v", i, got, err)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("extraiu %d vezes, esperado 1", n)
	}
}

func TestArtworkCache_RemembersMissingArtwork(t *testing.T) {
	var calls int32
	c := fakeCache(func(context.Context, string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("exit status 1")
	})
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := c.Get(ctx, "/m/sem-capa.mp3"); !errors.Is(err, errNoArtworkFile) {
			t.Fatalf("esperado errNoArtworkFile, got %v", err)
		}
	}
	// é ESTE o caso que mais se repete no acervo real: sem cache negativo, cada
	// carga da tela abriria um ffmpeg por faixa sem capa, pra sempre.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("extraiu %d vezes uma faixa sem capa, esperado 1", n)
	}
}

func TestArtworkCache_EmptyOutputCountsAsMissing(t *testing.T) {
	c := fakeCache(func(context.Context, string) ([]byte, error) { return []byte{}, nil })
	if _, err := c.Get(context.Background(), "/m/vazio.mp3"); !errors.Is(err, errNoArtworkFile) {
		t.Fatalf("ffmpeg que sai 0 mas não escreve nada = sem capa, got %v", err)
	}
}

func TestArtworkCache_SingleFlightAndBoundedConcurrency(t *testing.T) {
	var inFlight, peak, calls int32
	release := make(chan struct{})
	c := fakeCache(func(context.Context, string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		n := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		<-release // segura até o teste soltar, pra medir o pico de simultâneos
		atomic.AddInt32(&inFlight, -1)
		return []byte("capa"), nil
	})

	var wg sync.WaitGroup
	// 6 arquivos distintos (mede o semáforo) × 3 pedidos cada (mede single-flight)
	for f := 0; f < 6; f++ {
		for r := 0; r < 3; r++ {
			wg.Add(1)
			go func(f int) {
				defer wg.Done()
				_, _ = c.Get(context.Background(), "/m/"+string(rune('a'+f))+".mp3")
			}(f)
		}
	}
	time.Sleep(80 * time.Millisecond) // deixa a fila encher antes de soltar
	close(release)
	wg.Wait()

	if n := atomic.LoadInt32(&calls); n != 6 {
		t.Fatalf("18 pedidos de 6 arquivos deveriam render 6 extrações, got %d", n)
	}
	if p := atomic.LoadInt32(&peak); p > artworkMaxParallel {
		t.Fatalf("pico de %d processos simultâneos, teto é %d", p, artworkMaxParallel)
	}
}

func TestArtworkCache_MemoryBudget(t *testing.T) {
	big := make([]byte, artworkMaxBytes/2+1) // dois já estouram
	c := fakeCache(func(context.Context, string) ([]byte, error) { return big, nil })
	ctx := context.Background()
	_, _ = c.Get(ctx, "/m/1.mp3")
	_, _ = c.Get(ctx, "/m/2.mp3")

	c.mu.Lock()
	total := c.bytes
	c.mu.Unlock()
	if total > artworkMaxBytes {
		t.Fatalf("cache passou do orçamento: %d > %d", total, artworkMaxBytes)
	}
}
