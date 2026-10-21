// Command http menjalankan server saja (dipakai make dev).
package main

import (
	"context"
	"os"

	"grimoire/cmd/internal/server"
)

func main() {
	server.LoadDotEnv()
	os.Exit(server.ServeFromEnv(context.Background(), os.Getenv, os.Stderr))
}
