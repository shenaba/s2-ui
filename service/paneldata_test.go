package service

import (
	"path/filepath"
	"testing"

	"github.com/shenaba/2s-ui/database"
)

// The config cache keys on lastUpdateSeq, not on lastUpdate. Both move on every
// write, but lastUpdate is unix SECONDS: two writes inside one second store the
// same value, so an entry built between them compared equal and the second
// write's config was served from the pre-write snapshot for the rest of the TTL.
// The panel refreshes through this path right after a save, and the websocket
// push that would repair it lands 200ms later -- inside the TTL, on the same
// stale entry -- so the staleness was not reliably self-healing either.
func TestConfigCacheRebuildsForASecondWriteInTheSameSecond(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "cache.db")); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseDBForTest(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	const host = "panel.example"
	// One wall-clock second, marked twice -- what two saves in quick succession
	// (or one bulk drawer firing several) actually do.
	const oneSecond = int64(1700000000)
	var pd PanelDataService

	MarkLastUpdate(oneSecond)
	_, _, first, err := pd.configHalf(host)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	// Nothing marked in between: the entry must still be served, or this test
	// would pass simply by having broken the cache.
	_, _, cached, err := pd.configHalf(host)
	if err != nil {
		t.Fatalf("cached read: %v", err)
	}
	if cached != first {
		t.Fatalf("cache did not serve an unchanged config: %d then %d", first, cached)
	}

	MarkLastUpdate(oneSecond)
	_, _, afterSecondWrite, err := pd.configHalf(host)
	if err != nil {
		t.Fatalf("build after the second write: %v", err)
	}
	if afterSecondWrite == first {
		t.Error("a second write inside the same second was served the pre-write config")
	}
}
