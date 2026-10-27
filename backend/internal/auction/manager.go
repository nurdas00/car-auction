package auction

import (
	"context"
	"log"
	"sync"
	"time"

	"autoauction/internal/bid"
	"autoauction/internal/lot"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Manager держит по одной Room на каждый активный лот.
type Manager struct {
	mu    sync.Mutex
	rooms map[uuid.UUID]*Room

	pool *pgxpool.Pool
	lots *lot.Repo
	bids *bid.Repo
}

func NewManager(pool *pgxpool.Pool, lots *lot.Repo, bids *bid.Repo) *Manager {
	return &Manager{rooms: make(map[uuid.UUID]*Room), pool: pool, lots: lots, bids: bids}
}

// StartRoom запускает комнату для лота, уже переведённого в status=active
// (вызывается сразу после Approve).
func (m *Manager) StartRoom(l *lot.Lot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.rooms[l.ID]; exists {
		return
	}
	room := newRoom(l, m.pool, m.lots, m.bids, m.removeRoom)
	m.rooms[l.ID] = room
	go room.run()
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
	activeLots, err := m.lots.ListActive(ctx)
	if err != nil {
		return err
	}
	for i := range activeLots {
		l := activeLots[i]
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
	status := lot.StatusFinishedUnsold
	if hadBid && (l.ReservePriceKGS == nil || (l.CurrentPriceKGS != nil && *l.CurrentPriceKGS >= *l.ReservePriceKGS)) {
		status = lot.StatusFinishedSold
	}
	return m.lots.Finalize(ctx, l.ID, status)
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
