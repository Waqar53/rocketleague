package matchmaking

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"projectvelocity/backend/internal/shared/types"
)

// Ticket is a queue entry.
type Ticket struct {
	TicketID    string
	PlayerID    string
	DisplayName string
	MMR         int
	Region      string
	Playlist    string
	JoinedAt    time.Time
	Status      string // searching|matched|cancelled
}

// QueueManager provides in-memory matchmaking for local and staging usage.
type QueueManager struct {
	mu          sync.RWMutex
	buckets     map[string][]*Ticket
	ticketIndex map[string]*Ticket
	assignment  map[string]*types.MatchAssignment
	serverAddr  string
}

func NewQueueManager(serverAddr string) *QueueManager {
	if serverAddr == "" {
		serverAddr = "ws://localhost:9003/ws"
	}
	return &QueueManager{
		buckets:     make(map[string][]*Ticket),
		ticketIndex: make(map[string]*Ticket),
		assignment:  make(map[string]*types.MatchAssignment),
		serverAddr:  serverAddr,
	}
}

func bucketKey(region, playlist string) string {
	if region == "" {
		region = "global"
	}
	if playlist == "" {
		playlist = "ranked-1v1"
	}
	return region + "|" + playlist
}

func nextID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

// Join adds player to queue.
func (q *QueueManager) Join(req types.QueueJoinRequest) types.QueueJoinResponse {
	now := time.Now().UTC()
	ticket := &Ticket{
		TicketID:    nextID("t"),
		PlayerID:    req.PlayerID,
		DisplayName: req.DisplayName,
		MMR:         req.MMR,
		Region:      req.Region,
		Playlist:    req.Playlist,
		JoinedAt:    now,
		Status:      "searching",
	}
	key := bucketKey(req.Region, req.Playlist)

	q.mu.Lock()
	defer q.mu.Unlock()
	q.buckets[key] = append(q.buckets[key], ticket)
	q.ticketIndex[ticket.TicketID] = ticket

	return types.QueueJoinResponse{TicketID: ticket.TicketID, Status: ticket.Status}
}

// Leave removes ticket from queue.
func (q *QueueManager) Leave(ticketID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.ticketIndex[ticketID]
	if !ok {
		return false
	}

	key := bucketKey(t.Region, t.Playlist)
	bucket := q.buckets[key]
	for i := range bucket {
		if bucket[i].TicketID == ticketID {
			bucket = append(bucket[:i], bucket[i+1:]...)
			break
		}
	}
	q.buckets[key] = bucket
	t.Status = "cancelled"
	delete(q.ticketIndex, ticketID)
	delete(q.assignment, ticketID)
	return true
}

// Poll returns current ticket status and assignment if available.
func (q *QueueManager) Poll(ticketID string) types.QueuePollResponse {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if a, ok := q.assignment[ticketID]; ok {
		copyA := *a
		return types.QueuePollResponse{TicketID: ticketID, Status: "matched", Assignment: &copyA}
	}

	t, ok := q.ticketIndex[ticketID]
	if !ok {
		return types.QueuePollResponse{TicketID: ticketID, Status: "not_found"}
	}
	return types.QueuePollResponse{TicketID: ticketID, Status: t.Status}
}

// Run continuously evaluates queue and creates matches.
func (q *QueueManager) Run(ctx context.Context, cadence time.Duration, playersPerMatch int) {
	if cadence <= 0 {
		cadence = time.Second
	}
	if playersPerMatch < 2 {
		playersPerMatch = 2
	}

	ticker := time.NewTicker(cadence)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.process(playersPerMatch)
		}
	}
}

func (q *QueueManager) process(playersPerMatch int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now().UTC()
	for key, bucket := range q.buckets {
		sort.SliceStable(bucket, func(i, j int) bool {
			return bucket[i].JoinedAt.Before(bucket[j].JoinedAt)
		})

		used := make(map[int]bool)
		if len(bucket) >= playersPerMatch {
			for i := 0; i < len(bucket)-1; i++ {
				if used[i] {
					continue
				}
				a := bucket[i]
				if a.Status != "searching" {
					continue
				}

				best := -1
				bestDiff := math.MaxInt32
				for j := i + 1; j < len(bucket); j++ {
					if used[j] {
						continue
					}
					b := bucket[j]
					if b.Status != "searching" {
						continue
					}
					diff := abs(a.MMR - b.MMR)
					threshold := q.allowedMMRDiff(a, b, now)
					if diff <= threshold && diff < bestDiff {
						best = j
						bestDiff = diff
					}
				}

				if best == -1 {
					continue
				}

				b := bucket[best]
				used[i] = true
				used[best] = true

				players := []string{a.PlayerID, b.PlayerID}
				region, playlist := splitKey(key)
				assignment := &types.MatchAssignment{
					TicketID:    a.TicketID,
					MatchID:     nextID("m"),
					Region:      region,
					Playlist:    playlist,
					Players:     players,
					BotFill:     false,
					ServerAddr:  q.serverAddr,
					FoundAtUnix: now.Unix(),
				}
				assignment2 := *assignment
				assignment2.TicketID = b.TicketID

				a.Status = "matched"
				b.Status = "matched"
				q.assignment[a.TicketID] = assignment
				q.assignment[b.TicketID] = &assignment2
			}
		}

		region, playlist := splitKey(key)
		remaining := make([]*Ticket, 0, len(bucket))
		for i, t := range bucket {
			if used[i] || t.Status != "searching" {
				continue
			}
			if now.Sub(t.JoinedAt) >= 4*time.Second {
				t.Status = "matched"
				q.assignment[t.TicketID] = &types.MatchAssignment{
					TicketID:    t.TicketID,
					MatchID:     nextID("m"),
					Region:      region,
					Playlist:    playlist,
					Players:     []string{t.PlayerID, "bot"},
					BotFill:     true,
					ServerAddr:  q.serverAddr,
					FoundAtUnix: now.Unix(),
				}
				continue
			}
			remaining = append(remaining, t)
		}
		q.buckets[key] = remaining
	}
}

func splitKey(key string) (region, playlist string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return "global", "ranked-1v1"
}

func (q *QueueManager) allowedMMRDiff(a, b *Ticket, now time.Time) int {
	base := 60
	wa := now.Sub(a.JoinedAt).Seconds()
	wb := now.Sub(b.JoinedAt).Seconds()
	wait := math.Max(wa, wb)
	bonus := int(wait * 12)
	if bonus > 650 {
		bonus = 650
	}
	return base + bonus
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
