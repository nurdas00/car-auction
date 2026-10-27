package auction

import (
	"context"
	"log"
	"sync"
	"time"

	"autoauction/internal/bid"
	"autoauction/internal/lot"
	"autoauction/internal/notify"
	"autoauction/internal/user"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Manager держит по одной Room на каждый активный лот.
type Manager struct {
	mu    sync.Mutex
	rooms map[uuid.UUID]*Room

	pool     *pgxpool.Pool
	lots     *lot.Repo
	bids     *bid.Repo
	users    *user.Repo
	notifier notify.Notifier
}

func NewManager(pool *pgxpool.Pool, lots *lot.Repo, bids *bid.Repo, users *user.Repo, notifier notify.Notifier) *Manager {
	return &Manager{
		rooms: make(map[uuid.UUID]*Room), pool: pool, lots: lots, bids: bids,
		users: users, notifier: notifier,
	}
}

// StartRoom запускает комнату для лота, уже переведённого в status=active
// (вызывается сразу после Approve).
func (m *Manager) StartRoom(l *lot.Lot) {
	if l.ListingType != lot.ListingTypeAuction {
		return // витрина с фиксированной ценой не торгуется
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.rooms[l.ID]; exists {
		return
	}
	room := newRoom(l, m.pool, m.lots, m.bids, m.users, m.notifier, m.removeRoom)
	m.rooms[l.ID] = room
	go room.run()
}

// Cancel снимает активный лот с публикации по решению админа (ТЗ раздел 4.6):
// останавливает комнату немедленно, минуя обычную бизнес-логику finish()
// (резерв/наличие ставок) — итог принудительный, независимо от текущих ставок.
// Дожидается фактического результата записи в БД — иначе вызывающий код
// (HTTP-хендлер) мог бы отрапортовать об успехе до того, как отмена
// действительно сохранилась (см. code review).
func (m *Manager) Cancel(ctx context.Context, lotID uuid.UUID) error {
	room, ok := m.getRoom(lotID)
	if !ok {
		// Лот с фиксированной ценой не торгуется и Room для него не создаётся
		// (см. StartRoom/Resume) — отменяем его прямой записью в БД, минуя
		// Room-машинерию, которая для него и не нужна (см. code review).
		l, err := m.lots.GetByID(ctx, lotID)
		if err != nil || l.ListingType != lot.ListingTypeFixedPrice {
			return ErrLotNotActive
		}
		if err := m.lots.Finalize(ctx, lotID, lot.StatusCancelled); err != nil {
			return ErrLotNotActive
		}
		return nil
	}

	respCh := make(chan error, 1)
	select {
	case room.cancelCh <- respCh:
	case <-room.done:
		return ErrLotNotActive
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-respCh:
		return err
	case <-room.done:
		return ErrLotNotActive
	}
}

func (m *Manager) removeRoom(lotID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rooms, lotID)
}

func (m *Manager) getRoom(lotID uuid.UUID) (*Room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[lotID]
	return r, ok
}

// Resume поднимает комнаты для всех лотов, которые были активны на момент
// прошлого завершения работы сервера. Лоты, у которых время уже истекло
// (сервер был выключен дольше, чем оставалось до конца торгов), финализируются
// немедленно без повторного прогона таймера.
func (m *Manager) Resume(ctx context.Context) error {
	activeLots, err := m.lots.ListActive(ctx, lot.Filter{}) // без лимита — нужны все активные лоты
	if err != nil {
		return err
	}
	for i := range activeLots {
		l := activeLots[i]
		if l.ListingType != lot.ListingTypeAuction {
			continue // витрина с фиксированной ценой не торгуется — Room ей не нужна
		}
		if l.EndAt != nil && !time.Now().Before(*l.EndAt) {
			if err := m.finalizeExpired(ctx, &l); err != nil {
				log.Printf("auction: finalize expired lot %s on resume: %v", l.ID, err)
			}
			continue
		}
		m.StartRoom(&l)
	}
	return nil
}

func (m *Manager) finalizeExpired(ctx context.Context, l *lot.Lot) error {
	_, hadBid, err := m.bids.HighestBidder(ctx, l.ID)
	if err != nil {
		return err
	}
	currentPriceKGS := l.StartPriceKGS
	if l.CurrentPriceKGS != nil {
		currentPriceKGS = *l.CurrentPriceKGS
	}
	status := decideOutcome(hadBid, currentPriceKGS, l.ReservePriceKGS)
	if err := m.lots.Finalize(ctx, l.ID, status); err != nil {
		return err
	}
	sendFinishNotifications(ctx, m.notifier, m.users, m.bids, l.ID, l.SellerID, status, currentPriceKGS)
	return nil
}

type BidOutcome struct {
	CurrentPriceKGS int64     `json:"current_price_kgs"`
	EndAt           time.Time `json:"end_at"`
	Extended        bool      `json:"extended"`
}

// PlaceBid сериализует ставку через канал комнаты — см. Room.run.
func (m *Manager) PlaceBid(ctx context.Context, lotID, bidderID uuid.UUID, amountKGS int64) (*BidOutcome, error) {
	room, ok := m.getRoom(lotID)
	if !ok {
		return nil, ErrLotNotActive
	}

	respCh := make(chan bidResult, 1)
	select {
	case room.bidCh <- bidRequest{bidderID: bidderID, amountKGS: amountKGS, resp: respCh}:
	case <-room.done:
		return nil, ErrLotNotActive
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// К этому моменту запрос уже передан в комнату (send на bidCh состоялся),
	// и Room.run() гарантированно ответит в respCh почти немедленно — поэтому
	// здесь НЕ ждём ctx.Done(): иначе отмена контекста уже отправленного запроса
	// (например, таймаут HTTP-клиента) превращала бы успешно принятую и
	// разосланную ставку в ошибку для вызывающего (см. code review). room.done
	// остаётся как защита на случай, если комната всё же завершится без ответа.
	select {
	case res := <-respCh:
		if res.err != nil {
			return nil, res.err
		}
		return &BidOutcome{CurrentPriceKGS: res.currentPriceKGS, EndAt: res.endAt, Extended: res.extended}, nil
	case <-room.done:
		return nil, ErrLotNotActive
	}
}

// Subscribe подключает слушателя к живой трансляции лота (ставки/таймер/завершение).
func (m *Manager) Subscribe(lotID uuid.UUID) (*subscriber, bool) {
	room, ok := m.getRoom(lotID)
	if !ok {
		return nil, false
	}
	sub := &subscriber{send: make(chan []byte, 16), room: room}
	select {
	case room.subscribeCh <- sub:
	case <-room.done:
		return nil, false
	}
	return sub, true
}
