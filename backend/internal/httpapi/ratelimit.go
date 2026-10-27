package httpapi

import (
	"net/http"
	"sync"

	"autoauction/internal/auth"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// Защита от шквала ставок с одного аккаунта (бот/скрипт) — не от гонок между
// разными пользователями: те уже корректно сериализуются внутри auction.Room.
// Это отдельная, более грубая защита на уровне HTTP: ограничивает скорость
// запросов на запись одного актора независимо от того, валидны ли сами ставки
// по цене (ТЗ раздел 5, "защита от скриптового перебития").
const (
	bidRateLimit = 5  // ставок в секунду на пользователя
	bidRateBurst = 10 // всплеск
)

// rateLimitBids регистрируется один раз на маршрут при сборке роутера — карта
// лимитеров живёт на всё время работы сервера, по одному лимитеру на пользователя.
func (s *Server) rateLimitBids(next http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[uuid.UUID]*rate.Limiter)

	limiterFor := func(userID uuid.UUID) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := limiters[userID]
		if !ok {
			l = rate.NewLimiter(rate.Limit(bidRateLimit), bidRateBurst)
			limiters[userID] = l
		}
		return l
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromContext(r.Context())
		if !limiterFor(userID).Allow() {
			writeError(w, http.StatusTooManyRequests, "too many bids, slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}
