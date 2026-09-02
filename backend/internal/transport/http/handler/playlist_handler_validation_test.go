package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/streampulse/backend/internal/application/dto"
)

// TestPlaylistAPI_MalformedIDIsBadRequest: a malformed id/trackID in the path
// is rejected with 400 before reaching the DB (previously surfaced as a 500 via
// Postgres 22P02). A well-formed but absent id still yields 404.
func TestPlaylistAPI_MalformedIDIsBadRequest(t *testing.T) {
	srv, jwt := newServer()
	tok := tokenFor(t, jwt, "alice")

	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "P"})
	p := decodeBody[dto.PlaylistResponse](t, rec)

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/playlists/not-a-uuid", nil},
		{http.MethodPatch, "/api/v1/playlists/not-a-uuid", dto.UpdatePlaylistRequest{}},
		{http.MethodDelete, "/api/v1/playlists/not-a-uuid", nil},
		{http.MethodPost, "/api/v1/playlists/not-a-uuid/tracks", dto.AddTrackRequest{Title: "x"}},
		{http.MethodPut, "/api/v1/playlists/not-a-uuid/tracks/order", dto.ReorderRequest{}},
		// malformed trackID under a valid playlist id
		{http.MethodDelete, "/api/v1/playlists/" + p.ID + "/tracks/not-a-uuid", nil},
	}
	for _, tc := range cases {
		if rec := do(t, srv, tc.method, tc.path, tok, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s -> %d, want 400", tc.method, tc.path, rec.Code)
		}
	}

	if rec := do(t, srv, http.MethodGet, "/api/v1/playlists/11111111-1111-1111-1111-111111111111", tok, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("well-formed absent id -> %d, want 404", rec.Code)
	}
}

// TestPlaylistAPI_TrackTextTooLong: title/artist beyond the VARCHAR(200) limit
// fail as 400 (previously a database 500), mirroring the playlist-name rule.
func TestPlaylistAPI_TrackTextTooLong(t *testing.T) {
	srv, jwt := newServer()
	tok := tokenFor(t, jwt, "alice")
	rec := do(t, srv, http.MethodPost, "/api/v1/playlists", tok, dto.CreatePlaylistRequest{Name: "P"})
	p := decodeBody[dto.PlaylistResponse](t, rec)
	tracksURL := "/api/v1/playlists/" + p.ID + "/tracks"

	over := strings.Repeat("a", 201)
	if rec := do(t, srv, http.MethodPost, tracksURL, tok, dto.AddTrackRequest{Title: over}); rec.Code != http.StatusBadRequest {
		t.Fatalf("201-char title -> %d, want 400", rec.Code)
	}
	if rec := do(t, srv, http.MethodPost, tracksURL, tok, dto.AddTrackRequest{Title: "ok", Artist: over}); rec.Code != http.StatusBadRequest {
		t.Fatalf("201-char artist -> %d, want 400", rec.Code)
	}
	// The 200-char boundary is accepted.
	if rec := do(t, srv, http.MethodPost, tracksURL, tok, dto.AddTrackRequest{Title: strings.Repeat("a", 200)}); rec.Code != http.StatusCreated {
		t.Fatalf("200-char title -> %d, want 201", rec.Code)
	}
}
