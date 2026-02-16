# Sprint 0 Implementation (Aligned to Project Velocity PRD)

## Source of Truth
- Blueprint: `/Users/waqarazim/Desktop/rocketleague/PROJECT_VELOCITY_PRD_ARCHITECTURE_AGENT_PLAYBOOK.md`

## Delivered in this sprint

### 1. Authoritative Simulation Core
Mapped PRD sections:
- `4.1 Gameplay and Simulation Requirements`
- `5.2 Server Simulation Budget`
- `7.3 Physics and Gameplay Simulation`
- `7.4 Networking Architecture`

Implementation:
- `backend/internal/simulation/world.go`
  - server-authoritative world state
  - fixed-tick simulation path (120Hz in runtime loop)
  - car input application and movement
  - ball physics with gravity, bounces, and wall constraints
  - car-ball collision response
  - goal detection and kickoff reset
  - event feed (`goal`, `shot_on_goal`, `kickoff`)

### 2. Realtime Session Server
Mapped PRD sections:
- `4.2 Multiplayer and Social Requirements`
- `7.4 Networking Architecture`

Implementation:
- `backend/cmd/gameserver/main.go`
  - websocket endpoint: `/ws`
  - health endpoint: `/health`
  - client input ingestion and validation
  - snapshot replication at 60Hz
  - simulation loop at 120Hz

### 3. Matchmaking Service
Mapped PRD sections:
- `4.2 Multiplayer and Social Requirements`
- `7.6 Matchmaking Design`

Implementation:
- `backend/internal/matchmaking/queue.go`
  - queue buckets by `region|playlist`
  - MMR window expansion based on queue time
  - assignment generation with server endpoint
- `backend/cmd/matchmaker/main.go`
  - `/v1/queue/join`
  - `/v1/queue/poll`
  - `/v1/queue/leave`
  - `/health`

### 4. API Gateway + Guest Auth
Mapped PRD sections:
- `7.5 Backend Microservices`

Implementation:
- `backend/cmd/gateway/main.go`
  - guest auth endpoint: `/v1/auth/guest`
  - matchmaking proxy endpoints via gateway
  - bearer token validation for protected endpoints

### 5. Telemetry and Observability Seed
Mapped PRD sections:
- `9.1 Telemetry and Tracing`
- `9.2 Metrics and Alerts`

Implementation:
- `backend/cmd/telemetry/main.go`
  - event ingest endpoint: `/v1/events`
  - recent event listing endpoint
  - metrics endpoint: `/metrics` (Prometheus format)

### 6. Local Runtime and Ops Tooling
Mapped PRD sections:
- `8.2 CI Pipeline Stages`
- `8.3 CD and Runtime Safety`

Implementation:
- `infra/docker-compose.yml`
- `infra/prometheus/prometheus.yml`
- `backend/Dockerfile`
- `Makefile`
- `scripts/smoke_test.sh`
- `scripts/run_full_stack.sh`

### 7. Playable Web Client
Mapped PRD sections:
- `4.2 Multiplayer and Social Requirements`
- `7.1 Client Runtime Architecture`
- `7.2 Rendering Architecture`

Implementation:
- `client/index.html`
- `client/styles.css`
- `client/main.js`

Features:
- guest auth and matchmaking flow through gateway
- websocket session connection and authoritative state sync
- keyboard controls and input stream (WASD/boost/jump/handbrake)
- real-time 3D arena, cars, ball, chase camera, scoreboard/HUD
- event feed and basic ping telemetry

## API Contracts
Shared models:
- `backend/internal/shared/types/types.go`

Primary request flows:
1. Guest auth (`gateway`) -> token
2. Queue join (`gateway` -> `matchmaker`) -> ticket
3. Queue poll -> assignment with `server_addr`
4. Client connects to game websocket -> receives `welcome` + periodic `state`

## Quality controls included now
- Unit tests for simulation and matchmaking:
  - `backend/internal/simulation/world_test.go`
  - `backend/internal/matchmaking/queue_test.go`

## Deferred to next sprint
- persistent storage (PostgreSQL/Redis)
- anti-cheat signal pipeline
- dedicated session allocator/fleet manager
- Unreal client integration and replay service
- production auth/provider integration
