package fake_test

import (
	"context"
	"testing"
	"time"

	"battle-go-api/internal/db/fake"
	"battle-go-api/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// ── BattleRepository ──────────────────────────────────────────────────────────

func TestBattleRepo_CreateAndFindByUUID(t *testing.T) {
	repo := fake.NewBattleRepository()
	b := &models.Battle{UUID: "abc-123", Type: models.BattleTypeP2P, Status: models.BattleStatusWaiting, CourseID: 1, LevelGroupID: 2}

	err := repo.Create(ctx, b)
	require.NoError(t, err)
	assert.Greater(t, b.ID, 0, "ID assigned")

	found, err := repo.FindByUUID(ctx, "abc-123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, b.ID, found.ID)
}

func TestBattleRepo_FindByUUID_NotFound_ReturnsNil(t *testing.T) {
	repo := fake.NewBattleRepository()
	found, err := repo.FindByUUID(ctx, "no-such-uuid")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestBattleRepo_FindWaiting_MatchesAllCriteria(t *testing.T) {
	repo := fake.NewBattleRepository()
	b := &models.Battle{
		UUID: "w1", Type: models.BattleTypeP2P, LobbyType: models.LobbyTypeGrammar,
		CourseID: 5, LevelGroupID: 3, Status: models.BattleStatusWaiting,
	}
	require.NoError(t, repo.Create(ctx, b))

	found, err := repo.FindWaiting(ctx, models.BattleTypeP2P, models.LobbyTypeGrammar, 5, 3)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "w1", found.UUID)
}

func TestBattleRepo_FindWaiting_WrongCourseID_ReturnsNil(t *testing.T) {
	repo := fake.NewBattleRepository()
	b := &models.Battle{UUID: "w1", Type: models.BattleTypeP2P, LobbyType: models.LobbyTypeGrammar, CourseID: 5, LevelGroupID: 3, Status: models.BattleStatusWaiting}
	require.NoError(t, repo.Create(ctx, b))

	found, err := repo.FindWaiting(ctx, models.BattleTypeP2P, models.LobbyTypeGrammar, 99, 3)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestBattleRepo_FindExpiredOnQueue(t *testing.T) {
	repo := fake.NewBattleRepository()
	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Minute)

	expired := &models.Battle{UUID: "e1", Status: models.BattleStatusOnQueue, ExpireTime: &past}
	notYet := &models.Battle{UUID: "e2", Status: models.BattleStatusOnQueue, ExpireTime: &future}
	require.NoError(t, repo.Create(ctx, expired))
	require.NoError(t, repo.Create(ctx, notYet))

	results, err := repo.FindExpiredOnQueue(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "e1", results[0].UUID)
}

func TestBattleRepo_Save_UpdatesRecord(t *testing.T) {
	repo := fake.NewBattleRepository()
	b := &models.Battle{UUID: "s1", Status: models.BattleStatusWaiting}
	require.NoError(t, repo.Create(ctx, b))

	b.Status = models.BattleStatusOnQueue
	require.NoError(t, repo.Save(ctx, b))

	found, _ := repo.FindByUUID(ctx, "s1")
	assert.Equal(t, models.BattleStatusOnQueue, found.Status)
}

func TestBattleRepo_Delete_RemovesBattle(t *testing.T) {
	repo := fake.NewBattleRepository()
	b := &models.Battle{UUID: "d1"}
	require.NoError(t, repo.Create(ctx, b))

	require.NoError(t, repo.Delete(ctx, b.ID))

	found, _ := repo.FindByUUID(ctx, "d1")
	assert.Nil(t, found)
}

// ── MemberRepository ──────────────────────────────────────────────────────────

func TestMemberRepo_CreateAndFind(t *testing.T) {
	repo := fake.NewMemberRepository()
	m := &models.BattleMember{BattleID: 1, StudentID: 42, Status: models.MemberStatusNotConfirmed}

	require.NoError(t, repo.Create(ctx, m))
	assert.Greater(t, m.ID, 0)

	found, err := repo.FindByBattleAndStudent(ctx, 1, 42)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, m.ID, found.ID)
}

func TestMemberRepo_CountByBattle(t *testing.T) {
	repo := fake.NewMemberRepository()
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 1}))
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 2}))
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 2, StudentID: 3}))

	count, err := repo.CountByBattle(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestMemberRepo_CountConfirmedByBattle(t *testing.T) {
	repo := fake.NewMemberRepository()
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 1, Status: models.MemberStatusConfirmed}))
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 2, Status: models.MemberStatusNotConfirmed}))

	count, err := repo.CountConfirmedByBattle(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestMemberRepo_ExistsInBattle(t *testing.T) {
	repo := fake.NewMemberRepository()
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 42}))

	exists, err := repo.ExistsInBattle(ctx, 1, 42)
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = repo.ExistsInBattle(ctx, 1, 99)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestMemberRepo_DeleteByBattle_RemovesAll(t *testing.T) {
	repo := fake.NewMemberRepository()
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 1}))
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 1, StudentID: 2}))
	require.NoError(t, repo.Create(ctx, &models.BattleMember{BattleID: 2, StudentID: 3}))

	require.NoError(t, repo.DeleteByBattle(ctx, 1))

	count, _ := repo.CountByBattle(ctx, 1)
	assert.Equal(t, 0, count)

	count, _ = repo.CountByBattle(ctx, 2)
	assert.Equal(t, 1, count, "other battle unaffected")
}

func TestMemberRepo_Save_UpdatesAnswers(t *testing.T) {
	repo := fake.NewMemberRepository()
	m := &models.BattleMember{BattleID: 1, StudentID: 1}
	require.NoError(t, repo.Create(ctx, m))

	m.IsFinished = true
	m.Answers = []models.Answer{{QuestionID: 1, IsCorrect: true, Points: 500}}
	require.NoError(t, repo.Save(ctx, m))

	found, _ := repo.FindByBattleAndStudent(ctx, 1, 1)
	assert.True(t, found.IsFinished)
	require.Len(t, found.Answers, 1)
	assert.Equal(t, 500, found.Answers[0].Points)
}

// ── StudentReader ─────────────────────────────────────────────────────────────

func TestStudentReader_FindByID(t *testing.T) {
	r := fake.NewStudentReader()
	r.AddStudent(&models.Student{ID: 10, LevelID: 5, CourseID: 2, LevelGroupID: 3})

	s, err := r.FindByID(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, s.LevelID)
}

func TestStudentReader_FindByID_Missing_ReturnsError(t *testing.T) {
	r := fake.NewStudentReader()
	_, err := r.FindByID(ctx, 99)
	assert.Error(t, err)
}

func TestStudentReader_FindTestingUserIDs_ExcludesGiven(t *testing.T) {
	r := fake.NewStudentReader()
	r.AddStudent(&models.Student{ID: 1, IsTestingUser: true})
	r.AddStudent(&models.Student{ID: 2, IsTestingUser: true})
	r.AddStudent(&models.Student{ID: 3, IsTestingUser: false})

	ids, err := r.FindTestingUserIDs(ctx, []int{1})
	require.NoError(t, err)
	assert.Contains(t, ids, 2)
	assert.NotContains(t, ids, 1)
	assert.NotContains(t, ids, 3)
}

// ── ResultWriter ──────────────────────────────────────────────────────────────

func TestResultWriter_UpsertStudentBattle_WinLoseCount(t *testing.T) {
	w := fake.NewResultWriter()

	require.NoError(t, w.UpsertStudentBattle(ctx, 1, true))
	require.NoError(t, w.UpsertStudentBattle(ctx, 1, true))
	require.NoError(t, w.UpsertStudentBattle(ctx, 1, false))

	rec := w.StudentBattleFor(1)
	require.NotNil(t, rec)
	assert.Equal(t, 2, rec.WinCount)
	assert.Equal(t, 1, rec.LoseCount)
}
