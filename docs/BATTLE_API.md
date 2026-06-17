# Battle API — Frontend Integration Guide

Realtime competitive quiz. Two (P2P) or four (GROUP) students answer the same
questions; fastest-correct wins. Go backend, PostgreSQL for live state, Ably for
realtime events.

---

## 1. Base + Auth

| | |
|---|---|
| Base URL | `http://<host>:8080` (dev: `http://localhost:8080`) |
| Auth | `Authorization: Bearer <JWT>` on **every** request |
| JWT | RS256, issued by the main app. Must include `user_id` (student id), `aud: "go-api"`. Valid until `exp`. |

All responses are JSON. Errors: `{ "message": "<reason>" }` with an HTTP status (see each endpoint).

---

## 2. Enums

| Concept | Values |
|---|---|
| **Battle type** | P2P=`100` (2 players) · GROUP=`200` (4) · AI=`300` (1 human + bot) |
| **Lobby type** | GRAMMAR=`100` · VOCABULARY=`200` |
| **Battle status** | WAITING=`100` → ON_QUEUE=`400` → ON_GOING=`200` → FINISHED=`300` |
| **Member status** | NOT_CONFIRMED=`100` · CONFIRMED=`200` |

### Status flow
```
find ─▶ WAITING(100) ──(lobby full)──▶ ON_QUEUE(400) ──(all confirm)──▶ ON_GOING(200) ──(all finish / time up)──▶ FINISHED(300)
```
- **ON_QUEUE → 20s** confirm window. If not all confirm, lobby is reset/deleted.
- **ON_GOING → `15 × questionCount + 15`s** play window (10 Q = 165s). Unanswered → blank-filled.
- **Per-member idle → 30s** (`2 × question_time`). If a student's `current_question` doesn't advance for 30s, that student is auto-finished — remaining questions blank-filled (`(no answer)`). Other members keep playing; the battle ends once everyone is finished. The frontend sees this via the member's Ably event (`is_finished: true`).
- AI battles skip queue/confirm and start immediately.

---

## 3. Endpoints

### POST `/student/v1/battle/find`
Find an open lobby or create one.
```json
// request
{ "type": 100, "lobby_type": 200 }
```
```json
// 200 OK
{
  "message": "Please wait other members to join",
  "model": {
    "uuid": "a6aee3be-…",
    "status": 100,
    "members": [
      { "student_id": 28, "status": 100, "current_question": 1, "is_finished": false, "place": null, "points": null, "answers": null }
    ],
    "token": "<ABLY_TOKEN>"
  }
}
```
- Save `model.uuid` — needed for every later call.
- Use `model.token` to connect to Ably (see §4).
- Errors: `404` student not found · `422` no level / demo limit exceeded.

### POST `/student/v1/battle/confirm`
Confirm attendance. When the **last** member confirms, the battle starts (status → 200).
```json
{ "battle_id": "a6aee3be-…" }
```
Returns same `{ model }` shape. **Must be called within 20s** of ON_QUEUE.
Errors: `404` battle not found (expired/deleted) · `403` not a member.

### GET `/student/v1/battle/:uuid/questions`  ← **fetch when status = 200**
Questions are **not** sent over Ably. Once you see ON_GOING, fetch them here.
Members only.
```json
// 200 OK
{
  "uuid": "a6aee3be-…",
  "status": 200,
  "questions": [
    {
      "id": 164,
      "value": "key",                       // prompt shown to the student
      "label": "Berilgan so'zning … toping", // instruction
      "order": 1,
      "no_value": true,
      "config": [],
      "options": [
        { "order": 1, "alternatives": [
          { "id": 164, "type": 100, "value": "kalit" },  // type 100 = CORRECT answer
          { "id": 175, "type": 200, "value": "soyabon" },
          { "id": 165, "type": 200, "value": "noutbuk" },
          { "id": 161, "type": 200, "value": "kitob" }
        ]}
      ]
    }
    // … N questions
  ]
}
```
- All members get the **same** question set (frozen at start).
- Correct alternative has `type: 100`. Distractors `type: 200`.
- Errors: `403` not a member · `404` battle not found.

### POST `/student/v1/battle/answer`
Submit one answer. Idempotent — re-sending the same `question_id` is ignored.
```json
{
  "battle_id": "a6aee3be-…",
  "question_id": 164,
  "values": ["kalit"],    // the chosen alternative's "value" (text), case-insensitive
  "answer_time": 4200     // milliseconds the student took
}
```
```json
// 200 OK
{ "status": true, "message": "Success" }
// unknown question_id:
{ "status": false, "message": "Question does not exist" }
```
- Server advances `current_question`. The student is `is_finished` after the last one.
- Errors: `422` battle not started / finished · `403` not a member · `404` battle not found.
- Correct = 500 base points; wrong = 0.

### POST `/student/v1/battle/leave`
```json
{ "battle_id": "a6aee3be-…" }
```
- WAITING/ON_QUEUE → just removed from lobby.
- ON_GOING (confirmed) → remaining questions blank-filled, marked finished.
- AI battle → battle deleted.
- `200 { "message": "You have left battle" }`.

### POST `/student/v1/battle/change-type`
Convert a waiting P2P lobby into an AI battle (adds a bot, starts immediately).
```json
{ "battle_id": "a6aee3be-…" }
```
Returns `{ message, model }`.

### GET `/student/v1/battle/:uuid`
Full battle snapshot (status, members, results, token). **No questions** here — use the questions endpoint.
```json
{
  "uuid": "a6aee3be-…",
  "status": 300,
  "members": [
    { "student_id": 26, "status": 200, "current_question": 11, "is_finished": true, "place": 1, "points": 1, "answers": [ … ] },
    { "student_id": 28, "status": 200, "current_question": 11, "is_finished": true, "place": 2, "points": 0, "answers": [ … ] }
  ],
  "token": "<ABLY_TOKEN>"
}
```

#### Answer object (inside `members[].answers`)
```json
{
  "question_id": 164, "question": "key", "values": ["kalit"],
  "time": 4200, "is_correct": true, "points": 500,
  "correct_answer": "kalit", "correct_answers": ["kalit"]
}
```

---

## 4. Realtime (Ably **or** WebSocket — server picks)

Realtime carries **events only** — never questions or answer content. Use it to drive
screen transitions and show opponents' live progress; fetch the actual data over HTTP.

The server runs ONE transport, chosen by `REALTIME_DRIVER` (env). Every battle response
tells the client which, in an additive `realtime` field on the battle model:
```jsonc
"token": "<ably token | empty for ws>",
"realtime": { "driver": "ably", "url": null }                       // Ably
"realtime": { "driver": "ws",   "url": "ws://host/student/v1/battle/ws?battle=<uuid>" } // WebSocket
```
Client logic:
```js
if (model.realtime.driver === "ws") {
  const ws = new WebSocket(model.realtime.url + "&token=" + jwt);   // your existing JWT
  ws.onmessage = e => { const {channel, data} = JSON.parse(e.data); /* … */ };
} else {
  const ably = new Ably.Realtime({ token: model.token });           // scoped subscribe token
  // subscribe to channels below
}
```

### WebSocket driver
- Connect: `GET ws(s)://host/student/v1/battle/ws?battle=<uuid>&token=<jwt>` (same JWT as REST).
- One connection per battle; you receive **all** events for it (battle + every member).
- Each message: `{"channel":"<uuid>|<uuid>:<sid>", "data":{…}}` — same `data` payloads as Ably below.

### Ably driver
Use the `token` from any battle response (scoped, subscribe-only):
```js
const ably = new Ably.Realtime({ token: model.token });
```
The token grants **subscribe** on this battle's channels (battle + all members).

### Channels
| Channel | Purpose |
|---|---|
| `{uuid}` | battle-level events (status changes) |
| `{uuid}:{studentId}` | one per member — progress/events. Note the **colon** `:`. |

Subscribe to `{uuid}` and `{uuid}:{id}` for every member (ids come from `members[]`).
The token capability is `{uuid}:*`, so you can watch every opponent.

### Event: battle channel `{uuid}`
```json
{
  "type": "battle",
  "battle_id": "a6aee3be-…",
  "status": 200,            // watch this: 100→400→200→300
  "winners": [],
  "question_time": 15,
  "expire_time": "2026-05-31 14:22:47",  // ON_QUEUE deadline
  "starting_time": "2026-05-31 14:23:01",
  "end_time": "2026-05-31 14:25:46"      // ON_GOING deadline
}
```
React to `status`:
- `400` → show confirm button (countdown to `expire_time`).
- `200` → **fetch `GET /:uuid/questions`** and start playing (countdown to `end_time`).
- `300` → fetch `GET /:uuid` and show results.
- `{ "deleted": true }` → lobby gone (expired / everyone left).

### Event: member channel `{uuid}:{studentId}`
```json
{
  "type": "battle_member",
  "battle_id": "a6aee3be-…",
  "student_id": 28,
  "status": 200,            // 200 = confirmed
  "current_question": 4,    // opponent progress (1-based)
  "is_finished": false
}
// or { "deleted": true } when a member leaves
```
Use `current_question` / `is_finished` to render opponent progress bars.

---

## 5. Typical client sequence (one player)

```
1. POST /find                     → save uuid + token; connect Ably; subscribe {uuid} and {uuid}:{eachMember}
2. (Ably battle status 400)       → show "Confirm" (20s)
3. POST /confirm
4. (Ably battle status 200)       → GET /:uuid/questions ; render Q[current_question-1]
5. for each question:
     POST /answer {question_id, values:[value], answer_time}
     (server advances current_question; opponent progress arrives via Ably member events)
6. (Ably battle status 300)       → GET /:uuid ; show places + points
```

---

## 6. Result rules (so the UI can explain scores)

1. Bonus for fastest **correct** answer per question — P2P: 1st +50. GROUP: 1st +120, 2nd +70, 3rd +50.
2. Total points → placement (1st, 2nd, …).
3. **A student with >50% wrong answers gets 0 reward points, regardless of placement.**
4. Reward points: battles with <6 members → 1st = 1 pt, rest = 0. ≥6 members → 1st=3, 2nd=2, 3rd=1.

---

## 7. Notes / current limits (dev build)

- CORS is open (`*`) — fine for dev; lock down for prod.
- JWT issuance endpoint on the main (PHP) app must be wired so the frontend can obtain a `go-api` token.
- A reference web client lives at `battle-test.html` (open in 2 tabs) — shows the full flow + live Ably log.
