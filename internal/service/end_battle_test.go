package service_test

import (
	"context"
	"testing"

	"battle-go-api/internal/db/fake"
	"battle-go-api/internal/models"
	svc "battle-go-api/internal/service"
	fakesvc "battle-go-api/internal/service/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEndBattle() (
	*fake.BattleRepository,
	*fake.MemberRepository,
	*fake.ResultWriter,
	*fakesvc.RealtimeService,
	*svc.EndBattleService,
) {
	b := fake.NewBattleRepository()
	m := fake.NewMemberRepository()
	r := fake.NewResultWriter()
	rt := fakesvc.NewRealtimeService()
	return b, m, r, rt, svc.NewEndBattleService(b, m, r, rt)
}

func makeAnsweredMember(battleID, studentID, points int, isFinished bool) *models.BattleMember {
	p := points
	return &models.BattleMember{
		BattleID:        battleID,
		StudentID:       studentID,
		Status:          models.MemberStatusConfirmed,
		CurrentQuestion: 1,
		IsFinished:      isFinished,
		Answers: []models.Answer{
			{QuestionID: 1, Question: "Q1", IsCorrect: points > 0, Points: points, Time: 5000, Values: []string{"A"}, CorrectAnswer: "A"},
		},
		Points: &p,
	}
}

func TestEndBattle_SetsFinishedStatus(t *testing.T) {
	battles, members, results, _, s := setupEndBattle()

	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{makeGrammarQuestion(1)}}
	require.NoError(t, battles.Create(context.Background(), b))

	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, Status: models.MemberStatusConfirmed, CurrentQuestion: 1, IsFinished: true}
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, Status: models.MemberStatusConfirmed, CurrentQuestion: 1, IsFinished: true}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1, m2}, false)
	require.NoError(t, err)

	assert.Equal(t, models.BattleStatusFinished, b.Status)

	// PG battle saved
	saved := battles.All()
	require.Len(t, saved, 1)
	assert.Equal(t, models.BattleStatusFinished, saved[0].Status)

	// MySQL battle saved
	assert.Len(t, results.SavedBattles(), 1)
}

func TestEndBattle_FillsBlanksWhenRequired(t *testing.T) {
	battles, members, _, _, s := setupEndBattle()

	q1 := makeGrammarQuestion(1)
	q2 := makeGrammarQuestion(2)
	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{q1, q2}}
	require.NoError(t, battles.Create(context.Background(), b))

	// m1 answered Q1 but not Q2 (unfinished)
	m1 := &models.BattleMember{
		BattleID: b.ID, StudentID: 1, Status: models.MemberStatusConfirmed,
		CurrentQuestion: 2, IsFinished: false,
		Answers: []models.Answer{{QuestionID: 1, Question: "Q", IsCorrect: true, Points: 500, Time: 1000, Values: []string{"A"}, CorrectAnswer: "A"}},
	}
	// m2 is finished
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, Status: models.MemberStatusConfirmed, CurrentQuestion: 2, IsFinished: true,
		Answers: []models.Answer{
			{QuestionID: 1, Question: "Q", IsCorrect: true, Points: 500, Time: 2000, Values: []string{"A"}, CorrectAnswer: "A"},
			{QuestionID: 2, Question: "Q", IsCorrect: false, Points: 0, Time: 3000, Values: []string{"x"}, CorrectAnswer: "A"},
		},
	}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1, m2}, true)
	require.NoError(t, err)

	// m1 should have 2 answers now (Q2 blank-filled)
	assert.Len(t, m1.Answers, 2)
	assert.True(t, m1.IsFinished)
	assert.Equal(t, 2, m1.CurrentQuestion)

	// m2 unchanged
	assert.Len(t, m2.Answers, 2)
}

func TestEndBattle_NoFillBlanksWhenNotRequired(t *testing.T) {
	battles, members, _, _, s := setupEndBattle()

	q := makeGrammarQuestion(1)
	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{q}}
	require.NoError(t, battles.Create(context.Background(), b))

	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, IsFinished: true, Answers: []models.Answer{
		{QuestionID: 1, IsCorrect: true, Points: 500, Time: 1000, Values: []string{"A"}},
	}}
	require.NoError(t, members.Create(context.Background(), m1))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1}, false)
	require.NoError(t, err)

	assert.Len(t, m1.Answers, 1) // not changed
}

func TestEndBattle_CalculatesResults(t *testing.T) {
	battles, members, _, _, s := setupEndBattle()

	q := makeGrammarQuestion(1)
	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{q}}
	require.NoError(t, battles.Create(context.Background(), b))

	// m1 answers correctly fast, m2 correct slow
	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, IsFinished: true,
		Answers: []models.Answer{{QuestionID: 1, Question: "Q", IsCorrect: true, Points: 500, Time: 1000, Values: []string{"A"}, CorrectAnswer: "A"}},
	}
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, IsFinished: true,
		Answers: []models.Answer{{QuestionID: 1, Question: "Q", IsCorrect: true, Points: 500, Time: 5000, Values: []string{"A"}, CorrectAnswer: "A"}},
	}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1, m2}, false)
	require.NoError(t, err)

	// m1 should be 1st place with 1 point (P2P, < 6 members)
	require.NotNil(t, m1.Place)
	require.NotNil(t, m1.Points)
	assert.Equal(t, 1, *m1.Place)
	assert.Equal(t, 1, *m1.Points) // 1st place, < 6 members

	// m1 got +50 bonus (P2P fastest correct)
	assert.Equal(t, 550, m1.Answers[0].Points)

	require.NotNil(t, m2.Place)
	assert.Equal(t, 2, *m2.Place)
}

func TestEndBattle_UpdatesWinLossStats(t *testing.T) {
	battles, members, results, _, s := setupEndBattle()

	q := makeGrammarQuestion(1)
	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{q}}
	require.NoError(t, battles.Create(context.Background(), b))

	// m1 wins (will get points=1), m2 loses
	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, IsFinished: true,
		Answers: []models.Answer{{QuestionID: 1, Question: "Q", IsCorrect: true, Points: 500, Time: 1000, Values: []string{"A"}, CorrectAnswer: "A"}},
	}
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, IsFinished: true,
		Answers: []models.Answer{{QuestionID: 1, Question: "Q", IsCorrect: false, Points: 0, Time: 1000, Values: []string{"X"}, CorrectAnswer: "A"}},
	}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1, m2}, false)
	require.NoError(t, err)

	// Student 1 wins
	rec1 := results.StudentBattleFor(1)
	require.NotNil(t, rec1)
	assert.Equal(t, 1, rec1.WinCount)
	assert.Equal(t, 0, rec1.LoseCount)

	// Student 2 loses
	rec2 := results.StudentBattleFor(2)
	require.NotNil(t, rec2)
	assert.Equal(t, 0, rec2.WinCount)
	assert.Equal(t, 1, rec2.LoseCount)
}

func TestEndBattle_PublishesToRealtime(t *testing.T) {
	battles, members, _, rt, s := setupEndBattle()

	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{makeGrammarQuestion(1)}}
	require.NoError(t, battles.Create(context.Background(), b))

	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, IsFinished: true}
	require.NoError(t, members.Create(context.Background(), m1))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1}, false)
	require.NoError(t, err)

	assert.Len(t, rt.BattlePublishes, 1)
	assert.Equal(t, models.BattleStatusFinished, rt.BattlePublishes[0].Battle.Status)
}

func TestEndBattle_SavesMembersToMySQL(t *testing.T) {
	battles, members, results, _, s := setupEndBattle()

	b := &models.Battle{UUID: "end-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnGoing, Questions: []models.Question{makeGrammarQuestion(1)}}
	require.NoError(t, battles.Create(context.Background(), b))

	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, IsFinished: true}
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, IsFinished: true}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))

	err := s.Execute(context.Background(), b, []*models.BattleMember{m1, m2}, false)
	require.NoError(t, err)

	// 2 members saved to MySQL
	assert.Len(t, results.SavedMembers(), 2)
}
