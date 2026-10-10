include .env
export
.PHONY: help up down logs logs-t logs-cf logs-cf-t logs-all logs-all-t logs-errors logs-500 logs-4xx logs-important logs-rate logs-auth logs-forbidden logs-filter restart rebuild run build test lint tidy clean check migrate-up migrate-down migrate-version migrate-new migrate-force

help:
	@echo "RepoReef Backend — Komutlar:"
	@echo ""
	@echo "  ── DOCKER ──"
	@echo "  up             Stack'i baslat"
	@echo "  down           Stack'i durdur"
	@echo "  restart        Backend'i yeniden baslat"
	@echo "  rebuild        Kod degisti, yeniden derle"
	@echo ""
	@echo "  ── LOGS (bu terminal) ──"
	@echo "  logs           Backend loglari"
	@echo "  logs-cf        Cloudflared loglari"
	@echo "  logs-all       Tum loglar"
	@echo ""
	@echo "  ── LOGS (yeni tab) ──"
	@echo "  logs-t         Backend loglari (yeni tab)"
	@echo "  logs-cf-t      Cloudflared loglari (yeni tab)"
	@echo "  logs-all-t     Tum loglar (yeni tab)"
	@echo ""
	@echo "  ── LOGS (filtreli, canli) ──"
	@echo "  logs-errors    Sadece [ERROR]"
	@echo "  logs-500       Sadece 500 donen istekler"
	@echo "  logs-4xx       Sadece 4xx donen istekler"
	@echo "  logs-important 500 + rate limit + auth fail + forbidden + filter"
	@echo "  logs-rate      Rate limit olaylari"
	@echo "  logs-auth      Auth hatalari"
	@echo "  logs-forbidden Yetki hatalari"
	@echo "  logs-filter    Profanity filtre olaylari"
	@echo ""
	@echo "  ── KONTROL ──"
	@echo "  check          build + vet + lint (+ test)"
	@echo "  build          Binary uret"
	@echo "  test           Testleri calistir"
	@echo "  lint           golangci-lint"
	@echo "  tidy           go mod tidy"
	@echo "  clean          Temizle"
	@echo ""
	@echo "  ── MIGRATIONS ──"
	@echo "  migrate-up     Bekleyen migration'ları uygula"
	@echo "  migrate-down   Son migration'ı geri al"
	@echo "  migrate-version Aktif sürüm"
	@echo "  migrate-new    Yeni migration dosyası oluştur"
	@echo ""
	@echo "  cd ~/Masaüstü/RepoReef/Backend"

# ═══════════════════════════════════════════════════════════
# DOCKER
# ═══════════════════════════════════════════════════════════

up:
	docker compose up -d --build
	@echo ""
	@echo "Stack baslatildi. Loglar icin: make logs-all  (veya make logs-all-t)"

down:
	docker compose down

# ═══════════════════════════════════════════════════════════
# LOGS — Genel
# ═══════════════════════════════════════════════════════════

logs:
	docker compose logs -f --tail 20 backend

logs-t:
	gnome-terminal --tab --title="RepoReef - Backend Logs" -- bash -c "cd $(PWD) && docker compose logs -f --tail 20 backend; exec bash"

logs-cf:
	docker compose logs -f --tail 20 cloudflared

logs-cf-t:
	gnome-terminal --tab --title="RepoReef - Cloudflared Logs" -- bash -c "cd $(PWD) && docker compose logs -f --tail 20 cloudflared; exec bash"

logs-all:
	docker compose logs -f --tail 20 backend cloudflared

logs-all-t:
	gnome-terminal --tab --title="RepoReef - All Logs" -- bash -c "cd $(PWD) && docker compose logs -f --tail 20 backend cloudflared; exec bash"

# ═══════════════════════════════════════════════════════════
# LOGS — Filtreli (canli, sadece ilgili satirlar)
# ═══════════════════════════════════════════════════════════

logs-errors:
	docker compose logs -f --tail 0 backend | grep --line-buffered "\[ERROR\]"

logs-500:
	docker compose logs -f --tail 0 backend | grep --line-buffered -E "\| 500 \|"

logs-4xx:
	docker compose logs -f --tail 0 backend | grep --line-buffered -E "\| 4[0-9][0-9] \|"

logs-important:
	docker compose logs -f --tail 0 backend | grep --line-buffered -E "\[(ERROR|RATE LIMIT|AUTH FAIL|FORBIDDEN|FILTER)\]|\| 500 \|"

logs-rate:
	docker compose logs -f --tail 0 backend | grep --line-buffered "\[RATE LIMIT\]"

logs-auth:
	docker compose logs -f --tail 0 backend | grep --line-buffered "\[AUTH FAIL\]"

logs-forbidden:
	docker compose logs -f --tail 0 backend | grep --line-buffered "\[FORBIDDEN\]"

logs-filter:
	docker compose logs -f --tail 0 backend | grep --line-buffered "\[FILTER\]"

# ═══════════════════════════════════════════════════════════
# DOCKER — Restart / Rebuild
# ═══════════════════════════════════════════════════════════

restart:
	docker compose restart backend

rebuild:
	docker compose up -d --build backend

# ═══════════════════════════════════════════════════════════
# GO
# ═══════════════════════════════════════════════════════════

run:
	go run ./cmd/server

# ═══════════════════════════════════════════════════════════
# KONTROL
# ═══════════════════════════════════════════════════════════

check:
	@echo "═══ 1/4  build ═══"
	go build ./...
	@echo ""
	@echo "═══ 2/4  vet ═══"
	go vet ./...
	@echo ""
	@echo "═══ 3/4  lint ═══"
	golangci-lint run --timeout=5m
	@echo ""
	@echo "═══ 4/4  test ═══"
	@echo "(su an test yok — asagidaki satir testler yazilinca aktif edilecek)"
#	go test ./... -race -coverprofile=coverage.out
	@echo ""
	@echo "✔ Tum kontroller gecti."

build:
	go build -o reporeef-api ./cmd/server

test:
	go test ./... -race -coverprofile=coverage.out

lint:
	golangci-lint run --timeout=5m

tidy:
	go mod tidy

clean:
	rm -f reporeef-api coverage.out

	# ═══════════════════════════════════════════════════════════
# MIGRATIONS
# ═══════════════════════════════════════════════════════════

migrate-up:      ## Bekleyen migration'ları uygula
	@migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:    ## Son migration'ı geri al
	@migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-version: ## Aktif migration sürümü
	@migrate -path migrations -database "$(DATABASE_URL)" version

migrate-force:   ## Baseline (kullanım: make migrate-force V=2)
	migrate -path migrations -database "$(DATABASE_URL)" force $(V)

migrate-new:     ## Yeni migration (kullanım: make migrate-new NAME=add_foo)
	@if [ -z "$(NAME)" ]; then echo "Kullanım: make migrate-new NAME=add_foo"; exit 1; fi
	@N=$$(printf "%03d" $$(($$(migrate -path migrations -database "$(DATABASE_URL)" version 2>/dev/null | tail -1) + 1))); \
	touch migrations/$${N}_$(NAME).up.sql migrations/$${N}_$(NAME).down.sql; \
	echo "✔ Oluşturuldu: migrations/$${N}_$(NAME).{up,down}.sql"