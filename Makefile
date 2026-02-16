SHELL := /bin/zsh

.PHONY: tidy build test run-gateway run-matchmaker run-gameserver run-telemetry run-client run-full up down logs smoke

tidy:
	cd backend && go mod tidy

build:
	cd backend && go build ./...

test:
	cd backend && go test ./...

run-gateway:
	cd backend && GATEWAY_ADDR=:9000 MATCHMAKER_HTTP=http://localhost:9001 go run ./cmd/gateway

run-matchmaker:
	cd backend && MATCHMAKER_ADDR=:9001 GAME_WS_ADDR=ws://localhost:9003/ws go run ./cmd/matchmaker

run-gameserver:
	cd backend && GAME_ADDR=:9003 MATCH_DURATION_SEC=300 go run ./cmd/gameserver

run-telemetry:
	cd backend && TELEMETRY_ADDR=:9002 go run ./cmd/telemetry

run-client:
	python3 -m http.server 5173 --directory client

run-full:
	./scripts/run_full_stack.sh

up:
	docker compose -f infra/docker-compose.yml up --build -d

down:
	docker compose -f infra/docker-compose.yml down

logs:
	docker compose -f infra/docker-compose.yml logs -f --tail=200

smoke:
	./scripts/smoke_test.sh
