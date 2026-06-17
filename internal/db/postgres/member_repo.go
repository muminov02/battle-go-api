package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemberRepository implements db.MemberRepository using PostgreSQL.
type MemberRepository struct {
	pool *pgxpool.Pool
}

func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository {
	return &MemberRepository{pool: pool}
}

func (r *MemberRepository) Create(ctx context.Context, m *models.BattleMember) error {
	answersJSON, err := json.Marshal(m.Answers)
	if err != nil {
		return fmt.Errorf("member create marshal: %w", err)
	}
	if answersJSON == nil {
		answersJSON = []byte("[]")
	}

	return r.pool.QueryRow(ctx, `
		INSERT INTO battle_members
			(student_id, battle_id, status, current_question, answers, is_finished, type, last_question_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		m.StudentID, m.BattleID, m.Status, m.CurrentQuestion,
		answersJSON, m.IsFinished, m.Type, m.LastQuestionAt,
	).Scan(&m.ID)
}

func (r *MemberRepository) FindByBattleID(ctx context.Context, battleID int) ([]*models.BattleMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, student_id, battle_id, place, points, status,
		       current_question, answers, is_finished, type, last_question_at
		FROM battle_members WHERE battle_id = $1`, battleID)
	if err != nil {
		return nil, err
	}
	return collectMembers(rows)
}

func (r *MemberRepository) FindByBattleAndStudent(ctx context.Context, battleID, studentID int) (*models.BattleMember, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, student_id, battle_id, place, points, status,
		       current_question, answers, is_finished, type, last_question_at
		FROM battle_members WHERE battle_id = $1 AND student_id = $2`,
		battleID, studentID)
	return scanMember(row)
}

func (r *MemberRepository) CountByBattle(ctx context.Context, battleID int) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM battle_members WHERE battle_id = $1`, battleID,
	).Scan(&count)
	return count, err
}

func (r *MemberRepository) CountConfirmedByBattle(ctx context.Context, battleID int) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM battle_members WHERE battle_id = $1 AND status = $2`,
		battleID, models.MemberStatusConfirmed,
	).Scan(&count)
	return count, err
}

func (r *MemberRepository) ExistsInBattle(ctx context.Context, battleID, studentID int) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM battle_members WHERE battle_id = $1 AND student_id = $2)`,
		battleID, studentID,
	).Scan(&exists)
	return exists, err
}

func (r *MemberRepository) Save(ctx context.Context, m *models.BattleMember) error {
	answersJSON, err := json.Marshal(m.Answers)
	if err != nil {
		return fmt.Errorf("member save marshal: %w", err)
	}

	tag, err := r.pool.Exec(ctx, `
		UPDATE battle_members SET
			status = $1, current_question = $2, answers = $3,
			is_finished = $4, place = $5, points = $6, last_question_at = $7
		WHERE id = $8`,
		m.Status, m.CurrentQuestion, answersJSON,
		m.IsFinished, m.Place, m.Points, m.LastQuestionAt, m.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (r *MemberRepository) Delete(ctx context.Context, memberID int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM battle_members WHERE id = $1`, memberID)
	return err
}

func (r *MemberRepository) DeleteByBattle(ctx context.Context, battleID int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM battle_members WHERE battle_id = $1`, battleID)
	return err
}

func (r *MemberRepository) DeleteByBattleAndStudent(ctx context.Context, battleID, studentID int) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM battle_members WHERE battle_id = $1 AND student_id = $2`,
		battleID, studentID)
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func scanMember(row pgx.Row) (*models.BattleMember, error) {
	var m models.BattleMember
	var answersJSON []byte

	err := row.Scan(
		&m.ID, &m.StudentID, &m.BattleID, &m.Place, &m.Points,
		&m.Status, &m.CurrentQuestion, &answersJSON, &m.IsFinished, &m.Type, &m.LastQuestionAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if len(answersJSON) > 0 {
		if err = json.Unmarshal(answersJSON, &m.Answers); err != nil {
			return nil, fmt.Errorf("member scan unmarshal answers: %w", err)
		}
	}
	return &m, nil
}

func collectMembers(rows pgx.Rows) ([]*models.BattleMember, error) {
	defer rows.Close()
	var members []*models.BattleMember
	for rows.Next() {
		var m models.BattleMember
		var answersJSON []byte
		if err := rows.Scan(
			&m.ID, &m.StudentID, &m.BattleID, &m.Place, &m.Points,
			&m.Status, &m.CurrentQuestion, &answersJSON, &m.IsFinished, &m.Type, &m.LastQuestionAt,
		); err != nil {
			return nil, err
		}
		if len(answersJSON) > 0 {
			if err := json.Unmarshal(answersJSON, &m.Answers); err != nil {
				return nil, err
			}
		}
		members = append(members, &m)
	}
	return members, rows.Err()
}
