package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"autoauction/internal/auth"
	"autoauction/internal/lot"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type lotRequest struct {
	ListingType     string     `json:"listing_type,omitempty"` // "auction" (по умолчанию) | "fixed_price"
	Brand           string     `json:"brand"`
	Model           string     `json:"model"`
	Year            int        `json:"year"`
	MileageKm       int        `json:"mileage_km"`
	VIN             string     `json:"vin"`
	Condition       string     `json:"condition"`
	Description     string     `json:"description"`
	City            string     `json:"city"`
	Photos          []string   `json:"photos"`
	Videos          []string   `json:"videos"`
	StartPriceKGS   int64      `json:"start_price_kgs"`
	MinStepKGS      int64      `json:"min_step_kgs"`
	ReservePriceKGS *int64     `json:"reserve_price_kgs,omitempty"`
	StartAt         *time.Time `json:"start_at,omitempty"`
	EndAt           *time.Time `json:"end_at,omitempty"`
}

func (req lotRequest) toInput() lot.Input {
	in := lot.Input{
		ListingType:     req.ListingType,
		Brand:           req.Brand,
		Model:           req.Model,
		Year:            req.Year,
		MileageKm:       req.MileageKm,
		VIN:             req.VIN,
		Condition:       req.Condition,
		Description:     req.Description,
		City:            req.City,
		Photos:          req.Photos,
		Videos:          req.Videos,
		StartPriceKGS:   req.StartPriceKGS,
		MinStepKGS:      req.MinStepKGS,
		ReservePriceKGS: req.ReservePriceKGS,
		StartAt:         req.StartAt,
		EndAt:           req.EndAt,
	}
	// У витрины (ListingTypeFixedPrice) нет аукциона — эти поля не относятся
	// к делу и не проверяются validate(), поэтому отбрасываем их здесь, а не
	// доверяем клиенту их не присылать (см. code review).
	if req.ListingType == lot.ListingTypeFixedPrice {
		in.MinStepKGS = 0
		in.ReservePriceKGS = nil
		in.StartAt = nil
		in.EndAt = nil
	}
	return in
}

// validate — для витрины (ListingTypeFixedPrice) не требует шаг ставки и даты
// торгов: там нет самого аукциона, только цена и контакты продавца.
func (req lotRequest) validate() string {
	if req.Brand == "" || req.Model == "" {
		return "brand and model are required"
	}
	if req.Year < 1950 || req.Year > time.Now().Year()+1 {
		return "invalid year"
	}
	if req.StartPriceKGS <= 0 {
		return "start_price_kgs must be positive"
	}
	if req.ListingType != "" && req.ListingType != lot.ListingTypeAuction && req.ListingType != lot.ListingTypeFixedPrice {
		return "invalid listing_type"
	}
	if req.ListingType == lot.ListingTypeFixedPrice {
		return ""
	}
	if req.MinStepKGS <= 0 {
		return "min_step_kgs must be positive"
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
	Videos          *[]string  `json:"videos"`
	StartPriceKGS   *int64     `json:"start_price_kgs"`
	MinStepKGS      *int64     `json:"min_step_kgs"`
	ReservePriceKGS *int64     `json:"reserve_price_kgs"`
	StartAt         *time.Time `json:"start_at"`
	EndAt           *time.Time `json:"end_at"`
}

// mergeOnto строит полную lotRequest из текущего лота, подставляя присланные
// поля патча — так итоговое состояние можно провалидировать тем же validate(),
// что и при создании, даже если патч прислал только часть полей. ListingType
// всегда берётся из существующего лота — тип объявления патчем не меняется.
func (p lotPatchRequest) mergeOnto(existing *lot.Lot) lotRequest {
	merged := lotRequest{
		ListingType: existing.ListingType,
		Brand:       existing.Brand, Model: existing.Model, Year: existing.Year,
		MileageKm: existing.MileageKm, VIN: existing.VIN, Condition: existing.Condition,
		Description: existing.Description, City: existing.City, Photos: existing.Photos,
		Videos:        existing.Videos,
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
	if p.Videos != nil {
		merged.Videos = *p.Videos
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

// toPatch принимает тип объявления существующего лота (его нельзя сменить
// патчем — см. mergeOnto), чтобы отбросить auction-only поля для витрины:
// иначе PATCH на лот с ListingTypeFixedPrice мог бы задним числом записать
// reserve_price_kgs/start_at/end_at/min_step_kgs, которые validate() для
// витрины не проверяет (см. code review).
func (p lotPatchRequest) toPatch(listingType string) lot.Patch {
	patch := lot.Patch{
		Brand: p.Brand, Model: p.Model, Year: p.Year, MileageKm: p.MileageKm,
		VIN: p.VIN, Condition: p.Condition, Description: p.Description, City: p.City,
		Photos: p.Photos, Videos: p.Videos, StartPriceKGS: p.StartPriceKGS, MinStepKGS: p.MinStepKGS,
		ReservePriceKGS: p.ReservePriceKGS, StartAt: p.StartAt, EndAt: p.EndAt,
	}
	if listingType == lot.ListingTypeFixedPrice {
		patch.MinStepKGS = nil
		patch.ReservePriceKGS = nil
		patch.StartAt = nil
		patch.EndAt = nil
	}
	return patch
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

// lotResponse — обычный Lot плюс контакты продавца, видимые только для
// активной/проданной витрины с фиксированной ценой (ListingTypeFixedPrice):
// там нет торгов и приватности до завершения, которая есть у аукциона —
// покупатель должен сразу видеть, кому звонить.
type lotResponse struct {
	*lot.Lot
	SellerContact *sellerContact `json:"seller_contact,omitempty"`
}

type sellerContact struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

func (s *Server) handleGetLot(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	l, ok := s.loadViewableLot(w, r, id)
	if !ok {
		return
	}

	resp := lotResponse{Lot: l}
	showContact := l.ListingType == lot.ListingTypeFixedPrice &&
		(l.Status == lot.StatusActive || l.Status == lot.StatusFinishedSold)
	if showContact {
		if seller, err := s.Users.GetByID(r.Context(), l.SellerID); err == nil {
			resp.SellerContact = &sellerContact{Name: seller.Name, Phone: seller.Phone}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleMarkSold — продавец отмечает свою витрину проданной (ТЗ-дополнение:
// покупка вне платформы, тут только фиксация факта). Только для fixed_price.
func (s *Server) handleMarkSold(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := s.Lots.MarkSold(r.Context(), id, userID); err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": lot.StatusFinishedSold})
}

// canViewLot — черновики/лоты на модерации/отклонённые видны только продавцу
// и админу; активные и завершённые — всем авторизованным пользователям.
func canViewLot(l *lot.Lot, userID uuid.UUID, role string) bool {
	isOwnerOrAdmin := l.SellerID == userID || role == "admin"
	isPublicStatus := l.Status == lot.StatusActive || l.Status == lot.StatusFinishedSold || l.Status == lot.StatusFinishedUnsold
	return isOwnerOrAdmin || isPublicStatus
}

// loadViewableLot — общая точка для "загрузить лот + проверить canViewLot",
// используемая всеми хендлерами, которым нужен доступ к чужому лоту
// (сам лот, его ставки, добавление в избранное). Раньше каждый хендлер
// копировал эту пятистрочную последовательность отдельно, и уже успел
// разойтись (один из них сравнивал ошибку не с тем сентинелом) — см. code review.
// ok=false значит, что ответ уже записан и вызывающий хендлер должен выйти.
func (s *Server) loadViewableLot(w http.ResponseWriter, r *http.Request, id uuid.UUID) (*lot.Lot, bool) {
	l, err := s.Lots.GetByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err, lot.ErrForbidden)
		return nil, false
	}
	userID := auth.UserIDFromContext(r.Context())
	if !canViewLot(l, userID, auth.RoleFromContext(r.Context())) {
		writeError(w, http.StatusForbidden, "not allowed to view this lot")
		return nil, false
	}
	return l, true
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

// maxOffset — потолок пагинации публичного каталога, чтобы ?offset=<огромное
// число> не заставлял Postgres сканировать и отбрасывать миллионы строк
// без авторизации (см. code review).
const maxOffset = 100_000

// handleListActiveLots — публичный каталог. Поддерживает фильтры query-параметрами:
// brand, model, city (подстрока, регистронезависимо), year_min/year_max,
// price_min_kgs/price_max_kgs (по текущей цене), limit/offset (пагинация).
func (s *Server) handleListActiveLots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	listingType := q.Get("type")
	if listingType != "" && listingType != lot.ListingTypeAuction && listingType != lot.ListingTypeFixedPrice {
		writeError(w, http.StatusBadRequest, "invalid type")
		return
	}
	f := lot.Filter{
		ListingType: listingType,
		Brand:       q.Get("brand"),
		Model:       q.Get("model"),
		City:        q.Get("city"),
	}
	minYear, maxYear := 1900, time.Now().Year()+1
	if v, ok := parseQueryInt(q, "year_min"); ok {
		if v < minYear || v > maxYear {
			writeError(w, http.StatusBadRequest, "year_min out of range")
			return
		}
		f.YearMin = &v
	}
	if v, ok := parseQueryInt(q, "year_max"); ok {
		if v < minYear || v > maxYear {
			writeError(w, http.StatusBadRequest, "year_max out of range")
			return
		}
		f.YearMax = &v
	}
	if v, ok := parseQueryInt64(q, "price_min_kgs"); ok {
		f.PriceMinKGS = &v
	}
	if v, ok := parseQueryInt64(q, "price_max_kgs"); ok {
		f.PriceMaxKGS = &v
	}

	f.Limit = 20 // дефолтный размер страницы каталога
	if v, ok := parseQueryInt(q, "limit"); ok {
		f.Limit = v
	}
	if f.Limit <= 0 {
		f.Limit = 20
	} else if f.Limit > 100 {
		f.Limit = 100 // клэмп до максимума, а не сброс к дефолту (см. code review)
	}

	if v, ok := parseQueryInt(q, "offset"); ok {
		f.Offset = v
	}
	if f.Offset < 0 {
		f.Offset = 0
	} else if f.Offset > maxOffset {
		f.Offset = maxOffset // защита от неограниченного OFFSET на публичном эндпоинте
	}

	lots, err := s.Lots.ListActive(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list lots")
		return
	}
	writeJSON(w, http.StatusOK, lots)
}

func parseQueryInt(q url.Values, key string) (int, bool) {
	v := q.Get(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseQueryInt64(q url.Values, key string) (int64, bool) {
	v := q.Get(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
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

	l, err := s.Lots.Update(r.Context(), id, userID, req.toPatch(existing.ListingType))
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
