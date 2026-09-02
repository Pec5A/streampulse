# Acceptance test book — StreamPulse

> French version: [`CAHIER_DE_RECETTE.md`](CAHIER_DE_RECETTE.md).
> Method and coverage status: [`PLAN_DE_TESTS.en.md`](PLAN_DE_TESTS.en.md).
> RNCP criteria: **Ce3.2.2**, **Ce3.2.4**.

Functional expectations, stated from the user's point of view, and **the automated test that verifies each one**. A scenario with no test named next to it is a scenario we merely believe is covered.

### "Verified by" legend

- `TestX` — automated test that fails if the behaviour breaks
- **manual** — checked by hand, no automated guard
- **not covered** — expectation stated, nothing verifies it

### Roles

| Role | What they can do |
|---|---|
| Anonymous | browse live streams, listen |
| User | + favourites, playlists, manage their account |
| Broadcaster | + start a live stream, upload audio |
| Admin | + manage users and roles, see global stats |

---

## 1. Account and authentication (Y1 — on `main`)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| A-01 | visitor | create an account | 201, account created, password stored hashed | `TestAuthHandler_Register_Success` |
| A-02 | visitor | be refused if the email already exists | 409, no second account | `TestAuthHandler_Register_DuplicateEmailReturns409` |
| A-03 | visitor | be refused if my password is too short | 400, explicit message | `TestAuthHandler_Register_ShortPassword` |
| A-04 | user | log in | 200 + usable JWT | `TestAuthHandler_Login_Success` |
| A-05 | user | **not** learn whether the email or the password was wrong | identical 401 in both cases | `TestAuthUseCase_Login_UserNotFound`, `TestAuthUseCase_Login_WrongPassword` |
| A-06 | user | extend my session | 200 + new token; 401 without a valid token | `TestAuthHandler_Refresh_RequiresAuth` |
| A-07 | attacker | not get through with a tampered token | 401 | `TestJWTManager_RejectsTamperedToken` |
| A-08 | attacker | not get through with a missing or malformed header | 401 | `TestRequireAuth_MissingHeader`, `TestRequireAuth_MalformedHeader` |

## 2. Playlists (S1 — on `main`)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| P-01 | user | create a playlist | 201, I am the owner | `TestCreate_Success` |
| P-02 | user | be refused on an empty or over-long name | 400 | `TestCreate_EmptyName`, `TestCreate_NameTooLong` |
| P-03 | user | see only my playlists in my list | mine only | `TestListByOwner_FiltersByOwner` |
| P-04 | third party | not open a private playlist | 403/404 | `TestGet_PrivateForbiddenToOthers` |
| P-05 | third party | open a public playlist | 200 | `TestGet_PublicVisibleToOthers` |
| P-06 | user | append a track to the queue | next position assigned | `TestAddTrack_AppendsWithPosition` |
| P-07 | user | remove a track without leaving a hole | positions compacted | `TestRemoveTrack_CompactsPositions` |
| P-08 | user | reorder my queue | new order persisted, transactional | `TestReorderTracks_Success` |
| P-09 | user | be refused if the submitted order is not an exact permutation | 400, no partial change | `TestReorderTracks_InvalidSet` |
| P-10 | third party | not be able to change anything of someone else's | 403 on update/delete/reorder | `TestUpdate_ForbiddenForNonOwner`, `TestDelete_ForbiddenForNonOwner`, `TestReorderTracks_ForbiddenForNonOwner` |
| P-11 | anonymous | not reach playlists at all | 401 on every route | `TestPlaylistAPI_Unauthenticated` |
| P-12 | user | a malformed id not to cause a server error | 400, not 500 | `TestPlaylistAPI_MalformedIDIsBadRequest` |
| P-13 | user | find my playlists offline | last known state served from cache | **PR #26, in review** |

## 3. Administration (S2 — on `main`)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| AD-01 | admin | list users | 200 + list | `TestAdminAPI_StatsAndList` |
| AD-02 | admin | change a user's role | 200, role persisted | `TestAdminAPI_UpdateRole` |
| AD-03 | admin | be refused on a non-existent role | 400 | `TestAdminUseCase_UpdateRole_Invalid` |
| AD-04 | user | not reach the admin area | 403 | `TestAdminAPI_AccessControl`, `TestRequireAdmin` |
| AD-05 | anonymous | not reach the admin area | 401 | `TestRouter_AdminRoutesRequireAuthAndAdmin` |
| AD-06 | admin | see global stats | 200 + counters | `TestAdminUseCase_Stats` |
| AD-07 | low-vision user | navigate the app with a screen reader | `Semantics` labels, targets ≥ 48 dp, AA contrast | **manual** — see `accessibility/politique-accessibilite.md` |

## 4. GDPR (Y2 — PR #14)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| G-01 | user | export my personal data | 200 + JSON **without** the password hash | `TestUserUseCase_ExportData` |
| G-02 | user | delete my account | 204, data erased by cascade | `TestUserUseCase_DeleteMe` |
| G-03 | user | a second deletion not to break anything | 204 (idempotent), never 500 | `TestUserUseCase_DeleteMe_NotFound` |
| G-04 | attacker | not export or delete someone else's account | the id comes from the JWT, not the URL — IDOR impossible by construction | `TestUserHandler_*` |

## 5. Live streaming (K1 — PR #22)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| S-01 | broadcaster | start a live stream | 201, status `live` | `TestStreamUseCase_StartLive` |
| S-02 | listener | listen without an account | 200, chunked stream | `TestLiveStream_*` (routeur) |
| S-03 | N listeners | receive the same stream simultaneously | 25 listeners served, no mutual blocking | #22 end-to-end test |
| S-04 | slow listener | not slow the others down | non-blocking `Publish` (`select/default`) | `TestHub_*` |
| S-05 | dead listener | be evicted after 64 **consecutive** drops | eviction; transient jitter does not disconnect | `TestHub_*` |
| S-06 | third party | not stop someone else's stream | 403 | `TestStreamUseCase_*` |
| S-07 | operator | know the server holds N listeners in bounded memory | load and memory measurement | **not covered** — see `PLAN_DE_TESTS.en.md` §4.3 |

## 6. Broadcaster and upload (K2 — PR #23)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| U-01 | broadcaster | upload an audio file | 201, object stored atomically | `TestLocal_*` |
| U-02 | attacker | not upload disguised active content | anything sniffing as `text/` is rejected — audio is binary | `TestTrackUseCase_*` |
| U-03 | attacker | not escape the storage directory | path traversal rejected (9 cases) | `TestLocal_Resolve*` |
| U-04 | attacker | not bypass the size limit via `Content-Length` | bytes actually written are counted | `TestTrackUseCase_*` |
| U-05 | operator | an interrupted upload to leave nothing | neither final nor temporary file | `TestLocal_FailedUploadLeavesNoObject` |
| U-06 | listener | resume playback mid-file | 206 + correct `Content-Range` | #23 `Range` test |

## 7. Live chat (Y4 — PR #25, bonus)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| C-01 | participant | my message to reach everyone, myself included | fan-out to N participants | `TestLiveStream_ChatFansOut...` |
| C-02 | participant | stay in the room when the broadcaster reconnects | room preserved | `TestChatRegistry_OpenKeepsParticipantsAcrossBroadcasterReconnect` |
| C-03 | participant | the room to close when the stream ends | clean shutdown | `TestChatRegistry_*` |
| C-04 | attacker | not spoof a username | username resolved server-side, never from the client | `TestLiveStream_ChatFansOut...` |
| C-05 | attacker | not flood the room | length bounded **in runes**, connection not closed | `TestLiveStream_ChatRejectsOverlongMessages...` |

## 8. Operations (Y3 — PRs #14 and #28)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| O-01 | operator | know which version is running | `/health` returns `version` and `commit` | `TestHealth_ReportsBuildInfoWhenStamped` |
| O-02 | operator | tell technical from business errors | separate metrics, separate panels | `metrics.go` + dashboard |
| O-03 | operator | a scanner not to blow up memory | `<unmatched>` label, bounded cardinality | `TestTracing_CollapsesUnmatchedRoutes` + #14 metrics test |
| O-04 | operator | follow a request end to end | continuous trace, `traceparent` honoured | `TestTracing_ContinuesAnIncomingTrace` |
| O-05 | operator | link a log line to its trace | root `trace_id` on every JSON line | `TestNewLogger_AddsTraceCorrelationInsideASpan` |
| O-06 | operator | a 401 not to count as a server failure | only 5xx marks the error | `TestTracing_MarksOnlyServerErrorsAsFailed` |
| O-07 | operator | be alerted on an abnormal error rate | Prometheus rules + FR/EN runbooks | `deployments/prometheus/alerts.yml` |

## 9. Delivery (K3 — branch `ci/yassir-cd-pipeline`)

| # | As a | I want | Expected result | Verified by |
|---|---|---|---|---|
| D-01 | team | one image per commit on `main` | GHCR image tagged by sha | `api-image` job |
| D-02 | team | the published image to actually boot | boots against a real Postgres, `/health` ok | job verification step |
| D-03 | team | downloadable mobile artefacts | APK and web as artefacts | `mobile-artifacts` job |
| D-04 | team | a broken deployment not to replace the working version | `/health` check required by Fly before switching | `fly.toml` |
| D-05 | team | detect a deployment still serving the old version | production must report the run's commit | `deploy` job verification step |
| D-06 | team | roll back quickly | redeploy an earlier GHCR image | `runbooks/deploiement-production.md` §4 |
