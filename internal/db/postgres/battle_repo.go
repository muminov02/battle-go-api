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

// BattleRepository implements db.BattleRepository using PostgreSQL.
type BattleRepository struct {
	pool *pgxpool.Pool
}

func NewBattleRepository(pool *pgxpool.Pool) *BattleRepository {
	return &BattleRepository{pool: pool}
}

func (r *BattleRepository) Create(ctx context.Context, b *models.Battle) error {
	questionsJSON, err := json.Marshal(b.Questions)
	if err != nil {
		return fmt.Errorf("battle create marshal: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO battles
			(uuid, type, lobby_type, level_id, level_group_id, course_id,
			 status, start_time, expire_time, end_time, questions, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
		RETURNING id`,
		b.UUID, b.Type, b.LobbyType, b.LevelID, b.LevelGroupID, b.CourseID,
		b.Status, b.StartTime, b.ExpireTime, b.EndTime, questionsJSON,
	).Scan(&b.ID)
	return err
}

func (r *BattleRepository) FindByUUID(ctx context.Context, uuid string) (*models.Battle, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, uuid, type, lobby_type, level_id, level_group_id, course_id,
		       status, start_time, expire_time, end_time, questions, created_at
		FROM battles WHERE uuid = $1`, uuid)
	return scanBattle(row)
}

func (r *BattleRepository) FindWaiting(ctx context.Context, battleType, lobbyType, courseID, levelGroupID int) (*models.Battle, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, uuid, type, lobby_type, level_id, level_group_id, course_id,
		       status, start_time, expire_time, end_time, questions, created_at
		FROM battles
		WHERE status = $1 AND type = $2 AND lobby_type = $3
		  AND course_id = $4 AND level_group_id = $5
		LIMIT 1`,
		models.BattleStatusWaiting, battleType, lobbyType, courseID, levelGroupID)
	return scanBattle(row)
}

func (r *BattleRepository) FindExpiredOnQueue(ctx context.Context) ([]*models.Battle, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, uuid, type, lobby_type, level_id, level_group_id, course_id,
		       status, start_time, expire_time, end_time, questions, created_at
		FROM battles
		WHERE status = $1 AND expire_time < NOW()`,
		models.BattleStatusOnQueue)
	if err != nil {
		return nil, err
	}
	return collectBattles(rows)
}

func (r *BattleRepository) FindOnGoingExpired(ctx context.Context) ([]*models.Battle, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, uuid, type, lobby_type, level_id, level_group_id, course_id,
		       status, start_time, expire_time, end_time, questions, created_at
		FROM battles
		WHERE status = $1 AND end_time < NOW()`,
		models.BattleStatusOnGoing)
	if err != nil {
		return nil, err
	}
	return collectBattles(rows)
}

func (r *BattleRepository) FindOnGoingAllMembersFinished(ctx context.Context) ([]*models.Battle, error) {
	// Battles where status=ON_GOING and no member has is_finished=false
	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.uuid, b.type, b.lobby_type, b.level_id, b.level_group_id, b.course_id,
		       b.status, b.start_time, b.expire_time, b.end_time, b.questions, b.created_at
		FROM battles b
		WHERE b.status = $1
		  AND NOT EXISTS (
		      SELECT 1 FROM battle_members bm
		      WHERE bm.battle_id = b.id AND bm.is_finished = false
		  )`,
		models.BattleStatusOnGoing)
	if err != nil {
		return nil, err
	}
	return collectBattles(rows)
}

func (r *BattleRepository) FindOnGoing(ctx context.Context) ([]*models.Battle, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, uuid, type, lobby_type, level_id, level_group_id, course_id,
		       status, start_time, expire_time, end_time, questions, created_at
		FROM battles
		WHERE status = $1`,
		models.BattleStatusOnGoing)
	if err != nil {
		return nil, err
	}
	return collectBattles(rows)
}

func (r *BattleRepository) Save(ctx context.Context, b *models.Battle) error {
	questionsJSON, err := json.Marshal(b.Questions)
	if err != nil {
		return fmt.Errorf("battle save marshal: %w", err)
	}

	tag, err := r.pool.Exec(ctx, `
		UPDATE battles SET
			status = $1, start_time = $2, expire_time = $3, end_time = $4,
			questions = $5, level_id = $6
		WHERE id = $7`,
		b.Status, b.StartTime, b.ExpireTime, b.EndTime,
		questionsJSON, b.LevelID, b.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (r *BattleRepository) Delete(ctx context.Context, battleID int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM battles WHERE id = $1`, battleID)
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func scanBattle(row pgx.Row) (*models.Battle, error) {
	var b models.Battle
	var questionsJSON []byte

	err := row.Scan(
		&b.ID, &b.UUID, &b.Type, &b.LobbyType, &b.LevelID, &b.LevelGroupID, &b.CourseID,
		&b.Status, &b.StartTime, &b.ExpireTime, &b.EndTime, &questionsJSON, &b.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if len(questionsJSON) > 0 {
		if err = json.Unmarshal(questionsJSON, &b.Questions); err != nil {
			return nil, fmt.Errorf("battle scan unmarshal questions: %w", err)
		}
	}
	return &b, nil
}

func collectBattles(rows pgx.Rows) ([]*models.Battle, error) {
	defer rows.Close()
	var battles []*models.Battle
	for rows.Next() {
		var b models.Battle
		var questionsJSON []byte
		if err := rows.Scan(
			&b.ID, &b.UUID, &b.Type, &b.LobbyType, &b.LevelID, &b.LevelGroupID, &b.CourseID,
			&b.Status, &b.StartTime, &b.ExpireTime, &b.EndTime, &questionsJSON, &b.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(questionsJSON) > 0 {
			if err := json.Unmarshal(questionsJSON, &b.Questions); err != nil {
				return nil, err
			}
		}
		battles = append(battles, &b)
	}
	return battles, rows.Err()
}
