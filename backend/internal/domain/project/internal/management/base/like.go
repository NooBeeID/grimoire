package base

import "strings"

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikePattern membuat pola ILIKE "contains"; dipakai dengan: ILIKE $n ESCAPE '\'.
func LikePattern(q string) string {
	return "%" + likeEscaper.Replace(q) + "%"
}
