# Battle — Go Rewrite (Presentation)

Competitive quiz: students battle each other (P2P / GROUP) or a scripted AI.
The old PHP/Firebase implementation was rewritten in **Go**, with **PostgreSQL** for
live game state and **Ably/WebSocket** for realtime. MySQL (the existing app DB) is
read for questions/students and written only for final results.

---

## 1. Why rewrite

| Old (PHP) | New (Go) |
|---|---|
| Battle logic in Yii2 request classes | Standalone Go service |
| Firebase Realtime | **Ably _or_ native WebSocket** (switchable) |
| All state in MySQL | **PostgreSQL** for live state; MySQL only at finish |
| — | Faster, isolated, horizontally clearer; same API responses |

**API responses are byte-compatible with the old PHP API** (same envelope + member shape),
so existing clients keep working.

---

## 2. Architecture

```
        ┌─────────────┐     HTTP (Bearer JWT)      ┌──────────────────┐
 client │  frontend   │ ─────────────────────────▶ │   Go battle API  │
        │ (browser)   │ ◀──── realtime events ───── │   (:8080, Gin)   │
        └─────────────┘   Ably  OR  WebSocket        └───────┬──────────┘
              │                                               │
              │ login + get-jwt                               │ active state
              ▼                                               ▼
        ┌─────────────┐                              ┌──────────────────┐
        │  PHP (Yii2) │  issues go-api JWT           │   PostgreSQL     │
        │  + MySQL    │  (RS256, private key)        │ battles, members │
        └─────────────┘                              └──────────────────┘
              ▲                                               │ final results
              └───────────────── MySQL ◀──────────────────────┘
                       (read questions/students; write results at battle end)
```

| Concern | Store |
|---|---|
| Active battle state (during game) | **PostgreSQL** (Go-owned) |
| Questions / students / profiles | **MySQL** (read-only) |
| Final results (battle, members, win/loss) | **MySQL** (written once, at finish) |
| Realtime events | **Ably** or **WebSocket** |

Two processes: **API** (HTTP) and **Worker** (background daemons).

---

## 3. Battle lifecycle

```
 find ─▶ WAITING(100) ──lobby full──▶ ON_QUEUE(400) ──all confirm──▶ ON_GOING(200) ──finish──▶ FINISHED(300)
                                          │ 20s                          │ play
                                          ▼                              ▼
                                   kick if not confirmed         results → MySQL
```

| Type | value | members |
|---|---|---|
| P2P | 100 | 2 |
| GROUP | 200 | 4 |
| AI | 300 | 1 real + 1 bot |

**Timers**
| Timer | Value | Effect |
|---|---|---|
| Confirm window | **20s** (ON_QUEUE) | unconfirmed members kicked; empty lobby deleted |
| Per question | **15s** | auto-submit as wrong, advance (frontend) |
| Member idle | **30s** (2× question) | stuck member auto-finished with blanks |
| Whole battle | **15×Q + 15s** (10Q = 165s) | unanswered filled blank, battle ends |

AI battles skip queue/confirm and start immediately.

---

## 4. HTTP API

Base (server): `https://v2-backend-erp.englifyschool.com/v2/battle/...`
Auth: `Authorization: Bearer <go-api JWT>` on every call.

| Method | Path | Purpose |
|---|---|---|
| POST | `/find` | find/create a lobby |
| POST | `/confirm` | confirm attendance (last one starts the battle) |
| GET | `/:uuid/questions` | fetch questions once ON_GOING |
| POST | `/answer` | submit one answer (idempotent) |
| POST | `/leave` | leave mid-battle |
| POST | `/change-type` | convert waiting P2P → AI |
| GET | `/:uuid` | full battle snapshot |

**Response envelope** (matches PHP):
```json
{ "ok": true, "status_code": 200, "description": "Success", "result": { … } }
```
**Member shape** (matches PHP `BattleMemberResource`):
```json
{ "place": 1, "points": 1, "answers": "<json string>",
  "student": { "full_name": "...", "avatar": "...", "level": {…}, "point": 0, "themes": [] } }
```

---

## 5. Auth flow (logged-in users)

```
1. POST /student/v1/auth/login   (api-token + identity/password)  → user access token
2. POST /student/v1/battle/get-jwt  (api-token + Bearer user token) → go-api JWT   [PHP signs RS256]
3. Use go-api JWT as Bearer for all Go battle endpoints + realtime
```
PHP owns the private key and knows who's logged in; Go validates with the public key.

---

## 6. Questions

- Source: **`json_question`** table — pre-built sets, random by `level_group` + type
  (**GRAMMAR=100, VOCABULARY=110**).
- Fallback: dynamic generation from the word/exercise tables if no template exists.
- Frozen on the battle at start → both players get the **same** set.
- Correct alternative = **`type: 200`** (ANSWER); distractors = `type: 100` (OPTION).
- Fetched via `GET /:uuid/questions` once the battle is ON_GOING (not pushed over realtime).

---

## 7. Scoring & placement

**Per answer:** correct = 500 base points (+ speed bonus); wrong = 0.

**Placement (who is 1st, 2nd, …):**
1. **Most correct answers** wins.
2. Tie → **least total time** (faster wins).
3. Tie → lower id (stable).

**Reward points (to `student_battle` win/loss):**
- A member with **>50% wrong** → 0 points regardless of place.
- <6 members: 1st = 1 pt, rest 0.
- ≥6 members: 1st = 3, 2nd = 2, 3rd = 1.

Example result:
```
Umid Muminov  place 1  points 1  correct 10/10  time 20s
Joshqin .     place 2  points 0  correct  8/10  time 70s
```

---

## 8. Realtime (events only)

Realtime carries **events, never questions or answers** — it drives screen
transitions and shows opponents' progress; data is fetched over HTTP.

Server picks one transport (`REALTIME_DRIVER=ably|ws`); both emit the **same payloads**.

| Channel | Carries |
|---|---|
| `{uuid}` | battle status (created → queue → ongoing → finished) |
| `{uuid}:{studentId}` | member events: confirmed, current_question, is_finished |

```json
// battle event
{ "type":"battle", "status":200, "expire_time":…, "starting_time":…, "end_time":…, "question_time":15 }
// member event
{ "type":"battle_member", "student_id":26, "status":200, "current_question":4, "is_finished":false }
```

- **Ably:** client uses a scoped subscribe token from the response.
- **WebSocket:** client connects `wss://…/v2/battle/ws?battle=<uuid>&token=<jwt>`; the
  worker fans out via PostgreSQL `LISTEN/NOTIFY` so its events reach connected clients.
- The battle response carries `realtime: { driver, url }` so the client knows which to use.

---

## 9. Background worker (daemons)

| Daemon | Poll | Job |
|---|---|---|
| Kick | 5s | ON_QUEUE expired → kick unconfirmed; reset to WAITING or delete |
| Idle finish | 3s | member stuck >30s → blank-finish that member |
| End (expired) | 3s | ON_GOING past end_time → fill blanks, finish, write results |
| End (all done) | 3s | all members finished → finish, write results |

Finishing = calculate places/points → write battle + members + win/loss to MySQL → publish.

---

## 10. Deployment (test server)

- ARM binaries built on the server, run as **systemd** services `battle-api` + `battle-worker`
  (auto-restart, boot-start).
- Dedicated PostgreSQL DB `battle_go`; MySQL = existing `v2_erp` (read questions, write results).
- nginx on the existing domain routes `/v2/battle/*` → Go; PHP untouched.
- Demo client: `https://v2-api-erp.englifyschool.com/battle-demo.html`.

---

## 11. Status

- 100% behavioral parity with the PHP reference (responses, scoring, timers).
- ~130 unit tests + smoke test, all green.
- Live on the test server; full real-login battle verified end-to-end.
- Switchable realtime (Ably ↔ WebSocket) by one env var.
