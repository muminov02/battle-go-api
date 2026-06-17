package service

import (
	"context"
	"math/rand"

	"battle-go-api/internal/battle"
	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// makeAIOpponent picks a random testing user and creates a CONFIRMED AI member.
func makeAIOpponent(ctx context.Context, students db.StudentReader, members db.MemberRepository, b *models.Battle, excludeStudentIDs []int) (*models.BattleMember, error) {
	ids, err := students.FindTestingUserIDs(ctx, excludeStudentIDs)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrNoTestingUsers
	}

	m := &models.BattleMember{
		BattleID:        b.ID,
		StudentID:       ids[rand.Intn(len(ids))],
		Status:          models.MemberStatusConfirmed,
		IsFinished:      false,
		CurrentQuestion: 1,
		Type:            models.MemberTypeAI,
	}
	if err := members.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// fillAIAnswers pre-fills an AI member's answers for all questions.
// Mirrors PHP BattleQuestionService.makeAIOpponent:
//   - random_int(6,8) correct answers, rest wrong
//   - all with Time=8000ms (8s per question)
func fillAIAnswers(b *models.Battle, member *models.BattleMember) {
	questions := b.Questions
	if len(questions) == 0 {
		return
	}

	correctCount := rand.Intn(3) + 6 // 6, 7, or 8
	answers := make([]models.Answer, len(questions))

	for i, q := range questions {
		isCorrect := i < correctCount

		var correctAnswers []string
		for _, opt := range q.Options {
			for _, alt := range opt.Alternatives {
				if alt.Type == models.AlternativeTypeAnswer {
					correctAnswers = append(correctAnswers, alt.Value)
				}
			}
		}

		correctAnswer := ""
		if len(correctAnswers) > 0 {
			correctAnswer = correctAnswers[0]
		}

		value := "answer" // PHP uses "answer" as the placeholder wrong value
		points := 0
		if isCorrect {
			value = correctAnswer
			points = battle.BaseAnswerPoints
		}

		answers[i] = models.Answer{
			QuestionID:     q.ID,
			Question:       q.Value,
			Values:         []string{value},
			Time:           8000, // AI answers each question in 8s
			IsCorrect:      isCorrect,
			Points:         points,
			CorrectAnswer:  correctAnswer,
			CorrectAnswers: correctAnswers,
		}
	}

	member.Answers = answers
	member.IsFinished = true
	member.CurrentQuestion = len(questions)
}
