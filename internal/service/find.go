package service

import (
	"context"
	"fmt"
	"time"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// FindService handles POST /student/v1/battle/find.
type FindService struct {
	battles  db.BattleRepository
	members  db.MemberRepository
	students db.StudentReader
	qr       db.QuestionReader
	results  db.ResultWriter
	realtime RealtimeService
	config   Config
}

func NewFindService(
	battles db.BattleRepository,
	members db.MemberRepository,
	students db.StudentReader,
	qr db.QuestionReader,
	results db.ResultWriter,
	realtime RealtimeService,
	config Config,
) *FindService {
	return &FindService{battles, members, students, qr, results, realtime, config}
}

// FindResult is the outcome of a Find call.
type FindResult struct {
	Battle  *models.Battle
	Members []*models.BattleMember
	// Message is non-empty ("Please wait other members to join") for P2P/GROUP lobbies.
	// Empty for AI battles (battle starts immediately).
	Message string
}

// Execute finds or creates a battle lobby for the student.
// Mirrors PHP BattleFindRequest.getResult().
func (s *FindService) Execute(ctx context.Context, studentID, battleType, lobbyType int) (*FindResult, error) {
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return nil, ErrStudentNotFound
	}

	if student.LevelID == 0 {
		return nil, ErrStudentNoLevel
	}

	if err := s.checkDemoLimit(ctx, student); err != nil {
		return nil, err
	}

	existing, err := s.battles.FindWaiting(ctx, battleType, lobbyType, student.CourseID, student.LevelGroupID)
	if err != nil {
		return nil, fmt.Errorf("find: find waiting: %w", err)
	}

	var b *models.Battle
	var memberCount int

	if existing != nil {
		alreadyIn, err := s.members.ExistsInBattle(ctx, existing.ID, studentID)
		if err != nil {
			return nil, fmt.Errorf("find: exists check: %w", err)
		}
		if alreadyIn {
			mems, err := s.members.FindByBattleID(ctx, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("find: load members: %w", err)
			}
			return &FindResult{Battle: existing, Members: mems, Message: "Please wait other members to join"}, nil
		}

		b = existing

		// Add human member (PostgreSQL only; MySQL written at battle end)
		hm := s.newMember(b.ID, studentID, models.MemberTypeParticipant, battleType)
		if err := s.members.Create(ctx, hm); err != nil {
			return nil, fmt.Errorf("find: create participant: %w", err)
		}

		// AI opponent
		if battleType == models.BattleTypeAI {
			if _, err := makeAIOpponent(ctx, s.students, s.members, b, []int{studentID}); err != nil {
				return nil, fmt.Errorf("find: make ai opponent: %w", err)
			}
		}

		// PHP: if battle.level_id > student.level_id → lower it
		if b.LevelID > student.LevelID {
			b.LevelID = student.LevelID
			if err := s.battles.Save(ctx, b); err != nil {
				return nil, fmt.Errorf("find: update level: %w", err)
			}
		}

		memberCount, err = s.members.CountByBattle(ctx, b.ID)
		if err != nil {
			return nil, fmt.Errorf("find: count members: %w", err)
		}
	} else {
		// Create new battle
		memberCount = 1

		status := models.BattleStatusWaiting
		if battleType == models.BattleTypeAI {
			status = models.BattleStatusOnGoing
		}

		uuid, err := generateUUID()
		if err != nil {
			return nil, fmt.Errorf("find: generate uuid: %w", err)
		}

		b = &models.Battle{
			UUID:         uuid,
			Type:         battleType,
			LobbyType:    lobbyType,
			Status:       status,
			LevelID:      student.LevelID,
			LevelGroupID: student.LevelGroupID,
			CourseID:     student.CourseID,
		}
		if err := s.battles.Create(ctx, b); err != nil {
			return nil, fmt.Errorf("find: create battle: %w", err)
		}

		// Add creator member (PostgreSQL only; MySQL written at battle end)
		hm := s.newMember(b.ID, studentID, models.MemberTypeCreator, battleType)
		if err := s.members.Create(ctx, hm); err != nil {
			return nil, fmt.Errorf("find: create creator: %w", err)
		}

		// AI opponent
		if battleType == models.BattleTypeAI {
			if _, err := makeAIOpponent(ctx, s.students, s.members, b, []int{studentID}); err != nil {
				return nil, fmt.Errorf("find: make ai opponent: %w", err)
			}
		}
	}

	// AI: generate questions and start immediately
	if battleType == models.BattleTypeAI {
		return s.startAIBattle(ctx, b, studentID)
	}

	// P2P/GROUP: transition to ON_QUEUE when lobby is full
	if memberCount >= models.BattleMemberCount[battleType] {
		if err := s.setOnQueue(ctx, b); err != nil {
			return nil, err
		}
	}

	mems, err := s.members.FindByBattleID(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("find: load members: %w", err)
	}

	_ = s.realtime.PublishBattleWithMembers(ctx, b, mems) // non-fatal

	return &FindResult{Battle: b, Members: mems, Message: "Please wait other members to join"}, nil
}

// startAIBattle generates questions, starts battle, fills AI answers, and publishes.
func (s *FindService) startAIBattle(ctx context.Context, b *models.Battle, humanStudentID int) (*FindResult, error) {
	questions, err := fetchQuestions(ctx, b, s.qr, s.config)
	if err != nil {
		return nil, fmt.Errorf("start ai battle: fetch questions: %w", err)
	}

	if len(questions) == 0 {
		// PHP returns early if no questions — battle created but not started
		mems, _ := s.members.FindByBattleID(ctx, b.ID)
		return &FindResult{Battle: b, Members: mems}, nil
	}

	if err := startBattle(ctx, b, questions, s.battles); err != nil {
		return nil, err
	}

	mems, err := s.members.FindByBattleID(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("start ai battle: load members: %w", err)
	}

	// Publish battle + all members (PHP fires this before AI answers are filled)
	_ = s.realtime.PublishBattleWithMembers(ctx, b, mems)

	// Fill and save AI member answers (PostgreSQL only; MySQL written at battle end)
	for _, m := range mems {
		if m.Type != models.MemberTypeAI {
			continue
		}
		fillAIAnswers(b, m)
		if err := s.members.Save(ctx, m); err != nil {
			return nil, fmt.Errorf("start ai battle: save ai member: %w", err)
		}
		_ = s.realtime.PublishMember(ctx, b.UUID, m)
	}

	return &FindResult{Battle: b, Members: mems}, nil
}

func (s *FindService) setOnQueue(ctx context.Context, b *models.Battle) error {
	expire := time.Now().Add(20 * time.Second)
	b.Status = models.BattleStatusOnQueue
	b.ExpireTime = &expire
	if err := s.battles.Save(ctx, b); err != nil {
		return fmt.Errorf("find: set on queue: %w", err)
	}
	return nil
}

func (s *FindService) checkDemoLimit(ctx context.Context, student *models.Student) error {
	isDemo := false
	for _, v := range models.StudentStatusDemoValues {
		if student.Status == v {
			isDemo = true
			break
		}
	}
	if !isDemo {
		return nil
	}

	count, err := s.students.CountFinishedBattlesToday(ctx, student.ID)
	if err != nil {
		return fmt.Errorf("find: demo limit check: %w", err)
	}
	if count >= s.config.DemoLimit {
		return ErrDemoLimitExceeded
	}
	return nil
}

func (s *FindService) newMember(battleID, studentID, memberType, battleType int) *models.BattleMember {
	status := models.MemberStatusNotConfirmed
	if battleType == models.BattleTypeAI {
		status = models.MemberStatusConfirmed
	}
	return &models.BattleMember{
		BattleID:        battleID,
		StudentID:       studentID,
		Status:          status,
		Type:            memberType,
		CurrentQuestion: 1,
	}
}
