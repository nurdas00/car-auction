package auction

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"autoauction/internal/bid"
	"autoauction/internal/lot"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Anti-snipe (ТЗ, раздел 4.3): ставка в последние AntiSnipeWindow до конца торгов
// продлевает их на AntiSnipeExtension.
const (
	AntiSnipeWindow    = 30 * time.Second
	AntiSnipeExtension = 30 * time.Second
)

type subscriber struct {
	send chan []byte
	room *Room
}

// Messages — канал входящих трансляций для этого подписчика.
func (s *subscriber) Messages() <-chan []byte {
	return s.send
}

// Done закрывается, когда комната завершает работу — это надёжный сигнал для
// подписчика закрыть соединение, даже если он не получил итоговое сообщение
// "finished" (например, его буфер был переполнен в момент рассылки).
func (s *subscriber) Done() <-chan struct{} {
	return s.room.done
}

// Unsubscribe отписывает подключение от комнаты. Безопасно вызывать после того,
// как комната уже завершилась.
func (s *subscriber) Unsubscribe() {
	select {
	case s.room.unsubscribeCh <- s:
	case <-s.room.done:
	}
}

type bidRequest struct {
	bidderID  uuid.UUID
	amountKGS int64
	resp      chan bidResult
}

type bidResult struct {
	currentPriceKGS int64
	endAt           time.Time
	extended        bool
	err             error
}

// Room — один активный лот = одна goroutine. Все ставки по лоту проходят через
// bidCh и обрабатываются строго последовательно, поэтому конкурентные ставки
// не создают гонки без явных блокировок БД (см. ТЗ, раздел 7-8).
type Room struct {
	lotID           uuid.UUID
	sellerID        uuid.UUID
	minStepKGS      int64
	reservePriceKGS *int64
	currentPriceKGS int64
	endAt           time.Time
	hadBid          bool

	bidCh         chan bidRequest
	subscribeCh   chan *subscriber
	unsubscribeCh chan *subscriber
	stateCh       chan chan []byte

	pool *pgxpool.Pool
	lots *lot.Repo
	bids *bid.Repo

	done       chan struct{}
	onFinished func(uuid.UUID)
}

func newRoom(l *lot.Lot, pool *pgxpool.Pool, lots *lot.Repo, bids *bid.Repo, onFinished func(uuid.UUID)) *Room {
	current := l.StartPriceKGS
	hadBid := false
	if l.CurrentPriceKGS != nil {
		current = *l.CurrentPriceKGS
		// Approve() выставляет current_price_kgs = start_price_kgs и шаг ставки
		// всегда положителен, так что первая же ставка строго больше стартовой
		// цены — расхождение с ней надёжно означает "ставка уже была".
		hadBid = current != l.StartPriceKGS
	}
	endAt := time.Now().Add(24 * time.Hour)
	if l.EndAt != nil {
		endAt = *l.EndAt
	}
	return &Room{
		lotID:           l.ID,
		sellerID:        l.SellerID,
		minStepKGS:      l.MinStepKGS,
		reservePriceKGS: l.ReservePriceKGS,
		currentPriceKGS: current,
		endAt:           endAt,
		hadBid:          hadBid,
		bidCh:           make(chan bidRequest),
		subscribeCh:     make(chan *subscriber),
		unsubscribeCh:   make(chan *subscriber),
		stateCh:         make(chan chan []byte),
		pool:            pool,
		lots:            lots,
		bids:            bids,
		done:            make(chan struct{}),
		onFinished:      onFinished,
	}
}

func (r *Room) run() {
	subs := make(map[*subscriber]bool)
	timer := time.NewTimer(time.Until(r.endAt))
	defer timer.Stop()

	broadcast := func(v any) {
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		for s := range subs {
			select {
			case s.send <- data:
			default: // медленный подписчик — пропускаем кадр, не блокируем комнату
			}
		}
	}

	for {
		select {
		case s := <-r.subscribeCh:
			subs[s] = true
			if data, err := json.Marshal(r.stateSnapshot()); err == nil {
				select {
				case s.send <- data:
				default:
				}
			}

		case s := <-r.unsubscribeCh:
			delete(subs, s)

		case req := <-r.bidCh:
			res := r.handleBid(req.bidderID, req.amountKGS)
			if res.err == nil && res.extended {
				timer.Reset(time.Until(res.endAt))
			}
			req.resp <- res
			if res.err == nil {
				broadcast(bidAcceptedMessage{
					Type:            "bid_accepted",
					CurrentPriceKGS: res.currentPriceKGS,
					EndAt:           res.endAt,
					Extended:        res.extended,
				})
			}

		case respCh := <-r.stateCh:
			data, _ := json.Marshal(r.stateSnapshot())
			respCh <- data

		case <-timer.C:
			status, finalPrice := r.finish()
			broadcast(finishedMessage{
				Type:          "finished",
				Status:        status,
				FinalPriceKGS: finalPrice,
				Sold:          status == lot.StatusFinishedSold,
			})
			close(r.done)
			if r.onFinished != nil {
				r.onFinished(r.lotID)
			}
			return
		}
	}
}

func (r *Room) stateSnapshot() stateMessage {
	return stateMessage{Type: "state", CurrentPriceKGS: r.currentPriceKGS, EndAt: r.endAt}
}

func (r *Room) handleBid(bidderID uuid.UUID, amountKGS int64) bidResult {
	if !time.Now().Before(r.endAt) {
		return bidResult{err: ErrLotNotActive}
	}
	if bidderID == r.sellerID {
		return bidResult{err: ErrSellerCannotBid}
	}
	if amountKGS < r.currentPriceKGS+r.minStepKGS {
		return bidResult{err: ErrBidTooLow}
	}

	newEndAt := r.endAt
	extended := false
	if time.Until(r.endAt) <= AntiSnipeWindow {
		newEndAt = r.endAt.Add(AntiSnipeExtension)
		extended = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Запись в bids (неизменный аудит) и обновление lots.current_price_kgs/end_at
	// идут одной транзакцией — иначе при сбое второго шага ставка осталась бы
	// навсегда в истории, не будучи применена к лоту (см. code review).
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return bidResult{err: err}
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op если Commit уже прошёл

	if _, err := r.bids.CreateTx(ctx, tx, r.lotID, bidderID, amountKGS); err != nil {
		return bidResult{err: err}
	}
	if err := r.lots.RecordBidTx(ctx, tx, r.lotID, amountKGS, newEndAt); err != nil {
		return bidResult{err: err}
	}
	if err := tx.Commit(ctx); err != nil {
		return bidResult{err: err}
	}

	r.currentPriceKGS = amountKGS
	r.endAt = newEndAt
	r.hadBid = true

	return bidResult{currentPriceKGS: r.currentPriceKGS, endAt: r.endAt, extended: extended}
}

func (r *Room) finish() (status string, finalPriceKGS int64) {
	status = lot.StatusFinishedUnsold
	if r.hadBid && (r.reservePriceKGS == nil || r.currentPriceKGS >= *r.reservePriceKGS) {
		status = lot.StatusFinishedSold
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.lots.Finalize(ctx, r.lotID, status); err != nil {
		log.Printf("auction: finalize lot %s: %v", r.lotID, err)
	}
	return status, r.currentPriceKGS
}
