package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"battle-go-api/internal/models"
)

// ProfileReader builds the public student profile shown inside battle members.
// Mirrors PHP PublicProfileResource: full_name, avatar, level, point, themes.
type ProfileReader struct {
	db *sql.DB
}

func NewProfileReader(database *sql.DB) *ProfileReader {
	return &ProfileReader{db: database}
}

const (
	genderMale         = 1
	avatarDefaultMale  = "https://d8tj7d7pfsmw2.cloudfront.net/static/male.png"
	avatarDefaultOther = "https://d8tj7d7pfsmw2.cloudfront.net/static/female.png"
)

// GetPublicProfile returns the profile for one student. Never errors on missing
// profile/level — those become "-"/null, matching PHP.
func (r *ProfileReader) GetPublicProfile(ctx context.Context, studentID int) (*models.PublicProfile, error) {
	var (
		point                      int
		firstname, lastname        sql.NullString
		gender, avatarFileID       sql.NullInt64
		avatarBase, avatarPath     sql.NullString
		lID, lOrder, lGroup, lStat sql.NullInt64
		lCourse, lParent           sql.NullInt64
		lName                      sql.NullString
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT
			s.point,
			up.firstname, up.lastname, up.gender, up.avatar_file_id,
			af.base_url, af.path,
			l.id, l.name, l.`+"`order`"+`, l.level_group, l.parent_id, l.status, l.course_id
		FROM student s
		LEFT JOIN user_profile up      ON up.user_id = s.user_id
		LEFT JOIN file_storage_item af ON af.id = up.avatar_file_id
		LEFT JOIN level l              ON l.id = s.level_id
		WHERE s.user_id = ?`, studentID,
	).Scan(
		&point,
		&firstname, &lastname, &gender, &avatarFileID,
		&avatarBase, &avatarPath,
		&lID, &lName, &lOrder, &lGroup, &lParent, &lStat, &lCourse,
	)
	if err == sql.ErrNoRows {
		return &models.PublicProfile{FullName: "-", Themes: []models.ThemeInfo{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: query: %w", err)
	}

	p := &models.PublicProfile{Point: point, Themes: []models.ThemeInfo{}}

	hasProfile := firstname.Valid || lastname.Valid
	if hasProfile {
		p.FullName = firstname.String + " " + lastname.String
		av := avatarValue(avatarFileID, avatarBase, avatarPath, gender)
		p.Avatar = &av
	} else {
		p.FullName = "-"
		p.Avatar = nil
	}

	if lID.Valid {
		lvl := &models.LevelInfo{
			ID:         int(lID.Int64),
			Name:       lName.String,
			Order:      int(lOrder.Int64),
			LevelGroup: int(lGroup.Int64),
			Status:     int(lStat.Int64),
			CourseID:   int(lCourse.Int64),
			ImageURL:   nil, // level.content.config.active_images — empty in this env
		}
		if lParent.Valid {
			pid := int(lParent.Int64)
			lvl.ParentID = &pid
		}
		p.Level = lvl
	}

	themes, err := r.selectedThemes(ctx, studentID)
	if err != nil {
		return nil, err
	}
	p.Themes = themes

	return p, nil
}

// avatarValue mirrors UserProfile::getAvatar — uploaded file URL if set, else gender default.
func avatarValue(fileID sql.NullInt64, base, path sql.NullString, gender sql.NullInt64) string {
	if fileID.Valid && fileID.Int64 != 0 && base.Valid && path.Valid {
		return base.String + "/" + path.String
	}
	if gender.Valid && gender.Int64 == genderMale {
		return avatarDefaultMale
	}
	return avatarDefaultOther
}

func (r *ProfileReader) selectedThemes(ctx context.Context, studentID int) ([]models.ThemeInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT st.type, fi.base_url, fi.path
		FROM student_theme st
		INNER JOIN theme t            ON t.id = st.theme_id
		LEFT JOIN file_storage_item fi ON fi.id = t.file_id
		WHERE st.student_id = ? AND st.is_selected = 1`, studentID)
	if err != nil {
		return nil, fmt.Errorf("profile: themes: %w", err)
	}
	defer rows.Close()

	themes := []models.ThemeInfo{}
	for rows.Next() {
		var typ int
		var base, path sql.NullString
		if err := rows.Scan(&typ, &base, &path); err != nil {
			return nil, err
		}
		url := ""
		if base.Valid && path.Valid {
			url = base.String + "/" + path.String
		}
		themes = append(themes, models.ThemeInfo{Type: typ, URL: url})
	}
	return themes, rows.Err()
}
