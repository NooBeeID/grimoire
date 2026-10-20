package base

import "strings"

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikePattern membuat pola ILIKE "contains" dengan karakter khusus di-escape.
// Dipakai dengan: ILIKE $n ESCAPE '\'. Satu-satunya tempat escape ini didefinisikan.
func LikePattern(q string) string {
	return "%" + likeEscaper.Replace(q) + "%"
}
