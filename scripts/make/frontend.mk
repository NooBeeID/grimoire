.PHONY: fe-build fe-restore-placeholder fe-check fe-test

FE_DIST := backend/internal/infra/static/dist

fe-build: ## build frontend dan salin ke dist ter-embed
	cd frontend && npm ci && npm run build
	rm -rf $(FE_DIST)/* && cp -R frontend/dist/. $(FE_DIST)/

fe-restore-placeholder: ## kembalikan dist/index.html placeholder sebelum commit
	git checkout -- $(FE_DIST)/index.html

fe-check: ## svelte-check frontend
	cd frontend && npm run check

fe-test: ## vitest frontend
	cd frontend && npm test -- --run
