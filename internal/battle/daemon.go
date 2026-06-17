package battle

import (
	"time"

	"battle-go-api/internal/models"
)

// KickExpiredMembers returns the member IDs to delete and whether the battle
// itself should be deleted (no members remain after kick).
//
// AI battles: ALL members are kicked (PHP kicks everyone regardless of status).
// P2P/GROUP:  only NOT_CONFIRMED members are kicked.
func KickExpiredMembers(b *models.Battle, members []*models.BattleMember) (toKick []int, deleteBattle bool) {
	for _, m := range members {
		if b.Type == models.BattleTypeAI {
			toKick = append(toKick, m.ID)
		} else if m.Status == models.MemberStatusNotConfirmed {
			toKick = append(toKick, m.ID)
		}
	}

	// Count remaining members after kick
	kicked := make(map[int]bool, len(toKick))
	for _, id := range toKick {
		kicked[id] = true
	}
	remaining := 0
	for _, m := range members {
		if !kicked[m.ID] {
			remaining++
		}
	}

	return toKick, remaining == 0
}

// ShouldDeleteAIBattle returns true when the battle is AI type and any
// non-AI member is still on question 1 (student never actually played).
func ShouldDeleteAIBattle(b *models.Battle, members []*models.BattleMember) bool {
	if b.Type != models.BattleTypeAI {
		return false
	}
	for _, m := range members {
		if m.Type != models.MemberTypeAI && m.CurrentQuestion == 1 {
			return true
		}
	}
	return false
}

// AllMembersFinished returns true when every member has IsFinished=true.
// Returns false for empty slice.
func AllMembersFinished(members []*models.BattleMember) bool {
	if len(members) == 0 {
		return false
	}
	for _, m := range members {
		if !m.IsFinished {
			return false
		}
	}
	return true
}

// CalcBattleTimes returns start_time (now+10s) and end_time (now + qt*count + qt seconds).
func CalcBattleTimes(questionCount int) (startTime, endTime time.Time) {
	qt := models.QuestionTimeSeconds
	now := time.Now()
	startTime = now.Add(10 * time.Second)
	endTime = now.Add(time.Duration(qt*questionCount+qt) * time.Second)
	return
}

// MemberIdleTimeout is how long a member may stay on the same question before
// being auto-finished with blank answers. = 2 × question time.
const MemberIdleTimeout = 2 * models.QuestionTimeSeconds * time.Second

// IsMemberIdle reports whether an unfinished member has been stuck on the same
// question for at least MemberIdleTimeout. Baseline is the member's
// LastQuestionAt, falling back to the battle start time for question 1.
func IsMemberIdle(m *models.BattleMember, battleStart *time.Time, now time.Time) bool {
	if m.IsFinished {
		return false
	}
	base := m.LastQuestionAt
	if base == nil {
		base = battleStart
	}
	if base == nil {
		return false // battle not started yet — no clock
	}
	return now.Sub(*base) >= MemberIdleTimeout
}
