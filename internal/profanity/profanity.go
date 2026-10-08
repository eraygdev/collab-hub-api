package profanity

import (
	"strings"
	"unicode"
)

// ─────────────────────────────────────────────────────────
// KÜFÜR LİSTELERİ
// ─────────────────────────────────────────────────────────
// Kural: Ağır cinsel küfürler + ağır hakaretler.
// Hafif argo, sebze, günlük kullanımı olan kelimeler GİRMEZ.
//
// Kelime içi eşleşme YOK (exact word match) — "göt" listemizde
// ama "götür" masum, yakalanmaz.
// ─────────────────────────────────────────────────────────

var turkishProfanities = []string{
	// ═══════════ CİNSEL KÜFÜRLER ═══════════
	"sik", "sike", "sikim", "sikik", "sikeyim", "sikerim", "sikiyim",
	"siktir", "siktirgit", "sikecem", "sikecegim", "sikicem",
	"sikil", "sikilmis", "sikimsonik", "sikici",
	"yarrak", "yarak", "yarram", "yarragi",
	"got", "gotu", "gotunu", "gote", "gotun", "gotveren", "gotlek", "gotos",
	"amk", "amq", "amcik", "amina", "amini", "aminakoyim", "aminakoyayim",
	"amciklar", "amlar", "amck",
	"orospu", "orospucocugu", "orosbu", "orosbucocugu", "orospular",
	"pic", "picler", "pezevenk", "pezo", "pezevek",
	"tasak", "tassak", "tassaklar", "dalyarak", "dalyarrak",
	"fahise", "fahiseler", "kaltak", "kaltaklar", "surtuk", "surtukler",
	"kasar", "pust", "pustlar", "ibne", "ibneler",

	// ═══════════ AĞIR HAKARETLER ═══════════
	"serefsiz", "serefsizler", "serefsizlik",
	"yavsak", "yavsaklar",
	"sapik", "sapiklar",

	// ═══════════ YAYGIN KISALTMALAR ═══════════
	"aq", "amq", "mk", "mka", "awk", "sg",

	// ═══════════ ARGO ═══════════
	"bok", "boktan", "defol",
}

var englishProfanities = []string{
	// Ağır cinsel + hakaret
	"fuck", "fucks", "fucked", "fucker", "fuckers", "fucking",
	"shit", "shits", "shitty", "shitting", "shitted",
	"bitch", "bitches",
	"asshole", "assholes",
	"bastard", "bastards",
	"cunt", "cunts",
	"dick", "dicks", "dickhead", "dickheads",
	"cock", "cocks", "cocksucker",
	"pussy", "pussies",
	"whore", "whores",
	"slut", "sluts",
	"nigger", "niggers", "nigga", "niggas",
	"faggot", "faggots", "fag", "fags",
	"retard", "retards", "retarded",
	"arse", "arsehole", "arseholes",
	"wanker", "wankers",
	"twat", "twats",
	"prick", "pricks",
	"bollocks",
	"wtf", "stfu", "gtfo",
}

// allProfanities — tüm dillerdeki küfürlerin birleşik lookup map'i.
var allProfanities map[string]bool

func init() {
	allProfanities = make(map[string]bool, len(turkishProfanities)+len(englishProfanities))
	for _, w := range turkishProfanities {
		allProfanities[w] = true
	}
	for _, w := range englishProfanities {
		allProfanities[w] = true
	}
}

// ─────────────────────────────────────────────────────────
// NORMALİZASYON
// ─────────────────────────────────────────────────────────
// - Küçük harfe çevirir
// - Türkçe karakterleri ASCII karşılığına indirger
// - Leet-speak karakterlerini normal harfe çevirir
// - Noktalama işaretlerini boşluğa çevirir
// - Çoklu boşlukları teke indirir
//
// Leet-speak sadece kelime İÇİNDE uygulanır (ör. "s1kt1r" → "siktir").
// Kelime sınırları (boşluk) korunur — "s@ ktir" → "sa ktir" olur,
// yani atlatma girişimi yakalanmaz. Bu bilinçli bir tercih:
// yazım hatası/atlatma ile uğraşmak yerine bariz kullanımı yakalıyoruz.
// ─────────────────────────────────────────────────────────

func normalize(s string) string {
	s = strings.ToLower(s)

	replacer := strings.NewReplacer(
		// Türkçe karakterler
		"ı", "i", "ş", "s", "ğ", "g", "ç", "c", "ö", "o", "ü", "u",
		// Leet-speak → harf
		"@", "a", "4", "a",
		"1", "i", "!", "i", "|", "i",
		"0", "o",
		"3", "e",
		"5", "s", "$", "s",
		"7", "t",
	)
	s = replacer.Replace(s)

	// Harf, rakam, boşluk dışındaki her şeyi boşluğa çevir
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, s)

	// Çoklu boşlukları teke indir
	return strings.Join(strings.Fields(s), " ")
}

// Contains — metinde küfür var mı kontrol eder (exact word match).
//
//	Contains("göt")        → true
//	Contains("götür")      → false  (tek kelime "göt" değil)
//	Contains("göt veren")  → true   (çünkü "göt" ayrı kelime)
//	Contains("amk proje")  → true
func Contains(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}

	words := strings.Fields(normalize(text))
	for _, word := range words {
		if allProfanities[word] {
			return true
		}
	}
	return false
}
