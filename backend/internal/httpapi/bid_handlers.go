package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"autoauction/internal/auction"
	"autoauction/internal/auth"
	"autoauction/internal/lot"
)

type placeBidRequest struct {
	AmountKGS int64 `json:"amount_kgs"`
}

// maxBidKGS — защитный потолок на ставку. Не экономическое ограничение, а
// страховка от переполнения int64 в Room.handleBid при сравнении
// currentPriceKGS+minStepKGS (см. code review): 1 трлн сом с огромным запасом
// ниже math.MaxInt64 даже после прибавления любого реалистичного шага ставки.
const maxBidKGS int64 = 1_000_000_000_000

func (s *Server) handlePlaceBid(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())

	var req placeBidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AmountKGS <= 0 || req.AmountKGS > maxBidKGS {
		writeError(w, http.StatusBadRequest, "amount_kgs must be positive and realistic")
		return
	}

	outcome, err := s.Auction.PlaceBid(r.Context(), id, userID, req.AmountKGS)
	if err != nil {
		switch {
		case errors.Is(err, auction.ErrLotNotActive):
			writeError(w, http.StatusConflict, "lot is not active")
		case errors.Is(err, auction.ErrSellerCannotBid):
			writeError(w, http.StatusForbidden, "seller cannot bid on their own lot")
		case errors.Is(err, auction.ErrBidTooLow):
			writeError(w, http.StatusBadRequest, "bid must be at least current price plus the minimum step")
		default:
			writeError(w, http.StatusInternalServerError, "could not place bid")
		}
		return
	}
	writeJSON(w, http.StatusOK, outcome)
}

func (s *Server) handleListBids(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	// Та же проверка видимости, что и у самого лота (handleGetLot) — иначе
	// историю ставок по приватному draft/pending лоту мог увидеть кто угодно.
	l, err := s.Lots.GetByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if !canViewLot(l, userID, auth.RoleFromContext(r.Context())) {
		writeError(w, http.StatusForbidden, "not allowed to view this lot")
		return
	}

	bids, err := s.Bids.ListByLot(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list bids")
		return
	}
	writeJSON(w, http.StatusOK, bids)
}
