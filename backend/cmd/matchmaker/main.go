package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"projectvelocity/backend/internal/matchmaking"
	"projectvelocity/backend/internal/shared/logger"
	"projectvelocity/backend/internal/shared/types"
)

func main() {
	log := logger.New("matchmaker")
	addr := getenv("MATCHMAKER_ADDR", ":9001")
	serverAddr := getenv("GAME_WS_ADDR", "ws://localhost:9003/ws")

	manager := matchmaking.NewQueueManager(serverAddr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Run(ctx, time.Second, 2)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/v1/queue/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var req types.QueueJoinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		if req.PlayerID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "player_id_required"})
			return
		}
		if req.Region == "" {
			req.Region = "us-east"
		}
		if req.Playlist == "" {
			req.Playlist = "ranked-1v1"
		}
		if req.MMR <= 0 {
			req.MMR = 1000
		}

		resp := manager.Join(req)
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("/v1/queue/poll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		ticketID := r.URL.Query().Get("ticket_id")
		if ticketID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket_id_required"})
			return
		}
		writeJSON(w, http.StatusOK, manager.Poll(ticketID))
	})
	mux.HandleFunc("/v1/queue/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var body struct {
			TicketID string `json:"ticket_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		if body.TicketID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket_id_required"})
			return
		}
		if !manager.Leave(body.TicketID) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "ticket_not_found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("matchmaker listening on %s (game server=%s)", addr, serverAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}
