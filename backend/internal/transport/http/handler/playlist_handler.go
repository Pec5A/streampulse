package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

// PlaylistHandler exposes the playlist CRUD + track queue endpoints. Every
// route is mounted behind middleware.RequireAuth, so the caller's id is always
// available via middleware.UserID and is treated as the resource owner.
type PlaylistHandler struct {
	uc *usecase.PlaylistUseCase
}

func NewPlaylistHandler(uc *usecase.PlaylistUseCase) *PlaylistHandler {
	return &PlaylistHandler{uc: uc}
}

func (h *PlaylistHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	var req dto.CreatePlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := h.uc.Create(r.Context(), userID, req.Name, req.Description, req.IsPublic)
	if err != nil {
		h.writeUCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.PlaylistFrom(p))
}

func (h *PlaylistHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	playlists, err := h.uc.ListByOwner(r.Context(), userID)
	if err != nil {
		h.writeUCError(w, err)
		return
	}
	out := make([]dto.PlaylistResponse, len(playlists))
	for i := range playlists {
		out[i] = dto.PlaylistFrom(&playlists[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *PlaylistHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	p, tracks, err := h.uc.Get(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		h.writeUCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.PlaylistWithTracks(p, tracks))
}

func (h *PlaylistHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req dto.UpdatePlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := h.uc.Update(r.Context(), userID, r.PathValue("id"), req.Name, req.Description, req.IsPublic)
	if err != nil {
		h.writeUCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.PlaylistFrom(p))
}

func (h *PlaylistHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	if err := h.uc.Delete(r.Context(), userID, r.PathValue("id")); err != nil {
		h.writeUCError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PlaylistHandler) AddTrack(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req dto.AddTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	track, err := h.uc.AddTrack(r.Context(), userID, r.PathValue("id"), entity.Track{
		Title:           req.Title,
		Artist:          req.Artist,
		DurationSeconds: req.DurationSeconds,
		SourceURL:       req.SourceURL,
	})
	if err != nil {
		h.writeUCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.TrackFrom(*track))
}

func (h *PlaylistHandler) RemoveTrack(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	if err := h.uc.RemoveTrack(r.Context(), userID, r.PathValue("id"), r.PathValue("trackID")); err != nil {
		h.writeUCError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Reorder rewrites the track order for a playlist and returns the canonical,
// reordered view so the client can reconcile its optimistic update.
func (h *PlaylistHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req dto.ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	playlistID := r.PathValue("id")
	if err := h.uc.ReorderTracks(r.Context(), userID, playlistID, req.TrackIDs); err != nil {
		h.writeUCError(w, err)
		return
	}
	p, tracks, err := h.uc.Get(r.Context(), userID, playlistID)
	if err != nil {
		h.writeUCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.PlaylistWithTracks(p, tracks))
}

func (h *PlaylistHandler) writeUCError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "playlist or track not found")
	case errors.Is(err, usecase.ErrForbidden):
		writeError(w, http.StatusForbidden, "you do not own this playlist")
	case errors.Is(err, usecase.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, usecase.ErrInvalidReorder):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "position conflict, please retry")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
