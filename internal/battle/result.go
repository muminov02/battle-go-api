package battle

import (
	"sort"

	"battle-go-api/internal/models"
)

// ResultCalculator computes final placements and points for a finished battle.
type ResultCalculator struct {
	BattleType int
}

// AddBonusPoints adds speed-bonus points to the fastest correct answerer per question.
// Keyed by question text (mirrors PHP logic which uses answer.question as map key).
// P2P:   1st +50
// GROUP: 1st +120, 2nd +70, 3rd +50
func (r *ResultCalculator) AddBonusPoints(members []*models.BattleMember) {
	type entry struct {
		member      *models.BattleMember
		answerIndex int
		time        int
	}

	questionsMap := make(map[string][]entry)

	for _, m := range members {
		for i, a := range m.Answers {
			if a.IsCorrect {
				questionsMap[a.Question] = append(questionsMap[a.Question], entry{
					member:      m,
					answerIndex: i,
					time:        a.Time,
				})
			}
		}
	}

	for _, entries := range questionsMap {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].time < entries[j].time
		})

		if r.BattleType == models.BattleTypeP2P {
			if len(entries) > 0 {
				entries[0].member.Answers[entries[0].answerIndex].Points += 50
			}
		} else {
			bonuses := []int{120, 70, 50}
			for i, bonus := range bonuses {
				if i < len(entries) {
					entries[i].member.Answers[entries[i].answerIndex].Points += bonus
				}
			}
		}
	}
}

// SumPointsAndSetPlacement ranks members and sets Place + Points (reward points).
//
// Placement order: most CORRECT answers first; ties broken by least total TIME
// (sum of answer times, ms — faster wins). Further ties → by lower member id (stable).
//
// Reward points (unchanged rules):
//   - >50% wrong answers → 0 reward points regardless of place
//   - <6 members:  1st=1, rest=0
//   - >=6 members: 1st=3, 2nd=2, 3rd=1, rest=0
func (r *ResultCalculator) SumPointsAndSetPlacement(members []*models.BattleMember) {
	type slot struct {
		member       *models.BattleMember
		correct      int
		totalTime    int
		tooManyWrong bool
	}

	slots := make([]slot, len(members))
	for i, m := range members {
		correct, wrongCount, totalTime := 0, 0, 0
		for _, a := range m.Answers {
			totalTime += a.Time
			if a.IsCorrect {
				correct++
			} else {
				wrongCount++
			}
		}
		tooManyWrong := len(m.Answers) > 0 && wrongCount*2 >= len(m.Answers)
		slots[i] = slot{member: m, correct: correct, totalTime: totalTime, tooManyWrong: tooManyWrong}
	}

	// Most correct → least time → lower member id (stable, deterministic).
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].correct != slots[j].correct {
			return slots[i].correct > slots[j].correct
		}
		if slots[i].totalTime != slots[j].totalTime {
			return slots[i].totalTime < slots[j].totalTime
		}
		return slots[i].member.ID < slots[j].member.ID
	})

	for placement, s := range slots {
		place := placement + 1
		s.member.Place = &place

		pts := 0
		if !s.tooManyWrong {
			if len(members) >= 6 {
				switch place {
				case 1:
					pts = 3
				case 2:
					pts = 2
				case 3:
					pts = 1
				}
			} else if place == 1 {
				pts = 1
			}
		}
		s.member.Points = &pts
	}
}
