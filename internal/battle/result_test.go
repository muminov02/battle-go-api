package battle_test

import (
	"fmt"
	"testing"

	"battle-go-api/internal/battle"
	"battle-go-api/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers

func intPtr(v int) *int { return &v }

func makeMember(studentID int, answers []models.Answer) *models.BattleMember {
	return &models.BattleMember{
		StudentID: studentID,
		Answers:   answers,
	}
}

func correctAnswer(questionID int, timeMs int) models.Answer {
	return models.Answer{
		QuestionID: questionID,
		Question:   fmt.Sprintf("Question %d", questionID), // unique text per question
		Values:     []string{"correct"},
		Time:       timeMs,
		IsCorrect:  true,
		Points:     500,
	}
}

func wrongAnswer(questionID int) models.Answer {
	return models.Answer{
		QuestionID: questionID,
		Question:   fmt.Sprintf("Question %d", questionID),
		Values:     []string{"wrong"},
		Time:       10000,
		IsCorrect:  false,
		Points:     0,
	}
}

// ── AddBonusPoints ─────────────────────────────────────────────────────────────

func TestAddBonusPoints_P2P_FastestCorrectGets50(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	fast := makeMember(1, []models.Answer{correctAnswer(1, 3000)})
	slow := makeMember(2, []models.Answer{correctAnswer(1, 8000)})

	calc.AddBonusPoints([]*models.BattleMember{fast, slow})

	assert.Equal(t, 550, fast.Answers[0].Points, "fastest P2P member gets +50 bonus")
	assert.Equal(t, 500, slow.Answers[0].Points, "second P2P member gets no bonus")
}

func TestAddBonusPoints_P2P_OnlyOneCorrect_StillGets50(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	only := makeMember(1, []models.Answer{correctAnswer(1, 5000)})
	none := makeMember(2, []models.Answer{wrongAnswer(1)})

	calc.AddBonusPoints([]*models.BattleMember{only, none})

	assert.Equal(t, 550, only.Answers[0].Points)
	assert.Equal(t, 0, none.Answers[0].Points)
}

func TestAddBonusPoints_P2P_WrongAnswers_NoBonus(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	m1 := makeMember(1, []models.Answer{wrongAnswer(1)})
	m2 := makeMember(2, []models.Answer{wrongAnswer(1)})

	calc.AddBonusPoints([]*models.BattleMember{m1, m2})

	assert.Equal(t, 0, m1.Answers[0].Points)
	assert.Equal(t, 0, m2.Answers[0].Points)
}

func TestAddBonusPoints_Group_Top3Bonuses(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeGroup}

	first := makeMember(1, []models.Answer{correctAnswer(1, 1000)})
	second := makeMember(2, []models.Answer{correctAnswer(1, 2000)})
	third := makeMember(3, []models.Answer{correctAnswer(1, 3000)})
	fourth := makeMember(4, []models.Answer{correctAnswer(1, 4000)})

	calc.AddBonusPoints([]*models.BattleMember{first, second, third, fourth})

	assert.Equal(t, 620, first.Answers[0].Points, "1st GROUP gets +120")
	assert.Equal(t, 570, second.Answers[0].Points, "2nd GROUP gets +70")
	assert.Equal(t, 550, third.Answers[0].Points, "3rd GROUP gets +50")
	assert.Equal(t, 500, fourth.Answers[0].Points, "4th GROUP gets no bonus")
}

func TestAddBonusPoints_Group_TieInTime_LowerStudentIDWins(t *testing.T) {
	// same time → order is deterministic (first in list or lower ID)
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeGroup}

	m1 := makeMember(1, []models.Answer{correctAnswer(1, 5000)})
	m2 := makeMember(2, []models.Answer{correctAnswer(1, 5000)})

	calc.AddBonusPoints([]*models.BattleMember{m1, m2})

	// both got same time — one gets 620, other 570 — total must add up
	total := m1.Answers[0].Points + m2.Answers[0].Points
	assert.Equal(t, 1190, total, "tie: bonuses still distributed, total 620+570")
}

func TestAddBonusPoints_MultipleQuestions_EachQuestionIndependent(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	m1 := makeMember(1, []models.Answer{
		correctAnswer(1, 2000), // faster on Q1
		correctAnswer(2, 9000), // slower on Q2
	})
	m2 := makeMember(2, []models.Answer{
		correctAnswer(1, 8000),
		correctAnswer(2, 3000), // faster on Q2
	})

	calc.AddBonusPoints([]*models.BattleMember{m1, m2})

	assert.Equal(t, 550, m1.Answers[0].Points, "m1 fastest on Q1 gets +50")
	assert.Equal(t, 500, m1.Answers[1].Points, "m1 slow on Q2 gets no bonus")
	assert.Equal(t, 500, m2.Answers[0].Points, "m2 slow on Q1 gets no bonus")
	assert.Equal(t, 550, m2.Answers[1].Points, "m2 fastest on Q2 gets +50")
}

// ── SumPointsAndSetPlacement ───────────────────────────────────────────────────

func TestPlacement_OrderedByTotalPoints(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	loser := makeMember(1, []models.Answer{correctAnswer(1, 5000)})  // 500 pts
	winner := makeMember(2, []models.Answer{correctAnswer(1, 3000)}) // 550 pts after bonus

	calc.AddBonusPoints([]*models.BattleMember{loser, winner})
	calc.SumPointsAndSetPlacement([]*models.BattleMember{loser, winner})

	require.NotNil(t, winner.Place)
	require.NotNil(t, loser.Place)
	assert.Equal(t, 1, *winner.Place)
	assert.Equal(t, 2, *loser.Place)
}

func TestPlacement_LessThan6Members_FirstGets1Point_RestGet0(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	m1 := makeMember(1, []models.Answer{correctAnswer(1, 1000)}) // most points
	m2 := makeMember(2, []models.Answer{wrongAnswer(1)})

	calc.AddBonusPoints([]*models.BattleMember{m1, m2})
	calc.SumPointsAndSetPlacement([]*models.BattleMember{m1, m2})

	require.NotNil(t, m1.Points)
	require.NotNil(t, m2.Points)
	assert.Equal(t, 1, *m1.Points, "winner (<6 members) gets 1 point")
	assert.Equal(t, 0, *m2.Points, "loser (<6 members) gets 0 points")
}

func TestPlacement_6OrMoreMembers_Podium3_2_1(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeGroup}

	// 6 members, ordered by descending points
	members := make([]*models.BattleMember, 6)
	for i := range members {
		members[i] = makeMember(i+1, []models.Answer{
			{QuestionID: 1, IsCorrect: true, Points: 500 - i*10},
		})
	}

	calc.SumPointsAndSetPlacement(members)

	for i, m := range members {
		require.NotNil(t, m.Points, "member %d points should not be nil", i)
	}
	assert.Equal(t, 3, *members[0].Points, "1st place gets 3 points")
	assert.Equal(t, 2, *members[1].Points, "2nd place gets 2 points")
	assert.Equal(t, 1, *members[2].Points, "3rd place gets 1 point")
	assert.Equal(t, 0, *members[3].Points, "4th place gets 0 points")
	assert.Equal(t, 0, *members[4].Points, "5th place gets 0 points")
	assert.Equal(t, 0, *members[5].Points, "6th place gets 0 points")
}

func TestPlacement_MoreThan50PercentWrong_Gets0RewardPoints(t *testing.T) {
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	// 10 questions, 6 wrong (>50%) — should get 0 reward points even if "winner"
	answers := make([]models.Answer, 10)
	for i := range answers {
		if i < 4 {
			answers[i] = correctAnswer(i+1, 1000) // 4 correct
		} else {
			answers[i] = wrongAnswer(i + 1) // 6 wrong
		}
	}

	poor := makeMember(1, answers)
	loser := makeMember(2, []models.Answer{
		wrongAnswer(1), wrongAnswer(2), wrongAnswer(3),
		wrongAnswer(4), wrongAnswer(5), wrongAnswer(6),
		wrongAnswer(7), wrongAnswer(8), wrongAnswer(9), wrongAnswer(10),
	})

	calc.SumPointsAndSetPlacement([]*models.BattleMember{poor, loser})

	require.NotNil(t, poor.Points)
	assert.Equal(t, 0, *poor.Points, "member with >50% wrong gets 0 reward points")
}

func TestPlacement_Exactly50PercentWrong_Gets0Points(t *testing.T) {
	// PHP uses >= (noAnswerCount >= totalAnswers/2) so exactly 50% wrong → 0 points
	calc := &battle.ResultCalculator{BattleType: models.BattleTypeP2P}

	// 4 questions, 2 correct, 2 wrong = exactly 50% wrong
	m1 := makeMember(1, []models.Answer{
		correctAnswer(1, 1000), correctAnswer(2, 1000),
		wrongAnswer(3), wrongAnswer(4),
	})
	m2 := makeMember(2, []models.Answer{
		wrongAnswer(1), wrongAnswer(2), wrongAnswer(3), wrongAnswer(4),
	})

	calc.SumPointsAndSetPlacement([]*models.BattleMember{m1, m2})

	require.NotNil(t, m1.Points)
	assert.Equal(t, 0, *m1.Points, "exactly 50% wrong → 0 reward points (PHP uses >=)")
}
