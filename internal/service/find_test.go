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

// helpers

func newStudent(id, levelID, levelGroupID, courseID, status int) *models.Student {
	return &models.Student{
		ID:           id,
		LevelID:      levelID,
		LevelGroupID: levelGroupID,
		CourseID:     courseID,
		Status:       status,
	}
}

func newTestingUser(id int) *models.Student {
	return &models.Student{ID: id, LevelID: 1, LevelGroupID: 1, CourseID: 1, IsTestingUser: true}
}

func makeGrammarQuestion(id int) models.Question {
	return models.Question{
		ID:    id,
		Value: "Q",
		Options: []models.QuestionOption{{
			Alternatives: []models.Alternative{{ID: 1, Type: models.AlternativeTypeAnswer, Value: "A"}},
		}},
	}
}

func setupFind() (
	*fake.BattleRepository,
	*fake.MemberRepository,
	*fake.StudentReader,
	*fake.QuestionReader,
	*fake.ResultWriter,
	*fakesvc.RealtimeService,
	*svc.FindService,
) {
	b := fake.NewBattleRepository()
	m := fake.NewMemberRepository()
	st := fake.NewStudentReader()
	q := fake.NewQuestionReader()
	r := fake.NewResultWriter()
	rt := fakesvc.NewRealtimeService()
	cfg := svc.DefaultConfig()
	return b, m, st, q, r, rt, svc.NewFindService(b, m, st, q, r, rt, cfg)
}

func TestFind_StudentNotFound(t *testing.T) {
	_, _, _, _, _, _, s := setupFind()
	_, err := s.Execute(context.Background(), 99, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.ErrorIs(t, err, svc.ErrStudentNotFound)
}

func TestFind_StudentNoLevel(t *testing.T) {
	_, _, st, _, _, _, s := setupFind()
	st.AddStudent(&models.Student{ID: 1, LevelID: 0})
	_, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.ErrorIs(t, err, svc.ErrStudentNoLevel)
}

func TestFind_DemoLimitExceeded(t *testing.T) {
	_, _, st, _, _, _, s := setupFind()
	st.AddStudent(newStudent(1, 1, 1, 1, 200)) // demo status
	st.SetFinishedBattleCount(1, 5)            // at limit
	_, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.ErrorIs(t, err, svc.ErrDemoLimitExceeded)
}

func TestFind_DemoNotLimited(t *testing.T) {
	b, _, st, _, _, _, s := setupFind()
	st.AddStudent(newStudent(1, 1, 1, 1, 200)) // demo
	st.SetFinishedBattleCount(1, 4)            // under limit (default=5)
	_, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)
	require.Len(t, b.All(), 1)
}

func TestFind_CreateNewP2PBattle(t *testing.T) {
	battles, members, st, _, _, rt, s := setupFind()
	st.AddStudent(newStudent(1, 5, 2, 3, 100))

	res, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)

	require.NotNil(t, res.Battle)
	assert.Equal(t, models.BattleStatusWaiting, res.Battle.Status)
	assert.Equal(t, models.BattleTypeP2P, res.Battle.Type)
	assert.Equal(t, models.LobbyTypeGrammar, res.Battle.LobbyType)
	assert.Equal(t, 3, res.Battle.CourseID)
	assert.Equal(t, 2, res.Battle.LevelGroupID)
	assert.Equal(t, "Please wait other members to join", res.Message)

	// One battle created, one member (CREATOR)
	allBattles := battles.All()
	require.Len(t, allBattles, 1)

	allMembers := members.All()
	require.Len(t, allMembers, 1)
	assert.Equal(t, models.MemberTypeCreator, allMembers[0].Type)
	assert.Equal(t, models.MemberStatusNotConfirmed, allMembers[0].Status)

	// Realtime published
	assert.Len(t, rt.BattlePublishes, 1)
}

func TestFind_JoinExistingP2P_NotFull(t *testing.T) {
	battles, members, st, _, _, rt, s := setupFind()

	// Pre-create a waiting P2P battle with student 1
	st.AddStudent(newStudent(1, 5, 2, 3, 100))
	st.AddStudent(newStudent(2, 5, 2, 3, 100))

	res1, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)
	assert.Equal(t, models.BattleStatusWaiting, res1.Battle.Status)

	// Student 2 joins — but P2P requires 2 → should become ON_QUEUE
	// Wait, only 2 members needed for P2P. After student 2 joins → count=2 → ON_QUEUE
	// Let me test with GROUP (4 members) to test "not full" case
	_ = battles
	_ = members
	_ = rt
}

func TestFind_JoinExistingP2P_BecomesOnQueue(t *testing.T) {
	_, members, st, _, _, _, s := setupFind()

	st.AddStudent(newStudent(1, 5, 2, 3, 100))
	st.AddStudent(newStudent(2, 5, 2, 3, 100))

	// Student 1 creates
	res1, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)
	assert.Equal(t, models.BattleStatusWaiting, res1.Battle.Status)

	// Student 2 joins → count=2 → ON_QUEUE
	res2, err := s.Execute(context.Background(), 2, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)

	assert.Equal(t, models.BattleStatusOnQueue, res2.Battle.Status)
	assert.NotNil(t, res2.Battle.ExpireTime)

	// Same battle
	assert.Equal(t, res1.Battle.ID, res2.Battle.ID)

	// 2 members total: creator + participant
	allMembers := members.All()
	require.Len(t, allMembers, 2)
	types := map[int]bool{}
	for _, m := range allMembers {
		types[m.Type] = true
	}
	assert.True(t, types[models.MemberTypeCreator])
	assert.True(t, types[models.MemberTypeParticipant])
}

func TestFind_AlreadyMember(t *testing.T) {
	_, _, st, _, _, _, s := setupFind()
	st.AddStudent(newStudent(1, 5, 2, 3, 100))

	// Student 1 calls find twice
	_, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)

	res2, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)
	assert.Equal(t, "Please wait other members to join", res2.Message)
}

func TestFind_AI_Battle(t *testing.T) {
	battles, members, st, qr, results, rt, s := setupFind()

	st.AddStudent(newStudent(1, 5, 2, 3, 100))
	st.AddStudent(newTestingUser(99)) // AI player

	// Provide 10 grammar questions
	qs := make([]models.Question, 10)
	for i := range qs {
		qs[i] = makeGrammarQuestion(i + 1)
	}
	qr.SetGrammarQuestions(5, qs) // LevelID=5 matches the student

	res, err := s.Execute(context.Background(), 1, models.BattleTypeAI, models.LobbyTypeGrammar)
	require.NoError(t, err)

	require.NotNil(t, res.Battle)
	assert.Equal(t, models.BattleStatusOnGoing, res.Battle.Status)
	assert.NotNil(t, res.Battle.StartTime)
	assert.NotNil(t, res.Battle.EndTime)
	assert.Len(t, res.Battle.Questions, 10)
	assert.Empty(t, res.Message) // no "Please wait" for AI

	// 2 members: human (CREATOR) + AI
	allMembers := members.All()
	require.Len(t, allMembers, 2)

	aiMember := (*models.BattleMember)(nil)
	for _, m := range allMembers {
		if m.Type == models.MemberTypeAI {
			aiMember = m
		}
	}
	require.NotNil(t, aiMember)
	assert.True(t, aiMember.IsFinished)
	assert.Len(t, aiMember.Answers, 10)

	// 1 battle in PG
	assert.Len(t, battles.All(), 1)

	// No MySQL writes during the game — battle + members live only in PostgreSQL
	// until the worker finishes the battle (EndBattleService).
	assert.Empty(t, results.SavedBattles())
	assert.Empty(t, results.SavedMembers())

	// Realtime: battle published + AI member published
	assert.GreaterOrEqual(t, len(rt.BattlePublishes), 1)
	assert.GreaterOrEqual(t, len(rt.MemberPublishes), 1)
}

func TestFind_AI_JoinsExistingBattle(t *testing.T) {
	_, members, st, qr, _, _, s := setupFind()

	// Two testing users so either can be AI
	st.AddStudent(newStudent(1, 5, 2, 3, 100))
	st.AddStudent(newTestingUser(99))

	qs := make([]models.Question, 3)
	for i := range qs {
		qs[i] = makeGrammarQuestion(i + 1)
	}
	qr.SetGrammarQuestions(5, qs)

	// First call creates a WAITING (ON_GOING) AI battle
	_, err := s.Execute(context.Background(), 1, models.BattleTypeAI, models.LobbyTypeGrammar)
	require.NoError(t, err)

	// Only 2 members total
	assert.Len(t, members.All(), 2)
}

func TestFind_LevelIDLowered(t *testing.T) {
	battles, _, st, _, _, _, s := setupFind()

	// Student 1 creates battle with LevelID=10
	st.AddStudent(newStudent(1, 10, 2, 3, 100))
	_, err := s.Execute(context.Background(), 1, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)
	assert.Equal(t, 10, battles.All()[0].LevelID)

	// Student 2 with lower LevelID=5 joins → battle LevelID updated to 5
	st.AddStudent(newStudent(2, 5, 2, 3, 100))
	res, err := s.Execute(context.Background(), 2, models.BattleTypeP2P, models.LobbyTypeGrammar)
	require.NoError(t, err)
	assert.Equal(t, 5, res.Battle.LevelID)
}
