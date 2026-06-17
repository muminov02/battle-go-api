package service

import (
	"context"
	"fmt"

	"battle-go-api/internal/db"
	"battle-go-api/internal/models"
)

// ViewService handles GET /student/v1/battle/:uuid.
type ViewService struct {
	battles db.BattleRepository
	members db.MemberRepository
}

func NewViewService(battles db.BattleRepository, members db.MemberRepository) *ViewService {
	return &ViewService{battles, members}
}

// ViewResult holds the battle and its members.
type ViewResult struct {
	Battle  *models.Battle
	Members []*models.BattleMember
}

// Execute loads a battle and all its members by UUID.
func (s *ViewService) Execute(ctx context.Context, uuid string) (*ViewResult, error) {
	b, err := s.battles.FindByUUID(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("view: find battle: %w", err)
	}
	if b == nil {
		return nil, ErrBattleNotFound
	}

	mems, err := s.members.FindByBattleID(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("view: load members: %w", err)
	}

	return &ViewResult{Battle: b, Members: mems}, nil
}
