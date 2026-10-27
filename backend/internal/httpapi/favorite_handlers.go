package httpapi

import (
	"net/http"

	"autoauction/internal/auth"
)

func (s *Server) handleAddFavorite(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, ok := s.loadViewableLot(w, r, id); !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := s.Favorites.Add(r.Context(), userID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not add favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveFavorite(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := s.Favorites.Remove(r.Context(), userID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListFavorites(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	q := r.URL.Query()
	limit := 20
	if v, ok := parseQueryInt(q, "limit"); ok {
		limit = v
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	offset := 0
	if v, ok := parseQueryInt(q, "offset"); ok {
		offset = v
	}
	if offset < 0 {
		offset = 0
	} else if offset > maxOffset {
		offset = maxOffset
	}

	lots, err := s.Lots.ListFavoritedBy(r.Context(), userID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list favorites")
		return
	}
	writeJSON(w, http.StatusOK, lots)
}
