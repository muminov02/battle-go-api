package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	battlelogic "battle-go-api/internal/battle"
	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// AnswerService handles POST /student/v1/battle/answer (save-student-answer).
type AnswerService struct {
	battles  db.BattleRepository
	members  db.MemberRepository
	results  db.ResultWriter
	realtime RealtimeService
}

func NewAnswerService(
	battles db.BattleRepository,
	members db.MemberRepository,
	results db.ResultWriter,
	realtime RealtimeService,
) *AnswerService {
	return &AnswerService{battles, members, results, realtime}
}

// AnswerResult mirrors PHP: {status: true/false, message: "..."}.
type AnswerResult struct {
	Status  bool
	Message string
}

// Execute processes a submitted answer.
// Mirrors PHP BattleSaveStudentAnswerRequest.getResult().
// Returns (result, nil) always; uses result.Status=false for "question not found".
// Returns (nil, err) for unexpected errors.
func (s *AnswerService) Execute(ctx context.Context, studentID int, battleUUID string, questionID int, values []string, answerTime int) (*AnswerResult, error) {
	b, err := s.battles.FindByUUID(ctx, battleUUID)
	if err != nil {
		return nil, fmt.Errorf("answer: find battle: %w", err)
	}
	if b == nil {
		return nil, ErrBattleNotFound
	}

	// Validate battle status (PHP rejects WAITING, ON_QUEUE, FINISHED)
	if b.Status == models.BattleStatusWaiting || b.Status == models.BattleStatusOnQueue {
		return nil, ErrBattleNotStarted
	}
	if b.Status == models.BattleStatusFinished {
		return nil, ErrBattleFinished
	}

	m, err := s.members.FindByBattleAndStudent(ctx, b.ID, studentID)
	if err != nil {
		return nil, fmt.Errorf("answer: find member: %w", err)
	}
	if m == nil {
		return nil, ErrNotMember
	}

	isDuplicate, err := battlelogic.ProcessAnswer(b, m, questionID, values, answerTime)
	if err != nil {
		if errors.Is(err, battlelogic.ErrQuestionNotFound) {
			return &AnswerResult{Status: false, Message: "Question does not exist"}, nil
		}
		return nil, fmt.Errorf("answer: process: %w", err)
	}

	if isDuplicate {
		return &AnswerResult{Status: true, Message: "Success"}, nil
	}

	// Answer accepted → current_question advanced. Reset the idle clock.
	now := time.Now()
	m.LastQuestionAt = &now

	// Persist to PostgreSQL only; MySQL updated at battle end
	if err := s.members.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("answer: save member pg: %w", err)
	}
	_ = s.realtime.PublishMember(ctx, b.UUID, m)

	return &AnswerResult{Status: true, Message: "Success"}, nil
}
