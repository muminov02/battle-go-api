package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"

	"battle-go-api/internal/models"
)

// QuestionReader implements db.QuestionReader using MySQL.
// Replicates BattleExerciseQuestionService and BattleWordQuestionService logic.
type QuestionReader struct {
	db *sql.DB
}

func NewQuestionReader(database *sql.DB) *QuestionReader {
	return &QuestionReader{db: database}
}

// FindJsonTemplate returns a random pre-built question set from json_question table.
// Returns nil, nil if none found.
func (r *QuestionReader) FindJsonTemplate(ctx context.Context, levelGroupID, questionType int) ([]models.Question, error) {
	var dataJSON []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT data FROM json_question
		WHERE level_group = ? AND type = ?
		ORDER BY RAND() LIMIT 1`,
		levelGroupID, questionType,
	).Scan(&dataJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find json template: %w", err)
	}

	var questions []models.Question
	if err = json.Unmarshal(dataJSON, &questions); err != nil {
		return nil, fmt.Errorf("json template unmarshal: %w", err)
	}
	return questions, nil
}

// FindGrammarQuestions generates grammar questions via exercise/unit/level chain.
// Replicates BattleExerciseQuestionService.makeQuestions().
func (r *QuestionReader) FindGrammarQuestions(ctx context.Context, levelID int, count int) ([]models.Question, error) {
	// Get level info first
	var courseID, levelGroup, levelOrder int
	err := r.db.QueryRowContext(ctx,
		`SELECT course_id, level_group, `+"`order`"+` FROM level WHERE id = ?`, levelID,
	).Scan(&courseID, &levelGroup, &levelOrder)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("grammar: find level: %w", err)
	}

	// Replicate BattleExerciseQuestionService query
	rows, err := r.db.QueryContext(ctx, `
		SELECT eq.data, e.title AS label
		FROM exercise_question eq
		INNER JOIN exercise_part ep
			ON ep.exercise_id = eq.exercise_id
			AND ep.field_type = 0
			AND ep.field_value = 0
		INNER JOIN exercise e ON e.id = eq.exercise_id
		INNER JOIN exercise_component ec ON ec.id = e.component_id
		INNER JOIN unit u ON u.id = ec.model_id
		INNER JOIN level l ON l.id = u.level_id
		WHERE e.skill_type = 100
		  AND e.type IN (400, 300)
		  AND e.status = 100
		  AND ec.model_type = 100
		  AND ec.status = 100
		  AND u.status = 100
		  AND l.course_id = ?
		  AND (
		      l.level_group < ?
		      OR (l.level_group = ? AND l.`+"`order`"+` <= ?)
		  )
		  AND eq.id NOT IN (
		      SELECT question_id FROM exercise_question_part
		      WHERE type = 300
		  )
		ORDER BY RAND()
		LIMIT ?`,
		courseID, levelGroup, levelGroup, levelOrder, count,
	)
	if err != nil {
		return nil, fmt.Errorf("grammar: query questions: %w", err)
	}
	defer rows.Close()

	var questions []models.Question
	i := 0
	for rows.Next() {
		var dataJSON []byte
		var label string
		if err := rows.Scan(&dataJSON, &label); err != nil {
			return nil, err
		}
		var q models.Question
		if err := json.Unmarshal(dataJSON, &q); err != nil {
			return nil, fmt.Errorf("grammar: unmarshal question: %w", err)
		}
		q.Label = label
		questions = append(questions, q)
		i++
	}
	return questions, rows.Err()
}

// FindVocabularyQuestions generates vocabulary multiple-choice questions.
// Replicates BattleWordQuestionService.makeQuestions().
func (r *QuestionReader) FindVocabularyQuestions(
	ctx context.Context,
	levelID int,
	wordCount, optionCount int,
	translateForeignText, translateOriginText string,
) ([]models.Question, error) {
	words, err := r.fetchWords(ctx, levelID, wordCount)
	if err != nil || len(words) == 0 {
		return nil, err
	}

	questions := make([]models.Question, 0, len(words))
	middle := len(words) / 2

	// Shuffle a copy for distractors
	shuffled := make([]wordRow, len(words))
	copy(shuffled, words)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	for i, w := range words {
		translateEnglish := i < middle

		label := translateOriginText
		questionValue := w.translation
		answerValue := w.word
		if translateEnglish {
			label = translateForeignText
			questionValue = w.word
			answerValue = w.translation
		}

		alternatives := []models.Alternative{
			{ID: w.id, Type: models.AlternativeTypeAnswer, Value: answerValue},
		}
		for _, alt := range shuffled {
			if len(alternatives) >= optionCount {
				break
			}
			if alt.id == w.id {
				continue
			}
			v := alt.word
			if translateEnglish {
				v = alt.translation
			}
			alternatives = append(alternatives, models.Alternative{ID: alt.id, Type: models.AlternativeTypeOption, Value: v})
		}
		rand.Shuffle(len(alternatives), func(a, b int) { alternatives[a], alternatives[b] = alternatives[b], alternatives[a] })

		questions = append(questions, models.Question{
			ID:    w.id,
			Value: questionValue,
			Label: label,
			Order: 1,
			Options: []models.QuestionOption{
				{Order: 1, Alternatives: alternatives},
			},
			NoValue: true,
		})
	}

	rand.Shuffle(len(questions), func(i, j int) { questions[i], questions[j] = questions[j], questions[i] })
	return questions, nil
}

type wordRow struct {
	id          int
	word        string
	translation string
}

func (r *QuestionReader) fetchWords(ctx context.Context, levelID, count int) ([]wordRow, error) {
	var usedUnits []int
	var words []wordRow

	for len(words) < count {
		unitID, err := r.randomUnit(ctx, levelID, usedUnits)
		if err != nil || unitID == 0 {
			break
		}
		usedUnits = append(usedUnits, unitID)

		batch, err := r.unitWords(ctx, unitID, count-len(words))
		if err != nil {
			return nil, err
		}
		words = append(words, batch...)
	}
	return words, nil
}

func (r *QuestionReader) randomUnit(ctx context.Context, levelID int, used []int) (int, error) {
	query := `SELECT id FROM unit WHERE level_id = ? AND status = 100`
	args := []interface{}{levelID}

	if len(used) > 0 {
		query += ` AND id NOT IN (?`
		for i := 1; i < len(used); i++ {
			query += `,?`
		}
		query += `)`
		for _, id := range used {
			args = append(args, id)
		}
	}
	query += ` ORDER BY RAND() LIMIT 1`

	var id int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (r *QuestionReader) unitWords(ctx context.Context, unitID, limit int) ([]wordRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.id, w.word, w.translation
		FROM word w
		INNER JOIN unit_word_pack uwp ON uwp.word_id = w.id
		WHERE uwp.unit_id = ?
		ORDER BY RAND()
		LIMIT ?`, unitID, limit)
	if err != nil {
		return nil, fmt.Errorf("unit words: %w", err)
	}
	defer rows.Close()

	var words []wordRow
	for rows.Next() {
		var w wordRow
		if err := rows.Scan(&w.id, &w.word, &w.translation); err != nil {
			return nil, err
		}
		words = append(words, w)
	}
	return words, rows.Err()
}
