package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"autoauction/internal/auction"
	"autoauction/internal/auth"
	"autoauction/internal/bid"
	"autoauction/internal/lot"
	"autoauction/internal/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	JWTSecret string
	JWTTTL    time.Duration
	Users     *user.Repo
	Lots      *lot.Repo
	Bids      *bid.Repo
	Auction   *auction.Manager
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)

		r.Get("/lots", s.handleListActiveLots) // публичный каталог (Этап 2 добавит фильтры)
		r.Get("/ws/lots/{id}", s.handleLotWS)  // живая трансляция; авторизация через ?token=

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.JWTSecret))

			r.Get("/me", s.handleGetMe)
			r.Patch("/me", s.handleUpdateMe)

			r.Post("/lots", s.handleCreateLot)
			r.Get("/lots/mine", s.handleListMyLots)
			r.Get("/lots/{id}", s.handleGetLot)
			r.Patch("/lots/{id}", s.handleUpdateLot)
			r.Post("/lots/{id}/submit", s.handleSubmitLot)
			r.Delete("/lots/{id}", s.handleDeleteLot)
			r.Get("/lots/{id}/bids", s.handleListBids)
			r.Post("/lots/{id}/bids", s.handlePlaceBid)

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				r.Get("/admin/lots/pending", s.handleListPendingLots)
				r.Post("/admin/lots/{id}/approve", s.handleApproveLot)
				r.Post("/admin/lots/{id}/reject", s.handleRejectLot)
			})
		})
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
