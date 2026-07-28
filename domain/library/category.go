package library

// Category is a validated media bucket in the folder-tree catalog. It is a
// fixed, closed set: a Track can only belong to one of these. Identifiers are
// English (the pt-BR labels for the UI are presentation, served by the API);
// vinheta/comercial/programete are Categories, not separate Track types.
type Category string

const (
	// CategoryMusic is a regular music track (default).
	CategoryMusic Category = "music"
	// CategoryJingle is a station ID / vinheta.
	CategoryJingle Category = "jingle"
	// CategorySoundtrack is background music / trilha.
	CategorySoundtrack Category = "soundtrack"
	// CategoryEffect is a short sound effect / efeito.
	CategoryEffect Category = "effect"
	// CategoryCommercial is an ad played as media / comercial.
	CategoryCommercial Category = "commercial"
	// CategoryPromo is a short self-promo segment / programete.
	CategoryPromo Category = "promo"
	// CategorySponsorship is a sponsor mention / oferecimento.
	CategorySponsorship Category = "sponsorship"
	// CategoryExtra is any other media / extra.
	CategoryExtra Category = "extra"
)

// DefaultCategory is applied when a Track has no category set (scan/seed paths).
const DefaultCategory = CategoryMusic

// categoryOrder is the canonical display order of the fixed category set. It is
// also the single source of truth for validity.
var categoryOrder = []Category{
	CategoryMusic, CategoryJingle, CategorySoundtrack, CategoryEffect,
	CategoryCommercial, CategoryPromo, CategorySponsorship, CategoryExtra,
}

// Categories returns the fixed set of valid categories in display order.
func Categories() []Category {
	out := make([]Category, len(categoryOrder))
	copy(out, categoryOrder)
	return out
}

// ValidCategory reports whether c is one of the fixed categories.
func ValidCategory(c Category) bool {
	for _, k := range categoryOrder {
		if k == c {
			return true
		}
	}
	return false
}
