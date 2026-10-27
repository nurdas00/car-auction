package httpapi

import (
	"errors"
	"log"
	"net/http"

	"autoauction/internal/auth"
	"autoauction/internal/user"
)

// requireActiveUser — ставится сразу после auth.RequireAuth. JWT не несёт
// живого статуса блокировки (claims фиксируются на момент выдачи токена),
// поэтому блокировку уже открытой сессии проверяем отдельным запросом к БД:
// иначе заблокированный пользователь мог бы действовать до истечения JWT TTL
// (тот же компромисс устаревания claim'а принят и для роли в auth.RequireAdmin).
func (s *Server) requireActiveUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromContext(r.Context())
		blocked, err := s.Users.IsBlocked(r.Context(), userID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				writeError(w, http.StatusUnauthorized, "invalid session")
				return
			}
			// Отличаем "сбой БД" от "невалидная сессия" (см. code review) —
			// иначе временная проблема с БД выглядела бы как массовый logout
			// без единой строчки в логе, объясняющей причину.
			log.Printf("httpapi: requireActiveUser lookup for %s: %v", userID, err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if blocked {
			writeError(w, http.StatusForbidden, "account is blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}
