package matchmaking

import (
	"context"
	"testing"
	"time"

	"projectvelocity/backend/internal/shared/types"
)

func TestQueueMatchesSimilarMMR(t *testing.T) {
	q := NewQueueManager("ws://localhost:9003/ws")
	a := q.Join(types.QueueJoinRequest{PlayerID: "p1", DisplayName: "A", Region: "us-east", Playlist: "ranked-1v1", MMR: 1200})
	b := q.Join(types.QueueJoinRequest{PlayerID: "p2", DisplayName: "B", Region: "us-east", Playlist: "ranked-1v1", MMR: 1240})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Run(ctx, 10*time.Millisecond, 2)
	time.Sleep(50 * time.Millisecond)

	ap := q.Poll(a.TicketID)
	bp := q.Poll(b.TicketID)
	if ap.Status != "matched" || bp.Status != "matched" {
		t.Fatalf("expected both matched: a=%s b=%s", ap.Status, bp.Status)
	}
	if ap.Assignment == nil || bp.Assignment == nil {
		t.Fatal("expected assignments for both players")
	}
	if ap.Assignment.MatchID != bp.Assignment.MatchID {
		t.Fatalf("expected same match id: a=%s b=%s", ap.Assignment.MatchID, bp.Assignment.MatchID)
	}
}

func TestQueueWaitExpandsMMRWindow(t *testing.T) {
	q := NewQueueManager("ws://localhost:9003/ws")
	a := q.Join(types.QueueJoinRequest{PlayerID: "p1", DisplayName: "A", Region: "us-east", Playlist: "ranked-1v1", MMR: 900})
	b := q.Join(types.QueueJoinRequest{PlayerID: "p2", DisplayName: "B", Region: "us-east", Playlist: "ranked-1v1", MMR: 1500})

	q.mu.Lock()
	if ta, ok := q.ticketIndex[a.TicketID]; ok {
		ta.JoinedAt = time.Now().UTC().Add(-2 * time.Minute)
	}
	if tb, ok := q.ticketIndex[b.TicketID]; ok {
		tb.JoinedAt = time.Now().UTC().Add(-2 * time.Minute)
	}
	q.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Run(ctx, 10*time.Millisecond, 2)
	time.Sleep(50 * time.Millisecond)

	ap := q.Poll(a.TicketID)
	if ap.Status != "matched" {
		t.Fatalf("expected ticket to match after wait expansion, got=%s", ap.Status)
	}
}

func TestQueueSoloBotFill(t *testing.T) {
	q := NewQueueManager("ws://localhost:9003/ws")
	a := q.Join(types.QueueJoinRequest{PlayerID: "solo", DisplayName: "Solo", Region: "us-east", Playlist: "ranked-1v1", MMR: 1200})

	q.mu.Lock()
	if ta, ok := q.ticketIndex[a.TicketID]; ok {
		ta.JoinedAt = time.Now().UTC().Add(-10 * time.Second)
	}
	q.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Run(ctx, 10*time.Millisecond, 2)
	time.Sleep(50 * time.Millisecond)

	ap := q.Poll(a.TicketID)
	if ap.Status != "matched" {
		t.Fatalf("expected solo ticket to be matched with bot fill, got=%s", ap.Status)
	}
	if ap.Assignment == nil {
		t.Fatal("expected assignment for solo ticket")
	}
	if !ap.Assignment.BotFill {
		t.Fatal("expected bot_fill=true assignment for solo ticket")
	}
}
