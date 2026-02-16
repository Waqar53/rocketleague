package simulation

import (
	"math"
	"sync"
	"time"

	"projectvelocity/backend/internal/shared/types"
)

const (
	ArenaLength = 8192.0
	ArenaWidth  = 10240.0
	ArenaHeight = 2044.0

	GoalWidth  = 1785.51
	GoalHeight = 642.775

	CarRadius  = 95.0
	BallRadius = 91.25

	MaxCarSpeed   = 2300.0
	MaxBoostSpeed = 2300.0
	MaxDriveSpeed = 1410.0
	ThrottleAccel = 1600.0
	BrakeAccel    = 3500.0
	BoostAccel    = 991.666
	TurnRate      = 3.4 // rad/s baseline

	Gravity       = -650.0
	JumpVelocity  = 292.0
	JumpHoldAccel = 1460.0
	JumpHoldMax   = 0.2
	StickyForce   = 325.0
	StickyTime    = 3.0 / 120.0
	DoubleJumpMax = 1.25

	BallRestitution    = 0.60
	WallRestitution    = 0.78
	CarBallElasticity  = 0.94
	GroundFriction     = 0.9965
	CoastFriction      = 0.9960
	LateralGrip        = 0.78
	HandbrakeGrip      = 0.90
	HandbrakeTurnBoost = 1.35
	AirResistance      = 0.9992
	AirThrottleAccel   = 66.667
	AirReverseAccel    = 33.334
	BallMaxSpeed       = 6000.0

	BotSteerNormalization = 35.0
)

// PlayerSpawn defines initial player details at match creation.
type PlayerSpawn struct {
	PlayerID    string
	DisplayName string
	Team        string
}

type jumpContext struct {
	usedJumps     int
	timeSinceJump float64
	holdTime      float64
	stickyTime    float64
}

// World is the authoritative simulation state.
type World struct {
	mu             sync.RWMutex
	state          types.MatchState
	input          map[string]types.CarInput
	jump           map[string]*jumpContext
	lastShotByTeam map[string]int64
}

// NewWorld creates a world with kickoff positions.
func NewWorld(matchID string, duration time.Duration, players []PlayerSpawn) *World {
	cars := make(map[string]types.CarState, len(players))
	jump := make(map[string]*jumpContext, len(players))

	teamSlots := map[string]int{
		"orange": 0,
		"blue":   0,
	}
	for _, p := range players {
		team := p.Team
		if team != "blue" {
			team = "orange"
		}

		slot := teamSlots[team]
		teamSlots[team]++

		posX := -2048.0
		yaw := 0.0
		posY := kickoffSlotOffset(slot)
		if team == "blue" {
			posX = 2048.0
			yaw = 180.0
			posY = kickoffSlotOffset(slot)
		}

		cars[p.PlayerID] = types.CarState{
			PlayerID:    p.PlayerID,
			DisplayName: p.DisplayName,
			Team:        team,
			IsBot:       false,
			Position:    types.Vec3{X: posX, Y: posY, Z: CarRadius},
			Velocity:    types.Vec3{},
			Rotation:    types.Rotator{Yaw: yaw},
			Boost:       100,
			IsGrounded:  true,
		}
		jump[p.PlayerID] = &jumpContext{}
	}

	now := time.Now().UTC()
	w := &World{
		state: types.MatchState{
			MatchID:   matchID,
			Tick:      0,
			CreatedAt: now,
			Cars:      cars,
			Ball: types.BallState{
				Position: types.Vec3{X: 0, Y: 0, Z: BallRadius + 20},
				Velocity: types.Vec3{X: 0, Y: 0, Z: 0},
				Radius:   BallRadius,
			},
			Score: types.ScoreState{
				Orange:          0,
				Blue:            0,
				TimeRemainingMS: int(duration.Milliseconds()),
			},
			Events: []types.GameplayEvent{{
				Type:       "kickoff",
				OccurredMS: now.UnixMilli(),
			}},
		},
		input: make(map[string]types.CarInput, len(players)),
		jump:  jump,
		lastShotByTeam: map[string]int64{
			"orange": 0,
			"blue":   0,
		},
	}
	return w
}

// ApplyInput stores latest client input for the player.
func (w *World) ApplyInput(in types.CarInput) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.input[in.PlayerID] = clampInput(in)
}

// Tick advances the world simulation by dt seconds.
func (w *World) Tick(dt float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.state.Tick++
	w.state.Events = w.state.Events[:0]
	w.computeBotInputs()

	if w.state.Score.TimeRemainingMS > 0 {
		deltaMS := int(dt * 1000)
		if deltaMS < 1 {
			deltaMS = 1
		}
		w.state.Score.TimeRemainingMS -= deltaMS
		if w.state.Score.TimeRemainingMS < 0 {
			w.state.Score.TimeRemainingMS = 0
		}
	}

	for id, car := range w.state.Cars {
		in := w.input[id]
		prev := car.LastInput
		jc := w.jump[id]
		if jc == nil {
			jc = &jumpContext{}
			w.jump[id] = jc
		}
		updateCar(&car, in, prev, jc, dt)
		car.LastInput = in
		clampCarBounds(&car)
		w.state.Cars[id] = car
	}

	updateBall(&w.state.Ball, dt)
	clampBallBounds(&w.state.Ball)
	resolveCarBallCollisions(&w.state)
	w.detectShotOnGoal()
	w.detectGoalAndResetIfNeeded()
}

// Snapshot returns a deep copy of state for safe replication.
func (w *World) Snapshot() types.MatchState {
	w.mu.RLock()
	defer w.mu.RUnlock()

	copyCars := make(map[string]types.CarState, len(w.state.Cars))
	for k, v := range w.state.Cars {
		copyCars[k] = v
	}

	events := make([]types.GameplayEvent, len(w.state.Events))
	copy(events, w.state.Events)

	out := w.state
	out.Cars = copyCars
	out.Events = events
	return out
}

// EnsurePlayer inserts a player if not present and returns the assigned team.
func (w *World) EnsurePlayer(playerID, displayName string) string {
	w.mu.Lock()
	defer w.mu.Unlock()

	if c, ok := w.state.Cars[playerID]; ok {
		if displayName != "" {
			c.DisplayName = displayName
		}
		c.IsBot = false
		w.state.Cars[playerID] = c
		if _, ok := w.jump[playerID]; !ok {
			w.jump[playerID] = &jumpContext{}
		}
		return c.Team
	}

	orangeCount := 0
	blueCount := 0
	for _, c := range w.state.Cars {
		if c.IsBot {
			continue
		}
		if c.Team == "blue" {
			blueCount++
		} else {
			orangeCount++
		}
	}

	team := "orange"
	if orangeCount > blueCount {
		team = "blue"
	}

	teamSlot := orangeCount
	if team == "blue" {
		teamSlot = blueCount
	}
	pos := types.Vec3{X: -2048, Y: kickoffSlotOffset(teamSlot), Z: CarRadius}
	yaw := 0.0
	if team == "blue" {
		pos = types.Vec3{X: 2048, Y: kickoffSlotOffset(teamSlot), Z: CarRadius}
		yaw = 180.0
	}

	w.state.Cars[playerID] = types.CarState{
		PlayerID:    playerID,
		DisplayName: displayName,
		Team:        team,
		IsBot:       false,
		Position:    pos,
		Velocity:    types.Vec3{},
		Rotation:    types.Rotator{Yaw: yaw},
		Boost:       100,
		IsGrounded:  true,
	}
	w.jump[playerID] = &jumpContext{}

	w.state.Events = append(w.state.Events, types.GameplayEvent{
		Type:       "player_join",
		PlayerID:   playerID,
		Team:       team,
		OccurredMS: time.Now().UTC().UnixMilli(),
	})
	return team
}

// HumanCount returns number of human-controlled cars.
func (w *World) HumanCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	count := 0
	for _, c := range w.state.Cars {
		if !c.IsBot {
			count++
		}
	}
	return count
}

// FirstHumanID returns one active human player id, if any.
func (w *World) FirstHumanID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for id, c := range w.state.Cars {
		if !c.IsBot {
			return id
		}
	}
	return ""
}

// EnsureBotOpponent guarantees one opponent bot for a human player if needed.
func (w *World) EnsureBotOpponent(playerID string) string {
	w.mu.Lock()
	defer w.mu.Unlock()

	player, ok := w.state.Cars[playerID]
	if !ok {
		return ""
	}
	opp := "orange"
	if player.Team == "orange" {
		opp = "blue"
	}

	oppHumans := 0
	for _, c := range w.state.Cars {
		if c.Team == opp && !c.IsBot {
			oppHumans++
		}
	}
	if oppHumans > 0 {
		return ""
	}

	for id, c := range w.state.Cars {
		if c.Team == opp && c.IsBot {
			return id
		}
	}

	oppCount := 0
	for _, c := range w.state.Cars {
		if c.Team == opp {
			oppCount++
		}
	}

	botID := "bot_" + opp + "_" + time.Now().UTC().Format("150405.000000000")
	pos := types.Vec3{X: -2048, Y: kickoffSlotOffset(oppCount), Z: CarRadius}
	yaw := 0.0
	if opp == "blue" {
		pos = types.Vec3{X: 2048, Y: kickoffSlotOffset(oppCount), Z: CarRadius}
		yaw = 180.0
	}
	w.state.Cars[botID] = types.CarState{
		PlayerID:    botID,
		DisplayName: "Velocity Bot",
		Team:        opp,
		IsBot:       true,
		Position:    pos,
		Rotation:    types.Rotator{Yaw: yaw},
		Boost:       100,
		IsGrounded:  true,
	}
	w.jump[botID] = &jumpContext{}
	w.state.Events = append(w.state.Events, types.GameplayEvent{
		Type:       "player_join",
		PlayerID:   botID,
		Team:       opp,
		OccurredMS: time.Now().UTC().UnixMilli(),
	})
	return botID
}

// RemoveAllBots removes every bot, used when enough humans are available.
func (w *World) RemoveAllBots() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, c := range w.state.Cars {
		if c.IsBot {
			delete(w.state.Cars, id)
			delete(w.input, id)
			delete(w.jump, id)
			w.state.Events = append(w.state.Events, types.GameplayEvent{
				Type:       "player_leave",
				PlayerID:   id,
				Team:       c.Team,
				OccurredMS: time.Now().UTC().UnixMilli(),
			})
		}
	}
}

// RemovePlayer removes player from simulation state.
func (w *World) RemovePlayer(playerID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.state.Cars[playerID]
	if !ok {
		return
	}
	delete(w.state.Cars, playerID)
	delete(w.input, playerID)
	delete(w.jump, playerID)
	w.state.Events = append(w.state.Events, types.GameplayEvent{
		Type:       "player_leave",
		PlayerID:   playerID,
		Team:       c.Team,
		OccurredMS: time.Now().UTC().UnixMilli(),
	})
}

func (w *World) computeBotInputs() {
	now := time.Now().UTC().UnixMilli()
	for id, car := range w.state.Cars {
		if !car.IsBot {
			continue
		}
		in := botInputFor(car, w.state.Ball)
		in.PlayerID = id
		in.Sequence = w.state.Tick
		in.ClientMS = now
		w.input[id] = in
	}
}

func botInputFor(car types.CarState, ball types.BallState) types.CarInput {
	dx := ball.Position.X - car.Position.X
	dy := ball.Position.Y - car.Position.Y
	dz := ball.Position.Z - car.Position.Z
	dist2D := math.Hypot(dx, dy)

	targetYaw := math.Atan2(dy, dx) * 180 / math.Pi
	delta := normalizeSignedDeg(targetYaw - car.Rotation.Yaw)
	steer := clamp(delta/BotSteerNormalization, -1, 1)

	throttle := 1.0
	if math.Abs(delta) > 120 {
		throttle = -0.25
	}
	boost := math.Abs(delta) < 12 && dist2D > 600 && car.Boost > 15
	handbrake := math.Abs(delta) > 75
	jump := car.IsGrounded && dist2D < 250 && dz > 110

	return types.CarInput{
		Throttle:  throttle,
		Steer:     steer,
		Boost:     boost,
		Jump:      jump,
		Handbrake: handbrake,
	}
}

func (w *World) detectShotOnGoal() {
	b := w.state.Ball
	now := time.Now().UTC().UnixMilli()

	if b.Position.X > ArenaLength*0.35 && b.Velocity.X > 200 && math.Abs(b.Position.Y) <= GoalWidth*0.7 {
		if now-w.lastShotByTeam["orange"] >= 700 {
			w.state.Events = append(w.state.Events, types.GameplayEvent{
				Type:       "shot_on_goal",
				Team:       "orange",
				OccurredMS: now,
			})
			w.lastShotByTeam["orange"] = now
		}
	}
	if b.Position.X < -ArenaLength*0.35 && b.Velocity.X < -200 && math.Abs(b.Position.Y) <= GoalWidth*0.7 {
		if now-w.lastShotByTeam["blue"] >= 700 {
			w.state.Events = append(w.state.Events, types.GameplayEvent{
				Type:       "shot_on_goal",
				Team:       "blue",
				OccurredMS: now,
			})
			w.lastShotByTeam["blue"] = now
		}
	}
}

func (w *World) detectGoalAndResetIfNeeded() {
	b := w.state.Ball
	inGoalY := math.Abs(b.Position.Y) <= GoalWidth/2
	inGoalZ := b.Position.Z <= GoalHeight
	if !inGoalY || !inGoalZ {
		return
	}

	now := time.Now().UTC().UnixMilli()
	if b.Position.X >= ArenaLength/2 {
		w.state.Score.Orange++
		w.state.Events = append(w.state.Events, types.GameplayEvent{Type: "goal", Team: "orange", OccurredMS: now})
		w.resetKickoff("orange")
		return
	}
	if b.Position.X <= -ArenaLength/2 {
		w.state.Score.Blue++
		w.state.Events = append(w.state.Events, types.GameplayEvent{Type: "goal", Team: "blue", OccurredMS: now})
		w.resetKickoff("blue")
	}
}

func (w *World) resetKickoff(scoringTeam string) {
	w.state.Ball.Position = types.Vec3{X: 0, Y: 0, Z: BallRadius + 20}
	w.state.Ball.Velocity = types.Vec3{}

	teamSlots := map[string]int{
		"orange": 0,
		"blue":   0,
	}
	for id, car := range w.state.Cars {
		slot := teamSlots[car.Team]
		teamSlots[car.Team]++
		if car.Team == "orange" {
			car.Position = types.Vec3{X: -2048, Y: kickoffSlotOffset(slot), Z: CarRadius}
			car.Rotation = types.Rotator{Yaw: 0}
		} else {
			car.Position = types.Vec3{X: 2048, Y: kickoffSlotOffset(slot), Z: CarRadius}
			car.Rotation = types.Rotator{Yaw: 180}
		}
		car.Velocity = types.Vec3{}
		car.Boost = 100
		car.IsGrounded = true
		w.state.Cars[id] = car
	}

	w.state.Events = append(w.state.Events, types.GameplayEvent{
		Type:       "kickoff",
		Team:       scoringTeam,
		OccurredMS: time.Now().UTC().UnixMilli(),
	})
}

func clampInput(in types.CarInput) types.CarInput {
	if in.Throttle > 1 {
		in.Throttle = 1
	}
	if in.Throttle < -1 {
		in.Throttle = -1
	}
	if in.Steer > 1 {
		in.Steer = 1
	}
	if in.Steer < -1 {
		in.Steer = -1
	}
	return in
}

func updateCar(car *types.CarState, in types.CarInput, prev types.CarInput, jc *jumpContext, dt float64) {
	yawRad := car.Rotation.Yaw * math.Pi / 180.0

	speed2D := math.Hypot(car.Velocity.X, car.Velocity.Y)
	turnScale := 1.0 - math.Min(speed2D/MaxBoostSpeed, 0.75)
	turnRate := TurnRate * (0.55 + turnScale)
	if in.Handbrake && car.IsGrounded {
		turnRate *= HandbrakeTurnBoost
	}
	car.Rotation.Yaw += in.Steer * turnRate * dt * 180 / math.Pi
	car.Rotation.Yaw = normalizeDeg(car.Rotation.Yaw)
	yawRad = car.Rotation.Yaw * math.Pi / 180.0

	forwardX := math.Cos(yawRad)
	forwardY := math.Sin(yawRad)
	rightX := -forwardY
	rightY := forwardX

	forwardSpeed := car.Velocity.X*forwardX + car.Velocity.Y*forwardY
	lateralSpeed := car.Velocity.X*rightX + car.Velocity.Y*rightY

	accel := 0.0
	if car.IsGrounded {
		accel = in.Throttle * ThrottleAccel
		if in.Throttle*forwardSpeed < 0 {
			accel = in.Throttle * BrakeAccel
		}
	} else {
		if in.Throttle >= 0 {
			accel = in.Throttle * AirThrottleAccel
		} else {
			accel = in.Throttle * AirReverseAccel
		}
	}
	forwardSpeed += accel * dt

	usingBoost := in.Boost && car.Boost > 0
	if usingBoost && in.Throttle > 0 {
		forwardSpeed += BoostAccel * dt
		car.Boost -= 34.0 * dt
		if car.Boost < 0 {
			car.Boost = 0
		}
	} else {
		car.Boost += 8.0 * dt
		if car.Boost > 100 {
			car.Boost = 100
		}
	}

	if math.Abs(in.Throttle) < 0.05 && car.IsGrounded {
		forwardSpeed *= CoastFriction
	}

	maxSpeed := MaxCarSpeed
	if !usingBoost {
		maxSpeed = MaxDriveSpeed
	}
	forwardSpeed = clamp(forwardSpeed, -MaxCarSpeed, maxSpeed)

	grip := LateralGrip
	if in.Handbrake && car.IsGrounded {
		grip = HandbrakeGrip
	}
	if car.IsGrounded {
		lateralSpeed *= grip
	} else {
		lateralSpeed *= 0.985
	}

	car.Velocity.X = forwardX*forwardSpeed + rightX*lateralSpeed
	car.Velocity.Y = forwardY*forwardSpeed + rightY*lateralSpeed

	jumpPressed := in.Jump && !prev.Jump
	didFirstJump := false
	if car.IsGrounded {
		jc.usedJumps = 0
		jc.timeSinceJump = 0
		jc.holdTime = 0
		jc.stickyTime = 0
	}
	if jumpPressed && jc.usedJumps == 0 && car.IsGrounded {
		car.Velocity.Z += JumpVelocity
		car.IsGrounded = false
		jc.usedJumps = 1
		jc.timeSinceJump = 0
		jc.holdTime = 0
		jc.stickyTime = StickyTime
		didFirstJump = true
	}
	if jc.usedJumps > 0 && !car.IsGrounded {
		jc.timeSinceJump += dt
		if in.Jump && jc.holdTime < JumpHoldMax && jc.usedJumps == 1 {
			car.Velocity.Z += JumpHoldAccel * dt
			jc.holdTime += dt
		}
		if jc.stickyTime > 0 {
			car.Velocity.Z -= StickyForce * dt
			jc.stickyTime -= dt
		}
		if jumpPressed && !didFirstJump && jc.usedJumps == 1 && jc.timeSinceJump <= DoubleJumpMax {
			car.Velocity.Z += JumpVelocity
			dodgeX := forwardX*in.Throttle + rightX*in.Steer
			dodgeY := forwardY*in.Throttle + rightY*in.Steer
			mag := math.Hypot(dodgeX, dodgeY)
			if mag < 0.1 {
				dodgeX = forwardX
				dodgeY = forwardY
				mag = 1
			}
			dodgeX /= mag
			dodgeY /= mag
			car.Velocity.X += dodgeX * 500
			car.Velocity.Y += dodgeY * 500
			jc.usedJumps = 2
			jc.holdTime = JumpHoldMax
			jc.stickyTime = 0
		}
	}

	car.Velocity.Z += Gravity * dt
	if car.IsGrounded {
		car.Velocity.X *= GroundFriction
		car.Velocity.Y *= GroundFriction
	} else {
		car.Velocity.X *= AirResistance
		car.Velocity.Y *= AirResistance
	}

	car.Position.X += car.Velocity.X * dt
	car.Position.Y += car.Velocity.Y * dt
	car.Position.Z += car.Velocity.Z * dt

	if car.Position.Z <= CarRadius {
		car.Position.Z = CarRadius
		if car.Velocity.Z < 0 {
			car.Velocity.Z = 0
		}
		car.IsGrounded = true
		jc.stickyTime = 0
	}
	if car.Position.Z > ArenaHeight-CarRadius {
		car.Position.Z = ArenaHeight - CarRadius
		car.Velocity.Z *= -0.25
	}
}

func clampCarBounds(car *types.CarState) {
	halfL := ArenaLength / 2
	halfW := ArenaWidth / 2

	if car.Position.X < -halfL+CarRadius {
		car.Position.X = -halfL + CarRadius
		car.Velocity.X *= -0.3
	}
	if car.Position.X > halfL-CarRadius {
		car.Position.X = halfL - CarRadius
		car.Velocity.X *= -0.3
	}
	if car.Position.Y < -halfW+CarRadius {
		car.Position.Y = -halfW + CarRadius
		car.Velocity.Y *= -0.3
	}
	if car.Position.Y > halfW-CarRadius {
		car.Position.Y = halfW - CarRadius
		car.Velocity.Y *= -0.3
	}
}

func updateBall(ball *types.BallState, dt float64) {
	ball.Velocity.Z += Gravity * dt
	ball.Position.X += ball.Velocity.X * dt
	ball.Position.Y += ball.Velocity.Y * dt
	ball.Position.Z += ball.Velocity.Z * dt

	if ball.Position.Z <= ball.Radius+8 {
		ball.Velocity.X *= 0.9975
		ball.Velocity.Y *= 0.9975
	} else {
		ball.Velocity.X *= 0.9995
		ball.Velocity.Y *= 0.9995
	}
	ball.Velocity.Z *= 0.9994

	speed := math.Sqrt(ball.Velocity.X*ball.Velocity.X + ball.Velocity.Y*ball.Velocity.Y + ball.Velocity.Z*ball.Velocity.Z)
	if speed > BallMaxSpeed && speed > 0 {
		scale := BallMaxSpeed / speed
		ball.Velocity.X *= scale
		ball.Velocity.Y *= scale
		ball.Velocity.Z *= scale
	}
}

func clampBallBounds(ball *types.BallState) {
	halfL := ArenaLength / 2
	halfW := ArenaWidth / 2

	if ball.Position.Z < ball.Radius {
		ball.Position.Z = ball.Radius
		ball.Velocity.Z = -ball.Velocity.Z * BallRestitution
	}
	if ball.Position.Z > ArenaHeight-ball.Radius {
		ball.Position.Z = ArenaHeight - ball.Radius
		ball.Velocity.Z = -ball.Velocity.Z * BallRestitution
	}

	inGoalY := math.Abs(ball.Position.Y) <= GoalWidth/2
	inGoalZ := ball.Position.Z <= GoalHeight

	// Allow goal tunnel entry by not reflecting on X if in mouth.
	if !inGoalY || !inGoalZ {
		if ball.Position.X < -halfL+ball.Radius {
			ball.Position.X = -halfL + ball.Radius
			ball.Velocity.X = -ball.Velocity.X * WallRestitution
		}
		if ball.Position.X > halfL-ball.Radius {
			ball.Position.X = halfL - ball.Radius
			ball.Velocity.X = -ball.Velocity.X * WallRestitution
		}
	}
	if ball.Position.Y < -halfW+ball.Radius {
		ball.Position.Y = -halfW + ball.Radius
		ball.Velocity.Y = -ball.Velocity.Y * WallRestitution
	}
	if ball.Position.Y > halfW-ball.Radius {
		ball.Position.Y = halfW - ball.Radius
		ball.Velocity.Y = -ball.Velocity.Y * WallRestitution
	}
}

func resolveCarBallCollisions(state *types.MatchState) {
	for id, car := range state.Cars {
		dx := state.Ball.Position.X - car.Position.X
		dy := state.Ball.Position.Y - car.Position.Y
		dz := state.Ball.Position.Z - car.Position.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		minDist := CarRadius + state.Ball.Radius
		if dist <= 0 || dist >= minDist {
			continue
		}

		nx := dx / dist
		ny := dy / dist
		nz := dz / dist

		carDot := car.Velocity.X*nx + car.Velocity.Y*ny + car.Velocity.Z*nz
		ballDot := state.Ball.Velocity.X*nx + state.Ball.Velocity.Y*ny + state.Ball.Velocity.Z*nz
		rel := ballDot - carDot
		if rel > 0 {
			continue
		}

		impulse := -(1.0 + CarBallElasticity) * rel
		state.Ball.Velocity.X += impulse * nx
		state.Ball.Velocity.Y += impulse * ny
		state.Ball.Velocity.Z += impulse * nz

		overlap := minDist - dist
		state.Ball.Position.X += nx * overlap * 0.85
		state.Ball.Position.Y += ny * overlap * 0.85
		state.Ball.Position.Z += nz * overlap * 0.85

		car.Position.X -= nx * overlap * 0.15
		car.Position.Y -= ny * overlap * 0.15
		car.Position.Z -= nz * overlap * 0.15

		state.Cars[id] = car
	}
}

func normalizeDeg(d float64) float64 {
	for d >= 360 {
		d -= 360
	}
	for d < 0 {
		d += 360
	}
	return d
}

func normalizeSignedDeg(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d < -180 {
		d += 360
	}
	return d
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func kickoffSlotOffset(slot int) float64 {
	offsets := []float64{0, -500, 500}
	return offsets[slot%len(offsets)]
}
