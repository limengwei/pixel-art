package translate

import "strings"

// Tags translates the English words of text into space-separated Chinese
// tags, dropping duplicates. Missing words are skipped; an empty result
// means nothing was translatable.
func Tags(text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return r == ' ' || r == '_' || r == '-' || r == ',' || r == '.'
	})
	var b strings.Builder
	seen := make(map[string]bool, 8)
	for _, w := range words {
		zh, ok := enToZh[w]
		if !ok || seen[w] {
			continue
		}
		seen[w] = true
		for _, t := range strings.Fields(zh) {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(t)
		}
	}
	return b.String()
}
