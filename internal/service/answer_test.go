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

func setupAnswer() (
	*fake.BattleRepository,
	*fake.MemberRepository,
	*fake.ResultWriter,
	*fakesvc.RealtimeService,
	*svc.AnswerService,
) {
	b := fake.NewBattleRepository()
	m := fake.NewMemberRepository()
	r := fake.NewResultWriter()
	rt := fakesvc.NewRealtimeService()
	return b, m, r, rt, svc.NewAnswerService(b, m, r, rt)
}

func seedOnGoingBattle(t *testing.T, battles *fake.BattleRepository, members *fake.MemberRepository, studentID int) (*models.Battle, *models.BattleMember) {
	t.Helper()
	q := makeGrammarQuestion(1)
	q2 := makeGrammarQuestion(2)
	b := &models.Battle{
		UUID:      "battle-uuid",
		Type:      models.BattleTypeP2P,
		LobbyType: models.LobbyTypeGrammar,
		Status:    models.BattleStatusOnGoing,
		Questions: []models.Question{q, q2},
	}
	require.NoError(t, battles.Create(context.Background(), b))
	m := &models.BattleMember{BattleID: b.ID, StudentID: studentID, Status: models.MemberStatusConfirmed, CurrentQuestion: 1}
	require.NoError(t, members.Create(context.Background(), m))
	return b, m
}

func TestAnswer_BattleNotFound(t *testing.T) {
	_, _, _, _, s := setupAnswer()
	_, err := s.Execute(context.Background(), 1, "no-uuid", 1, []string{"A"}, 3000)
	require.ErrorIs(t, err, svc.ErrBattleNotFound)
}

func TestAnswer_BattleNotStarted_Waiting(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	b := &models.Battle{UUID: "x", Status: models.BattleStatusWaiting}
	require.NoError(t, battles.Create(context.Background(), b))
	m := &models.BattleMember{BattleID: b.ID, StudentID: 1}
	require.NoError(t, members.Create(context.Background(), m))

	_, err := s.Execute(context.Background(), 1, "x", 1, []string{"A"}, 3000)
	require.ErrorIs(t, err, svc.ErrBattleNotStarted)
}

func TestAnswer_BattleNotStarted_OnQueue(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	b := &models.Battle{UUID: "x", Status: models.BattleStatusOnQueue}
	require.NoError(t, battles.Create(context.Background(), b))
	m := &models.BattleMember{BattleID: b.ID, StudentID: 1}
	require.NoError(t, members.Create(context.Background(), m))

	_, err := s.Execute(context.Background(), 1, "x", 1, []string{"A"}, 3000)
	require.ErrorIs(t, err, svc.ErrBattleNotStarted)
}

func TestAnswer_BattleFinished(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	b := &models.Battle{UUID: "x", Status: models.BattleStatusFinished}
	require.NoError(t, battles.Create(context.Background(), b))
	m := &models.BattleMember{BattleID: b.ID, StudentID: 1}
	require.NoError(t, members.Create(context.Background(), m))

	_, err := s.Execute(context.Background(), 1, "x", 1, []string{"A"}, 3000)
	require.ErrorIs(t, err, svc.ErrBattleFinished)
}

func TestAnswer_NotMember(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	seedOnGoingBattle(t, battles, members, 1)
	_, err := s.Execute(context.Background(), 99, "battle-uuid", 1, []string{"A"}, 3000)
	require.ErrorIs(t, err, svc.ErrNotMember)
}

func TestAnswer_QuestionNotFound(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	seedOnGoingBattle(t, battles, members, 1)
	res, err := s.Execute(context.Background(), 1, "battle-uuid", 999, []string{"A"}, 3000)
	require.NoError(t, err)
	assert.False(t, res.Status)
	assert.Equal(t, "Question does not exist", res.Message)
}

func TestAnswer_CorrectAnswer(t *testing.T) {
	battles, members, _, rt, s := setupAnswer()
	seedOnGoingBattle(t, battles, members, 1)

	// Question 1 has correct answer "A"
	res, err := s.Execute(context.Background(), 1, "battle-uuid", 1, []string{"A"}, 3000)
	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, "Success", res.Message)

	// Member state updated
	m, err := members.FindByBattleAndStudent(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Len(t, m.Answers, 1)
	assert.Equal(t, 1, m.Answers[0].QuestionID)
	assert.True(t, m.Answers[0].IsCorrect)
	assert.Equal(t, 500, m.Answers[0].Points)
	assert.Equal(t, 2, m.CurrentQuestion) // advanced to Q2


	// Realtime published
	assert.Len(t, rt.MemberPublishes, 1)
}

func TestAnswer_WrongAnswer(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	seedOnGoingBattle(t, battles, members, 1)

	res, err := s.Execute(context.Background(), 1, "battle-uuid", 1, []string{"WRONG"}, 3000)
	require.NoError(t, err)
	assert.True(t, res.Status)

	m, err := members.FindByBattleAndStudent(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.False(t, m.Answers[0].IsCorrect)
	assert.Equal(t, 0, m.Answers[0].Points)
}

func TestAnswer_Duplicate(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	seedOnGoingBattle(t, battles, members, 1)

	_, err := s.Execute(context.Background(), 1, "battle-uuid", 1, []string{"A"}, 3000)
	require.NoError(t, err)

	// Submit same question again
	res, err := s.Execute(context.Background(), 1, "battle-uuid", 1, []string{"A"}, 3000)
	require.NoError(t, err)
	assert.True(t, res.Status)

	// Only one answer saved (no duplicate)
	m, err := members.FindByBattleAndStudent(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.Len(t, m.Answers, 1)

	// MySQL only saved once (first answer)
}

func TestAnswer_LastQuestion_SetsFinished(t *testing.T) {
	battles, members, _, _, s := setupAnswer()
	seedOnGoingBattle(t, battles, members, 1) // 2 questions

	s.Execute(context.Background(), 1, "battle-uuid", 1, []string{"A"}, 1000)
	s.Execute(context.Background(), 1, "battle-uuid", 2, []string{"A"}, 1000)

	m, err := members.FindByBattleAndStudent(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.True(t, m.IsFinished)
	assert.Len(t, m.Answers, 2)
}
