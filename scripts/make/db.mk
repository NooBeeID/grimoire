# Target database/migrasi. Di-include dari makefile utama bila diperlukan.

.PHONY: db-migrate db-test-integration

db-migrate: ## jalankan migrasi (migrate up)
	cd backend && go run ./cmd/cli migrate up

db-test-integration: ## test integrasi paket db (butuh TEST_DATABASE_URL)
	cd backend && go test -race -tags integration ./internal/infra/db/...
