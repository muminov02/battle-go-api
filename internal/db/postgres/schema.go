package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate creates all tables and indexes needed by the battle Go service.
// Safe to call on every startup — all statements use IF NOT EXISTS.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS battles (
			id            SERIAL PRIMARY KEY,
			uuid          VARCHAR(36) UNIQUE NOT NULL,
			type          INT NOT NULL,
			lobby_type    INT NOT NULL,
			level_id      INT NOT NULL,
			level_group_id INT NOT NULL,
			course_id     INT NOT NULL,
			status        INT NOT NULL DEFAULT 100,
			start_time    TIMESTAMPTZ,
			expire_time   TIMESTAMPTZ,
			end_time      TIMESTAMPTZ,
			questions     JSONB,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_battles_status
			ON battles(status)`,

		`CREATE INDEX IF NOT EXISTS idx_battles_waiting_match
			ON battles(type, lobby_type, course_id, level_group_id, status)`,

		`CREATE INDEX IF NOT EXISTS idx_battles_expire_time
			ON battles(expire_time) WHERE expire_time IS NOT NULL`,

		`CREATE INDEX IF NOT EXISTS idx_battles_end_time
			ON battles(end_time) WHERE end_time IS NOT NULL`,

		`CREATE TABLE IF NOT EXISTS battle_members (
			id               SERIAL PRIMARY KEY,
			student_id       INT NOT NULL,
			battle_id        INT NOT NULL REFERENCES battles(id) ON DELETE CASCADE,
			place            INT,
			points           INT,
			status           INT NOT NULL DEFAULT 100,
			current_question INT NOT NULL DEFAULT 1,
			answers          JSONB NOT NULL DEFAULT '[]'::jsonb,
			is_finished      BOOLEAN NOT NULL DEFAULT FALSE,
			type             INT NOT NULL DEFAULT 100,
			last_question_at TIMESTAMPTZ
		)`,

		// Idempotent column add for existing deployments (table created before this column).
		`ALTER TABLE battle_members ADD COLUMN IF NOT EXISTS last_question_at TIMESTAMPTZ`,

		`CREATE INDEX IF NOT EXISTS idx_battle_members_battle_id
			ON battle_members(battle_id)`,

		`CREATE UNIQUE INDEX IF NOT EXISTS idx_battle_members_battle_student
			ON battle_members(battle_id, student_id)`,

		// Partial index for daemon: find ON_GOING battles with all members finished
		`CREATE INDEX IF NOT EXISTS idx_battle_members_is_finished
			ON battle_members(battle_id, is_finished)`,
	}

	for _, stmt := range statements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}
