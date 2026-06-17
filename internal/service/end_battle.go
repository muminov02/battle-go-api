package service

import (
	"context"
	"fmt"

	battlelogic "battle-go-api/internal/battle"
	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// EndBattleService finalises a battle: fills blanks, calculates results, persists, publishes.
// Called by both daemons:
//   - Expired ON_GOING (end_time passed): fillBlanks=true
//   - All members finished: fillBlanks=false
type EndBattleService struct {
	battles  db.BattleRepository
	members  db.MemberRepository
	results  db.ResultWriter
	realtime RealtimeService
}

func NewEndBattleService(
	battles db.BattleRepository,
	members db.MemberRepository,
	results db.ResultWriter,
	realtime RealtimeService,
) *EndBattleService {
	return &EndBattleService{battles, members, results, realtime}
}

// Execute finalises the battle.
// members must already be loaded (callers load them before deciding to call Execute).
// When fillBlanks=true, unanswered questions are filled with blank wrong answers.
func (s *EndBattleService) Execute(ctx context.Context, b *models.Battle, members []*models.BattleMember, fillBlanks bool) error {
	if fillBlanks {
		for _, m := range members {
			if !m.IsFinished {
				battlelogic.FillBlanks(b, m)
			}
		}
	}

	// Calculate results
	calc := &battlelogic.ResultCalculator{BattleType: b.Type}
	calc.AddBonusPoints(members)
	calc.SumPointsAndSetPlacement(members)

	// Set battle FINISHED
	b.Status = models.BattleStatusFinished

	// Persist battle + members to PostgreSQL
	if err := s.battles.Save(ctx, b); err != nil {
		return fmt.Errorf("end battle: save battle pg: %w", err)
	}
	for _, m := range members {
		if err := s.members.Save(ctx, m); err != nil {
			return fmt.Errorf("end battle: save member pg %d: %w", m.ID, err)
		}
	}

	// First MySQL write for this battle. Battle row MUST be written before members,
	// because member INSERT resolves battle_id via SELECT id FROM battle WHERE uuid = ?.
	if err := s.results.SaveBattle(ctx, b); err != nil {
		return fmt.Errorf("end battle: save battle mysql: %w", err)
	}
	for _, m := range members {
		if err := s.results.UpdateMember(ctx, b.UUID, m); err != nil {
			return fmt.Errorf("end battle: save member mysql %d: %w", m.ID, err)
		}
	}

	// Update win/loss stats in MySQL
	for _, m := range members {
		won := m.Points != nil && *m.Points > 0
		if err := s.results.UpsertStudentBattle(ctx, m.StudentID, won); err != nil {
			return fmt.Errorf("end battle: upsert student battle %d: %w", m.StudentID, err)
		}
	}

	// Publish final results to realtime
	_ = s.realtime.PublishBattleWithMembers(ctx, b, members)

	return nil
}
