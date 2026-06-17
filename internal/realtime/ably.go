package realtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ably/ably-go/ably"

	"battle-go-api/internal/models"
)

// AblyService implements service.RealtimeService using Ably REST.
// Channel naming mirrors PHP Firebase: {uuid} and {uuid}-{student_id}.
type AblyService struct {
	rest *ably.REST
}

func NewAblyService(apiKey string) (*AblyService, error) {
	rest, err := ably.NewREST(ably.WithKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("ably: new rest: %w", err)
	}
	return &AblyService{rest: rest}, nil
}

// CreateBattleToken returns an Ably token string that grants the client subscribe
// access to the battle channel and ALL member channels of that battle.
// Capability uses Ably ":"-segment wildcards: "{uuid}:*" matches "{uuid}:26", "{uuid}:28", …
// so a client can watch every opponent's progress, not just its own.
func (s *AblyService) CreateBattleToken(ctx context.Context, battleUUID string, studentID int) (string, error) {
	cap := fmt.Sprintf(`{"%s":["subscribe"],"%s:*":["subscribe"]}`, battleUUID, battleUUID)
	tok, err := s.rest.Auth.RequestToken(ctx, &ably.TokenParams{
		Capability: cap,
	})
	if err != nil {
		return "", fmt.Errorf("ably: create token: %w", err)
	}
	return tok.Token, nil
}

// Driver / ConnectURL satisfy the handler's RealtimeInfo. Ably clients connect via
// the Ably SDK using the `token`, so there is no server-side connect URL.
func (s *AblyService) Driver() string                { return "ably" }
func (s *AblyService) ConnectURL(_ string) string    { return "" }

func (s *AblyService) PublishBattle(ctx context.Context, battle *models.Battle) error {
	return s.publish(ctx, battle.UUID, battlePayload(battle))
}

func (s *AblyService) PublishBattleWithMembers(ctx context.Context, battle *models.Battle, members []*models.BattleMember) error {
	if err := s.publish(ctx, battle.UUID, battlePayload(battle)); err != nil {
		return err
	}
	for _, m := range members {
		if err := s.publish(ctx, memberChannel(battle.UUID, m.StudentID), memberPayload(battle.UUID, m)); err != nil {
			return err
		}
	}
	return nil
}

func (s *AblyService) PublishMember(ctx context.Context, battleUUID string, member *models.BattleMember) error {
	return s.publish(ctx, memberChannel(battleUUID, member.StudentID), memberPayload(battleUUID, member))
}

func (s *AblyService) DeleteBattle(ctx context.Context, battleUUID string) error {
	return s.publish(ctx, battleUUID, map[string]any{"deleted": true, "battle_id": battleUUID})
}

func (s *AblyService) DeleteMember(ctx context.Context, battleUUID string, studentID int) error {
	return s.publish(ctx, memberChannel(battleUUID, studentID), map[string]any{"deleted": true})
}

// --- helpers ---

func (s *AblyService) publish(ctx context.Context, channel string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ably publish marshal: %w", err)
	}
	return s.rest.Channels.Get(channel).Publish(ctx, "update", string(data))
}

// memberChannel uses ":" so token capability "{uuid}:*" can wildcard-match all members.
func memberChannel(battleUUID string, studentID int) string {
	return fmt.Sprintf("%s:%d", battleUUID, studentID)
}

// battlePayload is an event notification only — NO questions.
// Clients fetch questions via GET /student/v1/battle/:uuid/questions when ON_GOING.
// Conveys: battle created / joined / confirmed / ON_GOING / FINISHED via status.
func battlePayload(b *models.Battle) map[string]any {
	p := map[string]any{
		"type":          "battle",
		"battle_id":     b.UUID,
		"status":        b.Status,
		"winners":       []any{},
		"question_time": models.QuestionTimeSeconds,
	}
	if b.ExpireTime != nil {
		p["expire_time"] = b.ExpireTime.Format("2006-01-02 15:04:05")
	} else {
		p["expire_time"] = nil
	}
	if b.StartTime != nil {
		p["starting_time"] = b.StartTime.Format("2006-01-02 15:04:05")
	} else {
		p["starting_time"] = nil
	}
	if b.EndTime != nil {
		p["end_time"] = b.EndTime.Format("2006-01-02 15:04:05")
	} else {
		p["end_time"] = nil
	}
	return p
}

// memberPayload is an event/progress notification only — NO answer content.
// Conveys: student confirmed / current question / finished.
func memberPayload(battleUUID string, m *models.BattleMember) map[string]any {
	return map[string]any{
		"type":             "battle_member",
		"student_id":       m.StudentID,
		"is_finished":      m.IsFinished,
		"status":           m.Status,
		"current_question": m.CurrentQuestion,
		"battle_id":        battleUUID,
	}
}
