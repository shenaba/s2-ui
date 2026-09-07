package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shenaba/2s-ui/database"
	"github.com/shenaba/2s-ui/database/model"
	"github.com/shenaba/2s-ui/logger"

	"github.com/op/go-logging"
)

// TopTags answers "which tags moved the most", which no other query does --
// GetStats is per-tag over time. Both directions are summed, because a report
// ranking uploads separately from downloads ranks nothing anyone asked about.
func TestTopTags(t *testing.T) {
	logger.InitLogger(logging.CRITICAL)

	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "toptags.db")); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseDBForTest(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	db := database.GetDB()

	now := time.Now().Unix()
	rows := []model.Stats{
		{Resource: "inbound", Tag: "busy", DateTime: now - 60, Direction: true, Traffic: 300},
		{Resource: "inbound", Tag: "busy", DateTime: now - 60, Direction: false, Traffic: 200},
		{Resource: "inbound", Tag: "quiet", DateTime: now - 60, Direction: true, Traffic: 10},
		// Outside the window.
		{Resource: "inbound", Tag: "yesterday", DateTime: now - 90000, Direction: true, Traffic: 9999},
		// A different resource: a client tag must not be ranked among inbounds.
		{Resource: "user", Tag: "alice", DateTime: now - 60, Direction: true, Traffic: 500},
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed %s/%s: %v", row.Resource, row.Tag, err)
		}
	}

	var stats StatsService
	got, err := stats.TopTags("inbound", now-3600, 10)
	if err != nil {
		t.Fatalf("TopTags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (busy, quiet): %+v", len(got), got)
	}
	if got[0].Tag != "busy" || got[0].Traffic != 500 {
		t.Errorf("busiest inbound is %+v, want busy with both directions summed to 500", got[0])
	}
	if got[1].Tag != "quiet" {
		t.Errorf("second row is %+v, want quiet", got[1])
	}

	users, err := stats.TopTags("user", now-3600, 10)
	if err != nil {
		t.Fatalf("TopTags(user): %v", err)
	}
	if len(users) != 1 || users[0].Tag != "alice" {
		t.Errorf("client ranking is %+v, want just alice", users)
	}
}

// The inbounds list shows one traffic figure per inbound. It cannot be the sum
// of that inbound's clients: a client's up/down is its total across every
// inbound it belongs to, so two inbounds sharing one busy client reported the
// same number. This is the figure the core actually measured per inbound —
// seeded from the stats table, then advanced by each flush.
func TestInboundTrafficSeedsThenAccumulates(t *testing.T) {
	logger.InitLogger(logging.CRITICAL)

	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "inboundtraffic.db")); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseDBForTest(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	db := database.GetDB()

	now := time.Now().Unix()
	rows := []model.Stats{
		{Resource: "inbound", Tag: "busy", DateTime: now - 60, Direction: true, Traffic: 300},
		{Resource: "inbound", Tag: "busy", DateTime: now - 60, Direction: false, Traffic: 200},
		// An older bucket still counts: this is a total, not a window.
		{Resource: "inbound", Tag: "busy", DateTime: now - 90000, Direction: true, Traffic: 1},
		{Resource: "inbound", Tag: "quiet", DateTime: now - 60, Direction: true, Traffic: 10},
		// The same client's traffic under another resource must not be folded in.
		{Resource: "user", Tag: "alice", DateTime: now - 60, Direction: true, Traffic: 500},
		{Resource: "outbound", Tag: "direct", DateTime: now - 60, Direction: true, Traffic: 700},
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed %s/%s: %v", row.Resource, row.Tag, err)
		}
	}

	// Package state outlives one test.
	InvalidateInboundTraffic()
	t.Cleanup(InvalidateInboundTraffic)

	var stats StatsService
	got := stats.GetInboundTraffic()
	want := map[string]int64{"busy": 501, "quiet": 10}
	if len(got) != len(want) {
		t.Fatalf("seeded totals %v, want %v", got, want)
	}
	for tag, traffic := range want {
		if got[tag] != traffic {
			t.Errorf("seeded %q = %d, want %d", tag, got[tag], traffic)
		}
	}

	// A flush advances the totals without re-reading the table, and an inbound
	// with no history at all starts from its delta.
	addInboundTraffic(map[string]int64{"busy": 7, "brandnew": 3})
	got = stats.GetInboundTraffic()
	if got["busy"] != 508 {
		t.Errorf("busy after flush = %d, want 508", got["busy"])
	}
	if got["brandnew"] != 3 {
		t.Errorf("brandnew after flush = %d, want 3", got["brandnew"])
	}

	// The returned map is a copy: a caller mutating it must not corrupt the
	// published totals.
	got["busy"] = 0
	if again := stats.GetInboundTraffic(); again["busy"] != 508 {
		t.Errorf("busy = %d after a caller mutated an earlier result, want 508", again["busy"])
	}
}
