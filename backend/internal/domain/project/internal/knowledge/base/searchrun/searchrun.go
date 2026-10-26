// Package searchrun mengorkestrasi satu pencarian knowledge (T20): ambil kamus sinonim lewat bus
// (fail-soft), susun SearchPlan, minta koreksi typo untuk kata tunggal, lalu jalankan Store.Search.
// Dipakai bersama oleh usecase HTTP search dan bus bussearch supaya perilaku keduanya identik.
package searchrun

import (
	"context"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/searchsynonym/listgroups"
)

// Store dipenuhi oleh *store.Store.
type Store interface {
	SuggestTerms(ctx context.Context, projectID string, words []string) (map[string][]string, error)
	Search(ctx context.Context, projectID string, plan base.SearchPlan, likePrefix string, limit int) ([]base.SearchHit, int, error)
}

// Synonyms menyediakan kelompok sinonim project (huruf kecil).
type Synonyms interface {
	Groups(ctx context.Context, projectID string) ([][]string, error)
}

type busSynonyms struct{ b *bus.Registry }

// NewBusSynonyms membaca kamus lewat bus project.searchsynonym.list-groups.
func NewBusSynonyms(b *bus.Registry) Synonyms { return busSynonyms{b: b} }

func (s busSynonyms) Groups(ctx context.Context, projectID string) ([][]string, error) {
	res, err := bus.CallTyped[listgroups.Response](ctx, s.b, constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS,
		listgroups.Request{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	return res.Groups, nil
}

// Outcome adalah hasil satu pencarian.
type Outcome struct {
	Hits     []base.SearchHit
	Total    int
	Expanded bool // sinonim atau koreksi typo dipakai
}

type Runner struct {
	store Store
	syn   Synonyms
	log   *slog.Logger
}

func New(st Store, syn Synonyms, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{store: st, syn: syn, log: log}
}

// Run menjalankan pencarian. Kegagalan kamus sinonim hanya di-log (warn, tanpa isi query) dan pencarian
// berlanjut tanpa sinonim (D4). Kegagalan SuggestTerms/Search dikembalikan terbungkus.
func (r *Runner) Run(ctx context.Context, projectID string, terms []base.Term, likePrefix string, limit int) (Outcome, error) {
	groups, err := r.syn.Groups(ctx, projectID)
	if err != nil {
		r.log.WarnContext(ctx, "knowledge: kamus sinonim tidak tersedia, mencari tanpa sinonim",
			"project_id", projectID, "err", err)
		groups = nil
	}
	plan := base.NewSearchPlan(terms, groups)
	if cands := plan.TypoCandidates(); len(cands) > 0 {
		sugg, err := r.store.SuggestTerms(ctx, projectID, cands)
		if err != nil {
			return Outcome{}, fmt.Errorf("searchrun: suggest terms: %w", err)
		}
		plan = plan.WithTypos(sugg)
	}
	hits, total, err := r.store.Search(ctx, projectID, plan, likePrefix, limit)
	if err != nil {
		return Outcome{}, fmt.Errorf("searchrun: search: %w", err)
	}
	return Outcome{Hits: hits, Total: total, Expanded: plan.Expanded()}, nil
}
