package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/streampulse/backend/internal/application/dto"
	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/repository"
)

// AdminHandler serves the admin-only endpoints. Routes are mounted behind
// RequireAuth + RequireAdmin, so every caller here is a verified admin.
type AdminHandler struct {
	uc *usecase.AdminUseCase
}

func NewAdminHandler(uc *usecase.AdminUseCase) *AdminHandler {
	return &AdminHandler{uc: uc}
}

func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	s, err := h.uc.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats failed")
		return
	}
	writeJSON(w, http.StatusOK, dto.StatsResponse{
		TotalUsers:        s.TotalUsers,
		TotalRegular:      s.RegularUsers,
		TotalBroadcasters: s.Broadcasters,
		TotalAdmins:       s.Admins,
	})
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	users, err := h.uc.ListUsers(r.Context(), offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	out := make([]dto.UserResponse, len(users))
	for i := range users {
		out[i] = dto.UserFrom(&users[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AdminHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	var req dto.UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := h.uc.UpdateRole(r.Context(), r.PathValue("id"), req.Role)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, usecase.ErrInvalidRole):
		writeError(w, http.StatusBadRequest, "invalid role (expected user, broadcaster or admin)")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "update failed")
	default:
		writeJSON(w, http.StatusOK, dto.UserFrom(u))
	}
}
