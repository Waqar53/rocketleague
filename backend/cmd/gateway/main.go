package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"projectvelocity/backend/internal/shared/logger"
	"projectvelocity/backend/internal/shared/types"
)

type authSession struct {
	PlayerID    string
	DisplayName string
	ExpiresAt   int64
}

type gateway struct {
	log          *logger.Logger
	matchmaker   string
	httpClient   *http.Client
	authMu       sync.RWMutex
	authSessions map[string]authSession
}

func main() {
	log := logger.New("gateway")
	addr := getenv("GATEWAY_ADDR", ":9000")
	matchmakerURL := getenv("MATCHMAKER_HTTP", "http://localhost:9001")

	g := &gateway{
		log:          log,
		matchmaker:   matchmakerURL,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		authSessions: make(map[string]authSession),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", g.handleHealth)
	mux.HandleFunc("/v1/auth/guest", g.handleGuestAuth)
	mux.HandleFunc("/v1/matchmaking/join", g.handleMatchJoin)
	mux.HandleFunc("/v1/matchmaking/poll", g.handleMatchPoll)
	mux.HandleFunc("/v1/matchmaking/leave", g.handleMatchLeave)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("gateway listening on %s (matchmaker=%s)", addr, matchmakerURL)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func (g *gateway) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *gateway) handleGuestAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}

	var req types.GuestAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = "pilot"
	}

	playerID := fmt.Sprintf("player_%d", time.Now().UTC().UnixNano())
	token, err := randomToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_generation_failed"})
		return
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Unix()

	g.authMu.Lock()
	g.authSessions[token] = authSession{PlayerID: playerID, DisplayName: req.DisplayName, ExpiresAt: expiresAt}
	g.authMu.Unlock()

	resp := types.GuestAuthResponse{
		PlayerID:    playerID,
		DisplayName: req.DisplayName,
		Token:       token,
		ExpiresAt:   expiresAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (g *gateway) handleMatchJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}

	session, ok := g.validateAuth(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_auth"})
		return
	}

	var req types.QueueJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	req.PlayerID = session.PlayerID
	req.DisplayName = session.DisplayName
	if req.Region == "" {
		req.Region = "us-east"
	}
	if req.Playlist == "" {
		req.Playlist = "ranked-1v1"
	}
	if req.MMR <= 0 {
		req.MMR = 1000
	}

	buf, _ := json.Marshal(req)
	code, body, err := g.proxyRequest(http.MethodPost, g.matchmaker+"/v1/queue/join", bytes.NewReader(buf))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "matchmaker_unavailable"})
		return
	}
	writeRawJSON(w, code, body)
}

func (g *gateway) handleMatchPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if _, ok := g.validateAuth(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_auth"})
		return
	}

	ticketID := r.URL.Query().Get("ticket_id")
	if ticketID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket_id_required"})
		return
	}

	url := g.matchmaker + "/v1/queue/poll?ticket_id=" + ticketID
	code, body, err := g.proxyRequest(http.MethodGet, url, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "matchmaker_unavailable"})
		return
	}
	writeRawJSON(w, code, body)
}

func (g *gateway) handleMatchLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if _, ok := g.validateAuth(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_auth"})
		return
	}

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	buf, _ := json.Marshal(body)
	code, out, err := g.proxyRequest(http.MethodPost, g.matchmaker+"/v1/queue/leave", bytes.NewReader(buf))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "matchmaker_unavailable"})
		return
	}
	writeRawJSON(w, code, out)
}

func (g *gateway) validateAuth(r *http.Request) (authSession, bool) {
	token := r.Header.Get("Authorization")
	if token == "" {
		return authSession{}, false
	}
	const prefix = "Bearer "
	if len(token) <= len(prefix) || token[:len(prefix)] != prefix {
		return authSession{}, false
	}
	token = token[len(prefix):]

	g.authMu.RLock()
	defer g.authMu.RUnlock()
	session, ok := g.authSessions[token]
	if !ok {
		return authSession{}, false
	}
	if session.ExpiresAt < time.Now().UTC().Unix() {
		return authSession{}, false
	}
	return session, true
}

func (g *gateway) proxyRequest(method, url string, body io.Reader) (int, []byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, payload, nil
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
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

func writeRawJSON(w http.ResponseWriter, code int, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(payload)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
