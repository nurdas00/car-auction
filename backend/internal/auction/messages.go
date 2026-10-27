package auction

import "time"

// Сообщения, транслируемые комнатой всем подписчикам по WebSocket.
// Формат ставок и цены — в сомах (KGS), см. ТЗ, раздел 2.

type stateMessage struct {
	Type            string    `json:"type"` // "state" — снимок при подключении
	CurrentPriceKGS int64     `json:"current_price_kgs"`
	EndAt           time.Time `json:"end_at"`
}

type bidAcceptedMessage struct {
	Type            string    `json:"type"` // "bid_accepted"
	CurrentPriceKGS int64     `json:"current_price_kgs"`
	EndAt           time.Time `json:"end_at"`
	Extended        bool      `json:"extended"` // anti-snipe продлил торги
}

type finishedMessage struct {
	Type          string `json:"type"` // "finished"
	Status        string `json:"status"`
	FinalPriceKGS int64  `json:"final_price_kgs"`
	Sold          bool   `json:"sold"`
}
