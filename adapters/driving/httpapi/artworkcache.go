package httpapi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

/*
Cache de capa embutida (APIC) + limite de processos.

O PROBLEMA (mediu-se derrubando a máquina, 0.2.26): o endpoint de artwork rodava
um `ffmpeg` POR REQUISIÇÃO. Uma estação com 84 faixas locais pinta 84 miniaturas;
cada recarga da tela repetia tudo, e o pior caso nem é a faixa COM capa — é a
faixa SEM capa, que sai em 404 e o browser volta a pedir na carga seguinte, pra
sempre. Recarregando a tela algumas dezenas de vezes durante os testes, a tabela
de processos do macOS estourou e a máquina parou de conseguir fazer fork.

REAPROVEITAR O MESMO PROCESSO NÃO É POSSÍVEL: o `ffmpeg` de linha de comando é
one-shot por arquivo de entrada — não existe modo servidor onde se mandem vários
caminhos pela mesma instância. O que dá pra fazer é reduzir o NÚMERO de execuções
a ~1 por faixa durante a vida do servidor, que é o efeito prático desejado:

  1. CACHE POSITIVO — os bytes da capa extraída ficam em memória;
  2. CACHE NEGATIVO — "esta faixa não tem capa" também é resposta, e é a mais
     comum num acervo de MP3 baixado. Sem guardá-la, o custo nunca cai;
  3. SINGLE-FLIGHT — deck ao vivo, deck engatilhado e a linha da lista pedem a
     MESMA capa quase junto; uma extração serve as três;
  4. SEMÁFORO — teto de execuções simultâneas, pra uma biblioteca fria não
     disparar 84 processos de uma vez (a VM de produção tem 1 CPU compartilhada).

A chave inclui tamanho e mtime do arquivo: editar a tag da faixa invalida o
registro sem ninguém precisar limpar cache.
*/

// teto de ffmpeg simultâneos. 2 porque a VM de produção é `shared-cpu-1x`: mais
// que isso só enfileira dentro do kernel e ainda multiplica processo.
const artworkMaxParallel = 2

// orçamento de memória do cache positivo. Capa costuma ter 50–300 KB; 32 MB
// segura um acervo caseiro inteiro e ainda cabe folgado numa VM de 1 GB.
const artworkMaxBytes = 32 << 20

// errNoArtworkFile é o "não tem capa" guardado no cache negativo.
var errNoArtworkFile = errors.New("httpapi: faixa sem capa embutida")

type artworkEntry struct {
	once sync.Once
	data []byte
	err  error
}

type artworkCache struct {
	mu      sync.Mutex
	entries map[string]*artworkEntry
	bytes   int
	sem     chan struct{}
	// extract é injetável: os testes trocam por uma função em memória, senão
	// dependeriam de ter ffmpeg instalado e de um MP3 com capa no disco.
	extract func(ctx context.Context, path string) ([]byte, error)
}

func newArtworkCache() *artworkCache {
	return &artworkCache{
		entries: map[string]*artworkEntry{},
		sem:     make(chan struct{}, artworkMaxParallel),
		extract: extractArtworkFFmpeg,
	}
}

// key identifica o CONTEÚDO do arquivo, não só o caminho: tamanho + mtime mudam
// quando a tag é reescrita, então a capa nova entra sozinha.
func artworkKey(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return path
	}
	return path + "|" + strconv.FormatInt(st.Size(), 10) + "|" +
		strconv.FormatInt(st.ModTime().UnixNano(), 10)
}

// Get devolve os bytes da capa, extraindo no máximo UMA vez por arquivo.
// Devolve errNoArtworkFile quando a faixa não tem capa (também memorizado).
func (c *artworkCache) Get(ctx context.Context, path string) ([]byte, error) {
	k := artworkKey(path)

	c.mu.Lock()
	e := c.entries[k]
	if e == nil {
		e = &artworkEntry{}
		c.entries[k] = e
	}
	c.mu.Unlock()

	// sync.Once dá o single-flight: quem chegar durante a extração espera aqui
	// em vez de abrir outro processo.
	e.once.Do(func() {
		c.sem <- struct{}{}
		defer func() { <-c.sem }()
		data, err := c.extract(ctx, path)
		if err != nil || len(data) == 0 {
			e.err = errNoArtworkFile
			return
		}
		e.data = data
		c.mu.Lock()
		c.bytes += len(data)
		if c.bytes > artworkMaxBytes {
			// Estouro do orçamento: descarta os POSITIVOS e recomeça, mantendo os
			// negativos (que custam ~nada e evitam o pior caso). Simples de
			// propósito — LRU aqui seria mais código pra um acervo caseiro, e o
			// custo de errar é reextrair uma capa.
			for key, ent := range c.entries {
				if ent.err == nil && ent != e {
					delete(c.entries, key)
				}
			}
			c.bytes = len(data)
		}
		c.mu.Unlock()
	})

	if e.err != nil {
		return nil, e.err
	}
	return e.data, nil
}

// extractArtworkFFmpeg tira a imagem embutida (APIC/PIC) sem re-encodar.
// `-map 0:v:0` pega o stream de imagem anexada; faixa sem capa faz o ffmpeg sair
// com código != 0, que vira o cache negativo.
func extractArtworkFFmpeg(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "quiet",
		"-i", path,
		"-map", "0:v:0",
		"-c", "copy",
		"-f", "image2pipe",
		"-",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
