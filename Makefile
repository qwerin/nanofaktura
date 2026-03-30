.PHONY: build test test-verbose lint run dev clean gen-types

## Sestaví Go binárku
build:
	go build -o bin/nanofaktura ./cmd/server/

## Spustí všechny testy (Go + frontend typecheck)
test:
	go test ./cmd/... ./internal/... -count=1
	cd web && npx tsc --noEmit

## Spustí testy s výpisem každého testu
test-verbose:
	go test ./cmd/... ./internal/... -v -count=1

## Spustí pouze handler/endpoint testy
test-api:
	go test ./internal/handler/... -v -count=1

## Spustí pouze model/výpočetní testy
test-models:
	go test ./internal/models/... ./internal/ares/... -v -count=1

## Sestaví produkční frontend bundle
build-web:
	cd web && npm run build

## Spustí backend (port 8080)
run: build
	./bin/nanofaktura

## Sestaví frontend + backend a spustí na :8080 (produkční mód lokálně)
run-full: build-web build
	NANOFAKTURA_STATIC_DIR=web/dist ./bin/nanofaktura

## Dev: backend + frontend Vite dev server (vyžaduje 2 terminály)
dev-backend:
	go run ./cmd/server/

dev-frontend:
	cd web && npm run dev

## Přegeneruje TypeScript typy z OpenAPI schématu backendu
gen-types:
	go run ./cmd/gen-schema/ > openapi.json
	cd web && npx openapi-typescript ../openapi.json -o src/api/schema.gen.ts
	rm openapi.json

## Smaže sestavené artefakty a dočasné DB soubory
clean:
	rm -f bin/nanofaktura
	rm -f nanofaktura.db nanofaktura.db-wal nanofaktura.db-shm
	cd web && rm -rf dist
