# Battle Service — Architecture Decision Records (ADR)

For **mobile & frontend developers** integrating with the Battle Go service.
Each record: the decision, why, and **what it means for your client**.

- Service base (test): `https://v2-backend-erp.englifyschool.com/v2/battle`
- Auth API base (test): `https://v2-api-erp.englifyschool.com`
- Status: **Accepted / live on test server**

Contents:
- [ADR-0001 Go service; PostgreSQL live state, MySQL final results](#adr-0001)
- [ADR-0002 Auth — PHP-issued go-api JWT](#adr-0002)
- [ADR-0003 Response envelope](#adr-0003)
- [ADR-0004 Realtime = events only; questions over HTTP](#adr-0004)
- [ADR-0005 Pluggable realtime: Ably or WebSocket](#adr-0005)
- [ADR-0006 Member object = public profile (PHP parity)](#adr-0006)
- [ADR-0007 Scoring & placement](#adr-0007)
- [ADR-0008 Timers (client must run the per-question countdown)](#adr-0008)
- [ADR-0009 Question alternative types (200 = correct)](#adr-0009)
- [ADR-0010 Path routing on the existing domain](#adr-0010)

---

<a name="adr-0001"></a>
## ADR-0001 — Go service; PostgreSQL for live state, MySQL for final results
**Status:** Accepted

**Context.** The PHP/Firebase battle was rewritten in Go. Active gameplay needs fast,
isolated state; the existing app data (questions, students, results) lives in MySQL.

**Decision.** A standalone Go service owns a **PostgreSQL** DB for *active* battle state.
**MySQL** is read-only for questions/students/profiles and is written **only when a battle
finishes** (final battle + members + win/loss).

**Consequences / client impact.**
- During a game, the source of truth is the Go service — always read state from its endpoints, not MySQL.
- A battle that never finishes (abandoned) may leave no MySQL record — don't depend on MySQL for in-progress battles.

---

<a name="adr-0002"></a>
## ADR-0002 — Auth: PHP-issued `go-api` JWT
**Status:** Accepted

**Context.** Only the PHP app knows who's logged in and holds the signing key. The Go
service must trust those identities without re-implementing login.

**Decision.** PHP issues an RS256 JWT (audience `go-api`) for the logged-in student. Go
validates it with the matching public key. The battle endpoints require
`Authorization: Bearer <go-api JWT>`.

**Flow:**
```
1. POST {AUTH}/student/v1/auth/login
   headers: api-token: <app token>          body: {"identity":"<phone>","password":"<pwd>"}
   -> result.token  (user access token)
2. POST {AUTH}/student/v1/battle/get-jwt
   headers: api-token: <app token>, Authorization: Bearer <user access token>
   -> result.token  (the go-api JWT)
3. Use that go-api JWT as Bearer for all /v2/battle/* calls + realtime.
```

**Consequences / client impact.**
- Cache the go-api JWT; it expires (24h) — re-run get-jwt when it does (401 from battle endpoints).
- The `api-token` header is required by the PHP API gate on login/get-jwt.
- Battle endpoints only check the go-api JWT (Bearer); no `api-token` needed there.

---

<a name="adr-0003"></a>
## ADR-0003 — Response envelope
**Status:** Accepted (mirrors the old PHP API)

**Decision.** Every response is wrapped:
```json
// success
{ "ok": true,  "status_code": 200, "description": "Success",          "result": { … } }
// validation error
{ "ok": false, "status_code": 422, "description": "Validation error", "errors": { … } }
// other error
{ "ok": false, "status_code": 404, "description": "<message>" }
```

**Consequences / client impact.**
- Read payload from `result` on success, `errors` on 422, `description` on other errors.
- Check `ok` / HTTP status, not just the body.

---

<a name="adr-0004"></a>
## ADR-0004 — Realtime carries events only; questions fetched over HTTP
**Status:** Accepted

**Context.** Pushing full question/answer data over realtime is heavy and leaks answers.

**Decision.** Realtime events contain **no questions and no answer content** — only status
and progress. Questions are fetched via `GET /v2/battle/{uuid}/questions` once the battle is
ON_GOING (status 200).

**Consequences / client impact.**
- On the realtime `status:200` event → call `GET /{uuid}/questions`, then render.
- All members get the **same** frozen question set.
- Show opponents' progress from realtime member events (current_question / is_finished).

---

<a name="adr-0005"></a>
## ADR-0005 — Pluggable realtime: Ably or WebSocket
**Status:** Accepted

**Context.** The product may use Ably or a self-hosted WebSocket; clients shouldn't hard-code one.

**Decision.** The server runs one transport (env `REALTIME_DRIVER=ably|ws`) and tells the
client which via an additive `realtime` field on the battle model:
```json
"token": "<ably token | empty for ws>",
"realtime": { "driver": "ably", "url": null }
"realtime": { "driver": "ws",   "url": "wss://host/v2/battle/ws?battle=<uuid>" }
```
Channels (same for both): `{uuid}` (battle) and `{uuid}:{studentId}` (per member).
Event payloads:
```json
{ "type":"battle",        "status":200, "expire_time":…, "starting_time":…, "end_time":…, "question_time":15 }
{ "type":"battle_member", "student_id":26, "status":200, "current_question":4, "is_finished":false }
```

**Consequences / client impact.**
- Branch on `realtime.driver`:
  - `ably` → Ably SDK with `model.token` (scoped subscribe).
  - `ws` → open `realtime.url + "&token=" + <go-api JWT>`; messages are `{ "channel": "...", "data": {…} }`.
- Subscribe to the battle channel + each member channel for opponent progress.
- Build a transport abstraction so switching drivers is a config change, not a rewrite.

---

<a name="adr-0006"></a>
## ADR-0006 — Member object = public profile (PHP parity)
**Status:** Accepted

**Decision.** Members match the old PHP shape:
```json
{
  "place": 1, "points": 1,
  "answers": "<json string | null>",          // raw JSON string, null until answered
  "student": {
    "full_name": "Umid Muminov",
    "avatar": "https://…",                     // url, or gender default
    "level": { "id":2,"name":"Beginner 2","order":2,"level_group":1,"parent_id":null,"status":1,"course_id":1,"image_url":null },
    "point": 0,
    "themes": [ { "type": 1, "url": "https://…" } ]
  }
}
```

**Consequences / client impact.**
- `answers` is a **string** — `JSON.parse` it. Each entry: `{ question_id, question, values, time, is_correct, points, correct_answer, correct_answers }`.
- There is **no `student_id`** on the member; identify "self" by your own JWT's user_id and track your own progress locally / via your member realtime channel.
- Members are ordered by `place` ascending.

---

<a name="adr-0007"></a>
## ADR-0007 — Scoring & placement
**Status:** Accepted

**Decision.**
- Per answer: correct = 500 base (+ small speed bonus); wrong = 0.
- **Placement:** (1) most correct answers, (2) tie → least total time (faster wins), (3) tie → lower id.
- **Reward points** (win/loss record): >50% wrong → 0 regardless of place; <6 members → 1st=1; ≥6 → 1st=3,2nd=2,3rd=1.

**Consequences / client impact.**
- To explain results in UI: rank by correct count, then time. Show `correct`, `wrong`, total `time`, `place`, `points`.
- A player who answers mostly wrong gets 0 points even if placed 1st — surface this.

---

<a name="adr-0008"></a>
## ADR-0008 — Timers (the client must run the per-question countdown)
**Status:** Accepted

**Decision.**
| Timer | Value | Who enforces |
|---|---|---|
| Confirm window | 20s (ON_QUEUE) | server (kick daemon) |
| Per question | 15s | **client** — on timeout, submit as `(no answer)` and advance |
| Member idle | 30s (2× question) | server (auto-finishes a stuck member) |
| Whole battle | 15×Q + 15s | server (fills blanks, ends) |

**Consequences / client impact.**
- Implement a **15s countdown per question**. On expiry, POST `/answer` with the unanswered question and move on (otherwise the 30s server idle-timeout finishes the member).
- Send `answer_time` in **milliseconds** (time the student took) — it drives the tie-break.
- Confirm within 20s of ON_QUEUE or the lobby is killed.
- AI bot answers every question in 8s.

---

<a name="adr-0009"></a>
## ADR-0009 — Question alternative types (200 = correct)
**Status:** Accepted

**Decision.** In a question's `options[].alternatives[]`, **`type: 200` = the correct answer
(ANSWER)**, `type: 100` = distractor (OPTION). Mirrors the PHP `ExerciseQuestionPartTypeEnum`.

**Consequences / client impact.**
- Don't reveal the `type:200` alternative to the user.
- Submit the chosen alternative's **`value`** (text) in `answer` `values: ["..."]`; correctness is by value match (case-insensitive).

---

<a name="adr-0010"></a>
## ADR-0010 — Path routing on the existing domain
**Status:** Accepted

**Decision.** The Go service is reached at `/v2/battle/*` on the existing backend domain
(nginx rewrites to the service's internal `/student/v1/battle/*`). PHP serves everything else.

**Endpoints:**
```
POST /v2/battle/find | confirm | answer | leave | change-type
GET  /v2/battle/{uuid} | {uuid}/questions
WS   wss://…/v2/battle/ws?battle={uuid}&token={jwt}
```
Enums: type P2P=100/GROUP=200/AI=300 · lobby GRAMMAR=100/VOCABULARY=200 · status WAITING=100/ON_QUEUE=400/ON_GOING=200/FINISHED=300 · member NOT_CONFIRMED=100/CONFIRMED=200.

**Consequences / client impact.**
- Use the `/v2/battle/*` base for all battle calls.
- CORS is open on the Go service; the PHP auth API is same-origin with the demo page on `v2-api-erp` — for cross-origin from your app, login/get-jwt run on the auth domain and battle on the backend domain.

---

### Quick start (client)
```
1. login (auth API) + get-jwt  → go-api JWT          [ADR-0002]
2. POST /v2/battle/find {type, lobby_type}           → battle model (uuid, status, members, token, realtime)
3. connect realtime per `realtime.driver`            [ADR-0005]
4. POST /v2/battle/confirm {battle_id}                (within 20s)
5. on status 200 → GET /v2/battle/{uuid}/questions    [ADR-0004]
6. per question: 15s countdown; POST /v2/battle/answer {battle_id, question_id, values, answer_time}
7. on status 300 → GET /v2/battle/{uuid} → show results [ADR-0006/0007]
```
See `BATTLE_API.md` for full request/response examples.
