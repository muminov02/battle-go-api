# Battle API — Frontend Integration Guide

Realtime competitive quiz. Two (P2P) or four (GROUP) students answer the same
questions; most-correct (tie-break: least total time) wins. You can also battle a
scripted **AI** bot. Go backend, PostgreSQL for live state, **WebSocket** for realtime
events (the server can also run Ably — it tells you which; see §5).

> This file is self-contained: an agent should be able to build the whole client from
> it alone. Pair with `openapi.yaml` (importable into Postman) for exact schemas.

---

## 1. Hosts & paths

| | Test server | Local dev |
|---|---|---|
| **Auth API** (PHP) | `https://v2-api-erp.englifyschool.com` | — |
| **Battle service** (Go) | `https://v2-backend-erp.englifyschool.com` | `http://localhost:8080` |
| Battle REST prefix | `/v2/battle/...` | `/student/v1/battle/...` |
| WebSocket | `wss://v2-backend-erp.englifyschool.com/v2/battle/ws` | `ws://localhost:8080/student/v1/battle/ws` |

The Go service registers routes under `/student/v1/battle/*`; on the test server nginx
also proxies them as `/v2/battle/*`. **You don't need to hardcode the WS URL** — every
battle response hands you `realtime.url` (see §5). Just append `&token=<jwt>`.

---

## 2. Auth (do this once, before any battle call)

Two steps, both on the **Auth API** host, both need the `api-token` header
(value `local` on the test server):

```
1. POST /student/v1/auth/login         → user access token
2. POST /student/v1/battle/get-jwt     → go-api JWT  (Bearer = the token from step 1)
```

```jsonc
// 1. POST {AUTH}/student/v1/auth/login   headers: { api-token: local }
{ "identity": "998922222222", "password": "30.08.2007" }
// → result.token  (user access token)

// 2. POST {AUTH}/student/v1/battle/get-jwt   headers: { api-token: local, Authorization: Bearer <user token> }
// → result.token  (the go-api JWT — RS256, contains user_id + aud:"go-api")
```

Use that **go-api JWT** as `Authorization: Bearer <jwt>` on **every** Battle-service call
*and* on the WebSocket (`?token=<jwt>`). It is valid until its `exp`.

---

## 3. Response envelope

Every response (success or error) is wrapped:

```jsonc
// success
{ "ok": true,  "status_code": 200, "description": "Success", "result": { ...payload... } }
// validation error (422)
{ "ok": false, "status_code": 422, "description": "type is required", "errors": { ... } }
// other error
{ "ok": false, "status_code": 404, "description": "Battle not found" }
```

**Everything below describes the contents of `result`.** Read `ok`/`status_code` to branch;
read `description` for the human message. (The WebSocket frames are *not* enveloped — §5.)

---

## 4. Enums & lifecycle

| Concept | Values |
|---|---|
| **Battle type** | P2P=`100` (2 players) · GROUP=`200` (4) · AI=`300` (1 human + bot) |
| **Lobby type** | GRAMMAR=`100` · VOCABULARY=`200` |
| **Battle status** | WAITING=`100` → ON_QUEUE=`400` → ON_GOING=`200` → FINISHED=`300` |
| **Member status** | NOT_CONFIRMED=`100` · CONFIRMED=`200` |
| **Alternative type** | **ANSWER=`200` (correct)** · OPTION=`100` (distractor) |

> ⚠️ Alternative `type`: **`200` is the correct answer, `100` is a wrong option.** (Don't
> rely on order — find the option whose `type == 200`.)

```
find ─▶ WAITING(100) ──(lobby full)──▶ ON_QUEUE(400) ──(all confirm)──▶ ON_GOING(200) ──(all finish / time up)──▶ FINISHED(300)
```
- **ON_QUEUE → 20s** confirm window. If not all confirm, lobby resets/deletes.
- **ON_GOING → `15 × questionCount + 15`s** play window (10 Q = 165s). Unanswered → blank-filled.
- **Per-question client timer → 15s** (`question_time` in the battle event). On timeout, submit as wrong and move on.
- **Per-member idle → 30s** (`2 × question_time`): if a student's `current_question` doesn't advance for 30s the server auto-finishes that student (remaining questions blank). Others keep playing; battle ends when everyone is finished. You see it as that member's WS event with `is_finished: true`.
- AI battles skip queue/confirm and start immediately; the bot answers each question in ~8s.

---

## 5. Realtime (WebSocket — events only)

Realtime carries **events only**, never questions or answer content. Use it to drive
screen transitions and show opponents' live progress; fetch the actual data over HTTP.

Every battle response carries a `realtime` field telling you the active transport:

```jsonc
"realtime": { "driver": "ws", "url": "ws://host/student/v1/battle/ws?battle=<uuid>" }  // WebSocket (current)
"realtime": { "driver": "ably", "url": null }   // if server is switched back to Ably; use model.token with the Ably SDK
```

Branch on `driver` so the client survives a server-side switch:

```js
let socket;
if (model.realtime.driver === "ws") {
  socket = new WebSocket(model.realtime.url + "&token=" + jwt);   // same go-api JWT
  socket.onmessage = e => {
    const { channel, data } = JSON.parse(e.data);
    handleEvent(channel, data);   // channel = "<uuid>" or "<uuid>:<studentId>"
  };
} else {
  const ably = new Ably.Realtime({ token: model.token });        // scoped subscribe token
  // subscribe to "<uuid>" and each "<uuid>:<studentId>"; messages carry the same `data`
}
```

### WebSocket protocol
- **Connect:** `GET ws(s)://host/.../ws?battle=<uuid>&token=<jwt>` — same JWT as REST. One
  connection per battle; you get **all** events for it (battle + every member). No subscribe
  message needed.
- **Each frame:** `{"channel": "<uuid>" | "<uuid>:<studentId>", "data": { ... }}`.
- **Send nothing** — it's receive-only (any client→server message is ignored).
- Reconnect on close; on reconnect, call `GET /:uuid` to re-sync state you may have missed.

### Event — battle channel `{uuid}`  (`data`)
```json
{
  "type": "battle",
  "battle_id": "a6aee3be-…",
  "status": 200,
  "winners": [],
  "question_time": 15,
  "expire_time": "2026-05-31 14:22:47",
  "starting_time": "2026-05-31 14:23:01",
  "end_time": "2026-05-31 14:25:46"
}
```
React to `status`:
- `400` → show **Confirm** button, count down to `expire_time`.
- `200` → **`GET /:uuid/questions`** and start playing, count down to `end_time`.
- `300` → **`GET /:uuid`** and show results.
- `{ "deleted": true, "battle_id": "…" }` → lobby gone (expired / everyone left).

### Event — member channel `{uuid}:{studentId}`  (`data`)
```json
{
  "type": "battle_member",
  "battle_id": "a6aee3be-…",
  "student_id": 28,
  "status": 200,
  "current_question": 4,
  "is_finished": false
}
// or { "deleted": true } when a member leaves
```
Use `current_question` / `is_finished` to render opponents' progress bars. The studentIds
come from `members[]`… but note the public profile in REST responses (§6) does **not**
expose `student_id` — match opponents by the order/identity you saw at join time, or treat
the member-channel events as the source of truth for per-opponent progress.

---

## 6. Endpoints

All paths below are shown with the local prefix `/student/v1/battle`; on the server use
`/v2/battle`. All are `Authorization: Bearer <go-api jwt>`. Shapes are the contents of
`result` (§3).

### POST `/find` — find or create a lobby
```json
// request
{ "type": 100, "lobby_type": 200 }
```
```jsonc
// result
{
  "message": "Please wait other members to join",   // optional
  "model": {
    "uuid": "a6aee3be-…",
    "status": 100,
    "token": "",                                     // Ably token; "" on ws driver
    "realtime": { "driver": "ws", "url": "ws://…/ws?battle=a6aee3be-…" },
    "members": [
      { "place": null, "points": null, "answers": null, "student": { …profile… } }
    ]
  }
}
```
- Save `model.uuid` — needed for every later call. Connect WS using `model.realtime.url`.
- Errors: `404` student not found · `422` no level / demo limit / missing field.

### POST `/confirm` — confirm attendance
```json
{ "battle_id": "a6aee3be-…" }
```
Returns the same `{ message?, model }` shape. **Call within 20s** of ON_QUEUE. When the
last member confirms, status → `200` (a battle WS event fires).
Errors: `404` battle not found (expired) · `403` not a member.

### GET `/:uuid/questions` — **fetch when status = 200**
Questions are **not** sent over realtime. Members only. All members get the same set.
```jsonc
// result
{
  "uuid": "a6aee3be-…",
  "status": 200,
  "questions": [
    {
      "id": 164,
      "value": "key",                          // prompt shown to the student
      "label": "Berilgan so'zning … toping",   // instruction
      "order": 1,
      "no_value": true,
      "config": null,
      "options": [
        { "order": 1, "alternatives": [
          { "id": 164, "type": 200, "value": "kalit" },   // type 200 = CORRECT
          { "id": 175, "type": 100, "value": "soyabon" }, // type 100 = distractor
          { "id": 165, "type": 100, "value": "noutbuk" },
          { "id": 161, "type": 100, "value": "kitob" }
        ]}
      ]
    }
    // … N questions
  ]
}
```
Errors: `403` not a member · `404` battle not found.

### POST `/answer` — submit one answer
Idempotent — re-sending the same `question_id` is ignored.
```json
{
  "battle_id": "a6aee3be-…",
  "question_id": 164,
  "values": ["kalit"],     // the chosen alternative's "value" text, case-insensitive
  "answer_time": 4200      // milliseconds the student took (drives the time tie-break)
}
```
```jsonc
// result
{ "status": true,  "message": "Success" }
{ "status": false, "message": "Question does not exist" }   // unknown question_id
```
- Server advances `current_question`; the student is `is_finished` after the last one.
- Correct = 500 base points; wrong = 0.
- Errors: `422` battle not started / finished · `403` not a member · `404` battle not found.

### POST `/leave`
```json
{ "battle_id": "a6aee3be-…" }
```
WAITING/ON_QUEUE → removed. ON_GOING (confirmed) → remaining questions blank-filled, marked
finished. AI battle → deleted. `result`: `{ "message": "You have left battle" }`.

### POST `/change-type` — convert a waiting P2P lobby to AI
```json
{ "battle_id": "a6aee3be-…" }
```
Adds a bot and starts immediately. Returns `{ message, model }`.

### GET `/:uuid` — full snapshot
Status, members (with results when FINISHED), token, realtime. **No questions** here.
```jsonc
// result
{
  "uuid": "a6aee3be-…",
  "status": 300,
  "token": "",
  "realtime": { "driver": "ws", "url": "ws://…/ws?battle=a6aee3be-…" },
  "members": [
    { "place": 1, "points": 1, "student": { …profile… }, "answers": "[{…},{…}]" },
    { "place": 2, "points": 0, "student": { …profile… }, "answers": "[{…}]" }
  ]
}
```
Members are ordered by `place` ASC. `answers` is a **JSON string** (or `null` until
answered) — `JSON.parse` it. Each entry:
```json
{
  "question_id": 164, "question": "key", "values": ["kalit"],
  "time": 4200, "is_correct": true, "points": 500,
  "correct_answer": "kalit", "correct_answers": ["kalit"]
}
```

### `student` (public profile, in every member)
```jsonc
{
  "full_name": "Umid Muminov",
  "avatar": "https://…/file.png",       // URL, or a gender-default image
  "level":   { "id": 3, "name": "Beginner 3", "order": 3, … },
  "point":   1240,
  "themes":  [ { "type": 1, "url": "…" } ]
}
```

---

## 7. Typical client sequence (one player)

```
1. login → get-jwt                      → store go-api JWT
2. POST /find                           → save uuid; connect WS via model.realtime.url + "&token="+jwt
3. (WS battle status 400)               → show "Confirm" (20s, expire_time)
4. POST /confirm
5. (WS battle status 200)               → GET /:uuid/questions ; render Q in order
6. for each question (15s timer each):
     POST /answer {question_id, values:[value], answer_time}
     (opponents' progress arrives via WS member events: current_question / is_finished)
7. (WS battle status 300)               → GET /:uuid ; show places + points + per-answer correctness/time
```

---

## 8. Result rules (so the UI can explain scores)

1. Placement = **most correct answers**; tie broken by **least total time** (lower wins).
2. Bonus for fastest **correct** answer per question — P2P: 1st +50. GROUP: 1st +120, 2nd +70, 3rd +50.
3. **A student with >50% wrong answers gets 0 reward points, regardless of placement.**
4. Reward points: <6 members → 1st = 1, rest = 0. ≥6 members → 1st=3, 2nd=2, 3rd=1.

---

## 9. Notes (dev build)

- CORS is open (`*`) — fine for dev; lock down for prod.
- A reference web client lives at `battle-test.html` (open in 2 tabs) — full flow + live WS log.
- Realtime transport is switchable server-side (`REALTIME_DRIVER=ws|ably`) **without** changing
  endpoints. Always read `model.realtime.driver` rather than assuming.
