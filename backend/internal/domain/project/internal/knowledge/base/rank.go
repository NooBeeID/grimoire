package base

// Konstanta pencarian T20. Nilai bobot diselaraskan dengan golden set (search-eval), bukan ditebak.
var RankWeights = [4]float32{0.1, 0.2, 0.5, 1.0} // urutan Postgres {D, C, B, A}

const (
	// StemFactor mengalikan skor ts_rank_cd indeks stem (search_stem) terhadap indeks simple.
	StemFactor = 0.5
	// PhraseBoost ditambahkan bila alternatif multi-kata cocok sebagai frasa di judul/heading.
	PhraseBoost = 0.5
	// MinCoverage: dokumen cocok bila memuat minimal ceil(n * MinCoverage) kelompok term.
	MinCoverage = 0.5
	// SearchScoreWindow membatasi berapa dokumen cocok yang diberi skor ts_rank_cd (mahal: detoast dua
	// tsvector per dokumen). Jendela dipilih menurut (cakupan, cocok judul/path, path); di bawah batas ini
	// urutan eksak, di atasnya hanya dokumen paling relevan menurut sinyal murah yang diurutkan penuh.
	SearchScoreWindow = 400

	// Koreksi typo (pg_trgm). TypoMinSimilarity >= ambang default operator % (0.3) agar indeks terpakai.
	TypoMinSimilarity = 0.4
	TypoMaxCandidates = 3
	TypoMinRunes      = 4
	TypoMaxRunes      = 64

	// Batas ukuran rencana query.
	MaxAltsPerGroup = 12
	MaxPlanAlts     = 64
)
