package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// StudentReader implements db.StudentReader using MySQL.
type StudentReader struct {
	db *sql.DB
}

func NewStudentReader(database *sql.DB) *StudentReader {
	return &StudentReader{db: database}
}

// FindByID fetches student with joined level data (level_id, level_group, course_id).
func (r *StudentReader) FindByID(ctx context.Context, studentID int) (*models.Student, error) {
	var s models.Student
	err := r.db.QueryRowContext(ctx, `
		SELECT
			s.user_id,
			s.status,
			s.is_testing_user,
			COALESCE(l.id, 0)           AS level_id,
			COALESCE(l.level_group, 0)  AS level_group_id,
			COALESCE(l.course_id, 0)    AS course_id
		FROM student s
		LEFT JOIN level l ON l.id = s.level_id
		WHERE s.user_id = ?`, studentID,
	).Scan(&s.ID, &s.Status, &s.IsTestingUser, &s.LevelID, &s.LevelGroupID, &s.CourseID)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("student find by id: %w", err)
	}
	return &s, nil
}

// FindTestingUserIDs returns user_id list of is_testing_user=1 students, excluding given IDs.
func (r *StudentReader) FindTestingUserIDs(ctx context.Context, excludeIDs []int) ([]int, error) {
	query := `SELECT user_id FROM student WHERE is_testing_user = 1`
	args := []interface{}{}

	if len(excludeIDs) > 0 {
		placeholders := make([]string, len(excludeIDs))
		for i, id := range excludeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND user_id NOT IN (%s)", strings.Join(placeholders, ","))
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find testing users: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountFinishedBattlesToday counts FINISHED battles for student today (for demo limit check).
func (r *StudentReader) CountFinishedBattlesToday(ctx context.Context, studentID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM battle b
		INNER JOIN battle_member bm ON bm.battle_id = b.id
		WHERE bm.student_id = ?
		  AND b.status = ?
		  AND DATE(b.created_at) = CURDATE()`,
		studentID, models.BattleStatusFinished,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count finished battles today: %w", err)
	}
	return count, nil
}
