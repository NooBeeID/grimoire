package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"grimoire/cmd/internal/server"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/logger"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

type searchQualityArgs struct {
	ProjectID string
	Since     time.Time
	Until     time.Time // nol = default sisi server (now)
	Window    time.Duration
	JSON      bool
}

const (
	searchQualityTimeout = 10 * time.Minute
	// Duplikasi nilai batas dari domain disengaja; bus tetap sumber kebenaran.
	minWindow = time.Second
	maxWindow = time.Hour

	sinceHint = "tidak terbaca (pakai 7d, 24h, RFC3339, atau YYYY-MM-DD)"
)

var relativeDays = regexp.MustCompile(`^\d+d$`)

// parseSince membaca waktu relatif (7d, 24h), RFC3339, atau YYYY-MM-DD (UTC).
func parseSince(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("kosong")
	}
	if relativeDays.MatchString(s) {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n <= 0 || n > 3650 {
			return time.Time{}, errors.New("hari di luar batas")
		}
		return now.Add(-time.Duration(n) * 24 * time.Hour), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return time.Time{}, errors.New("durasi harus positif")
		}
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("format tidak dikenal")
}

func (c *cli) searchQuality(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "report" {
		return c.badUsage("pemakaian: grimoire search-quality report --project ID --since 7d [--until ...] [--window 120s] [--json]")
	}
	fs := flag.NewFlagSet("search-quality report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	project := fs.String("project", "", "")
	sinceS := fs.String("since", "", "")
	untilS := fs.String("until", "", "")
	windowS := fs.String("window", "", "")
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(c.stdout, usage)
			return 0
		}
		return c.badUsage(err.Error())
	}
	if fs.NArg() > 0 {
		return c.badUsage(fmt.Sprintf("argumen tidak dikenal: %q", fs.Arg(0)))
	}
	if strings.TrimSpace(*project) == "" {
		return c.badUsage("--project wajib diisi")
	}
	if *sinceS == "" {
		return c.badUsage("--since wajib diisi")
	}
	now := c.now()
	since, err := parseSince(*sinceS, now)
	if err != nil {
		return c.badUsage("--since " + sinceHint)
	}
	a := searchQualityArgs{ProjectID: strings.TrimSpace(*project), Since: since, JSON: *asJSON}
	if *untilS != "" {
		until, err := parseSince(*untilS, now)
		if err != nil {
			return c.badUsage("--until " + sinceHint)
		}
		if !until.After(since) {
			return c.badUsage("--until harus setelah --since")
		}
		a.Until = until
	}
	if *windowS != "" {
		w, err := time.ParseDuration(*windowS)
		if err != nil {
			return c.badUsage("--window tidak terbaca (contoh 120s, 5m)")
		}
		if w < minWindow || w > maxWindow {
			return c.badUsage("--window harus 1s..1h")
		}
		a.Window = w
	}
	rep, err := c.searchQualityRun(ctx, a)
	if err != nil {
		c.reportSearchQualityError(err)
		return 1
	}
	if a.JSON {
		if err := writeQualityJSON(c.stdout, rep); err != nil {
			fmt.Fprintln(c.stderr, "search-quality report gagal; periksa log")
			return 1
		}
		return 0
	}
	writeQualityText(c.stdout, rep)
	return 0
}

// callSearchQuality merakit domain lewat server.WithDomains lalu memanggil bus laporan.
func (c *cli) callSearchQuality(ctx context.Context, a searchQualityArgs) (searchquality.Report, error) {
	var rep searchquality.Report
	cfg, _, ok := server.LoadConfig(c.getenv, c.stderr)
	if !ok {
		return rep, errAlreadyReported
	}
	// Stdout CLI harus bersih (JSON); log ke stderr.
	log := logger.NewWithWriter(cfg, c.stderr)
	ctx, cancel := context.WithTimeout(ctx, searchQualityTimeout)
	defer cancel()
	err := server.WithDomains(ctx, cfg, log, func(d app.Deps) error {
		res, err := bus.CallTyped[searchquality.Response](ctx, d.Bus,
			constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT,
			searchquality.Request{ProjectID: a.ProjectID, Since: a.Since, Until: a.Until, Window: a.Window})
		if err != nil {
			return err
		}
		rep = res.Report
		return nil
	})
	if err != nil && !isReportable(err) {
		log.Error("search-quality gagal", "error", err)
	}
	return rep, err
}

// reportSearchQualityError mencetak pesan aman ke stderr, tidak pernah err.Error() mentah.
func (c *cli) reportSearchQualityError(err error) {
	if errors.Is(err, errAlreadyReported) {
		return
	}
	var e *errs.Error
	if errors.As(err, &e) {
		fmt.Fprintln(c.stderr, "search-quality: "+e.Message)
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		fmt.Fprintln(c.stderr, `tabel belum ada; jalankan "grimoire migrate up" lebih dulu`)
		return
	}
	fmt.Fprintln(c.stderr, "search-quality report gagal; periksa log")
}

func writeMetric(w io.Writer, label string, m searchquality.Metric) {
	fmt.Fprintf(w, "%-26s%d/%d   %.1f%%\n", label, m.Count, m.Denominator, m.Rate*100)
}

func writeQualityText(w io.Writer, r searchquality.Report) {
	fmt.Fprintf(w, "Kualitas pencarian project %s\n", r.ProjectID)
	fmt.Fprintf(w, "Rentang  %s .. %s (jendela %ds)\n",
		r.Since.UTC().Format(time.RFC3339), r.Until.UTC().Format(time.RFC3339), r.WindowSeconds)
	fmt.Fprintf(w, "%-26s%d\n", "Pencarian", r.Searches)
	writeMetric(w, "Zero-result", r.ZeroResult)
	writeMetric(w, "Follow-up", r.FollowUp)
	writeMetric(w, "Reformulasi", r.Reformulation)
	parts := make([]string, 0, len(r.RankOpened))
	for _, b := range r.RankOpened {
		label := b.Label
		if label == searchquality.NotInResults {
			label = "tidak di hasil"
		}
		parts = append(parts, fmt.Sprintf("%s:%d", label, b.Count))
	}
	fmt.Fprintf(w, "%-26s%s\n", "Rank dibuka", strings.Join(parts, "  "))
	fmt.Fprintf(w, "%-26s%d (tanpa result_paths; tidak ikut follow-up/rank)\n", "Entri lama dilewati", r.LegacySearches)
}

func writeQualityJSON(w io.Writer, r searchquality.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(searchquality.Response{Report: r})
}
