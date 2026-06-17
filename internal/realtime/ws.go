package realtime

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"battle-go-api/internal/auth"
	"battle-go-api/internal/models"
)

// pgNotifyChannel is the Postgres LISTEN/NOTIFY channel carrying battle events.
const pgNotifyChannel = "battle_events"

// WSService is a native-WebSocket implementation of service.RealtimeService.
//
// Cross-process fan-out: the API and the worker are separate processes, but both
// share PostgreSQL. Publishes go out as `pg_notify('battle_events', …)`. The API
// process runs StartListener, which LISTENs and broadcasts to locally-connected
// WebSocket clients. So a worker-side publish (battle finished, idle, kick) still
// reaches clients connected to the API. Same event payloads as AblyService.
//
// Clients connect: GET /student/v1/battle/ws?battle=<uuid>&token=<jwt> and receive
// {"channel": "<uuid>|<uuid>:<sid>", "data": {...}} for that battle.
type WSService struct {
	pool    *pgxpool.Pool
	hub     *wsHub
	baseURL string
	pubKey  *rsa.PublicKey
}

func NewWSService(pool *pgxpool.Pool, baseURL string, pubKey *rsa.PublicKey) *WSService {
	return &WSService{pool: pool, hub: newWSHub(), baseURL: baseURL, pubKey: pubKey}
}

// notifyEnvelope is what travels through pg_notify.
type notifyEnvelope struct {
	Battle  string `json:"battle"`
	Channel string `json:"channel"`
	Data    any    `json:"data"`
}

// ── RealtimeService ─────────────────────────────────────────────────────────

func (s *WSService) PublishBattle(ctx context.Context, b *models.Battle) error {
	return s.emit(ctx, b.UUID, b.UUID, battlePayload(b))
}

func (s *WSService) PublishBattleWithMembers(ctx context.Context, b *models.Battle, members []*models.BattleMember) error {
	s.emit(ctx, b.UUID, b.UUID, battlePayload(b))
	for _, m := range members {
		s.emit(ctx, b.UUID, memberChannel(b.UUID, m.StudentID), memberPayload(b.UUID, m))
	}
	return nil
}

func (s *WSService) PublishMember(ctx context.Context, battleUUID string, m *models.BattleMember) error {
	return s.emit(ctx, battleUUID, memberChannel(battleUUID, m.StudentID), memberPayload(battleUUID, m))
}

func (s *WSService) DeleteBattle(ctx context.Context, battleUUID string) error {
	return s.emit(ctx, battleUUID, battleUUID, map[string]any{"deleted": true, "battle_id": battleUUID})
}

func (s *WSService) DeleteMember(ctx context.Context, battleUUID string, studentID int) error {
	return s.emit(ctx, battleUUID, memberChannel(battleUUID, studentID), map[string]any{"deleted": true})
}

// ── TokenProvider + RealtimeInfo (handler) ──────────────────────────────────

func (s *WSService) CreateBattleToken(_ context.Context, _ string, _ int) (string, error) {
	return "", nil // WS handshake authenticates with the JWT in ?token=
}

func (s *WSService) Driver() string { return "ws" }

// ConnectURL returns the public WS endpoint the client dials. baseURL is the FULL
// ws(s):// endpoint (WS_PUBLIC_URL), e.g. wss://host/v2/battle/ws behind a path proxy.
func (s *WSService) ConnectURL(battleUUID string) string {
	sep := "?"
	if strings.Contains(s.baseURL, "?") {
		sep = "&"
	}
	return s.baseURL + sep + "battle=" + battleUUID
}

// ── publish via pg_notify ───────────────────────────────────────────────────

func (s *WSService) emit(ctx context.Context, battleUUID, channel string, payload any) error {
	msg, err := json.Marshal(notifyEnvelope{Battle: battleUUID, Channel: channel, Data: payload})
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgNotifyChannel, string(msg))
	return err
}

// StartListener LISTENs for battle events and broadcasts them to local WS clients.
// Run in a goroutine in the API process. Reconnects on error.
func (s *WSService) StartListener(ctx context.Context) {
	for {
		if err := s.listen(ctx); err != nil {
			log.Printf("ws listener: %v (reconnecting in 2s)", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (s *WSService) listen(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+pgNotifyChannel); err != nil {
		return err
	}
	log.Println("ws listener: subscribed to pg notifications")

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var env notifyEnvelope
		if err := json.Unmarshal([]byte(n.Payload), &env); err != nil {
			continue
		}
		clientMsg, _ := json.Marshal(map[string]any{"channel": env.Channel, "data": env.Data})
		s.hub.broadcast(env.Battle, clientMsg)
	}
}

// ── WS endpoint ─────────────────────────────────────────────────────────────

func (s *WSService) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if _, err := auth.ValidateToken(token, s.pubKey); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	battleUUID := r.URL.Query().Get("battle")
	if battleUUID == "" {
		http.Error(w, "battle query param required", http.StatusBadRequest)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}

	conn := &wsConn{c: c, send: make(chan []byte, 32)}
	s.hub.subscribe(battleUUID, conn)
	defer func() {
		s.hub.unsubscribe(battleUUID, conn)
		c.CloseNow()
	}()

	go func() {
		for msg := range conn.send {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := c.Write(ctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}()

	for {
		if _, _, err := c.Read(context.Background()); err != nil {
			return // client disconnected
		}
	}
}

// ── hub ─────────────────────────────────────────────────────────────────────

type wsConn struct {
	c    *websocket.Conn
	send chan []byte
}

type wsHub struct {
	mu   sync.RWMutex
	subs map[string]map[*wsConn]bool // battleUUID -> conns
}

func newWSHub() *wsHub {
	return &wsHub{subs: make(map[string]map[*wsConn]bool)}
}

func (h *wsHub) subscribe(battleUUID string, c *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[battleUUID] == nil {
		h.subs[battleUUID] = make(map[*wsConn]bool)
	}
	h.subs[battleUUID][c] = true
}

func (h *wsHub) unsubscribe(battleUUID string, c *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.subs[battleUUID]; set != nil {
		if set[c] {
			delete(set, c)
			close(c.send)
		}
		if len(set) == 0 {
			delete(h.subs, battleUUID)
		}
	}
}

func (h *wsHub) broadcast(battleUUID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.subs[battleUUID] {
		select {
		case c.send <- msg:
		default: // slow consumer — drop rather than block
		}
	}
}
