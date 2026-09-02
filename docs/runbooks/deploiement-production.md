# Runbook — Mise en production / Production deployment (ticket K3)

Bilingue **FR / EN**. Chaîne de livraison continue : chaque commit sur `main` produit une image et des artefacts mobiles ; le déploiement pousse cette image exacte vers Render.

---

## FR

### 0. Le principe

L'application n'est **jamais construite par l'hébergeur**. Le job `api-image` publie une image sur GHCR, la vérifie en la démarrant contre un vrai Postgres, et le job `deploy` demande à Render de tirer **cette image-là** (`imageUrl` avec le sha du commit). L'artefact vérifié en CI et l'artefact en production sont le même, octet pour octet — et non deux builds qui se ressemblent.

Tant que la variable de dépôt `RENDER_SERVICE_ID` n'existe pas, le job `deploy` est **ignoré**, pas en échec : on n'annonce pas un déploiement qu'on ne fait pas.

### 0 bis. Ce que coûte le plan gratuit — à connaître avant la soutenance

| Contrainte | Conséquence |
|---|---|
| Le service **s'endort après 15 min** sans trafic | la première requête attend **~1 min** le réveil |
| La base gratuite **expire 30 jours** après sa création | 14 jours de grâce ensuite, puis les données sont supprimées |
| 0,1 CPU / 512 Mo | largement suffisant : le test de charge tient 1000 auditeurs dans 38 MiB |

**Le jour de la soutenance, réveille l'application cinq minutes avant de présenter** (`curl https://<service>.onrender.com/health`). Un jury qui attend une minute devant un écran blanc retiendra ça et pas l'architecture.

### 1. Mise en place initiale (une seule fois)

**a. Identifiant de registre.** Le dépôt est privé, donc le paquet GHCR l'est aussi. Créer un jeton GitHub *classic* avec la seule permission `read:packages`, puis dans Render : **Settings → Registry Credentials → Add**, nom **`ghcr`** (le nom doit correspondre à `creds:` dans `render.yaml`), registre `ghcr.io`, utilisateur = ton login GitHub, mot de passe = le jeton.

**b. Blueprint.** Render → **New → Blueprint**, pointer sur le dépôt. Il lit `render.yaml` et propose de créer le service et la base. Il demandera les deux valeurs marquées `sync: false` :

- `JWT_SECRET` — **au moins 32 caractères**, sinon l'API refuse de démarrer :
  ```bash
  openssl rand -base64 48
  ```
- `CORS_ALLOWED_ORIGINS` — laisser **vide** tant qu'aucun client web n'est déployé. Vide = aucun accès navigateur, ce qui est le bon défaut.

`METRICS_TOKEN` est généré par Render : personne ne le voit, donc personne ne peut le divulguer. Il est **obligatoire** — avec `ENVIRONMENT=production`, l'API refuse de démarrer sans lui plutôt que d'exposer `/metrics` (table des routes, volumes, latences, profil mémoire) sur Internet.

**c. Vérifier deux choses dans le dashboard** avant de continuer : que la base a bien été provisionnée sur le plan **free**, et que le service tire l'image sans erreur d'authentification au registre.

**d. Donner à la CI de quoi déployer.**

```bash
# Render → Account Settings → API Keys → Create API Key
gh secret set RENDER_API_KEY --repo Pec5A/streampulse

# L'identifiant du service se lit dans son URL de dashboard : srv-xxxxxxxx
gh variable set RENDER_SERVICE_ID --body srv-xxxxxxxx --repo Pec5A/streampulse
gh variable set RENDER_SERVICE_URL --body https://streampulse-api.onrender.com --repo Pec5A/streampulse
```

`RENDER_SERVICE_ID` est ce qui **active** le job `deploy`. Avant elle, rien ne se déploie.

### 2. Déploiement nominal

Rien à faire : un merge sur `main` déclenche `CD`. Suivre le run dans l'onglet Actions.

Le job échoue volontairement dans deux cas :
- le `healthCheckPath` de `render.yaml` ne répond pas → Render ne bascule pas le trafic, l'ancienne version continue de servir ;
- la prod ne renvoie pas le commit déployé au bout de 10 minutes → un déploiement « réussi » qui sert encore l'ancienne version est le mode de panne qu'on ne voit pas, donc on le rend bruyant.

### 3. Vérifier ce qui tourne réellement

```bash
curl -s https://streampulse-api.onrender.com/health
# {"status":"ok","version":"<tag>","commit":"<sha>"}
```

Le `commit` doit correspondre au SHA du dernier run vert. La première requête peut prendre une minute si le service dormait.

### 4. Rollback

```bash
curl -X POST "https://api.render.com/v1/services/$RENDER_SERVICE_ID/deploys" \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"imageUrl": "ghcr.io/pec5a/streampulse-api:sha-<sha-connu-bon>"}'
```

Le rollback est un **redéploiement d'une image antérieure**, pas un `git revert` : l'image de chaque commit de `main` existe déjà sur GHCR, taguée par sha long, donc revenir en arrière ne demande aucun build.

### 5. Incidents fréquents

| Symptôme | Cause probable | Action |
|---|---|---|
| Job `deploy` ignoré | `RENDER_SERVICE_ID` non définie | `gh variable set RENDER_SERVICE_ID` |
| `401` sur l'API Render | `RENDER_API_KEY` absente ou révoquée | en régénérer une dans Render |
| Render : échec de tirage de l'image | identifiant de registre absent ou jeton expiré | recréer la credential `ghcr` (§1a) |
| Logs : « JWT_SECRET is required and must be at least 32 characters » | valeur trop courte à la création | la remplacer par `openssl rand -base64 48` |
| Logs : « METRICS_TOKEN is required » | variable vidée dans le dashboard | la régénérer — refus de démarrer volontaire |
| Health check en échec au déploiement | `DATABASE_URL` non liée | vérifier que la base existe et que `fromDatabase` a résolu |
| Première requête très lente | service endormi (plan gratuit) | normal, ~1 min ; réveiller avant une démo |
| Erreurs base après ~30 jours | base gratuite expirée | recréer la base, ou passer sur un plan payant |
| `/health` sans `version` | image construite hors pipeline | redéployer depuis un tag `sha-…` de GHCR |
| L'app web ne peut pas appeler l'API | `CORS_ALLOWED_ORIGINS` vide | y mettre l'origine exacte du client web |

---

## EN

### 0. The principle

The application is **never built by the host**. The `api-image` job publishes an image to GHCR, verifies it by booting it against a real Postgres, and the `deploy` job asks Render to pull **that same image** (`imageUrl` with the commit sha). The artefact verified in CI and the artefact in production are byte-identical, not two builds that resemble each other.

As long as the `RENDER_SERVICE_ID` repository variable does not exist, the `deploy` job is **skipped**, not failed: we do not advertise a deployment we are not performing.

### 0b. What the free plan costs you — know this before the defence

| Constraint | Consequence |
|---|---|
| The service **sleeps after 15 min** without traffic | the first request waits **~1 min** for the wake-up |
| The free database **expires 30 days** after creation | 14-day grace period, then the data is deleted |
| 0.1 CPU / 512 MB | ample: the load test holds 1000 listeners in 38 MiB |

**On the day of the defence, wake the app five minutes before presenting** (`curl https://<service>.onrender.com/health`). A jury staring at a blank screen for a minute will remember that, not the architecture.

### 1. One-time setup

**a. Registry credential.** The repository is private, so the GHCR package is too. Create a GitHub *classic* token with only `read:packages`, then in Render: **Settings → Registry Credentials → Add**, name **`ghcr`** (it must match `creds:` in `render.yaml`), registry `ghcr.io`, username = your GitHub login, password = the token.

**b. Blueprint.** Render → **New → Blueprint**, point it at the repository. It reads `render.yaml` and offers to create the service and the database. It will ask for the two `sync: false` values:

- `JWT_SECRET` — **at least 32 characters**, otherwise the API refuses to start: `openssl rand -base64 48`
- `CORS_ALLOWED_ORIGINS` — leave **empty** until a web client is deployed. Empty means no browser access, which is the right default.

`METRICS_TOKEN` is generated by Render: nobody sees it, so nobody can leak it. It is **mandatory** — with `ENVIRONMENT=production` the API refuses to start without it rather than exposing `/metrics` (route table, volumes, latencies, memory profile) on the internet.

**c. Check two things in the dashboard** before going further: that the database really was provisioned on the **free** plan, and that the service pulls the image without a registry authentication error.

**d. Give CI what it needs to deploy.**

```bash
# Render → Account Settings → API Keys → Create API Key
gh secret set RENDER_API_KEY --repo Pec5A/streampulse

# The service id is in its dashboard URL: srv-xxxxxxxx
gh variable set RENDER_SERVICE_ID --body srv-xxxxxxxx --repo Pec5A/streampulse
gh variable set RENDER_SERVICE_URL --body https://streampulse-api.onrender.com --repo Pec5A/streampulse
```

`RENDER_SERVICE_ID` is what **enables** the deploy job. Before it, nothing deploys.

### 2. Normal deployment

Nothing to do: merging to `main` triggers `CD`. Follow the run in the Actions tab.

The job fails deliberately in two cases: the `healthCheckPath` does not answer (Render then keeps serving the previous version), or production does not report the deployed commit within 10 minutes — a "successful" deployment still serving the old version is the failure mode nobody notices, so it is made loud.

### 3. Check what is actually running

```bash
curl -s https://streampulse-api.onrender.com/health
# {"status":"ok","version":"<tag>","commit":"<sha>"}
```

`commit` must match the SHA of the last green run. The first request may take a minute if the service was asleep.

### 4. Rollback

```bash
curl -X POST "https://api.render.com/v1/services/$RENDER_SERVICE_ID/deploys" \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"imageUrl": "ghcr.io/pec5a/streampulse-api:sha-<known-good-sha>"}'
```

Rollback is **redeploying an earlier image**, not a `git revert`: every commit on `main` already has its image on GHCR tagged by long sha, so going back needs no build.

### 5. Common incidents

| Symptom | Likely cause | Action |
|---|---|---|
| `deploy` job skipped | `RENDER_SERVICE_ID` unset | `gh variable set RENDER_SERVICE_ID` |
| `401` from the Render API | `RENDER_API_KEY` missing or revoked | regenerate one in Render |
| Render: image pull failure | registry credential missing or token expired | recreate the `ghcr` credential (§1a) |
| Logs: "JWT_SECRET is required and must be at least 32 characters" | value too short at creation | replace with `openssl rand -base64 48` |
| Logs: "METRICS_TOKEN is required" | variable cleared in the dashboard | regenerate it — the refusal to start is deliberate |
| Health check fails on deploy | `DATABASE_URL` not linked | check the database exists and `fromDatabase` resolved |
| Very slow first request | service asleep (free plan) | normal, ~1 min; wake it before a demo |
| Database errors after ~30 days | free database expired | recreate it, or move to a paid plan |
| `/health` has no `version` | image built outside the pipeline | redeploy from a GHCR `sha-…` tag |
| The web app cannot call the API | `CORS_ALLOWED_ORIGINS` empty | set the web client's exact origin |
