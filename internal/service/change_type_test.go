package service_test

import (
	"context"
	"testing"

	"battle-go-api/internal/db/fake"
	"battle-go-api/internal/models"
	svc "battle-go-api/internal/service"
	fakesvc "battle-go-api/internal/service/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChangeType() (
	*fake.BattleRepository,
	*fake.MemberRepository,
	*fake.StudentReader,
	*fakesvc.RealtimeService,
	*svc.ChangeTypeService,
) {
	b := fake.NewBattleRepository()
	m := fake.NewMemberRepository()
	st := fake.NewStudentReader()
	rt := fakesvc.NewRealtimeService()
	return b, m, st, rt, svc.NewChangeTypeService(b, m, st, rt)
}

func seedWaitingBattle(t *testing.T, battles *fake.BattleRepository, members *fake.MemberRepository, studentID int) *models.Battle {
	t.Helper()
	b := &models.Battle{UUID: "ct-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusWaiting}
	require.NoError(t, battles.Create(context.Background(), b))
	m := &models.BattleMember{BattleID: b.ID, StudentID: studentID, Type: models.MemberTypeCreator, Status: models.MemberStatusNotConfirmed}
	require.NoError(t, members.Create(context.Background(), m))
	return b
}

func TestChangeType_BattleNotFound(t *testing.T) {
	_, _, _, _, s := setupChangeType()
	_, err := s.Execute(context.Background(), 1, "no-uuid")
	require.ErrorIs(t, err, svc.ErrBattleNotFound)
}

func TestChangeType_BattleNotWaiting(t *testing.T) {
	battles, members, _, _, s := setupChangeType()
	b := &models.Battle{UUID: "ct-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusOnQueue}
	require.NoError(t, battles.Create(context.Background(), b))
	m := &models.BattleMember{BattleID: b.ID, StudentID: 1}
	require.NoError(t, members.Create(context.Background(), m))

	_, err := s.Execute(context.Background(), 1, "ct-uuid")
	require.ErrorIs(t, err, svc.ErrBattleNotFound)
}

func TestChangeType_Success(t *testing.T) {
	battles, members, st, rt, s := setupChangeType()
	seedWaitingBattle(t, battles, members, 1)
	st.AddStudent(newTestingUser(99))

	res, err := s.Execute(context.Background(), 1, "ct-uuid")
	require.NoError(t, err)

	// Battle changed to AI, ON_QUEUE
	assert.Equal(t, models.BattleTypeAI, res.Battle.Type)
	assert.Equal(t, models.BattleStatusOnQueue, res.Battle.Status)
	assert.NotNil(t, res.Battle.ExpireTime)
	assert.Equal(t, "Please wait other members to join", res.Message)

	// 2 members: original human + new AI
	allMembers := members.All()
	require.Len(t, allMembers, 2)
	aiExists := false
	for _, m := range allMembers {
		if m.Type == models.MemberTypeAI {
			aiExists = true
			assert.Equal(t, models.MemberStatusConfirmed, m.Status)
		}
	}
	assert.True(t, aiExists)

	// Realtime: battle published + AI member published
	assert.Len(t, rt.BattlePublishes, 1)
	require.Len(t, rt.MemberPublishes, 1)
	assert.Equal(t, 99, rt.MemberPublishes[0].Member.StudentID)
}

func TestChangeType_AlreadyFull_NoAIAdded(t *testing.T) {
	battles, members, st, rt, s := setupChangeType()
	// Seed battle with 2 members already (AI type count = 2)
	b := &models.Battle{UUID: "ct-uuid", Type: models.BattleTypeP2P, Status: models.BattleStatusWaiting}
	require.NoError(t, battles.Create(context.Background(), b))
	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, Type: models.MemberTypeCreator}
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, Type: models.MemberTypeParticipant}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))
	st.AddStudent(newTestingUser(99))

	res, err := s.Execute(context.Background(), 1, "ct-uuid")
	require.NoError(t, err)

	// Battle unchanged (still P2P, WAITING)
	assert.Equal(t, models.BattleTypeP2P, res.Battle.Type)
	assert.Equal(t, models.BattleStatusWaiting, res.Battle.Status)

	// No AI member added
	assert.Len(t, members.All(), 2)

	// No realtime published
	assert.Empty(t, rt.BattlePublishes)
	assert.Empty(t, rt.MemberPublishes)
}
