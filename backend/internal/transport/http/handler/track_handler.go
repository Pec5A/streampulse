package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

const (
	// multipartMemory is how much of the form is kept in RAM before Go
	// spills to a temp file. Small on purpose: a 50 MiB upload must not be
	// 50 MiB of heap.
	multipartMemory = 8 << 20 // 8 MiB

	// multipartOverhead is slack for the multipart envelope (boundaries,
	// part headers, the title/artist fields) on top of the file itself.
	multipartOverhead = 1 << 20 // 1 MiB
)

type TrackHandler struct {
	uc *usecase.TrackUseCase
}

func NewTrackHandler(uc *usecase.TrackUseCase) *TrackHandler {
	return &TrackHandler{uc: uc}
}

func writeTrackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "track not found")
	case errors.Is(err, usecase.ErrStreamForbidden):
		writeError(w, http.StatusForbidden, "not your track")
	case errors.Is(err, usecase.ErrUnsupportedMedia):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, usecase.ErrFileTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "file exceeds the 50 MiB limit")
	case errors.Is(err, usecase.ErrEmptyFile):
		writeError(w, http.StatusBadRequest, "file is empty")
	case errors.Is(err, usecase.ErrInvalidTrack):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "track operation failed")
	}
}

// Upload accepts a multipart form with `title`, optional `artist`, and a
// `file` part carrying the audio.
func (h *TrackHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	// Cap the request at the transport boundary so an oversized upload is
	// refused while it streams, instead of after we have read all of it.
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxUploadBytes+multipartOverhead)

	// #nosec G120 -- the body is already bounded by the MaxBytesReader on the
	// line above, so this parse cannot grow past MaxUploadBytes+overhead.
	// gosec cannot see that relationship across statements. Proven by
	// TestTrackHandler_UploadRejectsAnOversizedBody, which asserts a 413.
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds the 50 MiB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	defer func() {
		// ParseMultipartForm may have spilled to temp files.
		if err := r.MultipartForm.RemoveAll(); err != nil {
			slog.Warn("cleanup multipart temp files", "err", err)
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "a `file` part is required")
		return
	}
	defer func() { _ = file.Close() }()

	track, err := h.uc.Upload(r.Context(), usecase.UploadInput{
		UploaderID:  userID,
		Title:       r.FormValue("title"),
		Artist:      r.FormValue("artist"),
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Body:        file,
	})
	if err != nil {
		writeTrackError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.AudioTrackFrom(track))
}

// List returns the public track catalogue.
func (h *TrackHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	tracks, err := h.uc.List(r.Context(), limit, offset)
	if err != nil {
		writeTrackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.AudioTracksFrom(tracks))
}

// ListMine returns the authenticated broadcaster's own uploads.
func (h *TrackHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	tracks, err := h.uc.ListMine(r.Context(), userID)
	if err != nil {
		writeTrackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.AudioTracksFrom(tracks))
}

func (h *TrackHandler) Get(w http.ResponseWriter, r *http.Request) {
	track, err := h.uc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTrackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.AudioTrackFrom(track))
}

// Audio serves the stored file.
//
// http.ServeContent, not io.Copy: it answers Range requests, which is what
// lets the mobile player seek inside a track instead of re-downloading it
// from the start. This is the payoff of Storage.Open returning a ReadSeeker.
func (h *TrackHandler) Audio(w http.ResponseWriter, r *http.Request) {
	track, body, err := h.uc.Open(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTrackError(w, err)
		return
	}
	defer func() { _ = body.Close() }()

	// Serve with the type we validated at upload time, never a sniffed one,
	// and forbid the browser from second-guessing it. Together with the
	// active-content check on upload, that closes the "upload HTML, get it
	// executed" path for good.
	w.Header().Set("Content-Type", track.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Accept-Ranges", "bytes")

	http.ServeContent(w, r, track.Filename, track.UpdatedAt, body)
}

func (h *TrackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	role, _ := middleware.UserRole(r.Context())

	if err := h.uc.Delete(r.Context(), r.PathValue("id"), userID, role); err != nil {
		writeTrackError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
