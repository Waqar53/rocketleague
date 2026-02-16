package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"projectvelocity/backend/internal/shared/logger"
	"projectvelocity/backend/internal/shared/types"
)

type telemetryStore struct {
	mu          sync.RWMutex
	recent      []types.TelemetryEvent
	totalIngest int64
	byType      map[string]int64
}

func main() {
	log := logger.New("telemetry")
	addr := getenv("TELEMETRY_ADDR", ":9002")
	store := &telemetryStore{
		recent: make([]types.TelemetryEvent, 0, 512),
		byType: make(map[string]int64),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var ev types.TelemetryEvent
			if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
				return
			}
			if ev.EventType == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_type_required"})
				return
			}
			if ev.EventID == "" {
				ev.EventID = fmt.Sprintf("ev_%d", time.Now().UTC().UnixNano())
			}
			if ev.Timestamp == 0 {
				ev.Timestamp = time.Now().UTC().UnixMilli()
			}
			store.ingest(ev)
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "event_id": ev.EventID})
		case http.MethodGet:
			limit := 100
			recent := store.listRecent(limit)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"count":  len(recent),
				"events": recent,
			})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		}
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		summary := store.summary()
		_, _ = fmt.Fprintln(w, "# HELP velocity_telemetry_events_total Total telemetry events ingested")
		_, _ = fmt.Fprintln(w, "# TYPE velocity_telemetry_events_total counter")
		_, _ = fmt.Fprintf(w, "velocity_telemetry_events_total %d\n", summary.Total)
		for typ, count := range summary.ByType {
			_, _ = fmt.Fprintf(w, "velocity_telemetry_events_by_type{event_type=\"%s\"} %d\n", typ, count)
		}
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("telemetry listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

type summary struct {
	Total  int64
	ByType map[string]int64
}

func (s *telemetryStore) ingest(ev types.TelemetryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalIngest++
	s.byType[ev.EventType]++
	s.recent = append(s.recent, ev)
	if len(s.recent) > 1000 {
		s.recent = s.recent[len(s.recent)-1000:]
	}
}

func (s *telemetryStore) listRecent(limit int) []types.TelemetryEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.recent) {
		limit = len(s.recent)
	}
	out := make([]types.TelemetryEvent, limit)
	copy(out, s.recent[len(s.recent)-limit:])
	return out
}

func (s *telemetryStore) summary() summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byType := make(map[string]int64, len(s.byType))
	for k, v := range s.byType {
		byType[k] = v
	}
	return summary{Total: s.totalIngest, ByType: byType}
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
