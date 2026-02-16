# Project Velocity (Build Foundation)

This repository starts the end-to-end implementation for a flagship arena car-soccer game, aligned with:
- `/Users/waqarazim/Desktop/rocketleague/PROJECT_VELOCITY_PRD_ARCHITECTURE_AGENT_PLAYBOOK.md`

## What is implemented now
- Authoritative game simulation server (120Hz sim, 60Hz replication)
- Matchmaking service (MMR + queue-time expansion)
- API gateway with guest auth and matchmaking proxy
- Telemetry ingest service + Prometheus metrics endpoint
- Playable browser 3D client (real-time controls, HUD, chase cam, live state sync)
- Local orchestration via Docker Compose
- Simulation and matchmaking unit tests
- Video-locked graphics/gameplay target spec

## Repository Layout
- `backend/cmd/gameserver`: realtime session server
- `backend/cmd/matchmaker`: queue and assignment service
- `backend/cmd/gateway`: external API and auth edge
- `backend/cmd/telemetry`: event ingest and metrics
- `backend/internal/simulation`: authoritative world/physics
- `backend/internal/matchmaking`: queue logic
- `backend/internal/shared/types`: cross-service contracts
- `client`: playable WebGL game client
- `docs/graphics/VIDEO_MATCH_SPEC.md`: locked visual/gameplay reference
- `docs/architecture/SPRINT0_IMPLEMENTATION.md`: PRD mapping for this sprint
- `infra/docker-compose.yml`: local stack
- `unreal/templates/Config`: UE 5.7 config templates for graphics/network baseline

## Quick Start

### 1. Build and test
```bash
make tidy
make build
make test
```

### 2. Run full local stack
```bash
make up
```

### 3. Open and play
```bash
open http://localhost:5173
```

### 4. Run smoke test (auth -> matchmaking -> match assignment)
```bash
make smoke
```

### 5. Stream logs
```bash
make logs
```

### 6. Stop stack
```bash
make down
```

## Single command (non-docker) local run
```bash
make run-full
```
Then open `http://localhost:5173`.

## Local Ports
- `9000`: gateway
- `9001`: matchmaker
- `9002`: telemetry (`/metrics` exposed)
- `9003`: game websocket server (`/ws`)
- `9090`: Prometheus
- `5173`: browser game client

## Next Build Steps
1. Integrate Unreal client with websocket input/state protocol.
2. Add PostgreSQL + Redis persistence for profiles, MMR, and queue durability.
3. Add anti-cheat event ingestion and rule engine.
4. Add replay capture and indexed retrieval APIs.
5. Add CI (lint/test/build/perf checks) and canary deployment workflow.
