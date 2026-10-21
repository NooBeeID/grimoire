package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/term"

	"grimoire/cmd/internal/buildinfo"
	"grimoire/cmd/internal/server"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/logger"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/createadmin"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

const usage = `Pemakaian: grimoire [perintah]

Perintah:
  serve                                    jalankan server (default)
  migrate up                               jalankan migrasi database
  create-admin --email E --name N          buat akun super admin pertama
  search-quality report --project ID --since 7d
                                           laporan kualitas pencarian dari audit
                                           (opsi: --until, --window 120s, --json)
  version                                  tampilkan versi
  help                                     tampilkan bantuan

Env wajib untuk serve/migrate: DATABASE_URL, SESSION_SECRET. Lihat docs/configuration.md.
`

type cli struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	getenv         func(string) string
	isTTY          func() bool
	readSecret     func(prompt string) (string, error)
	serve          func(ctx context.Context) int
	migrateUp      func(ctx context.Context) int
	createAdminRun func(ctx context.Context, a createAdminArgs) error

	now              func() time.Time
	searchQualityRun func(ctx context.Context, a searchQualityArgs) (searchquality.Report, error)
}

func newCLI(stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) *cli {
	c := &cli{stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv}
	c.isTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	c.readSecret = func(prompt string) (string, error) {
		fmt.Fprint(c.stderr, prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(c.stderr)
		return string(b), err
	}
	c.serve = func(ctx context.Context) int { return server.ServeFromEnv(ctx, c.getenv, c.stderr) }
	c.migrateUp = func(ctx context.Context) int {
		return server.MigrateFromEnv(ctx, c.getenv, c.stdout, c.stderr)
	}
	c.createAdminRun = c.callCreateAdmin
	c.now = time.Now
	c.searchQualityRun = c.callSearchQuality
	return c
}

func (c *cli) badUsage(msg string) int {
	if msg != "" {
		fmt.Fprintln(c.stderr, msg)
		fmt.Fprintln(c.stderr)
	}
	fmt.Fprint(c.stderr, usage)
	return 2
}

func (c *cli) run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return c.serve(ctx)
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(c.stdout, usage)
		return 0
	case "serve":
		if len(args) > 1 {
			return c.badUsage("serve tidak menerima argumen")
		}
		return c.serve(ctx)
	case "migrate":
		if len(args) != 2 || args[1] != "up" {
			return c.badUsage("pemakaian: grimoire migrate up")
		}
		return c.migrateUp(ctx)
	case "version":
		fmt.Fprintln(c.stdout, buildinfo.String())
		return 0
	case "create-admin":
		return c.createAdmin(ctx, args[1:])
	case "search-quality":
		return c.searchQuality(ctx, args[1:])
	}
	return c.badUsage(fmt.Sprintf("perintah tidak dikenal: %q", args[0]))
}

type createAdminArgs struct{ Email, Name, Password string }

func (c *cli) createAdmin(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	email := fs.String("email", "", "")
	name := fs.String("name", "", "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(c.stdout, usage)
			return 0
		}
		return c.badUsage(err.Error())
	}
	if fs.NArg() > 0 {
		return c.badUsage(fmt.Sprintf("argumen tidak dikenal: %q", fs.Arg(0)))
	}
	if addr, err := mail.ParseAddress(*email); err != nil || addr.Address != *email {
		return c.badUsage("--email wajib dan harus alamat email valid")
	}
	if strings.TrimSpace(*name) == "" {
		return c.badUsage("--name wajib diisi")
	}
	pw, err := c.resolvePassword()
	if err != nil {
		return c.badUsage(err.Error())
	}
	if err := c.createAdminRun(ctx, createAdminArgs{Email: *email, Name: strings.TrimSpace(*name), Password: pw}); err != nil {
		c.reportCreateAdminError(err)
		return 1
	}
	return 0
}

// errAlreadyReported menandai error yang pesannya sudah dicetak (mis. config tidak valid).
var errAlreadyReported = errors.New("sudah dilaporkan")

const createAdminTimeout = 30 * time.Second

// callCreateAdmin merakit domain lewat server.WithDomains lalu memanggil bus create-admin.
func (c *cli) callCreateAdmin(ctx context.Context, a createAdminArgs) error {
	cfg, _, ok := server.LoadConfig(c.getenv, c.stderr)
	if !ok {
		return errAlreadyReported
	}
	// Stdout CLI harus bersih; log ke stderr.
	log := logger.NewWithWriter(cfg, c.stderr)
	ctx, cancel := context.WithTimeout(ctx, createAdminTimeout)
	defer cancel()
	err := server.WithDomains(ctx, cfg, log, func(d app.Deps) error {
		res, err := bus.CallTyped[createadmin.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_CREATE_ADMIN,
			createadmin.Request{Email: a.Email, Name: a.Name, Password: a.Password})
		if err != nil {
			return err
		}
		fmt.Fprintf(c.stdout, "admin %s dibuat (id %s)\n", a.Email, res.UserID)
		return nil
	})
	if err != nil && !isReportable(err) {
		log.Error("create-admin gagal", "error", err)
	}
	return err
}

func isReportable(err error) bool {
	var e *errs.Error
	var pgErr *pgconn.PgError
	return errors.As(err, &e) || (errors.As(err, &pgErr) && pgErr.Code == "42P01")
}

// reportCreateAdminError mencetak pesan aman ke stderr: hanya Message dan fields dari
// *errs.Error, tidak pernah err.Error() mentah.
func (c *cli) reportCreateAdminError(err error) {
	if errors.Is(err, errAlreadyReported) {
		return
	}
	var e *errs.Error
	if errors.As(err, &e) {
		fmt.Fprintln(c.stderr, e.Message)
		if fields := detailFields(e.Details); len(fields) > 0 {
			keys := make([]string, 0, len(fields))
			for k := range fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(c.stderr, "  %s: %s\n", k, fields[k])
			}
		}
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		fmt.Fprintln(c.stderr, `tabel belum ada; jalankan "grimoire migrate up" lebih dulu`)
		return
	}
	fmt.Fprintln(c.stderr, "gagal membuat admin; periksa log")
}

// detailFields membaca details["fields"]: map string->string apa pun tipenya
// (FieldErrors milik domain tidak bisa diimpor dari cmd).
func detailFields(details any) map[string]string {
	m, ok := details.(map[string]any)
	if !ok {
		return nil
	}
	v := reflect.ValueOf(m["fields"])
	if !v.IsValid() || v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String || v.Type().Elem().Kind() != reflect.String {
		return nil
	}
	out := make(map[string]string, v.Len())
	for _, k := range v.MapKeys() {
		out[k.String()] = v.MapIndex(k).String()
	}
	return out
}
