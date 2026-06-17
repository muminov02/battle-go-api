package fake

import (
	"context"
	"sync"
	"time"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// BattleRepository is a thread-safe in-memory BattleRepository for tests.
type BattleRepository struct {
	mu     sync.RWMutex
	store  map[int]*models.Battle
	nextID int
}

func NewBattleRepository() *BattleRepository {
	return &BattleRepository{store: make(map[int]*models.Battle), nextID: 1}
}

func (r *BattleRepository) Create(_ context.Context, b *models.Battle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b.ID = r.nextID
	r.nextID++
	clone := cloneBattle(b)
	r.store[b.ID] = clone
	return nil
}

func (r *BattleRepository) FindByUUID(_ context.Context, uuid string) (*models.Battle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, b := range r.store {
		if b.UUID == uuid {
			return cloneBattle(b), nil
		}
	}
	return nil, nil
}

func (r *BattleRepository) FindWaiting(_ context.Context, battleType, lobbyType, courseID, levelGroupID int) (*models.Battle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, b := range r.store {
		if b.Status == models.BattleStatusWaiting &&
			b.Type == battleType &&
			b.LobbyType == lobbyType &&
			b.CourseID == courseID &&
			b.LevelGroupID == levelGroupID {
			return cloneBattle(b), nil
		}
	}
	return nil, nil
}

func (r *BattleRepository) FindExpiredOnQueue(_ context.Context) ([]*models.Battle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	var result []*models.Battle
	for _, b := range r.store {
		if b.Status == models.BattleStatusOnQueue && b.ExpireTime != nil && b.ExpireTime.Before(now) {
			result = append(result, cloneBattle(b))
		}
	}
	return result, nil
}

func (r *BattleRepository) FindOnGoingExpired(_ context.Context) ([]*models.Battle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	var result []*models.Battle
	for _, b := range r.store {
		if b.Status == models.BattleStatusOnGoing && b.EndTime != nil && b.EndTime.Before(now) {
			result = append(result, cloneBattle(b))
		}
	}
	return result, nil
}

func (r *BattleRepository) FindOnGoingAllMembersFinished(_ context.Context) ([]*models.Battle, error) {
	// Fake cannot JOIN — caller uses AllMembersFinished helper after fetching members.
	// Return all ON_GOING battles; service filters.
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*models.Battle
	for _, b := range r.store {
		if b.Status == models.BattleStatusOnGoing {
			result = append(result, cloneBattle(b))
		}
	}
	return result, nil
}

func (r *BattleRepository) FindOnGoing(_ context.Context) ([]*models.Battle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*models.Battle
	for _, b := range r.store {
		if b.Status == models.BattleStatusOnGoing {
			result = append(result, cloneBattle(b))
		}
	}
	return result, nil
}

func (r *BattleRepository) Save(_ context.Context, b *models.Battle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[b.ID]; !ok {
		return db.ErrNotFound
	}
	r.store[b.ID] = cloneBattle(b)
	return nil
}

func (r *BattleRepository) Delete(_ context.Context, battleID int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, battleID)
	return nil
}

// Add seeds a battle directly (test helper, skips ID auto-assign if already set).
func (r *BattleRepository) Add(b *models.Battle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.ID == 0 {
		b.ID = r.nextID
		r.nextID++
	}
	r.store[b.ID] = cloneBattle(b)
}

// All returns all stored battles (test helper).
func (r *BattleRepository) All() []*models.Battle {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*models.Battle, 0, len(r.store))
	for _, b := range r.store {
		out = append(out, cloneBattle(b))
	}
	return out
}

func cloneBattle(b *models.Battle) *models.Battle {
	c := *b
	if b.Questions != nil {
		c.Questions = make([]models.Question, len(b.Questions))
		copy(c.Questions, b.Questions)
	}
	return &c
}
