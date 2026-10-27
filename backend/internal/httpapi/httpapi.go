package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"autoauction/internal/auction"
	"autoauction/internal/auth"
	"autoauction/internal/bid"
	"autoauction/internal/favorite"
	"autoauction/internal/lot"
	"autoauction/internal/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Server struct {
	JWTSecret   string
	JWTTTL      time.Duration
	CORSOrigins []string // браузерный фронтенд (localhost:3000 в деве) ходит сюда напрямую
	UploadsDir  string   // локальный диск для фото/видео лотов (см. handleUpload); позже меняется на S3/R2
	Users       *user.Repo
	Lots        *lot.Repo
	Bids        *bid.Repo
	Favorites   *favorite.Repo
	Auction     *auction.Manager
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Раздача загруженных фото/видео — публично, без /api, как обычные статические
	// файлы (см. handleUpload). Имя файла — случайный UUID, обхода пути нет.
	// Листинг каталога отключён через noDirListingFS — иначе весь список
	// файлов (включая приватные черновики) было бы видно без авторизации.
	if s.UploadsDir != "" {
		fs := http.StripPrefix("/uploads/", http.FileServer(noDirListingFS{http.Dir(s.UploadsDir)}))
		r.Handle("/uploads/*", fs)
	}

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)

		r.Get("/lots", s.handleListActiveLots) // публичный каталог: фильтры brand/model/city/year_*/price_*_kgs, пагинация limit/offset
		r.Get("/ws/lots/{id}", s.handleLotWS)  // живая трансляция; авторизация через ?token=

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.JWTSecret))
			r.Use(s.requireActiveUser) // блокировка действует и на уже открытые сессии

			r.Get("/me", s.handleGetMe)
			r.Patch("/me", s.handleUpdateMe)

			r.Post("/uploads", s.handleUpload)

			r.Post("/lots", s.handleCreateLot)
			r.Get("/lots/mine", s.handleListMyLots)
			r.Get("/lots/{id}", s.handleGetLot)
			r.Patch("/lots/{id}", s.handleUpdateLot)
			r.Post("/lots/{id}/submit", s.handleSubmitLot)
			r.Post("/lots/{id}/mark-sold", s.handleMarkSold)
			r.Delete("/lots/{id}", s.handleDeleteLot)
			r.Get("/lots/{id}/bids", s.handleListBids)
			r.With(s.rateLimitBids).Post("/lots/{id}/bids", s.handlePlaceBid)
			r.Post("/lots/{id}/favorite", s.handleAddFavorite)
			r.Delete("/lots/{id}/favorite", s.handleRemoveFavorite)
			r.Get("/favorites", s.handleListFavorites)

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				r.Get("/admin/lots/pending", s.handleListPendingLots)
				r.Post("/admin/lots/{id}/approve", s.handleApproveLot)
				r.Post("/admin/lots/{id}/reject", s.handleRejectLot)
				r.Post("/admin/lots/{id}/cancel", s.handleCancelLot)
				r.Post("/admin/users/{id}/block", s.handleBlockUser)
				r.Post("/admin/users/{id}/unblock", s.handleUnblockUser)
				r.Get("/admin/stats", s.handleAdminStats)
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
