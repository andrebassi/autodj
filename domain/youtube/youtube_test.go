package youtube

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var base = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

func TestAccount_Connected(t *testing.T) {
	if (Account{}).Connected() {
		t.Fatal("zero account must not be connected")
	}
	if !(Account{AccessToken: "tok"}).Connected() {
		t.Fatal("account with token must be connected")
	}
}

func TestAccount_IsPremium(t *testing.T) {
	// Best-effort membership flag: never gates playout, only chip parity.
	if (Account{}).IsPremium() {
		t.Fatal("zero account must not be premium")
	}
	if !(Account{Premium: true}).IsPremium() {
		t.Fatal("account with Premium set must report premium")
	}
}

func TestAccount_Expired(t *testing.T) {
	if !(Account{}).Expired(base) {
		t.Fatal("zero ExpiresAt must be expired")
	}
	a := Account{ExpiresAt: base.Add(time.Hour)}
	if a.Expired(base) {
		t.Fatal("token valid an hour out must not be expired")
	}
	if !a.Expired(base.Add(time.Hour)) {
		t.Fatal("token at exact expiry must be expired")
	}
	if !a.Expired(base.Add(2 * time.Hour)) {
		t.Fatal("token past expiry must be expired")
	}
}

func TestAccount_NeedsRefresh(t *testing.T) {
	if !(Account{}).NeedsRefresh(base) {
		t.Fatal("zero ExpiresAt must need refresh")
	}
	a := Account{ExpiresAt: base.Add(time.Hour)}
	if a.NeedsRefresh(base) {
		t.Fatal("token an hour out must not need refresh")
	}
	if !a.NeedsRefresh(base.Add(time.Hour - refreshWindow/2)) {
		t.Fatal("token inside refresh window must need refresh")
	}
	if !a.NeedsRefresh(base.Add(time.Hour - refreshWindow)) {
		t.Fatal("token at refresh boundary must need refresh")
	}
	if !a.NeedsRefresh(base.Add(2 * time.Hour)) {
		t.Fatal("expired token must need refresh")
	}
}

func TestAccount_HasScope(t *testing.T) {
	a := Account{Scopes: []string{"https://www.googleapis.com/auth/youtube.readonly", "openid"}}
	if !a.HasScope("openid") {
		t.Fatal("granted scope must be present")
	}
	if a.HasScope("https://www.googleapis.com/auth/youtube.upload") {
		t.Fatal("ungranted scope must be absent")
	}
	if (Account{}).HasScope("openid") {
		t.Fatal("no scopes => none present")
	}
}

func TestPendingAuth_Expired(t *testing.T) {
	if !(PendingAuth{}).Expired(base) {
		t.Fatal("zero CreatedAt must be expired")
	}
	p := PendingAuth{State: "s", Verifier: "v", CreatedAt: base}
	if p.Expired(base.Add(pendingTTL - time.Second)) {
		t.Fatal("fresh pending must not be expired")
	}
	if p.Expired(base.Add(pendingTTL)) {
		t.Fatal("pending at exact TTL must not be expired")
	}
	if !p.Expired(base.Add(pendingTTL + time.Second)) {
		t.Fatal("pending past TTL must be expired")
	}
}

// --- paste-import: parsing dos links colados (0.2.17) ---

func TestVideoID(t *testing.T) {
	const id = "dQw4w9WgXcQ"
	cases := []struct {
		name, in, want string
	}{
		{"bare id", id, id},
		{"watch url", "https://www.youtube.com/watch?v=" + id, id},
		{"watch url sem scheme", "youtube.com/watch?v=" + id, id},
		{"watch url com params extras", "https://www.youtube.com/watch?app=desktop&v=" + id + "&t=42s", id},
		{"watch url com lista", "https://www.youtube.com/watch?v=" + id + "&list=PLabc123", id},
		{"youtu.be", "https://youtu.be/" + id, id},
		{"youtu.be com query", "https://youtu.be/" + id + "?t=90", id},
		{"shorts", "https://www.youtube.com/shorts/" + id, id},
		{"embed", "https://www.youtube.com/embed/" + id, id},
		{"live", "https://www.youtube.com/live/" + id, id},
		{"/v/ legado", "https://www.youtube.com/v/" + id, id},
		{"music.youtube", "https://music.youtube.com/watch?v=" + id, id},
		{"nocookie", "https://www.youtube-nocookie.com/embed/" + id, id},
		{"mobile", "https://m.youtube.com/watch?v=" + id, id},
		{"host com porta", "http://youtube.com:8080/watch?v=" + id, id},
		{"espaços em volta", "  https://youtu.be/" + id + "  ", id},

		{"vazio", "", ""},
		{"só espaço", "   ", ""},
		{"palavra solta", "musica", ""},
		{"id curto", "abc", ""},
		{"id com caractere inválido", "dQw4w9WgX!Q", ""},
		{"playlist sem vídeo", "https://www.youtube.com/playlist?list=PLabc123", ""},
		{"canal", "https://www.youtube.com/@canal", ""},
		{"outro host", "https://vimeo.com/watch?v=" + id, ""},
		{"host parecido", "https://notyoutube.com/watch?v=" + id, ""},
		{"url quebrada", "https://youtube.com/watch?v=%zz", ""},
		{"url inválida", "ht tp://youtube.com/watch?v=" + id, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VideoID(c.in); got != c.want {
				t.Fatalf("VideoID(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseLinks(t *testing.T) {
	const a, b, c = "dQw4w9WgXcQ", "9bZkp7q19f0", "kJQP7kiw5Fk"

	if got := ParseLinks("https://youtu.be/" + a + "; https://www.youtube.com/watch?v=" + b); len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("separador ; = %v", got)
	}
	if got := ParseLinks("https://youtu.be/" + a + "\n" + b + "\r\n" + c); len(got) != 3 {
		t.Fatalf("quebra de linha = %v", got)
	}
	if got := ParseLinks(a + "," + b); len(got) != 2 {
		t.Fatalf("vírgula = %v", got)
	}
	// duplicata colapsa preservando a ordem da primeira aparição
	if got := ParseLinks(a + ";" + b + ";https://youtu.be/" + a); len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("dedupe = %v", got)
	}
	// entradas inválidas somem sem derrubar as válidas
	if got := ParseLinks("lixo; " + a + " ;https://vimeo.com/1;"); len(got) != 1 || got[0] != a {
		t.Fatalf("filtro = %v", got)
	}
	if got := ParseLinks(""); len(got) != 0 {
		t.Fatalf("vazio = %v", got)
	}
	if got := ParseLinks("nada aqui"); len(got) != 0 {
		t.Fatalf("sem link = %v", got)
	}
}

func TestParseLinks_capMaxImportLinks(t *testing.T) {
	ids := make([]string, 0, MaxImportLinks+10)
	for i := 0; i < MaxImportLinks+10; i++ {
		// ids únicos de 11 chars: prefixo fixo + índice zero-padded
		ids = append(ids, fmt.Sprintf("vid%08d", i))
	}
	got := ParseLinks(strings.Join(ids, ";"))
	if len(got) != MaxImportLinks {
		t.Fatalf("len = %d, want %d (cap)", len(got), MaxImportLinks)
	}
	if got[0] != ids[0] {
		t.Fatalf("primeiro = %q, want %q (ordem preservada)", got[0], ids[0])
	}
}

func TestNormalizeBackend(t *testing.T) {
	cases := []struct{ in, want string }{
		{BackendAPI, BackendAPI},
		{BackendFree, BackendFree},
		{"", BackendFree},          // cliente antigo, sem o parâmetro
		{"innertube", BackendFree}, // desconhecido cai no caminho grátis
		{"API", BackendFree},       // case-sensitive de propósito
	}
	for _, c := range cases {
		if got := NormalizeBackend(c.in); got != c.want {
			t.Fatalf("NormalizeBackend(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
