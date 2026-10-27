package httpapi

import (
	"encoding/json"
	"net/http"

	"autoauction/internal/auth"
)

// Указатели: отсутствующее в JSON поле остаётся nil и не стирает текущее
// значение (см. user.Repo.UpdateProfile и code review).
type updateMeRequest struct {
	Name      *string `json:"name"`
	Phone     *string `json:"phone"`
	City      *string `json:"city"`
	AvatarURL *string `json:"avatar_url"`
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	u, err := s.Users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.Users.UpdateProfile(r.Context(), userID, req.Name, req.Phone, req.City, req.AvatarURL); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update profile")
		return
	}
	u, err := s.Users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
