package fake

import (
	"context"
	"sync"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// StudentReader is an in-memory StudentReader for tests.
type StudentReader struct {
	mu                  sync.RWMutex
	students            map[int]*models.Student
	finishedBattleCount map[int]int // studentID → count today
}

func NewStudentReader() *StudentReader {
	return &StudentReader{
		students:            make(map[int]*models.Student),
		finishedBattleCount: make(map[int]int),
	}
}

// AddStudent seeds a student (test setup helper).
func (r *StudentReader) AddStudent(s *models.Student) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.students[s.ID] = s
}

// SetFinishedBattleCount seeds the today count for demo limit tests.
func (r *StudentReader) SetFinishedBattleCount(studentID, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishedBattleCount[studentID] = count
}

func (r *StudentReader) FindByID(_ context.Context, studentID int) (*models.Student, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.students[studentID]
	if !ok {
		return nil, db.ErrNotFound
	}
	clone := *s
	return &clone, nil
}

func (r *StudentReader) FindTestingUserIDs(_ context.Context, excludeIDs []int) ([]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	excluded := make(map[int]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = true
	}
	var result []int
	for id, s := range r.students {
		if s.IsTestingUser && !excluded[id] {
			result = append(result, id)
		}
	}
	return result, nil
}

func (r *StudentReader) CountFinishedBattlesToday(_ context.Context, studentID int) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.finishedBattleCount[studentID], nil
}
