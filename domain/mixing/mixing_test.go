package mixing

import "testing"

func TestDefaultValid(t *testing.T) {
	d := Default()
	if err := d.Validate(); err != nil {
		t.Fatalf("default must validate, got %v", err)
	}
	if d.MusicaIn != 500 || d.MusicaOut != 2000 || d.NormalizeVol != 89 {
		t.Fatalf("unexpected default: %+v", d)
	}
	if !d.TrimMusicas || d.TrimVinhetas || !d.Normalize || !d.DetectVocal {
		t.Fatalf("unexpected default toggles: %+v", d)
	}
}

func TestTargetLUFS(t *testing.T) {
	cases := []struct {
		name string
		vol  int
		want float64
	}{
		// O default histórico (89) tem que cair no padrão de streaming — foi por
		// isso que a régua −24..−12 foi escolhida: nenhum dado precisou migrar.
		{"default 89 ≈ -13.3", 89, -13.32},
		{"mínimo", 0, -24},
		{"máximo", 100, -12},
		{"meio", 50, -18},
		// Valor corrompido no banco não pode virar alvo absurdo (ganho errado
		// silencioso). Clampa nos extremos.
		{"abaixo de zero clampa", -30, -24},
		{"acima de cem clampa", 500, -12},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Default()
			s.NormalizeVol = c.vol
			got := s.TargetLUFS()
			if got < c.want-0.01 || got > c.want+0.01 {
				t.Fatalf("TargetLUFS() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Settings)
		want error
	}{
		{"happy", func(*Settings) {}, nil},
		{"zero fades ok", func(s *Settings) {
			s.MusicaIn, s.MusicaOut, s.VinhetaIn, s.VinhetaOut, s.ComercialIn, s.ComercialOut = 0, 0, 0, 0, 0, 0
		}, nil},
		{"max fade ok", func(s *Settings) { s.MusicaOut = maxFadeMs }, nil},
		{"negative musica in", func(s *Settings) { s.MusicaIn = -1 }, ErrNegativeFade},
		{"negative comercial out", func(s *Settings) { s.ComercialOut = -5 }, ErrNegativeFade},
		{"fade too long", func(s *Settings) { s.VinhetaOut = maxFadeMs + 1 }, ErrFadeTooLong},
		{"overlap below zero", func(s *Settings) { s.OverlapVol = -1 }, ErrVolPercent},
		{"talk above 100", func(s *Settings) { s.TalkVol = 101 }, ErrVolPercent},
		{"normalize vol above 100", func(s *Settings) { s.NormalizeVol = 200 }, ErrVolPercent},
		{"bounds ok 0/100", func(s *Settings) { s.OverlapVol, s.TalkVol, s.NormalizeVol = 0, 100, 0 }, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Default()
			c.mut(&s)
			if got := s.Validate(); got != c.want {
				t.Fatalf("Validate() = %v, want %v", got, c.want)
			}
		})
	}
}
