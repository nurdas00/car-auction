package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"autoauction/internal/auction"
	"autoauction/internal/lot"
	"autoauction/internal/user"
)

func (s *Server) handleBlockUser(w http.ResponseWriter, r *http.Request) {
	s.setUserBlocked(w, r, true)
}

func (s *Server) handleUnblockUser(w http.ResponseWriter, r *http.Request) {
	s.setUserBlocked(w, r, false)
}

func (s *Server) setUserBlocked(w http.ResponseWriter, r *http.Request, blocked bool) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := s.Users.SetBlocked(r.Context(), id, blocked); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not update user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"is_blocked": blocked})
}

// handleCancelLot — админ снимает активный лот с публикации (ТЗ раздел 4.6).
// В отличие от Reject (который отклоняет лот ещё на модерации), это
// останавливает уже идущие торги.
func (s *Server) handleCancelLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := s.Auction.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, auction.ErrLotNotActive) {
			// ErrLotNotActive покрывает и "уже не существует", и "уже завершился
			// сам по таймеру буквально в эту секунду" (гонка с Room.run), и
			// "уже отменён" — подтягиваем текущий статус, чтобы админ видел
			// причину, а не одно и то же обезличенное сообщение (см. code review).
			if l, lerr := s.Lots.GetByID(r.Context(), id); lerr == nil {
				writeError(w, http.StatusConflict, fmt.Sprintf("lot is not active (current status: %s)", l.Status))
				return
			}
			writeError(w, http.StatusConflict, "lot is not active")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not cancel lot")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": lot.StatusCancelled})
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.Lots.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	totalBids, err := s.Bids.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		lot.Stats
		TotalBids int64 `json:"total_bids"`
	}{Stats: stats, TotalBids: totalBids})
}
