package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

const (
	// publishChunkSize is how much audio we read from the broadcaster per
	// iteration. 4 KB is small enough that a listener joining mid-stream
	// starts hearing audio quickly, and large enough that we are not doing a
	// syscall per handful of bytes.
	publishChunkSize = 4 * 1024

	// stopStreamTimeout bounds the deferred "mark this stream offline" write
	// that runs after the broadcaster has already disconnected.
	stopStreamTimeout = 5 * time.Second

	// maxWSFrameBytes caps a single WebSocket audio frame. Without a limit a
	// malicious client could ask us to buffer an arbitrarily large frame.
	maxWSFrameBytes = 1 << 20 // 1 MiB

	// maxChatMessageLen bounds a single chat message, in characters (not
	// bytes — counted with utf8.RuneCountInString so a message full of
	// multi-byte characters isn't cut short compared to an ASCII one).
	maxChatMessageLen = 500

	// maxChatFrameBytes caps a single incoming chat WebSocket frame. Chat
	// payloads are tiny JSON objects; this exists only to stop a malicious
	// client from asking us to buffer an arbitrarily large frame, mirroring
	// maxWSFrameBytes's role for audio.
	maxChatFrameBytes = 8 * 1024
)

type StreamHandler struct {
	uc *usecase.StreamUseCase
}

func NewStreamHandler(uc *usecase.StreamUseCase) *StreamHandler {
	return &StreamHandler{uc: uc}
}

// writeStreamError maps use-case errors onto HTTP status codes. Shared by
// every stream route so the mapping cannot drift between handlers.
func writeStreamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "stream not found")
	case errors.Is(err, usecase.ErrStreamForbidden):
		writeError(w, http.StatusForbidden, "not your stream")
	case errors.Is(err, usecase.ErrStreamNotLive):
		writeError(w, http.StatusNotFound, "stream is not live")
	case errors.Is(err, usecase.ErrInvalidStream):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "stream operation failed")
	}
}

// Create registers a new stream owned by the authenticated caller.
func (h *StreamHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	var req dto.CreateStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	stream, err := h.uc.Create(r.Context(), userID, req.Title, req.Description)
	if err != nil {
		writeStreamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.StreamFrom(stream, 0))
}

// List returns every stream, live or not, for the browse screen.
func (h *StreamHandler) List(w http.ResponseWriter, r *http.Request) {
	streams, err := h.uc.List(r.Context())
	if err != nil {
		writeStreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponses(streams))
}

// ListLive returns only streams someone is actually broadcasting right now.
func (h *StreamHandler) ListLive(w http.ResponseWriter, r *http.Request) {
	streams, err := h.uc.ListLive(r.Context())
	if err != nil {
		writeStreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponses(streams))
}

func (h *StreamHandler) Get(w http.ResponseWriter, r *http.Request) {
	stream, err := h.uc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.StreamFrom(stream, h.uc.ListenerCount(stream.ID)))
}

func (h *StreamHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	role, _ := middleware.UserRole(r.Context())

	if err := h.uc.Delete(r.Context(), r.PathValue("id"), userID, role); err != nil {
		writeStreamError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Publish is the native-client broadcast path: a long-lived request whose
// body is read continuously and fanned out to listeners.
//
// The stream goes live when this handler starts and offline when it returns,
// however it returns — normal EOF, network error or client disconnect.
func (h *StreamHandler) Publish(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	role, _ := middleware.UserRole(r.Context())
	streamID := r.PathValue("id")

	hub, err := h.uc.StartLive(r.Context(), streamID, userID, role)
	if err != nil {
		writeStreamError(w, err)
		return
	}
	defer h.stopLive(r.Context(), streamID, userID, role)

	// No early 200 here, deliberately. A conforming HTTP client stops
	// sending the request body as soon as a response arrives — Go's own
	// http.Transport does exactly that — so acknowledging the publish up
	// front would cut the broadcast off after the first chunk. The response
	// is the *result* of the session, written once publishing has ended.
	//
	// Failures before this point (401/403/404) still answer immediately,
	// which is what tells a rejected broadcaster to stop sending.
	published := h.pump(r, hub, streamID)

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, dto.PublishSummary{
		StreamID:       streamID,
		BytesPublished: published,
	})
}

// pump copies the broadcaster's body into the hub until either side ends,
// and reports how many bytes were broadcast.
func (h *StreamHandler) pump(r *http.Request, hub *streaming.Hub, streamID string) int64 {
	var published int64
	buf := make([]byte, publishChunkSize)

	for {
		select {
		case <-r.Context().Done():
			return published
		case <-hub.Done():
			return published
		default:
		}

		n, readErr := r.Body.Read(buf)
		if n > 0 {
			if err := hub.Publish(buf[:n]); err != nil {
				// The hub was closed under us (stream stopped elsewhere).
				return published
			}
			published += int64(n)
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				slog.InfoContext(r.Context(), "publish read ended", "stream_id", streamID, "err", readErr)
			}
			return published
		}
	}
}

// PublishWS is the browser broadcast path. Browsers cannot stream an HTTP
// request body, so web clients send binary audio frames over a WebSocket
// instead; each frame is one chunk for the hub.
func (h *StreamHandler) PublishWS(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	role, _ := middleware.UserRole(r.Context())
	streamID := r.PathValue("id")

	// Authorise before upgrading so failures are still a readable HTTP
	// response rather than an opaque socket close.
	hub, err := h.uc.StartLive(r.Context(), streamID, userID, role)
	if err != nil {
		writeStreamError(w, err)
		return
	}
	defer h.stopLive(r.Context(), streamID, userID, role)

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Same-origin is always allowed; these patterns cover the local dev
		// setup where the web client is served from a different port.
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "websocket accept", "stream_id", streamID, "err", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxWSFrameBytes)

	for {
		select {
		case <-hub.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "stream stopped")
			return
		default:
		}

		_, data, err := conn.Read(r.Context())
		if err != nil {
			// Normal client disconnect, or the request context ended.
			return
		}
		if len(data) == 0 {
			continue
		}
		if err := hub.Publish(data); err != nil {
			return
		}
	}
}

// Chat is a WebSocket endpoint for the live text chat that runs alongside a
// stream's audio. Unlike PublishWS or Listen, this connection is
// bidirectional — it carries the participant's own messages in and every
// participant's messages out — so it runs a dedicated read goroutine
// alongside the handler's own write loop.
//
// Open to any authenticated user, not just the broadcaster (see
// StreamUseCase.JoinChat): chat only needs an identity to attribute
// messages, the same way Listen needs none at all for audio. Accepts the
// JWT as a query parameter for the same reason as PublishWS — the browser
// WebSocket API cannot set headers on an upgrade — via RequireAuthWS.
func (h *StreamHandler) Chat(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	streamID := r.PathValue("id")

	// Authorise and resolve the display name before upgrading, so failures
	// are still a readable HTTP response rather than an opaque socket close
	// — same reasoning as PublishWS.
	hub, username, err := h.uc.JoinChat(r.Context(), streamID, userID)
	if err != nil {
		writeStreamError(w, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		slog.Error("chat websocket accept", "stream_id", streamID, "err", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxChatFrameBytes)

	_, incoming, leave, err := hub.Join()
	if err != nil {
		// The room closed between JoinChat's lookup and the upgrade above
		// (the broadcaster just stopped) — tell the client instead of
		// leaving them hanging on a socket that will never receive anything.
		_ = conn.Close(websocket.StatusNormalClosure, "chat closed")
		return
	}
	defer leave()

	ctx := r.Context()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		h.readChatMessages(ctx, conn, hub, streamID, userID, username)
	}()

	for {
		select {
		case msg, ok := <-incoming:
			if !ok {
				_ = conn.Close(websocket.StatusNormalClosure, "chat closed")
				return
			}
			if err := wsjson.Write(ctx, conn, msg); err != nil {
				return
			}
		case <-hub.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "chat closed")
			return
		case <-readDone:
			// The read side ended: client disconnected, sent a close frame,
			// or a read error occurred.
			return
		case <-ctx.Done():
			return
		}
	}
}

// readChatMessages reads text frames from conn, validates them, and
// publishes valid ones to hub as a ChatMessage attributed to the joined
// participant — never to whatever a malicious client might put in the
// payload, since userID/username come from the server-side JoinChat call,
// not from the incoming frame. Runs until the connection errors or ctx ends.
func (h *StreamHandler) readChatMessages(ctx context.Context, conn *websocket.Conn, hub *streaming.ChatHub, streamID, userID, username string) {
	for {
		var in dto.ChatIncoming
		if err := wsjson.Read(ctx, conn, &in); err != nil {
			return
		}

		text := strings.TrimSpace(in.Text)
		switch {
		case text == "":
			continue
		case utf8.RuneCountInString(text) > maxChatMessageLen:
			_ = wsjson.Write(ctx, conn, dto.ChatErrorFrame{
				Error: fmt.Sprintf("message must be at most %d characters", maxChatMessageLen),
			})
			continue
		}

		msg := streaming.ChatMessage{
			ID:       uuid.NewString(),
			StreamID: streamID,
			UserID:   userID,
			Username: username,
			Text:     text,
			SentAt:   time.Now(),
		}
		if err := hub.Publish(msg); err != nil {
			return
		}
	}
}

// Listen streams a live broadcast back to a listener using chunked transfer
// encoding, which is what lets a mobile audio player treat the response as an
// endless source and start playing immediately.
//
// This route is intentionally public: listening does not require an account.
func (h *StreamHandler) Listen(w http.ResponseWriter, r *http.Request) {
	streamID := r.PathValue("id")

	hub, err := h.uc.LiveHub(streamID)
	if err != nil {
		writeStreamError(w, err)
		return
	}

	_, chunks, unsubscribe, err := hub.Subscribe()
	if err != nil {
		writeStreamError(w, err)
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	// Tells nginx-style proxies not to buffer the response; without it a
	// reverse proxy can hold chunks back and add seconds of latency.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flush(w)

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				// Hub closed: the broadcaster stopped, or we were evicted.
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flush(w)
		case <-r.Context().Done():
			return
		case <-hub.Done():
			return
		}
	}
}

// stopLive ends the live session after the broadcaster is gone. The request
// context is already cancelled at this point, so it runs on a detached one.
func (h *StreamHandler) stopLive(ctx context.Context, streamID, userID, role string) {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopStreamTimeout)
	defer cancel()

	if err := h.uc.StopLive(stopCtx, streamID, userID, role); err != nil {
		slog.ErrorContext(stopCtx, "stop live stream", "stream_id", streamID, "err", err)
	}
}

func (h *StreamHandler) toResponses(streams []entity.Stream) []dto.StreamResponse {
	out := make([]dto.StreamResponse, 0, len(streams))
	for i := range streams {
		out = append(out, dto.StreamFrom(&streams[i], h.uc.ListenerCount(streams[i].ID)))
	}
	return out
}

// flush pushes buffered bytes to the client immediately. Streaming responses
// are useless without it — the audio would arrive in large delayed batches.
// http.NewResponseController works through middleware wrappers, unlike a bare
// type assertion to http.Flusher.
func flush(w http.ResponseWriter) {
	_ = http.NewResponseController(w).Flush()
}
