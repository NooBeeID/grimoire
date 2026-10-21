// Package openapi adalah parser, validator, dan resolver OpenAPI 3.0/3.1 yang MURNI:
// hanya CPU atas []byte di memori. Package ini tidak boleh meng-import net, os, io/fs, dan sejenisnya
// (dibuktikan imports_test.go) sehingga tidak mungkin melakukan fetch atau membaca file (rule 7.10).
package openapi

// Limits membatasi kompleksitas dokumen sebelum dikonversi (billion laughs).
type Limits struct{ MaxDepth, MaxAliases, MaxNodes int }

// DefaultLimits adalah batas produksi.
var DefaultLimits = Limits{MaxDepth: 64, MaxAliases: 100, MaxNodes: 500_000}

// MaxIssues adalah jumlah issue maksimum yang dikembalikan Parse.
const MaxIssues = 20

// Format adalah format sumber dokumen.
type Format string

const (
	FormatYAML Format = "yaml"
	FormatJSON Format = "json"
)
