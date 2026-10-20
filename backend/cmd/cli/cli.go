package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"

	"golang.org/x/term"

	"grimoire/cmd/internal/buildinfo"
	"grimoire/cmd/internal/server"
)

const usage = `Pemakaian: grimoire [perintah]

Perintah:
  serve                                    jalankan server (default)
  migrate up                               jalankan migrasi database
  create-admin --email E --name N          buat akun super admin pertama
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
	if err := runCreateAdmin(ctx, createAdminArgs{Email: *email, Name: strings.TrimSpace(*name), Password: pw}); err != nil {
		fmt.Fprintln(c.stderr, err)
		return 1
	}
	return 0
}

// TODO(T01): ganti isi fungsi ini. Rakit config, db.Open, bus, core.Register (lewat
// server helper), lalu bus.Call(constants.BUS_CORE_AUTH_CREATE_ADMIN, ...) dan
// cetak "admin <email> dibuat".
func runCreateAdmin(ctx context.Context, a createAdminArgs) error {
	return errors.New("create-admin belum tersedia (butuh T01)")
}
