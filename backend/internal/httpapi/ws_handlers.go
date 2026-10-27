package httpapi

import (
	"net/http"

	"autoauction/internal/auth"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handleLotWS транслирует живые обновления лота (цена, таймер, anti-snipe,
// завершение торгов). Ставки принимаются через REST POST /api/lots/{id}/bids —
// это разделяет "запись" (с авторизацией через Bearer-заголовок) и "чтение"
// (браузерный WebSocket не может отправить произвольные заголовки при
// хендшейке, поэтому токен передаётся в query).
func (s *Server) handleLotWS(w http.ResponseWriter, r *http.Request) {
	lotID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if _, err := auth.ParseToken(s.JWTSecret, token); err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	sub, ok := s.Auction.Subscribe(lotID)
	if !ok {
		http.Error(w, "lot is not active", http.StatusConflict)
		return
	}
	// Регистрируем отписку сразу после успешной подписки — иначе при ошибке
	// Accept ниже подписчик навсегда останется висеть в комнате (см. code review).
	defer sub.Unsubscribe()

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx := conn.CloseRead(r.Context())
	for {
		select {
		case data := <-sub.Messages():
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		case <-sub.Done():
			// Комната завершилась — больше сообщений не будет, даже если
			// последнее ("finished") было пропущено из-за переполненного буфера.
			return
		case <-ctx.Done():
			return
		}
	}
}
