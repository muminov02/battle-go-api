package service

import (
	"context"
	"fmt"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// ConfirmService handles POST /student/v1/battle/confirm.
type ConfirmService struct {
	battles  db.BattleRepository
	members  db.MemberRepository
	qr       db.QuestionReader
	results  db.ResultWriter
	realtime RealtimeService
	config   Config
}

func NewConfirmService(
	battles db.BattleRepository,
	members db.MemberRepository,
	qr db.QuestionReader,
	results db.ResultWriter,
	realtime RealtimeService,
	config Config,
) *ConfirmService {
	return &ConfirmService{battles, members, qr, results, realtime, config}
}

// ConfirmResult is the outcome of a Confirm call.
type ConfirmResult struct {
	Battle  *models.Battle
	Members []*models.BattleMember
	// Message is non-empty when waiting for others to confirm.
	Message string
}

// Execute confirms the student's attendance for the battle.
// Mirrors PHP BattleConfirmRequest.getResult().
func (s *ConfirmService) Execute(ctx context.Context, studentID int, battleUUID string) (*ConfirmResult, error) {
	b, err := s.battles.FindByUUID(ctx, battleUUID)
	if err != nil {
		return nil, fmt.Errorf("confirm: find battle: %w", err)
	}
	if b == nil {
		return nil, ErrBattleNotFound
	}

	m, err := s.members.FindByBattleAndStudent(ctx, b.ID, studentID)
	if err != nil {
		return nil, fmt.Errorf("confirm: find member: %w", err)
	}
	if m == nil {
		return nil, ErrNotMember
	}

	// Mark confirmed in PostgreSQL only; MySQL updated at battle end
	m.Status = models.MemberStatusConfirmed
	if err := s.members.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("confirm: save member: %w", err)
	}
	_ = s.realtime.PublishMember(ctx, b.UUID, m)

	// Count confirmed members
	confirmedCount, err := s.members.CountConfirmedByBattle(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("confirm: count confirmed: %w", err)
	}

	if confirmedCount >= models.BattleMemberCount[b.Type] {
		return s.startConfirmedBattle(ctx, b)
	}

	mems, err := s.members.FindByBattleID(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("confirm: load members: %w", err)
	}
	return &ConfirmResult{Battle: b, Members: mems, Message: "Please wait other members to join"}, nil
}

// startConfirmedBattle generates questions and starts the battle when all members confirmed.
func (s *ConfirmService) startConfirmedBattle(ctx context.Context, b *models.Battle) (*ConfirmResult, error) {
	questions, err := fetchQuestions(ctx, b, s.qr, s.config)
	if err != nil {
		return nil, fmt.Errorf("confirm: fetch questions: %w", err)
	}

	if len(questions) == 0 {
		// PHP returns early if no questions found
		mems, _ := s.members.FindByBattleID(ctx, b.ID)
		return &ConfirmResult{Battle: b, Members: mems, Message: "Please wait other members to join"}, nil
	}

	if err := startBattle(ctx, b, questions, s.battles); err != nil {
		return nil, err
	}

	mems, err := s.members.FindByBattleID(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("confirm: load members: %w", err)
	}

	// Publish battle + all members
	_ = s.realtime.PublishBattleWithMembers(ctx, b, mems)

	// Fill AI member answers (a P2P battle converted to AI starts here, not in
	// find.startAIBattle, so the bot would otherwise never answer). Mirrors
	// FindService.startAIBattle. No-op for GROUP/P2P battles with no AI member.
	for _, m := range mems {
		if m.Type != models.MemberTypeAI {
			continue
		}
		fillAIAnswers(b, m)
		if err := s.members.Save(ctx, m); err != nil {
			return nil, fmt.Errorf("confirm: save ai member: %w", err)
		}
		_ = s.realtime.PublishMember(ctx, b.UUID, m)
	}

	return &ConfirmResult{Battle: b, Members: mems}, nil
}
