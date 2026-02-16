package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"projectvelocity/backend/internal/shared/logger"
	"projectvelocity/backend/internal/shared/types"
	"projectvelocity/backend/internal/simulation"
)

type client struct {
	playerID string
	conn     *websocket.Conn
	send     chan []byte
}

type server struct {
	log      *logger.Logger
	world    *simulation.World
	upgrader websocket.Upgrader

	mu      sync.RWMutex
	clients map[string]*client
}

func main() {
	log := logger.New("gameserver")
	addr := getEnv("GAME_ADDR", ":9003")
	matchID := getEnv("MATCH_ID", fmt.Sprintf("local_%d", time.Now().UTC().Unix()))
	durationSec := getEnvInt("MATCH_DURATION_SEC", 300)

	s := &server{
		log:   log,
		world: simulation.NewWorld(matchID, time.Duration(durationSec)*time.Second, nil),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients: make(map[string]*client),
	}

	go s.runSimulationLoop()
	go s.runReplicationLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ws", s.handleWS)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("authoritative game server listening on %s (match=%s)", addr, matchID)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("player_id")
	if playerID == "" {
		playerID = fmt.Sprintf("guest_%d", time.Now().UTC().UnixNano())
	}
	displayName := r.URL.Query().Get("display_name")
	if displayName == "" {
		displayName = playerID
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Printf("websocket upgrade error: %v", err)
		return
	}

	team := s.world.EnsurePlayer(playerID, displayName)
	s.maintainBotBalance(playerID)
	c := &client{playerID: playerID, conn: conn, send: make(chan []byte, 64)}
	s.register(c)

	s.log.Printf("client connected player=%s team=%s remote=%s", playerID, team, r.RemoteAddr)
	welcome := types.ServerEnvelope{
		Type:     "welcome",
		State:    ptrState(s.world.Snapshot()),
		ServerMS: time.Now().UTC().UnixMilli(),
		Message:  "connected",
	}
	if payload, err := json.Marshal(welcome); err == nil {
		select {
		case c.send <- payload:
		default:
		}
	}

	go s.writePump(c)
	s.readPump(c)
}

func (s *server) readPump(c *client) {
	defer func() {
		s.unregister(c.playerID)
		_ = c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.log.Printf("client disconnected player=%s", c.playerID)
				return
			}
			s.log.Printf("read error player=%s err=%v", c.playerID, err)
			return
		}

		var in types.ClientEnvelope
		if err := json.Unmarshal(msg, &in); err != nil {
			s.sendError(c, "bad_payload")
			continue
		}

		switch in.Type {
		case "input":
			if in.Input == nil {
				s.sendError(c, "missing_input")
				continue
			}
			in.Input.PlayerID = c.playerID
			s.world.ApplyInput(*in.Input)
		case "ping":
			pong := types.ServerEnvelope{Type: "pong", ServerMS: time.Now().UTC().UnixMilli()}
			if payload, err := json.Marshal(pong); err == nil {
				select {
				case c.send <- payload:
				default:
				}
			}
		default:
			s.sendError(c, "unsupported_message_type")
		}
	}
}

func (s *server) writePump(c *client) {
	ticker := time.NewTicker(20 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte("keepalive")); err != nil {
				return
			}
		}
	}
}

func (s *server) register(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c.playerID] = c
}

func (s *server) unregister(playerID string) {
	s.mu.Lock()
	if c, ok := s.clients[playerID]; ok {
		close(c.send)
		delete(s.clients, playerID)
	}
	s.mu.Unlock()

	s.world.RemovePlayer(playerID)
	s.maintainBotBalance("")
}

func (s *server) sendError(c *client, message string) {
	errPayload, _ := json.Marshal(types.ServerEnvelope{
		Type:    "error",
		Message: message,
	})
	select {
	case c.send <- errPayload:
	default:
	}
}

func (s *server) runSimulationLoop() {
	ticker := time.NewTicker(time.Second / 120)
	defer ticker.Stop()
	dt := 1.0 / 120.0

	for range ticker.C {
		s.world.Tick(dt)
	}
}

func (s *server) runReplicationLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	for range ticker.C {
		state := s.world.Snapshot()
		env := types.ServerEnvelope{
			Type:     "state",
			Tick:     state.Tick,
			State:    &state,
			ServerMS: time.Now().UTC().UnixMilli(),
		}
		payload, err := json.Marshal(env)
		if err != nil {
			s.log.Printf("marshal state failed: %v", err)
			continue
		}

		s.mu.RLock()
		for _, c := range s.clients {
			select {
			case c.send <- payload:
			default:
			}
		}
		s.mu.RUnlock()
	}
}

func (s *server) maintainBotBalance(preferredHuman string) {
	humans := s.world.HumanCount()
	switch {
	case humans <= 0:
		s.world.RemoveAllBots()
	case humans == 1:
		playerID := preferredHuman
		if playerID == "" {
			playerID = s.world.FirstHumanID()
		}
		if playerID != "" {
			s.world.EnsureBotOpponent(playerID)
		}
	default:
		s.world.RemoveAllBots()
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func ptrState(s types.MatchState) *types.MatchState {
	return &s
}
