# Cahier de recette — StreamPulse

> Version anglaise : [`CAHIER_DE_RECETTE.en.md`](CAHIER_DE_RECETTE.en.md).
> Méthode et état de la couverture : [`PLAN_DE_TESTS.md`](PLAN_DE_TESTS.md).
> Critères RNCP visés : **Ce3.2.2**, **Ce3.2.4**.

Les attentes fonctionnelles, exprimées du point de vue de l'utilisateur, et **le test automatisé qui les vérifie**. Un scénario sans test nommé en face est un scénario qu'on croit couvert.

### Légende de la colonne « Vérifié par »

- `TestX` — test automatisé qui échoue si le comportement casse
- **manuel** — vérifié à la main, pas de garde-fou automatique
- **non couvert** — attente exprimée, rien ne la vérifie

### Rôles

| Rôle | Ce qu'il peut faire |
|---|---|
| Anonyme | consulter les directs, écouter |
| User | + favoris, playlists, gestion de son compte |
| Diffuseur | + créer un direct, uploader de l'audio |
| Admin | + gérer les utilisateurs et les rôles, voir les stats globales |

---

## 1. Compte et authentification (Y1 — sur `main`)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| A-01 | visiteur | créer un compte | 201, compte créé, mot de passe stocké haché | `TestAuthHandler_Register_Success` |
| A-02 | visiteur | être refusé si l'email existe déjà | 409, aucun second compte | `TestAuthHandler_Register_DuplicateEmailReturns409` |
| A-03 | visiteur | être refusé si mon mot de passe est trop court | 400, message explicite | `TestAuthHandler_Register_ShortPassword` |
| A-04 | utilisateur | me connecter | 200 + JWT exploitable | `TestAuthHandler_Login_Success` |
| A-05 | utilisateur | **ne pas** savoir si c'est l'email ou le mot de passe qui est faux | 401 identique dans les deux cas | `TestAuthUseCase_Login_UserNotFound`, `TestAuthUseCase_Login_WrongPassword` |
| A-06 | utilisateur | prolonger ma session | 200 + nouveau token ; 401 sans token valide | `TestAuthHandler_Refresh_RequiresAuth` |
| A-07 | attaquant | ne pas passer avec un token modifié | 401 | `TestJWTManager_RejectsTamperedToken` |
| A-08 | attaquant | ne pas passer sans en-tête ou avec un en-tête malformé | 401 | `TestRequireAuth_MissingHeader`, `TestRequireAuth_MalformedHeader` |

## 2. Playlists (S1 — sur `main`)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| P-01 | user | créer une playlist | 201, je suis propriétaire | `TestCreate_Success` |
| P-02 | user | être refusé sur un nom vide ou trop long | 400 | `TestCreate_EmptyName`, `TestCreate_NameTooLong` |
| P-03 | user | ne voir que mes playlists dans ma liste | seules les miennes | `TestListByOwner_FiltersByOwner` |
| P-04 | tiers | ne pas ouvrir une playlist privée | 403/404 | `TestGet_PrivateForbiddenToOthers` |
| P-05 | tiers | ouvrir une playlist publique | 200 | `TestGet_PublicVisibleToOthers` |
| P-06 | user | ajouter une piste en fin de file | position suivante attribuée | `TestAddTrack_AppendsWithPosition` |
| P-07 | user | retirer une piste sans laisser de trou | positions recompactées | `TestRemoveTrack_CompactsPositions` |
| P-08 | user | réordonner ma file d'attente | nouvel ordre persisté, transactionnel | `TestReorderTracks_Success` |
| P-09 | user | être refusé si l'ordre envoyé n'est pas une permutation exacte | 400, aucun changement partiel | `TestReorderTracks_InvalidSet` |
| P-10 | tiers | ne rien pouvoir modifier chez autrui | 403 sur update/delete/reorder | `TestUpdate_ForbiddenForNonOwner`, `TestDelete_ForbiddenForNonOwner`, `TestReorderTracks_ForbiddenForNonOwner` |
| P-11 | anonyme | ne pas accéder aux playlists | 401 sur toutes les routes | `TestPlaylistAPI_Unauthenticated` |
| P-12 | user | un identifiant malformé ne provoque pas d'erreur serveur | 400, pas 500 | `TestPlaylistAPI_MalformedIDIsBadRequest` |
| P-13 | user | retrouver mes playlists hors ligne | dernier état servi depuis le cache | **PR #26, en review** |

## 3. Administration (S2 — sur `main`)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| AD-01 | admin | lister les utilisateurs | 200 + liste | `TestAdminAPI_StatsAndList` |
| AD-02 | admin | changer le rôle d'un utilisateur | 200, rôle persisté | `TestAdminAPI_UpdateRole` |
| AD-03 | admin | être refusé sur un rôle inexistant | 400 | `TestAdminUseCase_UpdateRole_Invalid` |
| AD-04 | user | ne pas atteindre l'espace admin | 403 | `TestAdminAPI_AccessControl`, `TestRequireAdmin` |
| AD-05 | anonyme | ne pas atteindre l'espace admin | 401 | `TestRouter_AdminRoutesRequireAuthAndAdmin` |
| AD-06 | admin | consulter les stats globales | 200 + compteurs | `TestAdminUseCase_Stats` |
| AD-07 | utilisateur malvoyant | naviguer l'app au lecteur d'écran | libellés `Semantics`, cibles ≥ 48 dp, contrastes AA | **manuel** — voir `accessibility/politique-accessibilite.md` |

## 4. RGPD (Y2 — PR #14)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| G-01 | user | exporter mes données personnelles | 200 + JSON **sans** le hash de mot de passe | `TestUserUseCase_ExportData` |
| G-02 | user | supprimer mon compte | 204, données effacées en cascade | `TestUserUseCase_DeleteMe` |
| G-03 | user | qu'une seconde suppression ne casse rien | 204 (idempotent), jamais 500 | `TestUserUseCase_DeleteMe_NotFound` |
| G-04 | attaquant | ne pas exporter ni supprimer le compte d'un autre | l'identifiant vient du JWT, pas de l'URL — IDOR impossible par construction | `TestUserHandler_*` |

## 5. Diffusion live (K1 — PR #22)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| S-01 | diffuseur | ouvrir un direct | 201, statut `live` | `TestStreamUseCase_StartLive` |
| S-02 | auditeur | écouter sans compte | 200, flux chunké | `TestLiveStream_*` (routeur) |
| S-03 | N auditeurs | recevoir le même flux simultanément | 25 auditeurs servis, aucun blocage mutuel | test bout-en-bout #22 |
| S-04 | auditeur lent | ne pas ralentir les autres | `Publish` non bloquant (`select/default`) | `TestHub_*` |
| S-05 | auditeur mort | être évincé après 64 pertes **consécutives** | éviction ; une gigue passagère ne déconnecte pas | `TestHub_*` |
| S-06 | tiers | ne pas arrêter le direct d'un autre | 403 | `TestStreamUseCase_*` |
| S-07 | exploitant | savoir que le serveur tient N auditeurs en mémoire bornée | mesure de charge et de mémoire | **non couvert** — cf. `PLAN_DE_TESTS.md` §4.3 |

## 6. Diffuseur et upload (K2 — PR #23)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| U-01 | diffuseur | uploader un fichier audio | 201, objet stocké atomiquement | `TestLocal_*` |
| U-02 | attaquant | ne pas uploader du contenu actif déguisé | rejet de tout ce qui sniffe en `text/` — l'audio est binaire | `TestTrackUseCase_*` |
| U-03 | attaquant | ne pas sortir du répertoire de stockage | traversée de chemin rejetée (9 cas) | `TestLocal_Resolve*` |
| U-04 | attaquant | ne pas contourner la taille via `Content-Length` | octets réellement écrits comptés | `TestTrackUseCase_*` |
| U-05 | exploitant | qu'un upload interrompu ne laisse rien | ni fichier final, ni temporaire | `TestLocal_FailedUploadLeavesNoObject` |
| U-06 | auditeur | reprendre une lecture au milieu | 206 + `Content-Range` correct | test `Range` #23 |

## 7. Chat en direct (Y4 — PR #25, bonus)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| C-01 | participant | voir mon message chez tous, moi compris | diffusion à N participants | `TestLiveStream_ChatFansOut...` |
| C-02 | participant | rester dans le salon si le diffuseur se reconnecte | salon conservé | `TestChatRegistry_OpenKeepsParticipantsAcrossBroadcasterReconnect` |
| C-03 | participant | que le salon ferme à la fin du direct | fermeture propre | `TestChatRegistry_*` |
| C-04 | attaquant | ne pas usurper un pseudo | pseudo résolu côté serveur, jamais depuis le client | `TestLiveStream_ChatFansOut...` |
| C-05 | attaquant | ne pas noyer le salon | longueur bornée **en runes**, connexion non fermée | `TestLiveStream_ChatRejectsOverlongMessages...` |

## 8. Exploitation (Y3 — PR #14 et #28)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| O-01 | exploitant | savoir quelle version tourne | `/health` renvoie `version` et `commit` | `TestHealth_ReportsBuildInfoWhenStamped` |
| O-02 | exploitant | distinguer erreurs techniques et métier | métriques séparées, panels séparés | `metrics.go` + dashboard |
| O-03 | exploitant | qu'un scanner ne fasse pas exploser la mémoire | label `<unmatched>`, cardinalité bornée | `TestTracing_CollapsesUnmatchedRoutes` + test métriques #14 |
| O-04 | exploitant | suivre une requête de bout en bout | trace continue, `traceparent` honoré | `TestTracing_ContinuesAnIncomingTrace` |
| O-05 | exploitant | relier un log à sa trace | `trace_id` racine dans chaque ligne JSON | `TestNewLogger_AddsTraceCorrelationInsideASpan` |
| O-06 | exploitant | qu'un 401 ne compte pas comme une panne serveur | seules les 5xx marquent l'erreur | `TestTracing_MarksOnlyServerErrorsAsFailed` |
| O-07 | exploitant | être alerté sur un taux d'erreur anormal | règles Prometheus + runbooks FR/EN | `deployments/prometheus/alerts.yml` |

## 9. Livraison (K3 — branche `ci/yassir-cd-pipeline`)

| # | En tant que | Je veux | Résultat attendu | Vérifié par |
|---|---|---|---|---|
| D-01 | équipe | une image par commit de `main` | image GHCR taguée par sha | job `api-image` |
| D-02 | équipe | que l'image publiée démarre vraiment | boot contre un vrai Postgres, `/health` ok | étape de vérification du job |
| D-03 | équipe | des artefacts mobiles téléchargeables | APK et web en artefacts | job `mobile-artifacts` |
| D-04 | équipe | qu'un déploiement cassé ne remplace pas la version qui marche | check `/health` exigé par Fly avant bascule | `fly.toml` |
| D-05 | équipe | détecter un déploiement qui sert encore l'ancienne version | la prod doit annoncer le commit du run | étape de vérification du job `deploy` |
| D-06 | équipe | revenir en arrière vite | redéploiement d'une image GHCR antérieure | `runbooks/deploiement-production.md` §4 |
