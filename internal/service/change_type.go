package service

import (
	"context"
	"fmt"
	"time"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// ChangeTypeService handles POST /student/v1/battle/change-type-to-ai.
type ChangeTypeService struct {
	battles  db.BattleRepository
	members  db.MemberRepository
	students db.StudentReader
	realtime RealtimeService
}

func NewChangeTypeService(
	battles db.BattleRepository,
	members db.MemberRepository,
	students db.StudentReader,
	realtime RealtimeService,
) *ChangeTypeService {
	return &ChangeTypeService{battles, members, students, realtime}
}

// ChangeTypeResult is the outcome of a ChangeType call.
type ChangeTypeResult struct {
	Battle  *models.Battle
	Members []*models.BattleMember
	Message string
}

// Execute converts a WAITING P2P battle to an AI battle.
// Mirrors PHP BattleChangeTypeToAIRequest.getResult().
func (s *ChangeTypeService) Execute(ctx context.Context, studentID int, battleUUID string) (*ChangeTypeResult, error) {
	b, err := s.battles.FindByUUID(ctx, battleUUID)
	if err != nil {
		return nil, fmt.Errorf("change type: find battle: %w", err)
	}
	if b == nil || b.Status != models.BattleStatusWaiting {
		return nil, ErrBattleNotFound
	}

	memberCount, err := s.members.CountByBattle(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("change type: count members: %w", err)
	}

	if memberCount < models.BattleMemberCount[models.BattleTypeAI] {
		expire := time.Now().Add(20 * time.Second)
		b.Type = models.BattleTypeAI
		b.Status = models.BattleStatusOnQueue
		b.ExpireTime = &expire

		if err := s.battles.Save(ctx, b); err != nil {
			return nil, fmt.Errorf("change type: save battle: %w", err)
		}

		aiMember, err := makeAIOpponent(ctx, s.students, s.members, b, []int{studentID})
		if err != nil {
			return nil, fmt.Errorf("change type: ai opponent: %w", err)
		}

		// PHP: updateBattle (empty winners/questions), then updateMember (AI member)
		_ = s.realtime.PublishBattle(ctx, b)
		_ = s.realtime.PublishMember(ctx, b.UUID, aiMember)
	}

	mems, err := s.members.FindByBattleID(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("change type: load members: %w", err)
	}

	return &ChangeTypeResult{
		Battle:  b,
		Members: mems,
		Message: "Please wait other members to join",
	}, nil
}
