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

func setupLeave() (
	*fake.BattleRepository,
	*fake.MemberRepository,
	*fake.ResultWriter,
	*fakesvc.RealtimeService,
	*svc.LeaveService,
) {
	b := fake.NewBattleRepository()
	m := fake.NewMemberRepository()
	r := fake.NewResultWriter()
	rt := fakesvc.NewRealtimeService()
	return b, m, r, rt, svc.NewLeaveService(b, m, r, rt)
}

func seedBattle(t *testing.T, battles *fake.BattleRepository, members *fake.MemberRepository, battleType, status, memberStatus int, studentIDs ...int) *models.Battle {
	t.Helper()
	b := &models.Battle{UUID: "leave-uuid", Type: battleType, Status: status, Questions: []models.Question{makeGrammarQuestion(1)}}
	require.NoError(t, battles.Create(context.Background(), b))
	for i, id := range studentIDs {
		mt := models.MemberTypeCreator
		if i > 0 {
			mt = models.MemberTypeParticipant
		}
		m := &models.BattleMember{BattleID: b.ID, StudentID: id, Status: memberStatus, Type: mt, CurrentQuestion: 1}
		require.NoError(t, members.Create(context.Background(), m))
	}
	return b
}

func TestLeave_BattleNotFound(t *testing.T) {
	_, _, _, _, s := setupLeave()
	err := s.Execute(context.Background(), 1, "no-uuid")
	require.ErrorIs(t, err, svc.ErrBattleNotFound)
}

func TestLeave_BattleFinished(t *testing.T) {
	battles, members, _, _, s := setupLeave()
	seedBattle(t, battles, members, models.BattleTypeP2P, models.BattleStatusFinished, models.MemberStatusConfirmed, 1)
	err := s.Execute(context.Background(), 1, "leave-uuid")
	require.ErrorIs(t, err, svc.ErrBattleFinished)
}

func TestLeave_AI_DeletesBattleAndMembers(t *testing.T) {
	battles, members, _, rt, s := setupLeave()
	seedBattle(t, battles, members, models.BattleTypeAI, models.BattleStatusOnGoing, models.MemberStatusConfirmed, 1, 99)

	err := s.Execute(context.Background(), 1, "leave-uuid")
	require.NoError(t, err)

	// Battle deleted
	assert.Empty(t, battles.All())
	// Members deleted
	assert.Empty(t, members.All())
	// Realtime delete called
	require.Len(t, rt.DeletedBattles, 1)
	assert.Equal(t, "leave-uuid", rt.DeletedBattles[0])
}

func TestLeave_P2P_Waiting_DeletesMember(t *testing.T) {
	battles, members, _, rt, s := setupLeave()
	seedBattle(t, battles, members, models.BattleTypeP2P, models.BattleStatusWaiting, models.MemberStatusNotConfirmed, 1, 2)

	err := s.Execute(context.Background(), 1, "leave-uuid")
	require.NoError(t, err)

	// Only student 2 remains
	remaining := members.All()
	require.Len(t, remaining, 1)
	assert.Equal(t, 2, remaining[0].StudentID)

	// Battle not deleted
	assert.Len(t, battles.All(), 1)

	// Realtime delete for member
	require.Len(t, rt.DeletedMembers, 1)
	assert.Equal(t, 1, rt.DeletedMembers[0].StudentID)
}

func TestLeave_P2P_OnGoing_ConfirmedMember_FillsBlanks(t *testing.T) {
	battles, members, results, rt, s := setupLeave()
	seedBattle(t, battles, members, models.BattleTypeP2P, models.BattleStatusOnGoing, models.MemberStatusConfirmed, 1)

	err := s.Execute(context.Background(), 1, "leave-uuid")
	require.NoError(t, err)

	// Member still exists but with filled answers
	m, err := members.FindByBattleAndStudent(context.Background(), 1, 1)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.True(t, m.IsFinished)
	assert.Len(t, m.Answers, 1) // 1 blank answer for 1 question

	// No MySQL write on leave — member state persists in PostgreSQL,
	// written to MySQL at battle end (EndBattleService).
	assert.Empty(t, results.SavedMembers())

	// Realtime: member published (not deleted)
	assert.Len(t, rt.MemberPublishes, 1)
	assert.Empty(t, rt.DeletedMembers)
}

func TestLeave_P2P_OnGoing_NotConfirmedMember_DeletesMember(t *testing.T) {
	battles, members, _, rt, s := setupLeave()
	seedBattle(t, battles, members, models.BattleTypeP2P, models.BattleStatusOnGoing, models.MemberStatusNotConfirmed, 1)

	err := s.Execute(context.Background(), 1, "leave-uuid")
	require.NoError(t, err)

	assert.Empty(t, members.All())
	assert.Len(t, rt.DeletedMembers, 1)
}
