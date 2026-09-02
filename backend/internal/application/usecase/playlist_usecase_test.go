package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

// fakePlaylistRepo is an in-memory repository.PlaylistRepository. It enforces
// the same invariants a real adapter must (contiguous positions, permutation
// reorder), which lets us test the use case's rules without a database.
type fakePlaylistRepo struct {
	mu        sync.Mutex
	playlists map[string]*entity.Playlist
	tracks    map[string][]entity.Track // playlistID -> tracks ordered by position
	seq       int
}

func newFakePlaylistRepo() *fakePlaylistRepo {
	return &fakePlaylistRepo{
		playlists: map[string]*entity.Playlist{},
		tracks:    map[string][]entity.Track{},
	}
}

func (r *fakePlaylistRepo) nextID(prefix string) string {
	r.seq++
	return fmt.Sprintf("%s-%d", prefix, r.seq)
}

func (r *fakePlaylistRepo) Create(_ context.Context, p *entity.Playlist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = r.nextID("pl")
	}
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	cp := *p
	r.playlists[p.ID] = &cp
	return nil
}

func (r *fakePlaylistRepo) FindByID(_ context.Context, id string) (*entity.Playlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.playlists[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}

func (r *fakePlaylistRepo) ListByOwner(_ context.Context, ownerID string) ([]entity.Playlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []entity.Playlist
	for _, p := range r.playlists {
		if p.OwnerID == ownerID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *fakePlaylistRepo) Update(_ context.Context, p *entity.Playlist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.playlists[p.ID]; !ok {
		return repository.ErrNotFound
	}
	p.UpdatedAt = time.Now()
	cp := *p
	r.playlists[p.ID] = &cp
	return nil
}

func (r *fakePlaylistRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.playlists[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.playlists, id)
	delete(r.tracks, id)
	return nil
}

func (r *fakePlaylistRepo) ListTracks(_ context.Context, playlistID string) ([]entity.Track, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]entity.Track(nil), r.tracks[playlistID]...), nil
}

func (r *fakePlaylistRepo) AddTrack(_ context.Context, t *entity.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = r.nextID("tr")
	}
	t.Position = len(r.tracks[t.PlaylistID])
	t.CreatedAt = time.Now()
	r.tracks[t.PlaylistID] = append(r.tracks[t.PlaylistID], *t)
	return nil
}

func (r *fakePlaylistRepo) RemoveTrack(_ context.Context, playlistID, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := r.tracks[playlistID]
	out := make([]entity.Track, 0, len(ts))
	found := false
	for _, t := range ts {
		if t.ID == trackID {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return repository.ErrNotFound
	}
	for i := range out {
		out[i].Position = i
	}
	r.tracks[playlistID] = out
	return nil
}

func (r *fakePlaylistRepo) ReorderTracks(_ context.Context, playlistID string, orderedTrackIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	byID := map[string]entity.Track{}
	for _, t := range r.tracks[playlistID] {
		byID[t.ID] = t
	}
	out := make([]entity.Track, 0, len(orderedTrackIDs))
	for i, id := range orderedTrackIDs {
		t, ok := byID[id]
		if !ok {
			return repository.ErrNotFound
		}
		t.Position = i
		out = append(out, t)
	}
	r.tracks[playlistID] = out
	return nil
}

var _ repository.PlaylistRepository = (*fakePlaylistRepo)(nil)

// ---- helpers ----

const owner = "user-owner"

func newUC() *PlaylistUseCase { return NewPlaylistUseCase(newFakePlaylistRepo()) }

func mustCreate(t *testing.T, uc *PlaylistUseCase) *entity.Playlist {
	t.Helper()
	p, err := uc.Create(context.Background(), owner, "My Mix", "chill vibes", false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return p
}

func mustAddTrack(t *testing.T, uc *PlaylistUseCase, playlistID, title string) *entity.Track {
	t.Helper()
	tr, err := uc.AddTrack(context.Background(), owner, playlistID, entity.Track{Title: title})
	if err != nil {
		t.Fatalf("AddTrack(%q) error = %v", title, err)
	}
	return tr
}

// ---- Create ----

func TestCreate_Success(t *testing.T) {
	uc := newUC()
	p, err := uc.Create(context.Background(), owner, "  Road Trip  ", "desc", true)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if p.Name != "Road Trip" {
		t.Errorf("Name = %q, want trimmed 'Road Trip'", p.Name)
	}
	if p.OwnerID != owner || !p.IsPublic {
		t.Errorf("owner/isPublic = %q/%v, want %q/true", p.OwnerID, p.IsPublic, owner)
	}
	if p.ID == "" {
		t.Error("expected a generated id")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	uc := newUC()
	if _, err := uc.Create(context.Background(), owner, "   ", "", false); !errors.Is(err, ErrValidation) {
		t.Fatalf("Create() error = %v, want ErrValidation", err)
	}
}

func TestCreate_NameTooLong(t *testing.T) {
	uc := newUC()
	long := make([]byte, maxNameLen+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := uc.Create(context.Background(), owner, string(long), "", false); !errors.Is(err, ErrValidation) {
		t.Fatalf("Create() error = %v, want ErrValidation", err)
	}
}

// ---- Get ----

func TestGet_OwnerSeesPrivateWithTracks(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	mustAddTrack(t, uc, p.ID, "A")
	mustAddTrack(t, uc, p.ID, "B")

	got, tracks, err := uc.Get(context.Background(), owner, p.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("Get id = %q, want %q", got.ID, p.ID)
	}
	if len(tracks) != 2 || tracks[0].Title != "A" || tracks[1].Title != "B" {
		t.Errorf("tracks = %+v, want [A B] in order", tracks)
	}
}

func TestGet_PublicVisibleToOthers(t *testing.T) {
	uc := newUC()
	p, err := uc.Create(context.Background(), owner, "Public", "", true)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, _, err := uc.Get(context.Background(), "someone-else", p.ID); err != nil {
		t.Fatalf("Get() by other on public error = %v, want nil", err)
	}
}

func TestGet_PrivateForbiddenToOthers(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc) // private
	if _, _, err := uc.Get(context.Background(), "someone-else", p.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Get() error = %v, want ErrForbidden", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	uc := newUC()
	if _, _, err := uc.Get(context.Background(), owner, "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

// ---- ListByOwner ----

func TestListByOwner_FiltersByOwner(t *testing.T) {
	uc := newUC()
	mustCreate(t, uc)
	mustCreate(t, uc)
	if _, err := uc.Create(context.Background(), "other", "Theirs", "", false); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	mine, err := uc.ListByOwner(context.Background(), owner)
	if err != nil {
		t.Fatalf("ListByOwner() error = %v", err)
	}
	if len(mine) != 2 {
		t.Fatalf("ListByOwner() len = %d, want 2", len(mine))
	}
}

// ---- Update ----

func TestUpdate_OwnerPartial(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	newName := "Renamed"
	updated, err := uc.Update(context.Background(), owner, p.ID, &newName, nil, nil)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("Name = %q, want Renamed", updated.Name)
	}
	if updated.Description != p.Description {
		t.Errorf("Description = %q, want unchanged %q", updated.Description, p.Description)
	}
}

func TestUpdate_ForbiddenForNonOwner(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	name := "hijack"
	if _, err := uc.Update(context.Background(), "attacker", p.ID, &name, nil, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Update() error = %v, want ErrForbidden", err)
	}
}

func TestUpdate_EmptyNameRejected(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	blank := "   "
	if _, err := uc.Update(context.Background(), owner, p.ID, &blank, nil, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("Update() error = %v, want ErrValidation", err)
	}
}

// ---- Delete ----

func TestDelete_OwnerSuccess(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	if err := uc.Delete(context.Background(), owner, p.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := uc.Get(context.Background(), owner, p.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("after Delete, Get error = %v, want ErrNotFound", err)
	}
}

func TestDelete_ForbiddenForNonOwner(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	if err := uc.Delete(context.Background(), "attacker", p.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Delete() error = %v, want ErrForbidden", err)
	}
}

// ---- Tracks ----

func TestAddTrack_AppendsWithPosition(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	t0 := mustAddTrack(t, uc, p.ID, "first")
	t1 := mustAddTrack(t, uc, p.ID, "second")
	if t0.Position != 0 || t1.Position != 1 {
		t.Fatalf("positions = %d,%d, want 0,1", t0.Position, t1.Position)
	}
}

func TestAddTrack_EmptyTitleRejected(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	if _, err := uc.AddTrack(context.Background(), owner, p.ID, entity.Track{Title: "  "}); !errors.Is(err, ErrValidation) {
		t.Fatalf("AddTrack() error = %v, want ErrValidation", err)
	}
}

func TestAddTrack_ForbiddenForNonOwner(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	if _, err := uc.AddTrack(context.Background(), "attacker", p.ID, entity.Track{Title: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("AddTrack() error = %v, want ErrForbidden", err)
	}
}

func TestRemoveTrack_CompactsPositions(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	a := mustAddTrack(t, uc, p.ID, "A")
	mustAddTrack(t, uc, p.ID, "B")
	c := mustAddTrack(t, uc, p.ID, "C")

	if err := uc.RemoveTrack(context.Background(), owner, p.ID, a.ID); err != nil {
		t.Fatalf("RemoveTrack() error = %v", err)
	}
	_, tracks, err := uc.Get(context.Background(), owner, p.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("len = %d, want 2", len(tracks))
	}
	if tracks[0].Title != "B" || tracks[0].Position != 0 {
		t.Errorf("tracks[0] = %q@%d, want B@0", tracks[0].Title, tracks[0].Position)
	}
	if tracks[1].Title != "C" || tracks[1].Position != 1 {
		t.Errorf("tracks[1] = %q@%d, want C@1", tracks[1].Title, tracks[1].Position)
	}
	if tracks[1].ID != c.ID {
		t.Errorf("tracks[1].ID = %q, want %q", tracks[1].ID, c.ID)
	}
}

func TestReorderTracks_Success(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	a := mustAddTrack(t, uc, p.ID, "A")
	b := mustAddTrack(t, uc, p.ID, "B")
	c := mustAddTrack(t, uc, p.ID, "C")

	if err := uc.ReorderTracks(context.Background(), owner, p.ID, []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatalf("ReorderTracks() error = %v", err)
	}
	_, tracks, err := uc.Get(context.Background(), owner, p.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	gotOrder := []string{tracks[0].Title, tracks[1].Title, tracks[2].Title}
	want := []string{"C", "A", "B"}
	for i := range want {
		if gotOrder[i] != want[i] || tracks[i].Position != i {
			t.Fatalf("order = %v (positions off), want %v", gotOrder, want)
		}
	}
}

func TestReorderTracks_InvalidSet(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	a := mustAddTrack(t, uc, p.ID, "A")
	mustAddTrack(t, uc, p.ID, "B")

	// Missing one id / contains a foreign id -> rejected before touching repo.
	if err := uc.ReorderTracks(context.Background(), owner, p.ID, []string{a.ID}); !errors.Is(err, ErrInvalidReorder) {
		t.Fatalf("ReorderTracks(subset) error = %v, want ErrInvalidReorder", err)
	}
	if err := uc.ReorderTracks(context.Background(), owner, p.ID, []string{a.ID, "ghost"}); !errors.Is(err, ErrInvalidReorder) {
		t.Fatalf("ReorderTracks(foreign) error = %v, want ErrInvalidReorder", err)
	}
}

func TestReorderTracks_ForbiddenForNonOwner(t *testing.T) {
	uc := newUC()
	p := mustCreate(t, uc)
	a := mustAddTrack(t, uc, p.ID, "A")
	if err := uc.ReorderTracks(context.Background(), "attacker", p.ID, []string{a.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ReorderTracks() error = %v, want ErrForbidden", err)
	}
}
