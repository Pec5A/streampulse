package streaming

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRegistry_OpenGetClose(t *testing.T) {
	reg := NewRegistry()

	if _, err := reg.Get("nope"); err != ErrStreamNotFound {
		t.Errorf("Get on unknown stream = %v, want ErrStreamNotFound", err)
	}
	if reg.IsLive("nope") {
		t.Error("IsLive on unknown stream = true, want false")
	}

	hub := reg.Open(context.Background(), "s1")
	if !reg.IsLive("s1") {
		t.Error("IsLive after Open = false, want true")
	}
	got, err := reg.Get("s1")
	if err != nil {
		t.Fatalf("Get after Open: %v", err)
	}
	if got != hub {
		t.Error("Get returned a different hub than Open")
	}

	reg.Close("s1")
	if reg.IsLive("s1") {
		t.Error("IsLive after Close = true, want false")
	}
	if _, err := reg.Get("s1"); err != ErrStreamNotFound {
		t.Errorf("Get after Close = %v, want ErrStreamNotFound", err)
	}
	select {
	case <-hub.Done():
	case <-time.After(time.Second):
		t.Error("Close did not terminate the hub")
	}
}

func TestRegistry_CloseOnUnknownStreamIsANoOp(t *testing.T) {
	reg := NewRegistry()
	reg.Close("never-opened") // must not panic
}

func TestRegistry_ReopenReplacesAndReleasesThePreviousHub(t *testing.T) {
	// A broadcaster whose connection dropped reconnects: the stale hub must
	// be closed so its listeners are not stranded on a dead stream.
	reg := NewRegistry()

	first := reg.Open(context.Background(), "s1")
	_, ch, _, err := first.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	second := reg.Open(context.Background(), "s1")
	if first == second {
		t.Fatal("Open returned the same hub twice")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("stale listener channel still open after reopen")
		}
	case <-time.After(time.Second):
		t.Error("reopening did not release the previous hub's listeners")
	}

	if got, _ := reg.Get("s1"); got != second {
		t.Error("registry still points at the stale hub")
	}
}

func TestRegistry_ListenerCountIsZeroForOfflineStreams(t *testing.T) {
	reg := NewRegistry()

	if got := reg.ListenerCount("offline"); got != 0 {
		t.Errorf("ListenerCount for an offline stream = %d, want 0", got)
	}

	hub := reg.Open(context.Background(), "s1")
	defer reg.Close("s1")

	for range 3 {
		if _, _, _, err := hub.Subscribe(); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}
	if got := reg.ListenerCount("s1"); got != 3 {
		t.Errorf("ListenerCount = %d, want 3", got)
	}
}

func TestRegistry_LiveIDs(t *testing.T) {
	reg := NewRegistry()
	reg.Open(context.Background(), "a")
	reg.Open(context.Background(), "b")
	defer reg.CloseAll()

	ids := reg.LiveIDs()
	if len(ids) != 2 {
		t.Fatalf("LiveIDs = %v, want 2 entries", ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("LiveIDs = %v, want both a and b", ids)
	}
}

func TestRegistry_CloseAllReleasesEveryHub(t *testing.T) {
	reg := NewRegistry()

	hubs := make([]*Hub, 0, 5)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		hubs = append(hubs, reg.Open(context.Background(), id))
	}

	reg.CloseAll()

	if got := reg.LiveIDs(); len(got) != 0 {
		t.Errorf("LiveIDs after CloseAll = %v, want empty", got)
	}
	for i, h := range hubs {
		select {
		case <-h.Done():
		case <-time.After(time.Second):
			t.Errorf("hub %d not closed by CloseAll", i)
		}
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	// Run under -race.
	reg := NewRegistry()
	defer reg.CloseAll()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i%5))
			for range 50 {
				reg.Open(context.Background(), id)
				_, _ = reg.Get(id)
				_ = reg.IsLive(id)
				_ = reg.ListenerCount(id)
				_ = reg.LiveIDs()
				reg.Close(id)
			}
		}(i)
	}
	wg.Wait()
}
