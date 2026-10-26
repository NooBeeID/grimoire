package store

import (
	"context"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

// searchSQL adalah kueri pencarian T20. Parameter: $1 project, $2..$4 kolom paralel SearchPlan.Params
// (teks, grup, stem_only), $5 RankWeights, $6 StemFactor, $7 PhraseBoost, $8 prefix LIKE, $9 MinCoverage,
// $10 limit, $11 HeadlineOptions, $12 SearchScoreWindow. Input user hanya masuk sebagai teks ke phraseto_tsquery (tidak pernah
// sebagai operator).
//
//   - alt: satu tsquery per alternatif (simple untuk indeks `search`, indonesian||english untuk `search_stem`;
//     varian peluluhan hanya ke indeks stem).
//   - grp: OR alternatif per kelompok term; q: OR semua kelompok (penyaring GIN); n = jumlah kelompok.
//   - pre: dokumen cocok (penyaring memakai InitPlan `(SELECT qs FROM q)` agar GIN terpakai), jumlah kelompok cocok
//     (matched; satu kelompok -> 1 tanpa subkueri), cakupan minimal ceil(n * MinCoverage), total seluruh cocok.
//     Hanya SearchScoreWindow dokumen teratas menurut (matched, judul/path cocok, path) yang diberi skor mahal,
//     sehingga biaya ts_rank_cd tidak tumbuh dengan jumlah dokumen yang cocok (term umum di project besar).
//     Kandidat diambil lewat UNION dua pencarian GIN (search, search_stem) agar planner tidak memilih seq scan
//     yang men-detoast semua tsvector.
//   - cand: skor berbobot (+ boost frasa) hanya untuk jendela itu.
//   - hits: urut matched DESC, skor DESC, path; LIMIT.
//   - akhir: bagian (heading) terbaik per dokumen untuk cuplikan; sections NULL atau satu bagian -> bagian utuh tanpa
//     menghitung peringkat bagian.
const searchSQL = `
WITH alt AS (
  SELECT u.g,
         CASE WHEN u.stem_only THEN NULL ELSE phraseto_tsquery('simple', u.a) END AS qs,
         phraseto_tsquery('indonesian', u.a) || phraseto_tsquery('english', u.a) AS qt
  FROM unnest($2::text[], $3::int[], $4::bool[]) AS u(a, g, stem_only)
),
grp AS (
  SELECT g, knowledge_tsquery_or_agg(qs) AS qs, knowledge_tsquery_or_agg(qt) AS qt FROM alt GROUP BY g
),
q AS (
  SELECT knowledge_tsquery_or_agg(qs) AS qs, knowledge_tsquery_or_agg(qt) AS qt, count(*) AS n FROM grp
),
phr AS (
  SELECT knowledge_tsquery_or_agg(qs) AS qs FROM alt WHERE qs IS NOT NULL AND numnode(qs) > 1
),
pre AS (
  SELECT m.*, count(*) OVER () AS total
  FROM (
    SELECT f.id, f.path, f.title,
           CASE WHEN q.n = 1 THEN 1 ELSE (SELECT count(*) FROM grp WHERE f.search @@ grp.qs OR f.search_stem @@ grp.qt) END AS matched,
           (to_tsvector('simple', f.path || ' ' || f.title) @@ q.qs)::int AS head
    FROM knowledge_files f, q
    WHERE f.project_id = $1
      AND f.id IN (SELECT i1.id FROM knowledge_files i1 WHERE i1.search @@ (SELECT qs FROM q)
                   UNION
                   SELECT i2.id FROM knowledge_files i2 WHERE i2.search_stem @@ (SELECT qt FROM q))
      AND f.path LIKE $8 ESCAPE '\'
  ) m, q
  WHERE m.matched >= ceil(q.n * $9::float8)
  ORDER BY m.matched DESC, m.head DESC, m.path COLLATE "C"
  LIMIT $12
),
cand AS (
  SELECT p.id, p.path, p.title, p.matched, p.total,
         (ts_rank_cd($5::float4[], f.search, q.qs, 1)
          + $6::float8 * ts_rank_cd($5::float4[], f.search_stem, q.qt, 1)
          + CASE WHEN phr.qs IS NOT NULL
                  AND (to_tsvector('simple', f.title) || to_tsvector('simple', coalesce(f.sections, '[]'::jsonb))) @@ phr.qs
                 THEN $7::float8 ELSE 0 END)::float8 AS score
  FROM pre p JOIN knowledge_files f ON f.id = p.id, q, phr
),
hits AS (
  SELECT c.*
  FROM cand c
  ORDER BY c.matched DESC, c.score DESC, c.path COLLATE "C"
  LIMIT $10
)
SELECT h.path, h.title, h.score, h.total, s.heading AS section, s.headline
FROM hits h
JOIN knowledge_files f ON f.id = h.id
CROSS JOIN q
CROSS JOIN LATERAL (
  SELECT b.heading,
         CASE WHEN to_tsvector('simple', b.body) @@ q.qs
              THEN ts_headline('simple', b.body, q.qs, $11)
              ELSE ts_headline('indonesian', b.body, q.qt, $11) END AS headline
  FROM (
    SELECT x.heading, x.body
    FROM (
      SELECT e->>'h' AS heading, i,
             replace(replace(substr(f.content, (e->>'s')::int + 1, least((e->>'e')::int - (e->>'s')::int, 200000)),
                             E'\x01', ''), E'\x02', '') AS body
      FROM jsonb_array_elements(coalesce(f.sections,
             jsonb_build_array(jsonb_build_object('h', '', 's', 0, 'e', char_length(f.content)))))
           WITH ORDINALITY AS x0(e, i)
    ) x
    ORDER BY CASE WHEN jsonb_array_length(coalesce(f.sections, '[]'::jsonb)) < 2 THEN 0::float4
                  ELSE ts_rank_cd(to_tsvector('simple', x.heading || ' ' || x.body), q.qs)
                     + ts_rank_cd(to_tsvector('indonesian', x.body) || to_tsvector('english', x.body), q.qt) END DESC,
             x.i
    LIMIT 1
  ) b
) s
ORDER BY h.matched DESC, h.score DESC, h.path COLLATE "C"`

type hitRow struct {
	Path     string  `db:"path"`
	Title    string  `db:"title"`
	Score    float64 `db:"score"`
	Total    int     `db:"total"`
	Section  string  `db:"section"`
	Headline string  `db:"headline"`
}

// Search menjalankan pencarian berbobot atas SearchPlan (sinonim, varian, typo). likePrefix dari
// base.LikePrefix. Mengembalikan hit teratas (maks limit) dan total dokumen cocok (0 bila tidak ada).
// Plan tanpa grup atau projectID bukan uuid -> kosong tanpa query.
func (s *Store) Search(ctx context.Context, projectID string, plan base.SearchPlan, likePrefix string, limit int) ([]base.SearchHit, int, error) {
	if !ValidUUID(projectID) || len(plan.Groups) == 0 {
		return []base.SearchHit{}, 0, nil
	}
	pp := plan.Params()
	var rows []hitRow
	if err := s.q.SelectContext(ctx, &rows, searchSQL, projectID, pp.Texts, pp.Groups, pp.StemOnly,
		base.RankWeights[:], base.StemFactor, base.PhraseBoost, likePrefix, base.MinCoverage, limit, HeadlineOptions, base.SearchScoreWindow); err != nil {
		return nil, 0, err
	}
	hits := make([]base.SearchHit, 0, len(rows))
	total := 0
	for _, r := range rows {
		total = r.Total
		hits = append(hits, base.SearchHit{Path: r.Path, Title: r.Title, Score: r.Score, Headline: r.Headline, Section: r.Section})
	}
	return hits, total, nil
}

const suggestTermsSQL = `
SELECT w.word AS word, t.term AS term
FROM unnest($2::text[]) AS w(word)
CROSS JOIN LATERAL (
  SELECT DISTINCT kt.term, similarity(kt.term, w.word) AS sim
  FROM knowledge_terms kt
  WHERE kt.project_id = $1 AND kt.term % w.word
    AND similarity(kt.term, w.word) >= $3::float4 AND kt.term <> w.word
  ORDER BY sim DESC, kt.term
  LIMIT $4
) t
WHERE NOT EXISTS (
  SELECT 1 FROM knowledge_files f
  WHERE f.project_id = $1
    AND (f.search @@ phraseto_tsquery('simple', w.word)
         OR f.search_stem @@ (phraseto_tsquery('indonesian', w.word) || phraseto_tsquery('english', w.word)))
)
ORDER BY w.word, t.sim DESC, t.term`

type suggestRow struct {
	Word string `db:"word"`
	Term string `db:"term"`
}

// SuggestTerms mengusulkan koreksi typo dari kosakata project (knowledge_terms, trigram) untuk kata yang
// tidak cocok satu dokumen pun. Hasil: kata -> kandidat (urut kemiripan, maks TypoMaxCandidates). projectID
// bukan uuid atau words kosong -> map kosong tanpa query.
func (s *Store) SuggestTerms(ctx context.Context, projectID string, words []string) (map[string][]string, error) {
	out := map[string][]string{}
	if !ValidUUID(projectID) || len(words) == 0 {
		return out, nil
	}
	var rows []suggestRow
	if err := s.q.SelectContext(ctx, &rows, suggestTermsSQL, projectID, words,
		float32(base.TypoMinSimilarity), base.TypoMaxCandidates); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Word] = append(out[r.Word], r.Term)
	}
	return out, nil
}
