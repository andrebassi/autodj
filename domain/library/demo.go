package library

import (
	"fmt"
	"time"

	"autodj/domain/shared"
)

// demoSeed is one canned catalog track (fictional titles). Served as real,
// enqueueable library tracks so the UI media list has no hardcoded data.
type demoSeed struct {
	artist string
	title  string
	dur    time.Duration
}

func sec(m, s int) time.Duration { return time.Duration(m*60+s) * time.Second }

// demoCopies repeats the canned catalog N times (distinct ids). Kept at 1 for
// the real demo seed; bump temporarily only to stress-test list scrolling.
const demoCopies = 1

var demoSeeds = []demoSeed{
	{"Astro Azul", "Noite de Neon", sec(3, 12)},
	{"Banda Eva do Sul", "Verão Sem Fim", sec(3, 44)},
	{"Marés & Luar", "Ainda Acredito", sec(4, 1)},
	{"Cidade Clara", "Tudo Certo", sec(3, 20)},
	{"Vento Sul", "Domingo à Tarde", sec(3, 58)},
	{"Eco da Serra", "O Outro Lado", sec(3, 33)},
	{"Praia Norte", "Batida do Mar", sec(4, 10)},
	{"Rio Prata", "Segundo Sol Nascente", sec(3, 47)},
	{"Nébula 9", "Gravidade Zero", sec(3, 5)},
	{"Aurora Lenta", "Cinema Mudo", sec(4, 22)},
	{"Fábrica de Sonhos", "Dia Especial", sec(3, 29)},
	{"Trem das Onze e Meia", "Última Estação", sec(3, 51)},
	{"Coral Elétrico", "Onda Curta", sec(3, 16)},
	{"Sol de Inverno", "Rua da Saudade", sec(4, 8)},
	{"Mirante", "Horizonte Aberto", sec(3, 37)},
}

// DemoTracks returns the canned demo catalog as validated local Tracks. Used to
// seed an empty repo so /library/tracks serves real, enqueueable rows instead of
// the frontend carrying a hardcoded media list.
func DemoTracks() []*Track {
	out := make([]*Track, 0, len(demoSeeds)*demoCopies)
	n := 0
	for c := 0; c < demoCopies; c++ {
		for _, s := range demoSeeds {
			n++
			id := fmt.Sprintf("demo-%02d", n)
			// Seeds are compile-time-constant and always valid (local source,
			// non-empty id, positive duration) so NewTrack never errors here.
			t, _ := NewTrack(id, shared.SourceLocal, s.title, s.dur)
			t.Artist = s.artist
			t.Category = "music"
			t.Path = "/demo/" + id + ".mp3"
			out = append(out, t)
		}
	}
	return out
}
