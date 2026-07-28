package app

import (
	"context"

	"autodj/domain/mixing"
	"autodj/ports/outbound"
)

// MixService reads and writes the station's fine-mixing settings through the
// repo. It validates on write so an out-of-range payload never reaches storage.
type MixService struct {
	repo  outbound.MixSettingsRepo
	stems StemBatch // optional; nil = ligar a opção vale só p/ faixa nova
}

// NewMixService builds the mix settings service.
func NewMixService(repo outbound.MixSettingsRepo) *MixService {
	return &MixService{repo: repo}
}

// Get returns the current fine-mixing settings (defaults if never saved).
func (s *MixService) Get(ctx context.Context) (mixing.Settings, error) {
	return s.repo.Get(ctx)
}

// Save validates then persists the settings, returning the stored value. On a
// validation error the store is left untouched.
func (s *MixService) Save(ctx context.Context, in mixing.Settings) (mixing.Settings, error) {
	if err := in.Validate(); err != nil {
		return mixing.Settings{}, err
	}
	/* LIGAR o preparo automático vale para o ACERVO INTEIRO, não só para faixa
	   nova. Sem isto, marcar a opção com 83 músicas na biblioteca não produziria
	   nada visível e o operador concluiria que não funciona.

	   Só na TRANSIÇÃO desligado→ligado: salvar a configuração de novo com a opção
	   já marcada (mexer em qualquer outro campo faz isso) não pode reenfileirar o
	   acervo. `Prepare` é idempotente, então não haveria trabalho repetido — mas
	   haveria um lote inteiro percorrido a cada clique no ⚙, à toa. */
	prev, _ := s.repo.Get(ctx)
	if err := s.repo.Save(ctx, in); err != nil {
		return mixing.Settings{}, err
	}
	if s.stems != nil && in.AutoInstrumental && !prev.AutoInstrumental {
		// em goroutine: são dezenas de faixas, e quem espera é uma requisição HTTP.
		go s.stems.PrepareAll(context.WithoutCancel(ctx))
	}
	return s.repo.Get(ctx)
}

/*
SetStemBatch liga o preparo em lote ao interruptor. Opcional: sem ele (máquina sem
a ferramenta), ligar a opção só vale para faixa nova.

Interface mínima pelo mesmo motivo do `StemQueue` da biblioteca — o serviço de
configuração não tem nada a ver com fila, estado ou erro de separação.
*/
func (s *MixService) SetStemBatch(b StemBatch) { s.stems = b }

// StemBatch é o mínimo que a configuração precisa: enfileirar o acervo todo.
type StemBatch interface {
	PrepareAll(ctx context.Context) int
}
