// Package content holds the station's canned UI catalogs — the reference data
// the operator screens render (categories, folders, cartwall, web tabs, program
// preview rows). It is mock/seed data today, served through the API so the
// frontend carries no hardcoded lists. Pure: no external deps, no logic beyond
// returning the fixed catalogs. JSON tags live here because these values are the
// wire contract for the /ui/* endpoints (view catalogs, not domain aggregates).
package content

// Category is one operator media category (top bar of the Operacional screen).
type Category struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Ico   string `json:"ico"`
}

// Folder is a navigable media folder (folder-tree, no registration).
type Folder struct {
	Name  string `json:"name"`
	Ico   string `json:"ico"`
	Count int    `json:"count"`
}

// CartButton is one instant-play button of the cartwall.
type CartButton struct {
	Label string `json:"label"`
	Dur   string `json:"dur"`
	Ico   string `json:"ico"`
	Hue   int    `json:"hue"`
}

// Marker is a waveform-editor special marker preset.
type Marker struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Ico   string `json:"ico"`
	Color string `json:"color"`
}

// MixConfig is the default fine-mixing configuration (ms / %).
type MixConfig struct {
	MusicaIn     int  `json:"musicaIn"`
	MusicaOut    int  `json:"musicaOut"`
	VinhetaIn    int  `json:"vinhetaIn"`
	VinhetaOut   int  `json:"vinhetaOut"`
	ComercialIn  int  `json:"comercialIn"`
	ComercialOut int  `json:"comercialOut"`
	OverlapVol   int  `json:"overlapVol"`
	TalkVol      int  `json:"talkVol"`
	TrimMusicas  bool `json:"trimMusicas"`
	TrimVinhetas bool `json:"trimVinhetas"`
	Normalize    bool `json:"normalize"`
	NormalizeVol int  `json:"normalizeVol"`
	DetectVocal  bool `json:"detectVocal"`
	// AutoInstrumental: preparar o instrumental (IA) de toda faixa local nova.
	AutoInstrumental bool `json:"autoInstrumental"`
}

// Operational is the whole Operacional-screen catalog in one payload.
type Operational struct {
	Categories   []Category   `json:"categories"`
	MusicSubtabs []string     `json:"musicSubtabs"`
	Folders      []Folder     `json:"folders"`
	Cartwall     []CartButton `json:"cartwall"`
	Markers      []Marker     `json:"markers"`
	Mix          MixConfig    `json:"mix"`
}

// SubTab is a nested web-panel tab (e.g. WhatsApp under Sociais).
type SubTab struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Framed bool   `json:"framed"`
	Brand  string `json:"brand"`
}

// WebTab is one tab of the Internet side panel.
type WebTab struct {
	Name    string   `json:"name"`
	Icon    string   `json:"icon"`
	URL     string   `json:"url"`
	Framed  bool     `json:"framed"`
	Subtabs []SubTab `json:"subtabs,omitempty"`
}

// ProgramRow is one preview row of the program list (above/below the playing
// track). Kind drives the frontend row styling; Text is set only for locucao.
type ProgramRow struct {
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Dur   string `json:"dur,omitempty"`
	Text  string `json:"text,omitempty"`
}

// Program is the mock program preview (rich RadioPro-style rows).
type Program struct {
	Above []ProgramRow `json:"above"`
	Below []ProgramRow `json:"below"`
}

// GradeLoad is the per-hour scheduling load of the day's grade: Hourly[h] is the
// number of items programmed for hour h (0..23). Feeds the Player's 24h histogram
// (occupancy map, not a waveform). Mock/seed today; the scheduler (Épico 4) will
// derive it from the real grade.
type GradeLoad struct {
	Hourly []int `json:"hourly"` // always 24 entries, index = hour of day
}

// OperationalCatalog returns the canned Operacional-screen catalog.
func OperationalCatalog() Operational {
	return Operational{
		Categories: []Category{
			{"instantaneas", "Instantâneas", "⚡"},
			{"musicas", "Músicas", "🎵"},
			{"spotify", "Spotify", "🟢"},
			{"youtube", "YouTube", "🔴"},
			{"vinhetas", "Vinhetas", "⭐"},
			{"trilhas", "Trilhas", "🎬"},
			{"extras", "Extras", "🗂"},
			{"comerciais", "Comerciais", "📢"},
			{"programetes", "Programetes", "🎙"},
			{"outros", "Outros", "🕐"},
			{"oferecimentos", "Oferecimentos", "❤️"},
			{"infocantores", "Info Cantores", "🧑‍🎤"},
			{"aniversarios", "Aniversários", "🎂"},
		},
		MusicSubtabs: []string{
			"Pastas", "Favoritas", "Últimas Inseridas", "Manuais",
		},
		Folders: []Folder{
			{"INTERNACIONAIS", "🌐", 42},
			{"NACIONAIS", "🦜", 37},
		},
		Cartwall: []CartButton{
			{"A Mais Ouvida", "00:02.5", "🕐", 275},
			{"A Música de Hoje", "00:05.7", "🎵", 210},
			{"É Sucesso", "00:02.9", "🎧", 285},
			{"Efeito Transição", "00:00.8", "🪄", 30},
			{"Exclusiva", "00:05.6", "🦉", 200},
			{"Fácil Assim — Jovem", "00:04.7", "🦀", 320},
			{"Fácil Assim 01", "00:04.8", "🐿", 0},
			{"Fácil Assim 02", "00:06.2", "🐘", 90},
			{"Fácil Assim 03", "00:06.3", "🐤", 45},
			{"Hora — Voz Feminina", "00:01.0", "🕓", 260},
			{"Lenta — Jovem", "00:03.1", "🎹", 210},
			{"Ligou Tocou", "00:02.5", "📻", 130},
			{"Mais Música", "00:04.0", "➕", 190},
			{"Média Jovem", "00:07.5", "🍲", 20},
			{"O Som do Milênio", "00:06.6", "🎶", 340},
			{"Rádio do Coração", "00:05.2", "💙", 265},
			{"Sinal Hora", "00:03.8", "🛰", 195},
			{"Sintonize", "00:03.3", "🎚", 350},
			{"TopHits", "00:02.7", "🎸", 160},
			{"Você Sintoniza", "00:04.3", "😎", 280},
		},
		// Marker palette — keys are the real domain MarkerKind values
		// (library.Marker{Start,Intro,Hook,Stamp,MixIn,MixOut,End}) so every
		// palette button produces a marker that validates on PUT /markers.
		// Labels/colors/icons are presentation. Guarded by content_test parity.
		Markers: []Marker{
			{"start", "Start", "🟠", "#e8871e"},
			{"intro", "Introdução", "🔵", "#2f6fd0"},
			{"mixin", "Mix In", "🟣", "#8b5cf6"},
			{"hook", "Refrão", "🟢", "#3aa84f"},
			{"stamp", "Carimbo", "🔴", "#d64545"},
			{"mixout", "Mix Out", "🟣", "#6d3fd0"},
			{"end", "End", "🟤", "#a4530b"},
		},
		Mix: MixConfig{
			MusicaIn: 500, MusicaOut: 2000,
			VinhetaIn: 50, VinhetaOut: 1000,
			ComercialIn: 50, ComercialOut: 1000,
			OverlapVol: 30, TalkVol: 20,
			TrimMusicas: true, TrimVinhetas: false,
			Normalize: true, NormalizeVol: 89,
			DetectVocal:      true,
			AutoInstrumental: false,
		},
	}
}

// EmptyGradeLoad returns a zeroed 24h histogram — the clean-install grade before
// any grid is programmed (no mock occupancy).
func EmptyGradeLoad() GradeLoad { return GradeLoad{Hourly: make([]int, 24)} }

// WebTabs returns the Internet side-panel tab catalog.
func WebTabs() []WebTab {
	return []WebTab{
		{Name: "Notícias", Icon: "📰", URL: "https://news.google.com/home?hl=pt-BR&gl=BR", Framed: false},
		{Name: "Jornais", Icon: "🗞", URL: "https://www.google.com/search?igu=1&q=jornais+do+dia+capas", Framed: true},
		{Name: "Sociais", Icon: "💬", URL: "https://web.whatsapp.com/", Framed: false, Subtabs: []SubTab{
			{Name: "WhatsApp", URL: "https://web.whatsapp.com/", Framed: false, Brand: "whatsapp"},
			{Name: "Twitter", URL: "https://twitter.com/", Framed: false, Brand: "twitter"},
			{Name: "Facebook", URL: "https://www.facebook.com/", Framed: false, Brand: "facebook"},
		}},
		{Name: "Top 10", Icon: "🔟", URL: "https://www.google.com/search?igu=1&q=top+10+m%C3%BAsicas+brasil", Framed: true},
		{Name: "Cinema", Icon: "🎬", URL: "https://www.google.com/search?igu=1&q=estreias+cinema", Framed: true},
		{Name: "Signos", Icon: "♒", URL: "https://www.google.com/search?igu=1&q=hor%C3%B3scopo+do+dia", Framed: true},
		{Name: "Tempo", Icon: "⛅", URL: "https://embed.windy.com/embed2.html?lat=-27.6&lon=-48.55&zoom=7", Framed: true},
		{Name: "Info", Icon: "ℹ️", URL: "https://www.google.com/search?igu=1&q=efem%C3%A9rides+de+hoje", Framed: true},
	}
}

// ProgramPreview returns the mock program rows (above/below the playing track).
func ProgramPreview() Program {
	return Program{
		Above: []ProgramRow{
			{Kind: "music", Label: "Astro Azul — Noite de Neon (Radio Edit)", Dur: "00:01:00"},
			{Kind: "vinheta", Label: "AutoDJ (Mídia) — Jovem", Dur: "00:00:07"},
			{Kind: "music", Label: "Marés & Luar — Ainda Acredito (Trance Mix)", Dur: "00:00:59"},
			{Kind: "vinheta", Label: "Vinheta — Rádio do Coração", Dur: "00:00:05"},
			{Kind: "music", Label: "Cidade Clara — Tudo Certo", Dur: "00:01:00"},
			{Kind: "vinheta", Label: "AutoDJ — Fácil Assim 02 (Alice)", Dur: "00:00:06"},
			{Kind: "music", Label: "Vento Sul — Domingo à Tarde (Radio Edit)", Dur: "00:01:00"},
			{Kind: "bloco", Label: "Bloco comercial (16:15)", Dur: "00:00:44"},
			{Kind: "abert", Label: "Abert. B. Comercial — Módulos", Dur: "00:00:04"},
		},
		Below: []ProgramRow{
			{Kind: "horacerta", Label: "Hora Certa", Dur: "00:00:02"},
			{Kind: "comercial", Label: "Tecnologia Com Praticidade", Dur: "00:00:05"},
			{Kind: "comercial", Label: "Melhor Custo Benefício", Dur: "00:00:12"},
			{Kind: "comercial", Label: "Tudo Em Um Só Produto", Dur: "00:00:05"},
			{Kind: "temperatura", Label: "Temperatura Automática", Dur: "00:00:04"},
			{Kind: "vinhetaL", Label: "Vh Facilidade Praticidade — Jovem", Dur: "00:00:08"},
			{Kind: "vinheta", Label: "Vinheta — Exclusiva", Dur: "00:00:05"},
			{Kind: "music", Label: "Praia Norte — Batida do Mar (Feat. Nina)", Dur: "00:01:00"},
			{Kind: "vinheta", Label: "Vh Lenta — Jovem", Dur: "00:00:03"},
			{Kind: "music", Label: "Eco da Serra — O Outro Lado", Dur: "00:01:00"},
		},
	}
}

// GradeLoadPreview returns the mock per-hour grade occupancy (24 entries): a
// typical station profile — light overnight, drive-time peaks morning and late
// afternoon. Replaced by real scheduler data in Épico 4.
func GradeLoadPreview() GradeLoad {
	return GradeLoad{Hourly: []int{
		6, 4, 3, 3, 4, 7, 14, 22, 26, 24, 21, 20, // 00..11
		23, 25, 22, 19, 24, 28, 27, 21, 16, 12, 9, 7, // 12..23
	}}
}
