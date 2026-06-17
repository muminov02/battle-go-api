package service

import (
	"context"

	"battle-go-api/internal/models"
)

// RealtimeService publishes battle events to realtime channels (Ably).
// Channel names mirror PHP Firebase: {battle_uuid} and {battle_uuid}-{student_id}.
type RealtimeService interface {
	// PublishBattle publishes battle state to the {battle_uuid} channel.
	PublishBattle(ctx context.Context, battle *models.Battle) error

	// PublishBattleWithMembers publishes battle state and all member states.
	PublishBattleWithMembers(ctx context.Context, battle *models.Battle, members []*models.BattleMember) error

	// PublishMember publishes a single member state to {battle_uuid}-{student_id}.
	PublishMember(ctx context.Context, battleUUID string, member *models.BattleMember) error

	// DeleteBattle removes the {battle_uuid} channel.
	DeleteBattle(ctx context.Context, battleUUID string) error

	// DeleteMember removes the {battle_uuid}-{student_id} channel.
	DeleteMember(ctx context.Context, battleUUID string, studentID int) error
}
