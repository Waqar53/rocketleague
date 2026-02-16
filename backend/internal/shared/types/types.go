package types

import "time"

// Vec3 represents a position or vector in world space.
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Rotator stores orientation in degrees.
type Rotator struct {
	Pitch float64 `json:"pitch"`
	Yaw   float64 `json:"yaw"`
	Roll  float64 `json:"roll"`
}

// CarInput is the per-tick player control input.
type CarInput struct {
	PlayerID  string  `json:"player_id"`
	Sequence  uint64  `json:"sequence"`
	Throttle  float64 `json:"throttle"` // -1..1
	Steer     float64 `json:"steer"`    // -1..1
	Boost     bool    `json:"boost"`
	Jump      bool    `json:"jump"`
	Handbrake bool    `json:"handbrake"`
	ClientMS  int64   `json:"client_ms"`
}

// CarState is the authoritative replicated state for a car.
type CarState struct {
	PlayerID    string   `json:"player_id"`
	DisplayName string   `json:"display_name"`
	Team        string   `json:"team"` // orange|blue
	IsBot       bool     `json:"is_bot"`
	Position    Vec3     `json:"position"`
	Velocity    Vec3     `json:"velocity"`
	Rotation    Rotator  `json:"rotation"`
	Boost       float64  `json:"boost"`
	IsGrounded  bool     `json:"is_grounded"`
	LastInput   CarInput `json:"last_input"`
}

// BallState is the authoritative state for the ball.
type BallState struct {
	Position Vec3    `json:"position"`
	Velocity Vec3    `json:"velocity"`
	Radius   float64 `json:"radius"`
}

// ScoreState tracks goals and timer.
type ScoreState struct {
	Orange          int `json:"orange"`
	Blue            int `json:"blue"`
	TimeRemainingMS int `json:"time_remaining_ms"`
}

// MatchState is replicated to all clients.
type MatchState struct {
	MatchID   string              `json:"match_id"`
	Tick      uint64              `json:"tick"`
	CreatedAt time.Time           `json:"created_at"`
	Cars      map[string]CarState `json:"cars"`
	Ball      BallState           `json:"ball"`
	Score     ScoreState          `json:"score"`
	Events    []GameplayEvent     `json:"events"`
}

// GameplayEvent tracks state changes worth UI/audio feedback.
type GameplayEvent struct {
	Type       string `json:"type"` // goal|save|shot_on_goal|demo|kickoff
	PlayerID   string `json:"player_id,omitempty"`
	Team       string `json:"team,omitempty"`
	OccurredMS int64  `json:"occurred_ms"`
}

// ClientEnvelope is sent from client to server.
type ClientEnvelope struct {
	Type  string    `json:"type"` // hello|input|ping
	Input *CarInput `json:"input,omitempty"`
}

// ServerEnvelope is sent from server to client.
type ServerEnvelope struct {
	Type     string      `json:"type"` // welcome|state|pong|error
	Tick     uint64      `json:"tick,omitempty"`
	State    *MatchState `json:"state,omitempty"`
	ServerMS int64       `json:"server_ms,omitempty"`
	Message  string      `json:"message,omitempty"`
	AckSeq   uint64      `json:"ack_seq,omitempty"`
}

// QueueJoinRequest requests matchmaking entry.
type QueueJoinRequest struct {
	PlayerID    string `json:"player_id"`
	DisplayName string `json:"display_name"`
	Region      string `json:"region"`
	Playlist    string `json:"playlist"`
	MMR         int    `json:"mmr"`
}

// QueueJoinResponse returns a ticket for polling.
type QueueJoinResponse struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status"`
}

// MatchAssignment is returned once a ticket is matched.
type MatchAssignment struct {
	TicketID    string   `json:"ticket_id"`
	MatchID     string   `json:"match_id"`
	Region      string   `json:"region"`
	Playlist    string   `json:"playlist"`
	Players     []string `json:"players"`
	BotFill     bool     `json:"bot_fill"`
	ServerAddr  string   `json:"server_addr"`
	FoundAtUnix int64    `json:"found_at_unix"`
}

// QueuePollResponse represents current matchmaking status.
type QueuePollResponse struct {
	TicketID   string           `json:"ticket_id"`
	Status     string           `json:"status"` // searching|matched|not_found
	Assignment *MatchAssignment `json:"assignment,omitempty"`
}

// GuestAuthRequest requests a guest player token.
type GuestAuthRequest struct {
	DisplayName string `json:"display_name"`
}

// GuestAuthResponse returns signed auth details.
type GuestAuthResponse struct {
	PlayerID    string `json:"player_id"`
	DisplayName string `json:"display_name"`
	Token       string `json:"token"`
	ExpiresAt   int64  `json:"expires_at"`
}

// TelemetryEvent represents a gameplay/platform event.
type TelemetryEvent struct {
	EventID   string                 `json:"event_id"`
	EventType string                 `json:"event_type"`
	MatchID   string                 `json:"match_id,omitempty"`
	PlayerID  string                 `json:"player_id,omitempty"`
	Timestamp int64                  `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}
