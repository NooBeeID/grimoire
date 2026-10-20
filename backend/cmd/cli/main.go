// Command cli adalah binary grimoire (serve, migrate, create-admin, version).
package main

import (
	"context"
	"os"
)

func main() {
	c := newCLI(os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	os.Exit(c.run(context.Background(), os.Args[1:]))
}
