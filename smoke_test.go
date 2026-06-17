// Smoke tests: run against a live API + worker.
//
// Prerequisites:
//   - go run ./cmd/api/main.go  (or built binary)
//   - go run ./cmd/worker/main.go
//   - SMOKE_API_URL env set (default: http://localhost:8080)
//   - SMOKE_JWT_1 and SMOKE_JWT_2 env set (valid RS256 JWTs for two students
//     in the same level group — generate with /tmp/gen_jwt.php)
//
// Run: go test -v -tags smoke -timeout 60s ./...
//
//go:build smoke

package smoke_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var apiURL = func() string {
	if v := os.Getenv("SMOKE_API_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}()

// call makes an authenticated POST/GET to the battle API.
func call(t *testing.T, method, path, jwt string, body interface{}) map[string]interface{} {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, apiURL+path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var env map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	status := float64(resp.StatusCode)

	// Unwrap PHP-style envelope {ok,status_code,description,result|errors}.
	if inner, ok := env["result"].(map[string]interface{}); ok {
		inner["_status"] = status
		return inner
	}
	if inner, ok := env["errors"].(map[string]interface{}); ok {
		inner["_status"] = status
		return inner
	}
	env["_status"] = status
	return env
}

func model(r map[string]interface{}) map[string]interface{} {
	if m, ok := r["model"].(map[string]interface{}); ok {
		return m
	}
	return r
}

// TestSmoke_FullBattleFlow runs a complete P2P vocabulary battle between two students.
func TestSmoke_FullBattleFlow(t *testing.T) {
	jwt1 := os.Getenv("SMOKE_JWT_1")
	jwt2 := os.Getenv("SMOKE_JWT_2")
	if jwt1 == "" || jwt2 == "" {
		t.Skip("SMOKE_JWT_1 and SMOKE_JWT_2 not set — skipping smoke test")
	}

	// ── FIND ────────────────────────────────────────────────────────────────
	t.Log("Step 1: student 1 FIND")
	r1 := call(t, "POST", "/student/v1/battle/find", jwt1, map[string]interface{}{"type": 100, "lobby_type": 200})
	assert.Equal(t, float64(200), r1["_status"], "find s1: %v", r1)
	uuid := model(r1)["uuid"].(string)
	require.NotEmpty(t, uuid)
	t.Logf("  UUID=%s", uuid)

	t.Log("Step 2: student 2 FIND (join)")
	r2 := call(t, "POST", "/student/v1/battle/find", jwt2, map[string]interface{}{"type": 100, "lobby_type": 200})
	assert.Equal(t, float64(200), r2["_status"], "find s2: %v", r2)
	assert.Equal(t, uuid, model(r2)["uuid"], "should join same battle")
	assert.Equal(t, float64(400), model(r2)["status"], "should be ON_QUEUE")

	// ── CONFIRM ──────────────────────────────────────────────────────────────
	t.Log("Step 3: student 1 CONFIRM")
	c1 := call(t, "POST", "/student/v1/battle/confirm", jwt1, map[string]interface{}{"battle_id": uuid})
	assert.Equal(t, float64(200), c1["_status"], "confirm s1: %v", c1)

	t.Log("Step 4: student 2 CONFIRM (starts battle)")
	c2 := call(t, "POST", "/student/v1/battle/confirm", jwt2, map[string]interface{}{"battle_id": uuid})
	assert.Equal(t, float64(200), c2["_status"], "confirm s2: %v", c2)
	assert.Equal(t, float64(200), model(c2)["status"], "battle should be ON_GOING")

	// Questions are no longer in the battle/confirm payload — fetch via dedicated endpoint.
	t.Log("Step 4b: fetch questions via GET /:uuid/questions")
	qr := call(t, "GET", "/student/v1/battle/"+uuid+"/questions", jwt1, nil)
	assert.Equal(t, float64(200), qr["_status"], "questions endpoint: %v", qr)
	questions, ok := qr["questions"].([]interface{})
	require.True(t, ok && len(questions) > 0, "no questions from questions endpoint")
	t.Logf("  %d questions", len(questions))

	// ── ANSWER ALL QUESTIONS ─────────────────────────────────────────────────
	t.Log("Step 5: answer all questions")
	for i, qi := range questions {
		q := qi.(map[string]interface{})
		qid := int(q["id"].(float64))

		// Find correct answer (alternative type=200 = ANSWER)
		correct := "wrong"
		for _, opti := range q["options"].([]interface{}) {
			opt := opti.(map[string]interface{})
			for _, alti := range opt["alternatives"].([]interface{}) {
				alt := alti.(map[string]interface{})
				if alt["type"].(float64) == 200 {
					correct = fmt.Sprintf("%v", alt["value"])
					break
				}
			}
		}

		a1 := call(t, "POST", "/student/v1/battle/answer", jwt1, map[string]interface{}{
			"battle_id": uuid, "question_id": qid, "values": []string{correct}, "answer_time": 5000,
		})
		assert.Equal(t, float64(200), a1["_status"], "answer s1 q%d: %v", i+1, a1)
		assert.True(t, a1["status"].(bool), "answer s1 q%d should succeed", i+1)

		a2 := call(t, "POST", "/student/v1/battle/answer", jwt2, map[string]interface{}{
			"battle_id": uuid, "question_id": qid, "values": []string{correct}, "answer_time": 4000,
		})
		assert.Equal(t, float64(200), a2["_status"], "answer s2 q%d: %v", i+1, a2)
	}

	// ── WAIT FOR WORKER TO END BATTLE ────────────────────────────────────────
	t.Log("Step 6: waiting for battle to finish (worker)")
	deadline := time.Now().Add(15 * time.Second)
	finished := false
	for time.Now().Before(deadline) {
		v := call(t, "GET", "/student/v1/battle/"+uuid, jwt1, nil)
		if v["status"] != nil && v["status"].(float64) == 300 {
			finished = true
			t.Log("  Battle FINISHED")
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t, finished, "battle never reached FINISHED status within 15s")

	// ── VERIFY FINAL RESULTS ─────────────────────────────────────────────────
	t.Log("Step 7: verify final results")
	v := call(t, "GET", "/student/v1/battle/"+uuid, jwt1, nil)
	assert.Equal(t, float64(200), v["_status"])
	assert.Equal(t, float64(300), v["status"], "status should be FINISHED")

	members := v["members"].([]interface{})
	require.Len(t, members, 2)
	for _, mi := range members {
		m := mi.(map[string]interface{})
		assert.NotNil(t, m["place"], "member should have a place")
		// PHP-parity member shape: {place, points, student:{…}, answers:"<json string>"}
		ans, _ := m["answers"].(string)
		assert.NotEmpty(t, ans, "answers should be a populated JSON string after finishing")
		student, _ := m["student"].(map[string]interface{})
		t.Logf("  student=%v place=%v points=%v answers_len=%d",
			student["full_name"], m["place"], m["points"], len(ans))
	}
}

// TestSmoke_AuthRequired verifies unauthenticated requests are rejected.
func TestSmoke_AuthRequired(t *testing.T) {
	req, _ := http.NewRequest("POST", apiURL+"/student/v1/battle/find",
		bytes.NewBufferString(`{"type":100,"lobby_type":200}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestSmoke_ValidationErrors verifies field validation responses.
func TestSmoke_ValidationErrors(t *testing.T) {
	jwt := os.Getenv("SMOKE_JWT_1")
	if jwt == "" {
		t.Skip("SMOKE_JWT_1 not set")
	}
	r := call(t, "POST", "/student/v1/battle/find", jwt, map[string]interface{}{})
	assert.Equal(t, float64(422), r["_status"])
	assert.Contains(t, r["message"].(string), "required")
}
