package fake

import (
	"context"
	"sync"

	"battle-go-api/internal/models"
)

// ResultWriter is an in-memory ResultWriter for tests.
type ResultWriter struct {
	mu             sync.RWMutex
	savedBattles   []*models.Battle
	savedMembers   []*models.BattleMember
	savedJsonData  []JsonDataEntry
	studentBattles map[int]*StudentBattleRecord
}

type JsonDataEntry struct {
	ModelID   int
	ModelType int
	Data      interface{}
}

type StudentBattleRecord struct {
	StudentID int
	WinCount  int
	LoseCount int
}

func NewResultWriter() *ResultWriter {
	return &ResultWriter{
		studentBattles: make(map[int]*StudentBattleRecord),
	}
}

func (w *ResultWriter) SaveBattle(_ context.Context, b *models.Battle) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	clone := *b
	w.savedBattles = append(w.savedBattles, &clone)
	return nil
}

func (w *ResultWriter) CreateMember(_ context.Context, _ string, m *models.BattleMember) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	clone := cloneMember(m)
	w.savedMembers = append(w.savedMembers, clone)
	return nil
}

func (w *ResultWriter) UpdateMember(_ context.Context, _ string, m *models.BattleMember) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	clone := cloneMember(m)
	w.savedMembers = append(w.savedMembers, clone)
	return nil
}

func (w *ResultWriter) SaveJsonData(_ context.Context, modelID, modelType int, data interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.savedJsonData = append(w.savedJsonData, JsonDataEntry{ModelID: modelID, ModelType: modelType, Data: data})
	return nil
}

func (w *ResultWriter) UpsertStudentBattle(_ context.Context, studentID int, won bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	rec, ok := w.studentBattles[studentID]
	if !ok {
		rec = &StudentBattleRecord{StudentID: studentID}
		w.studentBattles[studentID] = rec
	}
	if won {
		rec.WinCount++
	} else {
		rec.LoseCount++
	}
	return nil
}

// SavedBattles returns all battles written (test inspection helper).
func (w *ResultWriter) SavedBattles() []*models.Battle {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]*models.Battle(nil), w.savedBattles...)
}

// SavedMembers returns all members written (test inspection helper).
func (w *ResultWriter) SavedMembers() []*models.BattleMember {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]*models.BattleMember(nil), w.savedMembers...)
}

// StudentBattleFor returns the win/loss record for a student (test inspection helper).
func (w *ResultWriter) StudentBattleFor(studentID int) *StudentBattleRecord {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.studentBattles[studentID]
}
