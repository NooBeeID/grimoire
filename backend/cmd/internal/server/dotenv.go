package server

import "github.com/joho/godotenv"

// LoadDotEnv memuat .env (cwd, lalu root repo bila dijalankan dari backend/) ke environment proses.
// Variabel yang sudah ada tidak ditimpa; file yang tidak ada diabaikan.
func LoadDotEnv() {
	// Dipanggil satu per satu: godotenv.Load berhenti di file pertama yang gagal dibuka.
	for _, f := range []string{".env", "../.env"} {
		_ = godotenv.Load(f)
	}
}
