package fake

import (
	"context"
	"sync"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// MemberRepository is a thread-safe in-memory MemberRepository for tests.
type MemberRepository struct {
	mu     sync.RWMutex
	store  map[int]*models.BattleMember
	nextID int
}

func NewMemberRepository() *MemberRepository {
	return &MemberRepository{store: make(map[int]*models.BattleMember), nextID: 1}
}

func (r *MemberRepository) Create(_ context.Context, m *models.BattleMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m.ID = r.nextID
	r.nextID++
	clone := cloneMember(m)
	r.store[m.ID] = clone
	return nil
}

func (r *MemberRepository) FindByBattleID(_ context.Context, battleID int) ([]*models.BattleMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*models.BattleMember
	for _, m := range r.store {
		if m.BattleID == battleID {
			result = append(result, cloneMember(m))
		}
	}
	return result, nil
}

func (r *MemberRepository) FindByBattleAndStudent(_ context.Context, battleID, studentID int) (*models.BattleMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.store {
		if m.BattleID == battleID && m.StudentID == studentID {
			return cloneMember(m), nil
		}
	}
	return nil, nil
}

func (r *MemberRepository) CountByBattle(_ context.Context, battleID int) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, m := range r.store {
		if m.BattleID == battleID {
			count++
		}
	}
	return count, nil
}

func (r *MemberRepository) CountConfirmedByBattle(_ context.Context, battleID int) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, m := range r.store {
		if m.BattleID == battleID && m.Status == models.MemberStatusConfirmed {
			count++
		}
	}
	return count, nil
}

func (r *MemberRepository) ExistsInBattle(_ context.Context, battleID, studentID int) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.store {
		if m.BattleID == battleID && m.StudentID == studentID {
			return true, nil
		}
	}
	return false, nil
}

func (r *MemberRepository) Save(_ context.Context, m *models.BattleMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[m.ID]; !ok {
		return db.ErrNotFound
	}
	r.store[m.ID] = cloneMember(m)
	return nil
}

func (r *MemberRepository) Delete(_ context.Context, memberID int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, memberID)
	return nil
}

func (r *MemberRepository) DeleteByBattle(_ context.Context, battleID int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, m := range r.store {
		if m.BattleID == battleID {
			delete(r.store, id)
		}
	}
	return nil
}

func (r *MemberRepository) DeleteByBattleAndStudent(_ context.Context, battleID, studentID int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, m := range r.store {
		if m.BattleID == battleID && m.StudentID == studentID {
			delete(r.store, id)
			return nil
		}
	}
	return nil
}

// Add seeds a member directly (test helper, skips ID auto-assign if already set).
func (r *MemberRepository) Add(m *models.BattleMember) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == 0 {
		m.ID = r.nextID
		r.nextID++
	}
	r.store[m.ID] = cloneMember(m)
}

// All returns all stored members (test helper).
func (r *MemberRepository) All() []*models.BattleMember {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*models.BattleMember, 0, len(r.store))
	for _, m := range r.store {
		out = append(out, cloneMember(m))
	}
	return out
}

func cloneMember(m *models.BattleMember) *models.BattleMember {
	c := *m
	if m.Answers != nil {
		c.Answers = make([]models.Answer, len(m.Answers))
		copy(c.Answers, m.Answers)
	}
	return &c
}
