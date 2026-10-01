.PHONY: build run dev-backend dev-frontend test test-api test-race test-cover test-e2e gen-types build-web clean

## Sestaví Go binárku (bez CGO)
build:
	CGO_ENABLED=0 go build -o bin/nanofaktura ./cmd/server/

## Sestaví a spustí backend na :8080
run: build
	./bin/nanofaktura

## Spustí backend přes go run
dev-backend:
	go run ./cmd/server/

## Vite dev server na :5173 (proxuje /api → :8080)
dev-frontend:
	cd web && npm run dev

## Go testy + typecheck a unit/komponentové testy frontendu (pokud jsou nainstalované node_modules)
test:
	go vet ./...
	go test ./... -count=1
	@if [ -d web/node_modules ]; then cd web && npm run typecheck && npm test; fi

## Go testy s race detektorem (pomalejší, API testy ~4 min)
test-race:
	go test -race ./... -count=1

## Pokrytí Go testy po balíčcích + celkem (profil v coverage.out)
test-cover:
	go test ./... -count=1 -coverprofile=coverage.out
	@go tool cover -func=coverage.out | tail -1

## End-to-end testy (Playwright): sestaví binárku + SPA, spustí server s fake službami
test-e2e:
	cd e2e && npm ci && npm test

## Jen API testy
test-api:
	go test ./internal/api/... -v -count=1

## Přegeneruje web/src/api/schema.gen.ts z OpenAPI backendu
gen-types:
	go run ./cmd/gen-schema > web/openapi.json
	cd web && npx openapi-typescript openapi.json -o src/api/schema.gen.ts
	rm web/openapi.json

## Produkční build frontendu
build-web:
	cd web && npm run build

## Smaže build artefakty a lokální SQLite DB
clean:
	rm -rf bin/ web/dist/
	rm -f nanofaktura.db nanofaktura.db-wal nanofaktura.db-shm
