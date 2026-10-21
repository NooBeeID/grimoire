# Seeder DB dev. Di-include otomatis oleh makefile utama.

.PHONY: seed

seed: dev-db ## seed super admin dev (admin@noobee.id / SeCr3t, wajib ganti password saat login)
	$(COMPOSE) exec -T db psql -v ON_ERROR_STOP=1 -U grimoire -d grimoire < scripts/seed/dev_admin.sql
