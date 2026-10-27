package httpapi

import (
	"net/http"
	"strings"

	"autoauction/internal/auth"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// wsOriginPatterns переводит CORSOrigins ("http://localhost:3000") в формат
// websocket.AcceptOptions.OriginPatterns, который матчится против хоста без схемы.
func (s *Server) wsOriginPatterns() []string {
	patterns := make([]string, 0, len(s.CORSOrigins))
	for _, o := range s.CORSOrigins {
		o = strings.TrimPrefix(o, "http://")
		o = strings.TrimPrefix(o, "https://")
		if o != "" {
			patterns = append(patterns, o)
		}
	}
	return patterns
}

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
	claims, err := auth.ParseToken(s.JWTSecret, token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	// Этот маршрут живёт вне группы с requireActiveUser (ws не проходит через
	// обычную цепочку middleware), поэтому блокировку проверяем здесь явно —
	// иначе заблокированный пользователь мог бы продолжать смотреть живые
	// торги по старому токену (см. code review).
	if blocked, err := s.Users.IsBlocked(r.Context(), claims.UserID); err != nil || blocked {
		http.Error(w, "account is blocked", http.StatusForbidden)
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

	// websocket.Accept по умолчанию принимает только одноимённые (same-origin)
	// запросы — иначе отклоняет апгрейд без явного CORS-preflight (WS его не
	// использует). Фронтенд в деве живёт на другом порту (3000 vs 8080),
	// поэтому разрешаем те же origin'ы, что и обычным HTTP-запросам.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.wsOriginPatterns(),
	})
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
