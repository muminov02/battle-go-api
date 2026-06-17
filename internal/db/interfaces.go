package db

import (
	"context"
	"errors"

	"battle-go-api/internal/models"
)

var ErrNotFound = errors.New("not found")

// BattleRepository manages active battle state in PostgreSQL.
type BattleRepository interface {
	// Create inserts a new battle, sets b.ID on success.
	Create(ctx context.Context, b *models.Battle) error

	// FindByUUID returns nil, nil when not found.
	FindByUUID(ctx context.Context, uuid string) (*models.Battle, error)

	// FindWaiting returns one WAITING battle matching type/lobbyType/courseID/levelGroupID, nil if none.
	FindWaiting(ctx context.Context, battleType, lobbyType, courseID, levelGroupID int) (*models.Battle, error)

	// FindExpiredOnQueue returns ON_QUEUE battles whose expire_time < now.
	FindExpiredOnQueue(ctx context.Context) ([]*models.Battle, error)

	// FindOnGoingExpired returns ON_GOING battles whose end_time < now.
	FindOnGoingExpired(ctx context.Context) ([]*models.Battle, error)

	// FindOnGoingAllMembersFinished returns ON_GOING battles where every member has is_finished=true.
	FindOnGoingAllMembersFinished(ctx context.Context) ([]*models.Battle, error)

	// FindOnGoing returns all ON_GOING battles (used for the per-member idle check).
	FindOnGoing(ctx context.Context) ([]*models.Battle, error)

	// Save updates all mutable fields of an existing battle.
	Save(ctx context.Context, b *models.Battle) error

	// Delete removes the battle and cascades to members.
	Delete(ctx context.Context, battleID int) error
}

// MemberRepository manages battle members in PostgreSQL.
type MemberRepository interface {
	// Create inserts a new member, sets m.ID on success.
	Create(ctx context.Context, m *models.BattleMember) error

	// FindByBattleID returns all members for a battle.
	FindByBattleID(ctx context.Context, battleID int) ([]*models.BattleMember, error)

	// FindByBattleAndStudent returns nil, nil when not found.
	FindByBattleAndStudent(ctx context.Context, battleID, studentID int) (*models.BattleMember, error)

	// CountByBattle returns total member count.
	CountByBattle(ctx context.Context, battleID int) (int, error)

	// CountConfirmedByBattle returns count of CONFIRMED members.
	CountConfirmedByBattle(ctx context.Context, battleID int) (int, error)

	// ExistsInBattle returns true if student is already a member.
	ExistsInBattle(ctx context.Context, battleID, studentID int) (bool, error)

	// Save updates all mutable fields of an existing member.
	Save(ctx context.Context, m *models.BattleMember) error

	// Delete removes one member.
	Delete(ctx context.Context, memberID int) error

	// DeleteByBattle removes all members of a battle.
	DeleteByBattle(ctx context.Context, battleID int) error

	// DeleteByBattleAndStudent removes one specific member.
	DeleteByBattleAndStudent(ctx context.Context, battleID, studentID int) error
}

// StudentReader reads student data from MySQL (read-only).
type StudentReader interface {
	// FindByID returns student with level/course data. Returns ErrNotFound if absent.
	FindByID(ctx context.Context, studentID int) (*models.Student, error)

	// FindTestingUserIDs returns user_id list of is_testing_user=1 students, excluding given IDs.
	FindTestingUserIDs(ctx context.Context, excludeIDs []int) ([]int, error)

	// CountFinishedBattlesToday counts FINISHED battles for student today (for demo limit).
	CountFinishedBattlesToday(ctx context.Context, studentID int) (int, error)
}

// ProfileReader builds the public student profile (member.student in responses).
type ProfileReader interface {
	GetPublicProfile(ctx context.Context, studentID int) (*models.PublicProfile, error)
}

// QuestionReader generates/fetches questions from MySQL (read-only).
type QuestionReader interface {
	// FindJsonTemplate returns cached questions from json_question table, nil if none.
	FindJsonTemplate(ctx context.Context, levelGroupID, questionType int) ([]models.Question, error)

	// FindGrammarQuestions generates grammar questions via exercise/unit/level chain.
	FindGrammarQuestions(ctx context.Context, levelID int, count int) ([]models.Question, error)

	// FindVocabularyQuestions generates vocabulary multiple-choice questions.
	FindVocabularyQuestions(ctx context.Context, levelID int, wordCount, optionCount int, translateForeignText, translateOriginText string) ([]models.Question, error)
}

// ResultWriter writes final battle results to MySQL.
type ResultWriter interface {
	// SaveBattle upserts battle row in MySQL battle table.
	SaveBattle(ctx context.Context, b *models.Battle) error

	// CreateMember inserts a new member row in MySQL (called once when member joins).
	CreateMember(ctx context.Context, battleUUID string, m *models.BattleMember) error

	// UpdateMember updates existing member row in MySQL (called at battle end/leave).
	UpdateMember(ctx context.Context, battleUUID string, m *models.BattleMember) error

	// SaveJsonData upserts a json_data row (model_id, model_type, data).
	SaveJsonData(ctx context.Context, modelID, modelType int, data interface{}) error

	// UpsertStudentBattle increments win or lose count in student_battle table.
	UpsertStudentBattle(ctx context.Context, studentID int, won bool) error
}
