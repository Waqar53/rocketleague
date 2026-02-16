package simulation

import (
	"math"
	"testing"
	"time"

	"projectvelocity/backend/internal/shared/types"
)

func TestTickDecreasesTimer(t *testing.T) {
	w := NewWorld("m1", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})
	before := w.Snapshot().Score.TimeRemainingMS
	w.Tick(1.0 / 120.0)
	after := w.Snapshot().Score.TimeRemainingMS
	if after >= before {
		t.Fatalf("expected timer to decrease, before=%d after=%d", before, after)
	}
}

func TestBoostConsumptionAndRegeneration(t *testing.T) {
	w := NewWorld("m2", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})
	w.ApplyInput(types.CarInput{PlayerID: "p1", Throttle: 1, Boost: true})
	for range 120 {
		w.Tick(1.0 / 120.0)
	}
	postBoost := w.Snapshot().Cars["p1"].Boost
	if postBoost >= 100 {
		t.Fatalf("expected boost to be consumed, got=%f", postBoost)
	}

	w.ApplyInput(types.CarInput{PlayerID: "p1", Throttle: 0, Boost: false})
	for range 240 {
		w.Tick(1.0 / 120.0)
	}
	regen := w.Snapshot().Cars["p1"].Boost
	if regen <= postBoost {
		t.Fatalf("expected boost to regenerate, before=%f after=%f", postBoost, regen)
	}
}

func TestGoalScoringIncrementsScore(t *testing.T) {
	w := NewWorld("m3", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})
	state := w.Snapshot()
	state.Ball.Position.X = ArenaLength/2 + 5
	state.Ball.Position.Y = 0
	state.Ball.Position.Z = 100

	w.mu.Lock()
	w.state.Ball = state.Ball
	w.mu.Unlock()

	w.Tick(1.0 / 120.0)
	s := w.Snapshot()
	if s.Score.Orange != 1 {
		t.Fatalf("expected orange score to increment, got=%d", s.Score.Orange)
	}
	if s.Ball.Position.X != 0 || s.Ball.Position.Y != 0 {
		t.Fatalf("expected ball reset to kickoff, got=%+v", s.Ball.Position)
	}
}

func TestSnapshotIsDeepCopy(t *testing.T) {
	w := NewWorld("m4", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})
	snap := w.Snapshot()
	car := snap.Cars["p1"]
	car.Position.X = 999999
	snap.Cars["p1"] = car

	snap2 := w.Snapshot()
	if snap2.Cars["p1"].Position.X == 999999 {
		t.Fatal("world state mutated through snapshot")
	}
}

func TestBotLifecycleForSingleHuman(t *testing.T) {
	w := NewWorld("m5", 10*time.Second, nil)
	w.EnsurePlayer("p1", "Pilot1")
	if count := w.HumanCount(); count != 1 {
		t.Fatalf("expected 1 human, got=%d", count)
	}
	botID := w.EnsureBotOpponent("p1")
	if botID == "" {
		t.Fatal("expected bot opponent to be added")
	}
	s := w.Snapshot()
	bot, ok := s.Cars[botID]
	if !ok || !bot.IsBot {
		t.Fatal("expected bot state in snapshot")
	}
	if bot.Team == s.Cars["p1"].Team {
		t.Fatal("expected bot on opposing team")
	}

	w.RemoveAllBots()
	s2 := w.Snapshot()
	for _, c := range s2.Cars {
		if c.IsBot {
			t.Fatal("expected all bots removed")
		}
	}
}

func TestRemovePlayer(t *testing.T) {
	w := NewWorld("m6", 10*time.Second, nil)
	w.EnsurePlayer("p1", "Pilot1")
	w.RemovePlayer("p1")
	if _, ok := w.Snapshot().Cars["p1"]; ok {
		t.Fatal("expected player removed from world")
	}
}

func TestForwardAccelerationIsResponsive(t *testing.T) {
	w := NewWorld("m7", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})
	w.ApplyInput(types.CarInput{PlayerID: "p1", Throttle: 1})
	for range 120 {
		w.Tick(1.0 / 120.0)
	}
	car := w.Snapshot().Cars["p1"]
	speed := math.Hypot(car.Velocity.X, car.Velocity.Y)
	if speed < 1200 {
		t.Fatalf("expected responsive acceleration, got speed=%f", speed)
	}
}

func TestCarBallCollisionTransfersMomentum(t *testing.T) {
	w := NewWorld("m8", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})
	w.mu.Lock()
	car := w.state.Cars["p1"]
	car.Position = types.Vec3{X: -200, Y: 0, Z: CarRadius}
	car.Velocity = types.Vec3{X: 2300, Y: 0, Z: 0}
	w.state.Cars["p1"] = car
	w.state.Ball.Position = types.Vec3{X: -20, Y: 0, Z: BallRadius}
	w.state.Ball.Velocity = types.Vec3{}
	w.mu.Unlock()

	for range 30 {
		w.Tick(1.0 / 120.0)
	}
	ball := w.Snapshot().Ball
	if ball.Velocity.X < 200 {
		t.Fatalf("expected ball to gain velocity from hit, got=%f", ball.Velocity.X)
	}
}

func TestJumpAndDoubleJumpWork(t *testing.T) {
	w := NewWorld("m9", 10*time.Second, []PlayerSpawn{{PlayerID: "p1", DisplayName: "p1", Team: "orange"}})

	w.ApplyInput(types.CarInput{PlayerID: "p1", Jump: true})
	w.Tick(1.0 / 120.0)
	afterFirst := w.Snapshot().Cars["p1"]
	if afterFirst.Velocity.Z <= 0 {
		t.Fatalf("expected upward velocity after first jump, got=%f", afterFirst.Velocity.Z)
	}

	// Release then press again to trigger double jump edge.
	w.ApplyInput(types.CarInput{PlayerID: "p1", Jump: false})
	for range 8 {
		w.Tick(1.0 / 120.0)
	}
	velBeforeSecond := w.Snapshot().Cars["p1"].Velocity.Z

	w.ApplyInput(types.CarInput{PlayerID: "p1", Jump: true, Throttle: 1})
	w.Tick(1.0 / 120.0)
	afterSecond := w.Snapshot().Cars["p1"]
	if afterSecond.Velocity.Z <= velBeforeSecond {
		t.Fatalf("expected double jump to increase vertical speed, before=%f after=%f", velBeforeSecond, afterSecond.Velocity.Z)
	}
}
