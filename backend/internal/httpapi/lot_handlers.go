package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"autoauction/internal/auth"
	"autoauction/internal/lot"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type lotRequest struct {
	Brand           string     `json:"brand"`
	Model           string     `json:"model"`
	Year            int        `json:"year"`
	MileageKm       int        `json:"mileage_km"`
	VIN             string     `json:"vin"`
	Condition       string     `json:"condition"`
	Description     string     `json:"description"`
	City            string     `json:"city"`
	Photos          []string   `json:"photos"`
	StartPriceKGS   int64      `json:"start_price_kgs"`
	MinStepKGS      int64      `json:"min_step_kgs"`
	ReservePriceKGS *int64     `json:"reserve_price_kgs,omitempty"`
	StartAt         *time.Time `json:"start_at,omitempty"`
	EndAt           *time.Time `json:"end_at,omitempty"`
}

func (req lotRequest) toInput() lot.Input {
	return lot.Input{
		Brand:           req.Brand,
		Model:           req.Model,
		Year:            req.Year,
		MileageKm:       req.MileageKm,
		VIN:             req.VIN,
		Condition:       req.Condition,
		Description:     req.Description,
		City:            req.City,
		Photos:          req.Photos,
		StartPriceKGS:   req.StartPriceKGS,
		MinStepKGS:      req.MinStepKGS,
		ReservePriceKGS: req.ReservePriceKGS,
		StartAt:         req.StartAt,
		EndAt:           req.EndAt,
	}
}

func (req lotRequest) validate() string {
	if req.Brand == "" || req.Model == "" {
		return "brand and model are required"
	}
	if req.Year < 1950 || req.Year > time.Now().Year()+1 {
		return "invalid year"
	}
	if req.StartPriceKGS <= 0 || req.MinStepKGS <= 0 {
		return "start_price_kgs and min_step_kgs must be positive"
	}
	if req.StartAt == nil || req.EndAt == nil {
		return "start_at and end_at are required"
	}
	if req.ReservePriceKGS != nil && *req.ReservePriceKGS < req.StartPriceKGS {
		return "reserve_price_kgs cannot be lower than start_price_kgs"
	}
	if req.StartAt != nil && req.EndAt != nil && !req.EndAt.After(*req.StartAt) {
		return "end_at must be after start_at"
	}
	return ""
}

// lotPatchRequest — тело PATCH /api/lots/{id}. В отличие от lotRequest (создание,
// требует все поля), здесь все поля — указатели: отсутствующее в JSON поле
// остаётся nil и не трогает текущее значение в БД (см. lot.Repo.Update и code review:
// раньше PATCH без reserve_price_kgs/vin/... тихо стирал их).
type lotPatchRequest struct {
	Brand           *string    `json:"brand"`
	Model           *string    `json:"model"`
	Year            *int       `json:"year"`
	MileageKm       *int       `json:"mileage_km"`
	VIN             *string    `json:"vin"`
	Condition       *string    `json:"condition"`
	Description     *string    `json:"description"`
	City            *string    `json:"city"`
	Photos          *[]string  `json:"photos"`
	StartPriceKGS   *int64     `json:"start_price_kgs"`
	MinStepKGS      *int64     `json:"min_step_kgs"`
	ReservePriceKGS *int64     `json:"reserve_price_kgs"`
	StartAt         *time.Time `json:"start_at"`
	EndAt           *time.Time `json:"end_at"`
}

// mergeOnto строит полную lotRequest из текущего лота, подставляя присланные
// поля патча — так итоговое состояние можно провалидировать тем же validate(),
// что и при создании, даже если патч прислал только часть полей.
func (p lotPatchRequest) mergeOnto(existing *lot.Lot) lotRequest {
	merged := lotRequest{
		Brand: existing.Brand, Model: existing.Model, Year: existing.Year,
		MileageKm: existing.MileageKm, VIN: existing.VIN, Condition: existing.Condition,
		Description: existing.Description, City: existing.City, Photos: existing.Photos,
		StartPriceKGS: existing.StartPriceKGS, MinStepKGS: existing.MinStepKGS,
		ReservePriceKGS: existing.ReservePriceKGS, StartAt: existing.StartAt, EndAt: existing.EndAt,
	}
	if p.Brand != nil {
		merged.Brand = *p.Brand
	}
	if p.Model != nil {
		merged.Model = *p.Model
	}
	if p.Year != nil {
		merged.Year = *p.Year
	}
	if p.MileageKm != nil {
		merged.MileageKm = *p.MileageKm
	}
	if p.VIN != nil {
		merged.VIN = *p.VIN
	}
	if p.Condition != nil {
		merged.Condition = *p.Condition
	}
	if p.Description != nil {
		merged.Description = *p.Description
	}
	if p.City != nil {
		merged.City = *p.City
	}
	if p.Photos != nil {
		merged.Photos = *p.Photos
	}
	if p.StartPriceKGS != nil {
		merged.StartPriceKGS = *p.StartPriceKGS
	}
	if p.MinStepKGS != nil {
		merged.MinStepKGS = *p.MinStepKGS
	}
	if p.ReservePriceKGS != nil {
		merged.ReservePriceKGS = p.ReservePriceKGS
	}
	if p.StartAt != nil {
		merged.StartAt = p.StartAt
	}
	if p.EndAt != nil {
		merged.EndAt = p.EndAt
	}
	return merged
}

func (p lotPatchRequest) toPatch() lot.Patch {
	return lot.Patch{
		Brand: p.Brand, Model: p.Model, Year: p.Year, MileageKm: p.MileageKm,
		VIN: p.VIN, Condition: p.Condition, Description: p.Description, City: p.City,
		Photos: p.Photos, StartPriceKGS: p.StartPriceKGS, MinStepKGS: p.MinStepKGS,
		ReservePriceKGS: p.ReservePriceKGS, StartAt: p.StartAt, EndAt: p.EndAt,
	}
}

func (s *Server) handleCreateLot(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	var req lotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	l, err := s.Lots.Create(r.Context(), userID, req.toInput())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create lot")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleGetLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	l, err := s.Lots.GetByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err, lot.ErrNotFound)
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if !canViewLot(l, userID, auth.RoleFromContext(r.Context())) {
		writeError(w, http.StatusForbidden, "not allowed to view this lot")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// canViewLot — черновики/лоты на модерации/отклонённые видны только продавцу
// и админу; активные и завершённые — всем авторизованным пользователям
// (используется и для самого лота, и для его истории ставок).
func canViewLot(l *lot.Lot, userID uuid.UUID, role string) bool {
	isOwnerOrAdmin := l.SellerID == userID || role == "admin"
	isPublicStatus := l.Status == lot.StatusActive || l.Status == lot.StatusFinishedSold || l.Status == lot.StatusFinishedUnsold
	return isOwnerOrAdmin || isPublicStatus
}

func (s *Server) handleListMyLots(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	lots, err := s.Lots.ListBySeller(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list lots")
		return
	}
	writeJSON(w, http.StatusOK, lots)
}

func (s *Server) handleListActiveLots(w http.ResponseWriter, r *http.Request) {
	lots, err := s.Lots.ListActive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list lots")
		return
	}
	writeJSON(w, http.StatusOK, lots)
}

func (s *Server) handleUpdateLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	var req lotPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	existing, err := s.Lots.GetByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	if existing.SellerID != userID {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}

	if msg := req.mergeOnto(existing).validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	l, err := s.Lots.Update(r.Context(), id, userID, req.toPatch())
	if err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleSubmitLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := s.Lots.Submit(r.Context(), id, userID); err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": lot.StatusPending})
}

func (s *Server) handleDeleteLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := s.Lots.Delete(r.Context(), id, userID); err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListPendingLots(w http.ResponseWriter, r *http.Request) {
	lots, err := s.Lots.ListPending(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list lots")
		return
	}
	writeJSON(w, http.StatusOK, lots)
}

func (s *Server) handleApproveLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	l, err := s.Lots.Approve(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	s.Auction.StartRoom(l)
	writeJSON(w, http.StatusOK, l)
}

type rejectRequest struct {
	Note string `json:"note"`
}

func (s *Server) handleRejectLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req rejectRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.Lots.Reject(r.Context(), id, req.Note); err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": lot.StatusRejected})
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func writeNotFoundOrError(w http.ResponseWriter, err, forbiddenSentinel error) {
	switch {
	case errors.Is(err, lot.ErrNotFound):
		writeError(w, http.StatusNotFound, "lot not found")
	case errors.Is(err, forbiddenSentinel):
		writeError(w, http.StatusForbidden, "not allowed")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
