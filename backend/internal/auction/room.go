package auction

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"autoauction/internal/bid"
	"autoauction/internal/lot"
	"autoauction/internal/notify"
	"autoauction/internal/user"

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
	currentPriceKGS  int64
	endAt            time.Time
	extended         bool
	bidderID         uuid.UUID
	previousBidderID uuid.UUID
	err              error
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
	currentBidderID uuid.UUID // uuid.Nil, если ставок ещё не было
	endAt           time.Time
	hadBid          bool

	bidCh         chan bidRequest
	subscribeCh   chan *subscriber
	unsubscribeCh chan *subscriber
	stateCh       chan chan []byte
	cancelCh      chan chan error

	pool     *pgxpool.Pool
	lots     *lot.Repo
	bids     *bid.Repo
	users    *user.Repo
	notifier notify.Notifier

	done       chan struct{}
	onFinished func(uuid.UUID)
}

func newRoom(l *lot.Lot, pool *pgxpool.Pool, lots *lot.Repo, bids *bid.Repo, users *user.Repo, notifier notify.Notifier, onFinished func(uuid.UUID)) *Room {
	current := l.StartPriceKGS
	hadBid := false
	var currentBidderID uuid.UUID
	if l.CurrentPriceKGS != nil {
		current = *l.CurrentPriceKGS
		// Approve() выставляет current_price_kgs = start_price_kgs и шаг ставки
		// всегда положителен, так что первая же ставка строго больше стартовой
		// цены — расхождение с ней надёжно означает "ставка уже была".
		hadBid = current != l.StartPriceKGS
	}
	if hadBid {
		// Восстанавливаем лидера после перезапуска сервера (см. Manager.Resume) —
		// без этого уведомление "вас перебили" ушло бы не тому, кому нужно. Если
		// запрос не удался — это лишь кэш для live-уведомлений об обгоне ставки;
		// при завершении торгов sendFinishNotifications всё равно сама заново
		// запросит победителя из bids, так что ошибка здесь не стоит потери
		// итоговых уведомлений (см. code review), но её стоит залогировать.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		id, ok, err := bids.HighestBidder(ctx, l.ID)
		cancel()
		if err != nil {
			log.Printf("auction: restore leading bidder for lot %s: %v", l.ID, err)
		} else if ok {
			currentBidderID = id
		}
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
		currentBidderID: currentBidderID,
		endAt:           endAt,
		hadBid:          hadBid,
		bidCh:           make(chan bidRequest),
		subscribeCh:     make(chan *subscriber),
		unsubscribeCh:   make(chan *subscriber),
		stateCh:         make(chan chan []byte),
		cancelCh:        make(chan chan error),
		pool:            pool,
		lots:            lots,
		bids:            bids,
		users:           users,
		notifier:        notifier,
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
				if res.previousBidderID != uuid.Nil && res.previousBidderID != res.bidderID {
					r.notifyOutbid(res.previousBidderID, res.currentPriceKGS)
				}
			}

		case respCh := <-r.stateCh:
			data, _ := json.Marshal(r.stateSnapshot())
			respCh <- data

		case <-timer.C:
			status, finalPrice := r.finish()
			r.shutdown(broadcast, status, finalPrice)
			return

		case respCh := <-r.cancelCh:
			// Админ снял лот с публикации (ТЗ раздел 4.6) — это не связано
			// с бизнес-логикой finish() (резерв/наличие ставок), статус
			// принудительный, независимо от того, были ставки или нет.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := r.lots.Finalize(ctx, r.lotID, lot.StatusCancelled)
			cancel()
			respCh <- err
			if err != nil {
				// Не удалось записать отмену в БД — комната остаётся активной,
				// админ увидит ошибку и может повторить попытку (см. code review:
				// раньше HTTP-ответ сообщал об успехе независимо от этого исхода).
				log.Printf("auction: cancel lot %s: %v", r.lotID, err)
				continue
			}
			r.shutdown(broadcast, lot.StatusCancelled, r.currentPriceKGS)
			return
		}
	}
}

// shutdown — общая последовательность завершения комнаты (рассылка итогового
// сообщения, фоновые уведомления, закрытие done, снятие из Manager), общая
// для таймера и принудительной отмены админом — раньше оба случая держали
// свою копию этой последовательности и уже успели разойтись (см. code review).
func (r *Room) shutdown(broadcast func(any), status string, finalPriceKGS int64) {
	broadcast(finishedMessage{
		Type:          "finished",
		Status:        status,
		FinalPriceKGS: finalPriceKGS,
		Sold:          status == lot.StatusFinishedSold,
	})
	r.notifyFinishResult(status, finalPriceKGS)
	close(r.done)
	if r.onFinished != nil {
		r.onFinished(r.lotID)
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

	previousBidderID := r.currentBidderID

	r.currentPriceKGS = amountKGS
	r.endAt = newEndAt
	r.hadBid = true
	r.currentBidderID = bidderID

	return bidResult{
		currentPriceKGS:  r.currentPriceKGS,
		endAt:            r.endAt,
		extended:         extended,
		bidderID:         bidderID,
		previousBidderID: previousBidderID,
	}
}

// notifyOutbid уведомляет перебитого лидера ставок (ТЗ раздел 4.5). Отправка
// идёт в фоне: недоставленное уведомление не должно блокировать комнату
// или проваливать саму ставку, которая уже принята и применена.
func (r *Room) notifyOutbid(previousBidderID uuid.UUID, newPriceKGS int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		u, err := r.users.GetByID(ctx, previousBidderID)
		if err != nil {
			log.Printf("auction: notify outbid for lot %s: %v", r.lotID, err)
			return
		}
		body := fmt.Sprintf("Вашу ставку на лоте перебили. Текущая цена: %d сом.", newPriceKGS)
		if err := r.notifier.Send(ctx, u.Email, "Вашу ставку перебили", body); err != nil {
			log.Printf("auction: send outbid notification for lot %s: %v", r.lotID, err)
		}
	}()
}

// notifyFinishResult уведомляет продавца и (при продаже) победителя об итоге
// торгов, обмениваясь контактами (ТЗ раздел 4.5). В фоне — доставка
// уведомления не должна задерживать завершение комнаты.
func (r *Room) notifyFinishResult(status string, finalPriceKGS int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sendFinishNotifications(ctx, r.notifier, r.users, r.bids, r.lotID, r.sellerID, status, finalPriceKGS)
	}()
}

func (r *Room) finish() (status string, finalPriceKGS int64) {
	status = decideOutcome(r.hadBid, r.currentPriceKGS, r.reservePriceKGS)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.lots.Finalize(ctx, r.lotID, status); err != nil {
		log.Printf("auction: finalize lot %s: %v", r.lotID, err)
	}
	return status, r.currentPriceKGS
}

// decideOutcome — единое правило "продано/не продано" (ТЗ раздел 4.3),
// общее для живого завершения (Room.finish) и восстановления уже истёкших
// лотов после рестарта сервера (Manager.finalizeExpired) — раньше это правило
// было продублировано в двух местах и могло разойтись (см. code review).
func decideOutcome(hadBid bool, currentPriceKGS int64, reservePriceKGS *int64) string {
	if hadBid && (reservePriceKGS == nil || currentPriceKGS >= *reservePriceKGS) {
		return lot.StatusFinishedSold
	}
	return lot.StatusFinishedUnsold
}

// sendFinishNotifications — обмен контактами продавца и победителя и
// уведомление об итоге торгов (ТЗ раздел 4.5). Вынесена в пакетную функцию,
// чтобы ей могли пользоваться и живая Room.notifyFinishResult, и
// Manager.finalizeExpired (у которой своей Room уже нет).
//
// Победитель запрашивается из bids заново, а не берётся из закэшированного
// Room.currentBidderID — так итоговое уведомление не зависит от того, успешно
// ли этот кэш восстановился при старте комнаты после перезапуска сервера
// (см. code review). Уведомление продавцу отправляется независимо от того,
// удалось ли определить победителя — отсутствие контакта покупателя не должно
// стоить продавцу уведомления о самом факте продажи.
func sendFinishNotifications(ctx context.Context, notifier notify.Notifier, users *user.Repo, bids *bid.Repo, lotID, sellerID uuid.UUID, status string, finalPriceKGS int64) {
	seller, err := users.GetByID(ctx, sellerID)
	if err != nil {
		log.Printf("auction: notify finish (seller lookup) for lot %s: %v", lotID, err)
		return
	}

	switch status {
	case lot.StatusFinishedSold:
		winnerID, ok, err := bids.HighestBidder(ctx, lotID)
		winner, winnerErr := (*user.User)(nil), error(nil)
		if err == nil && ok {
			winner, winnerErr = users.GetByID(ctx, winnerID)
		}
		if err != nil || !ok || winnerErr != nil {
			log.Printf("auction: notify finish (winner lookup) for lot %s: ok=%v bidErr=%v userErr=%v", lotID, ok, err, winnerErr)
			_ = notifier.Send(ctx, seller.Email, "Ваш лот продан", fmt.Sprintf(
				"Ваш лот продан за %d сом. Контакт покупателя пришлём отдельно.", finalPriceKGS,
			))
			return
		}
		sellerBody := fmt.Sprintf(
			"Ваш лот продан за %d сом. Контакт покупателя: %s, телефон: %s.",
			finalPriceKGS, winner.Name, winner.Phone,
		)
		winnerBody := fmt.Sprintf(
			"Вы выиграли торги за %d сом. Контакт продавца: %s, телефон: %s.",
			finalPriceKGS, seller.Name, seller.Phone,
		)
		_ = notifier.Send(ctx, seller.Email, "Ваш лот продан", sellerBody)
		_ = notifier.Send(ctx, winner.Email, "Вы выиграли торги", winnerBody)

	case lot.StatusFinishedUnsold:
		_ = notifier.Send(ctx, seller.Email, "Торги завершены без продажи",
			"Ставок было недостаточно, либо резервная цена не была достигнута.")

	case lot.StatusCancelled:
		_ = notifier.Send(ctx, seller.Email, "Лот снят с торгов администратором",
			"Ваш лот был отменён администрацией и больше не выставлен на аукцион.")
	}
}
