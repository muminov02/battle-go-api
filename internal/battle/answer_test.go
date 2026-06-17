package battle_test

import (
	"testing"

	"battle-go-api/internal/battle"
	"battle-go-api/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func battleWithQuestion(questions ...models.Question) *models.Battle {
	return &models.Battle{
		ID:        1,
		UUID:      "test-uuid",
		Status:    models.BattleStatusOnGoing,
		Questions: questions,
	}
}

func memberWithAnswers(battleID int, answers ...models.Answer) *models.BattleMember {
	return &models.BattleMember{
		ID:              1,
		BattleID:        battleID,
		StudentID:       42,
		Answers:         answers,
		CurrentQuestion: len(answers) + 1,
	}
}

func questionWith(id int, correctValue string) models.Question {
	return models.Question{
		ID:    id,
		Value: "What is the answer?",
		Options: []models.QuestionOption{
			{
				Order: 1,
				Alternatives: []models.Alternative{
					{ID: id, Type: models.AlternativeTypeAnswer, Value: correctValue},
					{ID: id + 100, Type: 200, Value: "wrong1"},
					{ID: id + 200, Type: 200, Value: "wrong2"},
				},
			},
		},
	}
}

// ── Correct / Wrong ────────────────────────────────────────────────────────────

func TestProcessAnswer_CorrectAnswer_500Points(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1)

	isDup, err := battle.ProcessAnswer(b, m, 1, []string{"apple"}, 3000)

	require.NoError(t, err)
	assert.False(t, isDup)
	require.Len(t, m.Answers, 1)
	assert.True(t, m.Answers[0].IsCorrect)
	assert.Equal(t, 500, m.Answers[0].Points)
	assert.Equal(t, 3000, m.Answers[0].Time)
}

func TestProcessAnswer_WrongAnswer_0Points(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1)

	isDup, err := battle.ProcessAnswer(b, m, 1, []string{"banana"}, 5000)

	require.NoError(t, err)
	assert.False(t, isDup)
	require.Len(t, m.Answers, 1)
	assert.False(t, m.Answers[0].IsCorrect)
	assert.Equal(t, 0, m.Answers[0].Points)
}

func TestProcessAnswer_CaseInsensitive(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "Apple"))
	m := memberWithAnswers(1)

	_, err := battle.ProcessAnswer(b, m, 1, []string{"apple"}, 1000)

	require.NoError(t, err)
	require.Len(t, m.Answers, 1)
	assert.True(t, m.Answers[0].IsCorrect)
}

func TestProcessAnswer_WhitespaceIgnored(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "hello world"))
	m := memberWithAnswers(1)

	_, err := battle.ProcessAnswer(b, m, 1, []string{"helloworld"}, 1000)

	require.NoError(t, err)
	require.Len(t, m.Answers, 1)
	assert.True(t, m.Answers[0].IsCorrect)
}

func TestProcessAnswer_StoresCorrectAnswers(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1)

	_, err := battle.ProcessAnswer(b, m, 1, []string{"banana"}, 1000)

	require.NoError(t, err)
	require.Len(t, m.Answers, 1)
	assert.Equal(t, "apple", m.Answers[0].CorrectAnswer)
	assert.Contains(t, m.Answers[0].CorrectAnswers, "apple")
}

// ── Idempotency ────────────────────────────────────────────────────────────────

func TestProcessAnswer_DuplicateQuestionID_Ignored(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"), questionWith(2, "banana"))
	m := memberWithAnswers(1, models.Answer{QuestionID: 1, IsCorrect: true, Points: 500})

	isDup, err := battle.ProcessAnswer(b, m, 1, []string{"apple"}, 2000)

	require.NoError(t, err)
	assert.True(t, isDup, "should be flagged as duplicate")
	assert.Len(t, m.Answers, 1, "no new answer appended")
}

// ── is_finished ────────────────────────────────────────────────────────────────

func TestProcessAnswer_LastQuestion_SetsIsFinished(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"), questionWith(2, "banana"))
	// already answered Q1
	m := memberWithAnswers(1, models.Answer{QuestionID: 1, IsCorrect: true, Points: 500})
	m.CurrentQuestion = 2

	_, err := battle.ProcessAnswer(b, m, 2, []string{"banana"}, 2000)

	require.NoError(t, err)
	require.Len(t, m.Answers, 2)
	assert.True(t, m.IsFinished, "member should be finished after last question")
	assert.Equal(t, 2, m.CurrentQuestion)
}

func TestProcessAnswer_NotLastQuestion_DoesNotSetIsFinished(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"), questionWith(2, "banana"))
	m := memberWithAnswers(1)
	m.CurrentQuestion = 1

	_, err := battle.ProcessAnswer(b, m, 1, []string{"apple"}, 1000)

	require.NoError(t, err)
	assert.False(t, m.IsFinished)
	assert.Equal(t, 2, m.CurrentQuestion, "current_question incremented")
}

func TestProcessAnswer_UnknownQuestionID_ReturnsError(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1)

	_, err := battle.ProcessAnswer(b, m, 999, []string{"apple"}, 1000)

	assert.Error(t, err, "unknown question_id should return error")
}

// ── FillBlanks ─────────────────────────────────────────────────────────────────

func TestFillBlanks_UnansweredQuestions_FilledAsWrong(t *testing.T) {
	b := battleWithQuestion(
		questionWith(1, "apple"),
		questionWith(2, "banana"),
		questionWith(3, "cherry"),
	)
	// member answered only Q1
	m := memberWithAnswers(1, models.Answer{QuestionID: 1, IsCorrect: true, Points: 500})

	battle.FillBlanks(b, m)

	require.Len(t, m.Answers, 3, "all 3 questions should be present")
	assert.Equal(t, 1, m.Answers[0].QuestionID, "Q1 preserved")
	assert.Equal(t, 2, m.Answers[1].QuestionID, "Q2 filled")
	assert.Equal(t, 3, m.Answers[2].QuestionID, "Q3 filled")
	assert.False(t, m.Answers[1].IsCorrect)
	assert.Equal(t, 0, m.Answers[1].Points)
}

func TestFillBlanks_AnsweredQuestions_NotOverwritten(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1, models.Answer{
		QuestionID: 1,
		IsCorrect:  true,
		Points:     500,
		Values:     []string{"apple"},
	})

	battle.FillBlanks(b, m)

	assert.Len(t, m.Answers, 1)
	assert.True(t, m.Answers[0].IsCorrect, "existing correct answer not overwritten")
	assert.Equal(t, 500, m.Answers[0].Points)
}

func TestFillBlanks_SetsIsFinished(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1)

	battle.FillBlanks(b, m)

	assert.True(t, m.IsFinished)
	assert.Equal(t, 1, m.CurrentQuestion)
}

func TestFillBlanks_BlankAnswer_UsesNoAnswerText(t *testing.T) {
	b := battleWithQuestion(questionWith(1, "apple"))
	m := memberWithAnswers(1)

	battle.FillBlanks(b, m)

	assert.Equal(t, []string{battle.NoAnswerText}, m.Answers[0].Values)
	assert.Equal(t, battle.NoAnswerTime, m.Answers[0].Time)
}
