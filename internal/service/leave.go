package service

import (
	"context"
	"fmt"

	battlelogic "battle-go-api/internal/battle"
	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// LeaveService handles POST /student/v1/battle/leave.
type LeaveService struct {
	battles  db.BattleRepository
	members  db.MemberRepository
	results  db.ResultWriter
	realtime RealtimeService
}

func NewLeaveService(
	battles db.BattleRepository,
	members db.MemberRepository,
	results db.ResultWriter,
	realtime RealtimeService,
) *LeaveService {
	return &LeaveService{battles, members, results, realtime}
}

// Execute removes the student from the battle.
// Mirrors PHP BattleLeaveRequest.getResult().
func (s *LeaveService) Execute(ctx context.Context, studentID int, battleUUID string) error {
	b, err := s.battles.FindByUUID(ctx, battleUUID)
	if err != nil {
		return fmt.Errorf("leave: find battle: %w", err)
	}
	if b == nil {
		return ErrBattleNotFound
	}
	if b.Status == models.BattleStatusFinished {
		return ErrBattleFinished
	}

	if b.Type == models.BattleTypeAI {
		// AI battle: delete all members and the battle itself
		if err := s.members.DeleteByBattle(ctx, b.ID); err != nil {
			return fmt.Errorf("leave: delete ai members: %w", err)
		}
		if err := s.battles.Delete(ctx, b.ID); err != nil {
			return fmt.Errorf("leave: delete ai battle: %w", err)
		}
		_ = s.realtime.DeleteBattle(ctx, b.UUID)
		return nil
	}

	// P2P/GROUP
	if b.Status == models.BattleStatusOnGoing {
		m, err := s.members.FindByBattleAndStudent(ctx, b.ID, studentID)
		if err != nil {
			return fmt.Errorf("leave: find member: %w", err)
		}

		if m != nil && m.Status == models.MemberStatusConfirmed {
			// Fill blank answers and mark finished (PostgreSQL only; MySQL written at battle end)
			battlelogic.FillBlanks(b, m)
			if err := s.members.Save(ctx, m); err != nil {
				return fmt.Errorf("leave: save member: %w", err)
			}
			_ = s.realtime.PublishMember(ctx, b.UUID, m)
			return nil
		}
	}

	// WAITING / ON_QUEUE, or ON_GOING with non-CONFIRMED member → delete member
	if err := s.members.DeleteByBattleAndStudent(ctx, b.ID, studentID); err != nil {
		return fmt.Errorf("leave: delete member: %w", err)
	}
	_ = s.realtime.DeleteMember(ctx, b.UUID, studentID)
	return nil
}
