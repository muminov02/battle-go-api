package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"battle-go-api/internal/models"
)

// JsonDataModelType constants mirror PHP's JsonDataModelEnum.
const (
	JsonDataModelBattle            = 100
	JsonDataModelBattleMember      = 200
	JsonDataModelBattleMemberDebug = 300
	JsonDataModelBattleDebug       = 400
)

// ResultWriter implements db.ResultWriter using MySQL.
// Called once at battle end to persist final results.
type ResultWriter struct {
	db *sql.DB
}

func NewResultWriter(database *sql.DB) *ResultWriter {
	return &ResultWriter{db: database}
}

// SaveBattle upserts the battle row in MySQL and writes json_data for questions.
// Mirrors PHP $battle->save() + JsonData::saveOrUpdate().
func (w *ResultWriter) SaveBattle(ctx context.Context, b *models.Battle) error {
	questionsJSON, err := json.Marshal(b.Questions)
	if err != nil {
		return fmt.Errorf("save battle marshal questions: %w", err)
	}

	_, err = w.db.ExecContext(ctx, `
		INSERT INTO battle (uuid, type, lobby_type, level_group_id, level_id, status, questions, start_time, end_time, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			status     = VALUES(status),
			questions  = VALUES(questions),
			start_time = VALUES(start_time),
			end_time   = VALUES(end_time)`,
		b.UUID, b.Type, b.LobbyType, b.LevelGroupID, b.LevelID,
		b.Status, questionsJSON, b.StartTime, b.EndTime)
	if err != nil {
		return fmt.Errorf("save battle upsert: %w", err)
	}

	var mysqlID int
	err = w.db.QueryRowContext(ctx, `SELECT id FROM battle WHERE uuid = ?`, b.UUID).Scan(&mysqlID)
	if err != nil {
		return fmt.Errorf("save battle get id: %w", err)
	}

	if err := w.upsertJsonData(ctx, mysqlID, JsonDataModelBattle, b.Questions); err != nil {
		return fmt.Errorf("save battle json_data: %w", err)
	}
	return nil
}

// CreateMember inserts a new battle_member row and writes its json_data (answers double-write).
func (w *ResultWriter) CreateMember(ctx context.Context, battleUUID string, m *models.BattleMember) error {
	answersJSON, err := json.Marshal(m.Answers)
	if err != nil {
		return fmt.Errorf("create member marshal: %w", err)
	}
	res, err := w.db.ExecContext(ctx, `
		INSERT INTO battle_member (battle_id, student_id, status, type, current_question, answers, place, points, is_finished)
		SELECT b.id, ?, ?, ?, ?, ?, ?, ?, ?
		FROM battle b WHERE b.uuid = ?`,
		m.StudentID, m.Status, m.Type, m.CurrentQuestion,
		answersJSON, m.Place, m.Points, m.IsFinished, battleUUID)
	if err != nil {
		return fmt.Errorf("create member insert: %w", err)
	}

	memberID, _ := res.LastInsertId()
	if memberID == 0 {
		// INSERT ... SELECT didn't surface an id — look it up.
		_ = w.db.QueryRowContext(ctx, `
			SELECT bm.id FROM battle_member bm
			INNER JOIN battle b ON b.id = bm.battle_id
			WHERE b.uuid = ? AND bm.student_id = ?
			ORDER BY bm.id DESC LIMIT 1`,
			battleUUID, m.StudentID).Scan(&memberID)
	}
	if memberID != 0 {
		if err := w.upsertJsonData(ctx, int(memberID), JsonDataModelBattleMember, m.Answers); err != nil {
			return fmt.Errorf("create member json_data: %w", err)
		}
	}
	return nil
}

// UpdateMember updates an existing battle_member row with final results.
// Called at battle end or when a member leaves mid-battle.
func (w *ResultWriter) UpdateMember(ctx context.Context, battleUUID string, m *models.BattleMember) error {
	answersJSON, err := json.Marshal(m.Answers)
	if err != nil {
		return fmt.Errorf("update member marshal: %w", err)
	}

	// Get the single canonical row id
	var memberID int
	err = w.db.QueryRowContext(ctx, `
		SELECT bm.id FROM battle_member bm
		INNER JOIN battle b ON b.id = bm.battle_id
		WHERE b.uuid = ? AND bm.student_id = ?
		ORDER BY bm.id ASC LIMIT 1`,
		battleUUID, m.StudentID).Scan(&memberID)
	if err == sql.ErrNoRows {
		// No row found — fall back to insert (shouldn't happen in normal flow)
		return w.CreateMember(ctx, battleUUID, m)
	}
	if err != nil {
		return fmt.Errorf("update member lookup: %w", err)
	}

	_, err = w.db.ExecContext(ctx, `
		UPDATE battle_member SET answers=?, place=?, points=?, is_finished=?,
		    current_question=?, status=?
		WHERE id=?`,
		answersJSON, m.Place, m.Points, m.IsFinished,
		m.CurrentQuestion, m.Status, memberID)
	if err != nil {
		return fmt.Errorf("update member: %w", err)
	}

	// Delete any duplicate rows for this member in this battle
	w.db.ExecContext(ctx, `DELETE FROM battle_member WHERE battle_id=`+
		`(SELECT id FROM battle WHERE uuid=?) AND student_id=? AND id!=?`,
		battleUUID, m.StudentID, memberID)

	if err := w.upsertJsonData(ctx, memberID, JsonDataModelBattleMember, m.Answers); err != nil {
		return fmt.Errorf("update member json_data: %w", err)
	}
	return nil
}

// SaveJsonData upserts a json_data row. Mirrors PHP JsonData::saveOrUpdate().
func (w *ResultWriter) SaveJsonData(ctx context.Context, modelID, modelType int, data interface{}) error {
	return w.upsertJsonData(ctx, modelID, modelType, data)
}

func (w *ResultWriter) upsertJsonData(ctx context.Context, modelID, modelType int, data interface{}) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("save json data marshal: %w", err)
	}

	var existingID int
	err = w.db.QueryRowContext(ctx,
		`SELECT id FROM json_data WHERE model_id = ? AND model_type = ? LIMIT 1`,
		modelID, modelType,
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		_, err = w.db.ExecContext(ctx,
			`INSERT INTO json_data (model_id, model_type, data) VALUES (?, ?, ?)`,
			modelID, modelType, dataJSON)
		return err
	}
	if err != nil {
		return fmt.Errorf("json data select: %w", err)
	}
	_, err = w.db.ExecContext(ctx,
		`UPDATE json_data SET data = ? WHERE id = ?`, dataJSON, existingID)
	return err
}

// UpsertStudentBattle increments win or lose count in student_battle table.
func (w *ResultWriter) UpsertStudentBattle(ctx context.Context, studentID int, won bool) error {
	if won {
		_, err := w.db.ExecContext(ctx, `
			INSERT INTO student_battle (student_id, win_count, lose_count)
			VALUES (?, 1, 0)
			ON DUPLICATE KEY UPDATE win_count = win_count + 1`,
			studentID)
		return err
	}
	_, err := w.db.ExecContext(ctx, `
		INSERT INTO student_battle (student_id, win_count, lose_count)
		VALUES (?, 0, 1)
		ON DUPLICATE KEY UPDATE lose_count = lose_count + 1`,
		studentID)
	return err
}

