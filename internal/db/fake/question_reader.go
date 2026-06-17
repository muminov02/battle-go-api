package fake

import (
	"context"
	"fmt"
	"sync"

	"battle-go-api/internal/models"
)

// QuestionReader is an in-memory QuestionReader for tests.
type QuestionReader struct {
	mu               sync.RWMutex
	templates        map[string][]models.Question // key: "levelGroupID:type"
	grammarQuestions map[int][]models.Question    // key: levelID
	vocabQuestions   map[int][]models.Question    // key: levelID
}

func NewQuestionReader() *QuestionReader {
	return &QuestionReader{
		templates:        make(map[string][]models.Question),
		grammarQuestions: make(map[int][]models.Question),
		vocabQuestions:   make(map[int][]models.Question),
	}
}

func templateKey(levelGroupID, questionType int) string {
	return fmt.Sprintf("%d:%d", levelGroupID, questionType)
}

// SetTemplate seeds a JSON template (test setup helper).
func (r *QuestionReader) SetTemplate(levelGroupID, questionType int, questions []models.Question) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[templateKey(levelGroupID, questionType)] = questions
}

// SetGrammarQuestions seeds grammar questions for a level (test setup helper).
func (r *QuestionReader) SetGrammarQuestions(levelID int, questions []models.Question) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grammarQuestions[levelID] = questions
}

// SetVocabQuestions seeds vocabulary questions for a level (test setup helper).
func (r *QuestionReader) SetVocabQuestions(levelID int, questions []models.Question) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vocabQuestions[levelID] = questions
}

func (r *QuestionReader) FindJsonTemplate(_ context.Context, levelGroupID, questionType int) ([]models.Question, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	qs, ok := r.templates[templateKey(levelGroupID, questionType)]
	if !ok {
		return nil, nil
	}
	return qs, nil
}

func (r *QuestionReader) FindGrammarQuestions(_ context.Context, levelID int, count int) ([]models.Question, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	qs := r.grammarQuestions[levelID]
	if count < len(qs) {
		qs = qs[:count]
	}
	return qs, nil
}

func (r *QuestionReader) FindVocabularyQuestions(_ context.Context, levelID int, wordCount, _ int, _, _ string) ([]models.Question, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	qs := r.vocabQuestions[levelID]
	if wordCount < len(qs) {
		qs = qs[:wordCount]
	}
	return qs, nil
}
