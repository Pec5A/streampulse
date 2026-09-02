package streaming

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func recvChatWithin(t *testing.T, ch <-chan ChatMessage, d time.Duration) ChatMessage {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed while a message was expected")
		}
		return msg
	case <-time.After(d):
		t.Fatal("timed out waiting for a message")
		return ChatMessage{}
	}
}

func TestChatHub_FanOutIsNToN(t *testing.T) {
	// The headline requirement: every participant, including the sender,
	// receives every message — unlike Hub, which is 1-to-N.
	hub := NewChatHub(context.Background(), "stream-1")
	defer hub.Close()

	const participants = 5
	var wg sync.WaitGroup
	received := make([][]ChatMessage, participants)
	var mu sync.Mutex

	for i := range participants {
		_, ch, leave, err := hub.Join()
		if err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		wg.Add(1)
		go func(idx int, ch <-chan ChatMessage, leave func()) {
			defer wg.Done()
			defer leave()
			for range participants {
				msg := recvChatWithin(t, ch, 5*time.Second)
				mu.Lock()
				received[idx] = append(received[idx], msg)
				mu.Unlock()
			}
		}(i, ch, leave)
	}

	for i := range participants {
		if err := hub.Publish(ChatMessage{ID: string(rune('a' + i)), Text: "hello"}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	wg.Wait()

	for i, msgs := range received {
		if len(msgs) != participants {
			t.Errorf("participant %d received %d messages, want %d", i, len(msgs), participants)
		}
	}
}

func TestChatHub_SenderReceivesItsOwnMessage(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-echo")
	defer hub.Close()

	_, ch, leave, err := hub.Join()
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	defer leave()

	if err := hub.Publish(ChatMessage{ID: "m1", Text: "hi"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	msg := recvChatWithin(t, ch, 2*time.Second)
	if msg.ID != "m1" {
		t.Errorf("ID = %q, want m1", msg.ID)
	}
}

func TestChatHub_PublishNeverBlocksOnASlowParticipant(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-slow")
	defer hub.Close()

	_, _, _, err := hub.Join() // deliberately never drained
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range ChatBuffer * 4 {
			_ = hub.Publish(ChatMessage{ID: string(rune(i))})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a participant that stopped reading")
	}

	if stats := hub.Stats(); stats.Dropped == 0 {
		t.Error("expected dropped messages for a participant that never reads")
	}
}

func TestChatHub_JoinAfterCloseReturnsErrChatClosed(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-closed")
	hub.Close()

	if _, _, _, err := hub.Join(); err != ErrChatClosed {
		t.Fatalf("Join() after close error = %v, want ErrChatClosed", err)
	}
}

func TestChatHub_PublishAfterCloseReturnsErrChatClosed(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-closed")
	hub.Close()

	if err := hub.Publish(ChatMessage{ID: "x"}); err != ErrChatClosed {
		t.Fatalf("Publish() after close error = %v, want ErrChatClosed", err)
	}
}

func TestChatHub_CloseClosesEveryParticipantChannel(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-close-fanout")

	const participants = 10
	chans := make([]<-chan ChatMessage, participants)
	for i := range participants {
		_, ch, _, err := hub.Join()
		if err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		chans[i] = ch
	}

	hub.Close()

	for i, ch := range chans {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("participant %d: channel still open after Close", i)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("participant %d: channel never closed", i)
		}
	}

	select {
	case <-hub.Done():
	case <-time.After(2 * time.Second):
		t.Error("Done() channel never closed")
	}
}

func TestChatHub_CloseIsIdempotent(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-double-close")
	hub.Close()
	hub.Close() // must not panic on a double close
}

func TestChatHub_LeaveIsIdempotentAndDecrementsParticipants(t *testing.T) {
	hub := NewChatHub(context.Background(), "stream-leave")
	defer hub.Close()

	_, _, leave, err := hub.Join()
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if got := hub.Stats().Participants; got != 1 {
		t.Fatalf("Participants = %d, want 1", got)
	}

	leave()
	leave() // must not panic or double-decrement

	if got := hub.Stats().Participants; got != 0 {
		t.Fatalf("Participants after leave = %d, want 0", got)
	}
}

func TestChatHub_DoesNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	for range 50 {
		hub := NewChatHub(context.Background(), "stream-leak")
		var wg sync.WaitGroup
		for range 20 {
			_, ch, leave, err := hub.Join()
			if err != nil {
				t.Fatalf("join: %v", err)
			}
			wg.Add(1)
			go func(ch <-chan ChatMessage, leave func()) {
				defer wg.Done()
				defer leave()
				for range ch {
				}
			}(ch, leave)
		}
		hub.Close()
		wg.Wait()
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		after := runtime.NumGoroutine()
		if after <= before+5 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine leak: before=%d after=%d", before, after)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestChatRegistry_OpenGetClose(t *testing.T) {
	reg := NewChatRegistry()

	if _, err := reg.Get("s1"); err != ErrStreamNotFound {
		t.Fatalf("Get() on unopened stream error = %v, want ErrStreamNotFound", err)
	}

	hub := reg.Open(context.Background(), "s1")
	got, err := reg.Get("s1")
	if err != nil {
		t.Fatalf("Get() after Open error = %v", err)
	}
	if got != hub {
		t.Error("Get() did not return the hub created by Open()")
	}

	reg.Close("s1")
	if _, err := reg.Get("s1"); err != ErrStreamNotFound {
		t.Fatalf("Get() after Close error = %v, want ErrStreamNotFound", err)
	}

	select {
	case <-hub.Done():
	case <-time.After(2 * time.Second):
		t.Error("Registry.Close did not close the underlying hub")
	}
}

func TestChatRegistry_OpenKeepsParticipantsAcrossBroadcasterReconnect(t *testing.T) {
	// StartLive calls Open on every (re)connection of the broadcaster. A chat
	// room must survive that: unlike the audio hub there is no privileged
	// publisher to recycle, and a closed room silently drops every participant.
	reg := NewChatRegistry()

	first := reg.Open(context.Background(), "s1")
	_, ch, _, err := first.Join()
	if err != nil {
		t.Fatalf("join first: %v", err)
	}

	second := reg.Open(context.Background(), "s1") // broadcaster reconnects
	if second != first {
		t.Fatal("Open() on an already-live stream returned a new hub, evicting the room")
	}

	select {
	case _, ok := <-ch:
		if !ok {
			t.Error("participant was disconnected by the broadcaster reconnect")
		}
	case <-time.After(200 * time.Millisecond):
		// No traffic and no close: the participant is still in the room.
	}

	got, err := reg.Get("s1")
	if err != nil || got != first {
		t.Fatalf("Get() = %v, %v, want the original hub", got, err)
	}
}

func TestChatRegistry_CloseThenOpenStartsAFreshRoom(t *testing.T) {
	// Close is the only way a room ends; reopening after it must not hand back
	// the closed hub, whose Join would fail with ErrChatClosed forever.
	reg := NewChatRegistry()

	first := reg.Open(context.Background(), "s1")
	reg.Close("s1")

	second := reg.Open(context.Background(), "s1")
	if second == first {
		t.Fatal("Open() after Close() returned the closed hub")
	}
	if _, _, _, err := second.Join(); err != nil {
		t.Fatalf("join the reopened room: %v", err)
	}
}

func TestChatRegistry_CloseAllClosesEveryRoom(t *testing.T) {
	reg := NewChatRegistry()

	hubs := make([]*ChatHub, 3)
	for i := range hubs {
		hubs[i] = reg.Open(context.Background(), string(rune('a'+i)))
	}

	reg.CloseAll()

	for i, h := range hubs {
		select {
		case <-h.Done():
		case <-time.After(2 * time.Second):
			t.Errorf("hub %d not closed by CloseAll", i)
		}
	}

	for i := range hubs {
		id := string(rune('a' + i))
		if _, err := reg.Get(id); err != ErrStreamNotFound {
			t.Errorf("Get(%q) after CloseAll error = %v, want ErrStreamNotFound", id, err)
		}
	}
}

func TestChatRegistry_CloseOnUnknownStreamIsANoOp(t *testing.T) {
	reg := NewChatRegistry()
	reg.Close("never-opened") // must not panic
}
