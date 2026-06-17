package fake

import (
	"context"

	"battle-go-api/internal/models"
)

// ProfileReader is an in-memory ProfileReader for tests.
type ProfileReader struct {
	profiles map[int]*models.PublicProfile
}

func NewProfileReader() *ProfileReader {
	return &ProfileReader{profiles: make(map[int]*models.PublicProfile)}
}

// Set seeds a profile for a student (test helper).
func (r *ProfileReader) Set(studentID int, p *models.PublicProfile) {
	r.profiles[studentID] = p
}

func (r *ProfileReader) GetPublicProfile(_ context.Context, studentID int) (*models.PublicProfile, error) {
	if p, ok := r.profiles[studentID]; ok {
		return p, nil
	}
	// default minimal profile so handler tests don't need to seed every student
	return &models.PublicProfile{FullName: "-", Themes: []models.ThemeInfo{}}, nil
}
