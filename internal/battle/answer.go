package battle

import (
	"errors"
	"strings"

	"battle-go-api/internal/models"
)

const BaseAnswerPoints = 500
const NoAnswerText = "(no answer)"
const NoAnswerTime = 10000

var ErrQuestionNotFound = errors.New("question not found")

// ProcessAnswer validates submitted values against the question, appends the answer
// to member.Answers, increments CurrentQuestion, and sets IsFinished on last question.
//
// Idempotent: if question_id already answered, returns (true, nil) without changes.
func ProcessAnswer(
	battle *models.Battle,
	member *models.BattleMember,
	questionID int,
	values []string,
	answerTime int,
) (isDuplicate bool, err error) {
	// Find question
	var question *models.Question
	for i := range battle.Questions {
		if battle.Questions[i].ID == questionID {
			question = &battle.Questions[i]
			break
		}
	}
	if question == nil {
		return false, ErrQuestionNotFound
	}

	// Check duplicate
	for _, a := range member.Answers {
		if a.QuestionID == questionID {
			return true, nil
		}
	}

	// Collect correct answer alternatives
	var correctAnswers []string
	for _, opt := range question.Options {
		for _, alt := range opt.Alternatives {
			if alt.Type == models.AlternativeTypeAnswer {
				correctAnswers = append(correctAnswers, alt.Value)
			}
		}
	}

	// Check correctness — mirror PHP logic:
	// for each option, count how many submitted values match a correct answer.
	// if correctCounter == len(values) → correct.
	normalize := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(s, " ", ""))
	}

	correctCounter := 0
	for _, alt := range correctAnswers {
		for _, v := range values {
			if normalize(alt) == normalize(v) {
				correctCounter++
			}
		}
	}

	isCorrect := correctCounter == len(values) && len(values) > 0
	points := 0
	if isCorrect {
		points = BaseAnswerPoints
	}

	correctAnswer := ""
	if len(correctAnswers) > 0 {
		correctAnswer = correctAnswers[0]
	}

	answer := models.Answer{
		QuestionID:     questionID,
		Question:       question.Value,
		Values:         values,
		Time:           answerTime,
		IsCorrect:      isCorrect,
		Points:         points,
		CorrectAnswer:  correctAnswer,
		CorrectAnswers: correctAnswers,
	}

	member.Answers = append(member.Answers, answer)

	// Advance current_question; set is_finished on last question
	if len(member.Answers) == len(battle.Questions) {
		member.IsFinished = true
		member.CurrentQuestion = len(battle.Questions)
	} else {
		member.CurrentQuestion++
	}

	return false, nil
}

// FillBlanks fills all unanswered questions for a member with blank wrong answers.
// Preserves existing answers. Sets IsFinished=true.
func FillBlanks(battle *models.Battle, member *models.BattleMember) {
	answered := make(map[int]bool, len(member.Answers))
	for _, a := range member.Answers {
		answered[a.QuestionID] = true
	}

	for _, q := range battle.Questions {
		if answered[q.ID] {
			continue
		}

		var correctAnswers []string
		for _, opt := range q.Options {
			for _, alt := range opt.Alternatives {
				if alt.Type == models.AlternativeTypeAnswer {
					correctAnswers = append(correctAnswers, alt.Value)
				}
			}
		}

		correctAnswer := "-"
		if len(correctAnswers) > 0 {
			correctAnswer = correctAnswers[0]
		}

		member.Answers = append(member.Answers, models.Answer{
			QuestionID:     q.ID,
			Question:       q.Value,
			Values:         []string{NoAnswerText},
			Time:           NoAnswerTime,
			IsCorrect:      false,
			Points:         0,
			CorrectAnswer:  correctAnswer,
			CorrectAnswers: correctAnswers,
		})
	}

	member.IsFinished = true
	member.CurrentQuestion = len(battle.Questions)
}
