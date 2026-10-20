# Penegak arsitektur dan tooling OpenAPI (T00 architecture-guard).
# api-bundle/api-check didefinisikan hanya di sini (tidak diduplikasi di makefile utama).

.PHONY: api-bundle api-check api-selftest depcruise test-arch

api-bundle: ## bundle api/root.yml ke api/openapi.yml
	./scripts/api-bundle.sh api/openapi.yml

api-check: ## gagal bila api/openapi.yml basi/diedit manual atau modul tidak direferensikan
	./scripts/api-check.sh

api-selftest: ## buktikan api-check benar-benar gagal
	./scripts/api-selftest.sh

depcruise: ## aturan impor frontend (dependency-cruiser)
	cd frontend && npm run depcruise

test-arch: ## tes arsitektur Go + frontend
	cd backend && go test -count=1 ./internal/archtest/...
	cd frontend && node scripts/test-arch.mjs
