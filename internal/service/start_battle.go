package service

import (
	"context"
	"fmt"

	battlelogic "battle-go-api/internal/battle"
	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// fetchQuestions returns questions for a battle based on lobby_type.
// Tries JsonQuestion template first (RAND()), falls back to dynamic generation.
// Returns nil, nil when no questions available (PHP does the same — generate returns early).
func fetchQuestions(ctx context.Context, b *models.Battle, qr db.QuestionReader, cfg Config) ([]models.Question, error) {
	// Try pre-defined json_question first (random by level_group + grammar/vocab type).
	// json_question.type uses JsonQuestionType (grammar=100, vocab=110), NOT lobby type.
	jsonType := models.JsonQuestionTypeForLobby(b.LobbyType)
	qs, err := qr.FindJsonTemplate(ctx, b.LevelGroupID, jsonType)
	if err != nil {
		return nil, fmt.Errorf("fetch questions template: %w", err)
	}
	if len(qs) > 0 {
		return qs, nil
	}

	// Dynamic generation
	switch b.LobbyType {
	case models.LobbyTypeGrammar:
		qs, err = qr.FindGrammarQuestions(ctx, b.LevelID, cfg.ExerciseQuestionCount)
	case models.LobbyTypeVocabulary:
		qs, err = qr.FindVocabularyQuestions(ctx, b.LevelID,
			cfg.WordQuestionCount, cfg.WordOptionCount,
			cfg.TranslateForeignText, cfg.TranslateOriginText)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch questions dynamic: %w", err)
	}
	return qs, nil
}

// startBattle sets questions, times, and ON_GOING status on battle; saves to PostgreSQL only.
// MySQL is written at battle end (EndBattleService). Does NOT publish to realtime — caller handles that.
func startBattle(
	ctx context.Context,
	b *models.Battle,
	questions []models.Question,
	battles db.BattleRepository,
) error {
	start, end := battlelogic.CalcBattleTimes(len(questions))
	b.Questions = questions
	b.StartTime = &start
	b.EndTime = &end
	b.Status = models.BattleStatusOnGoing

	if err := battles.Save(ctx, b); err != nil {
		return fmt.Errorf("start battle save pg: %w", err)
	}
	return nil
}
