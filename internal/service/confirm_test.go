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

func setupConfirm() (
	*fake.BattleRepository,
	*fake.MemberRepository,
	*fake.QuestionReader,
	*fake.ResultWriter,
	*fakesvc.RealtimeService,
	*svc.ConfirmService,
) {
	b := fake.NewBattleRepository()
	m := fake.NewMemberRepository()
	q := fake.NewQuestionReader()
	r := fake.NewResultWriter()
	rt := fakesvc.NewRealtimeService()
	cfg := svc.DefaultConfig()
	return b, m, q, r, rt, svc.NewConfirmService(b, m, q, r, rt, cfg)
}

func seedP2PBattle(t *testing.T, battles *fake.BattleRepository, members *fake.MemberRepository, levelID, levelGroupID int) (*models.Battle, *models.BattleMember, *models.BattleMember) {
	t.Helper()
	b := &models.Battle{UUID: "test-uuid", Type: models.BattleTypeP2P, LobbyType: models.LobbyTypeGrammar, Status: models.BattleStatusOnQueue, LevelID: levelID, LevelGroupID: levelGroupID}
	require.NoError(t, battles.Create(context.Background(), b))

	m1 := &models.BattleMember{BattleID: b.ID, StudentID: 1, Type: models.MemberTypeCreator, Status: models.MemberStatusNotConfirmed, CurrentQuestion: 1}
	m2 := &models.BattleMember{BattleID: b.ID, StudentID: 2, Type: models.MemberTypeParticipant, Status: models.MemberStatusNotConfirmed, CurrentQuestion: 1}
	require.NoError(t, members.Create(context.Background(), m1))
	require.NoError(t, members.Create(context.Background(), m2))
	return b, m1, m2
}

func TestConfirm_BattleNotFound(t *testing.T) {
	_, _, _, _, _, s := setupConfirm()
	_, err := s.Execute(context.Background(), 1, "no-such-uuid")
	require.ErrorIs(t, err, svc.ErrBattleNotFound)
}

func TestConfirm_NotMember(t *testing.T) {
	battles, members, _, _, _, s := setupConfirm()
	seedP2PBattle(t, battles, members, 5, 2)
	_, err := s.Execute(context.Background(), 99, "test-uuid") // student 99 not a member
	require.ErrorIs(t, err, svc.ErrNotMember)
}

func TestConfirm_WaitsForOthers(t *testing.T) {
	battles, members, _, _, _, s := setupConfirm()
	seedP2PBattle(t, battles, members, 5, 2)

	// Student 1 confirms — student 2 not yet
	res, err := s.Execute(context.Background(), 1, "test-uuid")
	require.NoError(t, err)
	assert.Equal(t, "Please wait other members to join", res.Message)
	assert.Equal(t, models.BattleStatusOnQueue, res.Battle.Status) // still waiting
}

func TestConfirm_AllConfirmed_StartsGrammarBattle(t *testing.T) {
	battles, members, qr, results, rt, s := setupConfirm()
	seedP2PBattle(t, battles, members, 5, 2)

	qs := []models.Question{makeGrammarQuestion(1), makeGrammarQuestion(2), makeGrammarQuestion(3)}
	qr.SetGrammarQuestions(5, qs)

	// Both students confirm
	_, err := s.Execute(context.Background(), 1, "test-uuid")
	require.NoError(t, err)

	res, err := s.Execute(context.Background(), 2, "test-uuid")
	require.NoError(t, err)

	assert.Empty(t, res.Message) // no waiting message when started
	assert.Equal(t, models.BattleStatusOnGoing, res.Battle.Status)
	assert.Len(t, res.Battle.Questions, 3)
	assert.NotNil(t, res.Battle.StartTime)
	assert.NotNil(t, res.Battle.EndTime)

	// No MySQL write during the game — only PostgreSQL until battle end
	require.Empty(t, results.SavedBattles())

	// Realtime: battle+members published on start
	assert.GreaterOrEqual(t, len(rt.BattlePublishes), 1)
}

func TestConfirm_AllConfirmed_UsesTemplate(t *testing.T) {
	battles, members, qr, _, _, s := setupConfirm()
	seedP2PBattle(t, battles, members, 5, 2)

	// Template takes priority over dynamic
	templateQs := []models.Question{makeGrammarQuestion(10), makeGrammarQuestion(11)}
	qr.SetTemplate(2, models.LobbyTypeGrammar, templateQs)
	qr.SetGrammarQuestions(5, []models.Question{makeGrammarQuestion(1)}) // should NOT be used

	_, err := s.Execute(context.Background(), 1, "test-uuid")
	require.NoError(t, err)
	res, err := s.Execute(context.Background(), 2, "test-uuid")
	require.NoError(t, err)

	assert.Equal(t, []models.Question{makeGrammarQuestion(10), makeGrammarQuestion(11)}, res.Battle.Questions)
}

// REGRESSION: a P2P battle converted to AI is started by Confirm (not
// find.startAIBattle), so Confirm must fill the AI opponent's answers — otherwise
// the bot never "plays" and shows no progress/results.
func TestConfirm_ConvertedAIBattle_FillsAIAnswers(t *testing.T) {
	battles, members, qr, _, _, s := setupConfirm()

	b := &models.Battle{UUID: "ai-uuid", Type: models.BattleTypeAI, LobbyType: models.LobbyTypeGrammar, Status: models.BattleStatusOnQueue, LevelID: 5, LevelGroupID: 2}
	require.NoError(t, battles.Create(context.Background(), b))
	human := &models.BattleMember{BattleID: b.ID, StudentID: 1, Type: models.MemberTypeCreator, Status: models.MemberStatusNotConfirmed, CurrentQuestion: 1}
	ai := &models.BattleMember{BattleID: b.ID, StudentID: 2, Type: models.MemberTypeAI, Status: models.MemberStatusConfirmed, CurrentQuestion: 1}
	require.NoError(t, members.Create(context.Background(), human))
	require.NoError(t, members.Create(context.Background(), ai))

	qr.SetGrammarQuestions(5, []models.Question{makeGrammarQuestion(1), makeGrammarQuestion(2), makeGrammarQuestion(3)})

	// Human confirms — AI already confirmed → battle starts.
	res, err := s.Execute(context.Background(), 1, "ai-uuid")
	require.NoError(t, err)
	assert.Equal(t, models.BattleStatusOnGoing, res.Battle.Status)

	// The AI member must now have answers (one per question) and be finished.
	aiAfter, err := members.FindByBattleAndStudent(context.Background(), b.ID, 2)
	require.NoError(t, err)
	assert.Len(t, aiAfter.Answers, 3, "AI opponent should have an answer per question")
	assert.True(t, aiAfter.IsFinished, "AI opponent should be marked finished")
}

func TestConfirm_MemberStatusUpdated(t *testing.T) {
	battles, members, _, _, rt, s := setupConfirm()
	seedP2PBattle(t, battles, members, 5, 2)

	_, err := s.Execute(context.Background(), 1, "test-uuid")
	require.NoError(t, err)

	// Member 1 should be CONFIRMED
	m, err := members.FindByBattleAndStudent(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.Equal(t, models.MemberStatusConfirmed, m.Status)

	// Realtime published for member
	assert.Len(t, rt.MemberPublishes, 1)
}
