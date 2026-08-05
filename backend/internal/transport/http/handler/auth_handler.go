// Package handler holds stdlib net/http handlers.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

type AuthHandler struct {
	uc *usecase.AuthUseCase
}

func NewAuthHandler(uc *usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{uc: uc}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Username == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email, username and a password of at least 8 characters are required")
		return
	}

	user, token, err := h.uc.Register(r.Context(), req.Email, req.Username, req.Password)
	switch {
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "email already registered")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "registration failed")
	default:
		writeJSON(w, http.StatusCreated, dto.AuthResponse{User: dto.UserFrom(user), Token: token})
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, token, err := h.uc.Login(r.Context(), req.Email, req.Password)
	switch {
	case errors.Is(err, usecase.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid credentials")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "login failed")
	default:
		writeJSON(w, http.StatusOK, dto.AuthResponse{User: dto.UserFrom(user), Token: token})
	}
}

// Refresh requires a currently-valid JWT (see middleware.RequireAuth on the
// route) and re-issues a fresh one for the same user.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	user, token, err := h.uc.Refresh(r.Context(), userID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "refresh failed")
	default:
		writeJSON(w, http.StatusOK, dto.AuthResponse{User: dto.UserFrom(user), Token: token})
	}
}
