package fake

import (
	"context"
	"sync"

	"battle-go-api/internal/models"
)

// BattlePublish records a PublishBattle or PublishBattleWithMembers call.
type BattlePublish struct {
	Battle  *models.Battle
	Members []*models.BattleMember
}

// MemberPublish records a PublishMember call.
type MemberPublish struct {
	BattleUUID string
	Member     *models.BattleMember
}

// DeletedMember records a DeleteMember call.
type DeletedMember struct {
	BattleUUID string
	StudentID  int
}

// RealtimeService is a test double for service.RealtimeService.
type RealtimeService struct {
	mu             sync.Mutex
	BattlePublishes []BattlePublish
	MemberPublishes []MemberPublish
	DeletedBattles  []string
	DeletedMembers  []DeletedMember
}

func NewRealtimeService() *RealtimeService {
	return &RealtimeService{}
}

func (f *RealtimeService) PublishBattle(_ context.Context, battle *models.Battle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BattlePublishes = append(f.BattlePublishes, BattlePublish{Battle: battle})
	return nil
}

func (f *RealtimeService) PublishBattleWithMembers(_ context.Context, battle *models.Battle, members []*models.BattleMember) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BattlePublishes = append(f.BattlePublishes, BattlePublish{Battle: battle, Members: members})
	return nil
}

func (f *RealtimeService) PublishMember(_ context.Context, battleUUID string, member *models.BattleMember) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.MemberPublishes = append(f.MemberPublishes, MemberPublish{BattleUUID: battleUUID, Member: member})
	return nil
}

func (f *RealtimeService) DeleteBattle(_ context.Context, battleUUID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DeletedBattles = append(f.DeletedBattles, battleUUID)
	return nil
}

func (f *RealtimeService) DeleteMember(_ context.Context, battleUUID string, studentID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DeletedMembers = append(f.DeletedMembers, DeletedMember{battleUUID, studentID})
	return nil
}

// CreateBattleToken satisfies handler.TokenProvider interface.
func (f *RealtimeService) CreateBattleToken(_ context.Context, _ string, _ int) (string, error) {
	return "fake-ably-token", nil
}

// Driver / ConnectURL satisfy handler.RealtimeInfo.
func (f *RealtimeService) Driver() string             { return "ably" }
func (f *RealtimeService) ConnectURL(_ string) string { return "" }
