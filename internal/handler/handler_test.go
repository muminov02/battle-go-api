package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"battle-go-api/internal/db/fake"
	"battle-go-api/internal/handler"
	"battle-go-api/internal/models"
	"battle-go-api/internal/service"
	fakesvc "battle-go-api/internal/service/fake"
)

// ── test harness ──────────────────────────────────────────────────────────────

type harness struct {
	router   http.Handler
	privKey  *rsa.PrivateKey
	battles  *fake.BattleRepository
	members  *fake.MemberRepository
	students *fake.StudentReader
	qr       *fake.QuestionReader
	results  *fake.ResultWriter
	rt       *fakesvc.RealtimeService
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	battles := fake.NewBattleRepository()
	members := fake.NewMemberRepository()
	students := fake.NewStudentReader()
	qr := fake.NewQuestionReader()
	results := fake.NewResultWriter()
	profiles := fake.NewProfileReader()
	rt := fakesvc.NewRealtimeService()

	cfg := service.DefaultConfig()

	findSvc := service.NewFindService(battles, members, students, qr, results, rt, cfg)
	confirmSvc := service.NewConfirmService(battles, members, qr, results, rt, cfg)
	answerSvc := service.NewAnswerService(battles, members, results, rt)
	leaveSvc := service.NewLeaveService(battles, members, results, rt)
	changeTypeSvc := service.NewChangeTypeService(battles, members, students, rt)
	endBattleSvc := service.NewEndBattleService(battles, members, results, rt)
	viewSvc := service.NewViewService(battles, members)

	h := handler.New(findSvc, confirmSvc, answerSvc, leaveSvc, changeTypeSvc, endBattleSvc, viewSvc, rt, profiles, rt)
	router := handler.NewRouter(h, &privKey.PublicKey, nil)

	return &harness{
		router:  router,
		privKey: privKey,
		battles: battles, members: members, students: students,
		qr: qr, results: results, rt: rt,
	}
}

func (h *harness) token(studentID int) string {
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"user_id": float64(studentID),
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "main",
		"aud":     "go-api",
	}).SignedString(h.privKey)
	return tok
}

func (h *harness) do(method, path string, body interface{}, studentID int) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.token(studentID))
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

// bodyJSON unwraps the PHP-style envelope so tests can assert the inner payload:
//   2xx → result, 422 → errors, else → the envelope (has description).
func bodyJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	if w.Code == http.StatusOK {
		if r, ok := env["result"].(map[string]interface{}); ok {
			return r
		}
	}
	if w.Code == http.StatusUnprocessableEntity {
		if e, ok := env["errors"].(map[string]interface{}); ok {
			return e
		}
	}
	return env
}

// ── auth middleware ────────────────────────────────────────────────────────────

func TestAuth_MissingToken(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest("POST", "/student/v1/battle/find", bytes.NewBufferString(`{"type":100,"lobby_type":200}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_InvalidToken(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest("POST", "/student/v1/battle/find", bytes.NewBufferString(`{"type":100,"lobby_type":200}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ── POST /find ────────────────────────────────────────────────────────────────

func TestFind_MissingFields(t *testing.T) {
	h := newHarness(t)
	h.students.AddStudent(&models.Student{ID: 1, LevelID: 1, LevelGroupID: 1, CourseID: 1})

	w := h.do("POST", "/student/v1/battle/find", map[string]interface{}{}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	body := bodyJSON(t, w)
	assert.Contains(t, body["message"], "required")
}

func TestFind_StudentNotFound(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/find", map[string]interface{}{"type": 100, "lobby_type": 200}, 999)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFind_StudentNoLevel(t *testing.T) {
	h := newHarness(t)
	h.students.AddStudent(&models.Student{ID: 1, LevelID: 0})

	w := h.do("POST", "/student/v1/battle/find", map[string]interface{}{"type": 100, "lobby_type": 200}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestFind_CreatesP2PBattle(t *testing.T) {
	h := newHarness(t)
	h.students.AddStudent(&models.Student{ID: 1, LevelID: 2, LevelGroupID: 1, CourseID: 1})

	w := h.do("POST", "/student/v1/battle/find", map[string]interface{}{"type": 100, "lobby_type": 200}, 1)
	require.Equal(t, http.StatusOK, w.Code)

	body := bodyJSON(t, w)
	model := body["model"].(map[string]interface{})
	assert.NotEmpty(t, model["uuid"])
	assert.Equal(t, float64(models.BattleStatusWaiting), model["status"])
	assert.Len(t, model["members"], 1)
	assert.Equal(t, "Please wait other members to join", body["message"])
}

func TestFind_TwoStudentsJoinSameBattle(t *testing.T) {
	h := newHarness(t)
	h.students.AddStudent(&models.Student{ID: 1, LevelID: 2, LevelGroupID: 1, CourseID: 1})
	h.students.AddStudent(&models.Student{ID: 2, LevelID: 2, LevelGroupID: 1, CourseID: 1})

	w1 := h.do("POST", "/student/v1/battle/find", map[string]interface{}{"type": 100, "lobby_type": 200}, 1)
	require.Equal(t, http.StatusOK, w1.Code)

	w2 := h.do("POST", "/student/v1/battle/find", map[string]interface{}{"type": 100, "lobby_type": 200}, 2)
	require.Equal(t, http.StatusOK, w2.Code)

	body2 := bodyJSON(t, w2)
	model2 := body2["model"].(map[string]interface{})
	// Same UUID
	body1 := bodyJSON(t, w1)
	assert.Equal(t, body1["model"].(map[string]interface{})["uuid"], model2["uuid"])
	// Both joined → ON_QUEUE
	assert.Equal(t, float64(models.BattleStatusOnQueue), model2["status"])
	assert.Len(t, model2["members"], 2)
}

// ── POST /confirm ─────────────────────────────────────────────────────────────

func TestConfirm_MissingBattleID(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/confirm", map[string]interface{}{}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Contains(t, bodyJSON(t, w)["message"], "required")
}

func TestConfirm_BattleNotFound(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/confirm", map[string]interface{}{"battle_id": "no-such-uuid"}, 1)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestConfirm_NotMember(t *testing.T) {
	h := newHarness(t)
	b := &models.Battle{UUID: "test-uuid", Status: models.BattleStatusOnQueue, Type: models.BattleTypeP2P}
	h.battles.Add(b)

	w := h.do("POST", "/student/v1/battle/confirm", map[string]interface{}{"battle_id": "test-uuid"}, 99)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ── POST /answer ──────────────────────────────────────────────────────────────

func TestAnswer_MissingFields(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/answer", map[string]interface{}{"battle_id": "x"}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestAnswer_BattleNotStarted(t *testing.T) {
	h := newHarness(t)
	b := &models.Battle{UUID: "test-uuid", Status: models.BattleStatusWaiting, Type: models.BattleTypeP2P}
	h.battles.Add(b)
	h.members.Add(&models.BattleMember{BattleID: b.ID, StudentID: 1, Status: models.MemberStatusConfirmed, CurrentQuestion: 1})

	w := h.do("POST", "/student/v1/battle/answer", map[string]interface{}{
		"battle_id": "test-uuid", "question_id": 1, "values": []string{"A"}, "answer_time": 3000,
	}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestAnswer_Success(t *testing.T) {
	h := newHarness(t)
	q := models.Question{ID: 1, Value: "Q", Options: []models.QuestionOption{
		{Alternatives: []models.Alternative{{ID: 1, Type: models.AlternativeTypeAnswer, Value: "A"}}},
	}}
	b := &models.Battle{UUID: "test-uuid", Status: models.BattleStatusOnGoing, Type: models.BattleTypeP2P,
		Questions: []models.Question{q}}
	h.battles.Add(b)
	h.members.Add(&models.BattleMember{BattleID: b.ID, StudentID: 1, Status: models.MemberStatusConfirmed, CurrentQuestion: 1})

	w := h.do("POST", "/student/v1/battle/answer", map[string]interface{}{
		"battle_id": "test-uuid", "question_id": 1, "values": []string{"A"}, "answer_time": 3000,
	}, 1)
	require.Equal(t, http.StatusOK, w.Code)
	body := bodyJSON(t, w)
	assert.True(t, body["status"].(bool))
	assert.Equal(t, "Success", body["message"])
}

// ── POST /leave ───────────────────────────────────────────────────────────────

func TestLeave_MissingBattleID(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/leave", map[string]interface{}{}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestLeave_BattleNotFound(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/leave", map[string]interface{}{"battle_id": "no-uuid"}, 1)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLeave_WaitingBattle(t *testing.T) {
	h := newHarness(t)
	b := &models.Battle{UUID: "test-uuid", Status: models.BattleStatusWaiting, Type: models.BattleTypeP2P}
	h.battles.Add(b)
	h.members.Add(&models.BattleMember{BattleID: b.ID, StudentID: 1, Status: models.MemberStatusNotConfirmed})

	w := h.do("POST", "/student/v1/battle/leave", map[string]interface{}{"battle_id": "test-uuid"}, 1)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, bodyJSON(t, w)["message"], "left")
}

// ── POST /change-type ─────────────────────────────────────────────────────────

func TestChangeType_MissingBattleID(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/student/v1/battle/change-type", map[string]interface{}{}, 1)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// ── GET /:uuid ────────────────────────────────────────────────────────────────

func TestView_NotFound(t *testing.T) {
	h := newHarness(t)
	w := h.do("GET", "/student/v1/battle/no-such-uuid", nil, 1)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestView_ReturnsModel(t *testing.T) {
	h := newHarness(t)
	b := &models.Battle{UUID: "view-uuid", Status: models.BattleStatusOnGoing, Type: models.BattleTypeP2P}
	h.battles.Add(b)
	h.members.Add(&models.BattleMember{BattleID: b.ID, StudentID: 1, Status: models.MemberStatusConfirmed})

	w := h.do("GET", "/student/v1/battle/view-uuid", nil, 1)
	require.Equal(t, http.StatusOK, w.Code)

	body := bodyJSON(t, w)
	assert.Equal(t, "view-uuid", body["uuid"])
	assert.Equal(t, float64(models.BattleStatusOnGoing), body["status"])
	assert.Len(t, body["members"], 1)
}
