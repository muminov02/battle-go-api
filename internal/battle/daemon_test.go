package battle_test

import (
	"testing"
	"time"

	"battle-go-api/internal/battle"
	"battle-go-api/internal/models"

	"github.com/stretchr/testify/assert"
)

// ── KickMember daemon logic ────────────────────────────────────────────────────

// KickExpiredMembers is the pure logic extracted from the daemon:
// given a battle and its members, returns which member IDs to delete
// and whether the battle itself should be deleted.
//
// Separate from DB I/O for testability.

func TestKickExpiredMembers_P2P_KicksNotConfirmed(t *testing.T) {
	b := &models.Battle{ID: 1, Type: models.BattleTypeP2P, Status: models.BattleStatusOnQueue}
	confirmed := &models.BattleMember{ID: 10, StudentID: 1, Status: models.MemberStatusConfirmed}
	notConfirmed := &models.BattleMember{ID: 20, StudentID: 2, Status: models.MemberStatusNotConfirmed}

	toKick, deleteBattle := battle.KickExpiredMembers(b, []*models.BattleMember{confirmed, notConfirmed})

	assert.Contains(t, toKick, 20, "NOT_CONFIRMED member should be kicked")
	assert.NotContains(t, toKick, 10, "CONFIRMED member should stay")
	assert.False(t, deleteBattle)
}

func TestKickExpiredMembers_AI_KicksAllMembers(t *testing.T) {
	b := &models.Battle{ID: 1, Type: models.BattleTypeAI, Status: models.BattleStatusOnQueue}
	m1 := &models.BattleMember{ID: 10, StudentID: 1, Status: models.MemberStatusConfirmed}
	m2 := &models.BattleMember{ID: 20, StudentID: 2, Status: models.MemberStatusConfirmed, Type: models.MemberTypeAI}

	toKick, deleteBattle := battle.KickExpiredMembers(b, []*models.BattleMember{m1, m2})

	assert.Len(t, toKick, 2, "AI battle kicks ALL members on expire")
	assert.True(t, deleteBattle, "no members remain → delete battle")
}

func TestKickExpiredMembers_NoMembersAfterKick_DeleteBattle(t *testing.T) {
	b := &models.Battle{ID: 1, Type: models.BattleTypeP2P}
	m := &models.BattleMember{ID: 10, StudentID: 1, Status: models.MemberStatusNotConfirmed}

	toKick, deleteBattle := battle.KickExpiredMembers(b, []*models.BattleMember{m})

	assert.Contains(t, toKick, 10)
	assert.True(t, deleteBattle, "all members kicked → delete battle")
}

func TestKickExpiredMembers_MembersRemain_DoNotDeleteBattle(t *testing.T) {
	b := &models.Battle{ID: 1, Type: models.BattleTypeP2P}
	confirmed := &models.BattleMember{ID: 10, Status: models.MemberStatusConfirmed}
	notConfirmed := &models.BattleMember{ID: 20, Status: models.MemberStatusNotConfirmed}

	_, deleteBattle := battle.KickExpiredMembers(b, []*models.BattleMember{confirmed, notConfirmed})

	assert.False(t, deleteBattle)
}

// ── EndBattle daemon logic ─────────────────────────────────────────────────────

// ShouldDeleteAIBattle returns true when an AI battle's human player
// is still on question 1 (never actually played).
func TestShouldDeleteAIBattle_MemberOnQuestion1_True(t *testing.T) {
	b := &models.Battle{Type: models.BattleTypeAI}
	human := &models.BattleMember{Type: models.MemberTypeParticipant, CurrentQuestion: 1}
	ai := &models.BattleMember{Type: models.MemberTypeAI, CurrentQuestion: 10}

	assert.True(t, battle.ShouldDeleteAIBattle(b, []*models.BattleMember{human, ai}))
}

func TestShouldDeleteAIBattle_MemberBeyondQuestion1_False(t *testing.T) {
	b := &models.Battle{Type: models.BattleTypeAI}
	human := &models.BattleMember{Type: models.MemberTypeParticipant, CurrentQuestion: 3}

	assert.False(t, battle.ShouldDeleteAIBattle(b, []*models.BattleMember{human}))
}

func TestShouldDeleteAIBattle_NonAIBattle_AlwaysFalse(t *testing.T) {
	b := &models.Battle{Type: models.BattleTypeP2P}
	m := &models.BattleMember{CurrentQuestion: 1}

	assert.False(t, battle.ShouldDeleteAIBattle(b, []*models.BattleMember{m}))
}

// AllMembersFinished returns true when every member has is_finished=true.
func TestAllMembersFinished_AllTrue(t *testing.T) {
	members := []*models.BattleMember{
		{IsFinished: true},
		{IsFinished: true},
	}
	assert.True(t, battle.AllMembersFinished(members))
}

func TestAllMembersFinished_OneNotFinished(t *testing.T) {
	members := []*models.BattleMember{
		{IsFinished: true},
		{IsFinished: false},
	}
	assert.False(t, battle.AllMembersFinished(members))
}

func TestAllMembersFinished_Empty_False(t *testing.T) {
	assert.False(t, battle.AllMembersFinished([]*models.BattleMember{}))
}

// ── Battle timing ──────────────────────────────────────────────────────────────

// CalcBattleTimes returns start_time and end_time for a battle.
// start_time = now + 10s
// end_time   = now + 15*questionCount + 15s
func TestCalcBattleTimes_CorrectOffsets(t *testing.T) {
	questionCount := 10
	before := time.Now()

	startTime, endTime := battle.CalcBattleTimes(questionCount)

	after := time.Now()

	// start_time: now+10s (±1s tolerance)
	expectedStart := before.Add(10 * time.Second)
	assert.WithinDuration(t, expectedStart, startTime, time.Second)

	// end_time: now + 15*10 + 15 = now+165s
	expectedDuration := time.Duration(15*questionCount+15) * time.Second
	expectedEnd := before.Add(expectedDuration)
	assert.WithinDuration(t, expectedEnd, endTime, time.Second)
	_ = after
}

// ── IsMemberIdle ────────────────────────────────────────────────────────────────

func TestIsMemberIdle_LastQuestionAtBeyondTimeout_True(t *testing.T) {
	now := time.Now()
	old := now.Add(-battle.MemberIdleTimeout - time.Second)
	m := &models.BattleMember{LastQuestionAt: &old}
	assert.True(t, battle.IsMemberIdle(m, nil, now))
}

func TestIsMemberIdle_RecentLastQuestionAt_False(t *testing.T) {
	now := time.Now()
	recent := now.Add(-5 * time.Second)
	m := &models.BattleMember{LastQuestionAt: &recent}
	assert.False(t, battle.IsMemberIdle(m, nil, now))
}

func TestIsMemberIdle_FinishedMember_False(t *testing.T) {
	now := time.Now()
	old := now.Add(-battle.MemberIdleTimeout - time.Minute)
	m := &models.BattleMember{LastQuestionAt: &old, IsFinished: true}
	assert.False(t, battle.IsMemberIdle(m, nil, now))
}

func TestIsMemberIdle_NilLastQuestion_UsesBattleStart(t *testing.T) {
	now := time.Now()
	start := now.Add(-battle.MemberIdleTimeout - time.Second)
	m := &models.BattleMember{} // never answered → fall back to battle start
	assert.True(t, battle.IsMemberIdle(m, &start, now))

	recentStart := now.Add(-3 * time.Second)
	assert.False(t, battle.IsMemberIdle(m, &recentStart, now))
}

func TestIsMemberIdle_NoBaseline_False(t *testing.T) {
	// battle not started, no last_question_at → no clock → not idle
	assert.False(t, battle.IsMemberIdle(&models.BattleMember{}, nil, time.Now()))
}
