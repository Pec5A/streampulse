# Runbook — Mise en production / Production deployment (ticket K3)

Bilingue **FR / EN**. Chaîne de livraison continue : chaque commit sur `main` produit une image et des artefacts mobiles ; le déploiement pousse cette image exacte vers Fly.io.

---

## FR

### 0. Le principe

L'application n'est **jamais construite par l'hébergeur**. Le job `api-image` publie une image sur GHCR, la vérifie en la démarrant contre un vrai Postgres, et le job `deploy` pousse **cette image-là** (`flyctl deploy --image`). L'artefact vérifié en CI et l'artefact en production sont le même, octet pour octet — et non deux builds qui se ressemblent.

Tant que la variable de dépôt `FLY_APP` n'est pas définie, le job `deploy` est **ignoré**, pas en échec : on n'annonce pas un déploiement qu'on ne fait pas.

### 1. Mise en place initiale (une seule fois)

```bash
# 1. Créer l'app et la base
flyctl auth login
flyctl apps create streampulse-api
flyctl postgres create --name streampulse-db --region cdg
flyctl postgres attach streampulse-db --app streampulse-api   # injecte DATABASE_URL

# 2. Le secret applicatif (jamais dans fly.toml, qui est versionné)
flyctl secrets set JWT_SECRET="$(openssl rand -base64 48)" --app streampulse-api

# 3. Donner à la CI de quoi déployer
flyctl tokens create deploy --app streampulse-api          # → coller dans le secret GitHub
gh secret set FLY_API_TOKEN --repo Pec5A/streampulse
gh variable set FLY_APP --body streampulse-api --repo Pec5A/streampulse
```

Le `gh variable set FLY_APP` est ce qui **active** le job `deploy`. Avant lui, rien ne se déploie.

### 2. Déploiement nominal

Rien à faire : un merge sur `main` déclenche `CD`. Suivre le run dans l'onglet Actions.

Le job échoue volontairement dans deux cas :
- le check `/health` de `fly.toml` ne passe pas → Fly ne bascule pas le trafic, l'ancienne version continue de servir ;
- la prod ne renvoie pas le commit déployé → un déploiement « réussi » qui sert encore l'ancienne version est le mode de panne qu'on ne voit pas, donc on le rend visible.

### 3. Vérifier ce qui tourne réellement

```bash
curl -s https://streampulse-api.fly.dev/health
# {"status":"ok","version":"<tag>","commit":"<sha>"}
```

Le `commit` doit correspondre au SHA du dernier run vert. S'il diffère, le déploiement n'a pas basculé — voir §4.

### 4. Rollback

```bash
flyctl releases --app streampulse-api            # lister les versions
flyctl deploy --app streampulse-api \
  --image ghcr.io/pec5a/streampulse-api:sha-<sha-connu-bon>
```

Le rollback est un **redéploiement d'une image antérieure**, pas un `git revert` : l'image du commit précédent existe déjà sur GHCR (le CD la construit à chaque push sur `main`, taguée par sha long), donc revenir en arrière ne demande aucun build et prend le temps d'un `flyctl deploy`.

### 5. Incidents fréquents

| Symptôme | Cause probable | Action |
|---|---|---|
| Job `deploy` ignoré | `FLY_APP` non définie | `gh variable set FLY_APP` |
| `Error: no access token available` | `FLY_API_TOKEN` absent ou expiré | régénérer via `flyctl tokens create deploy` |
| Health check en échec au déploiement | `DATABASE_URL` ou `JWT_SECRET` manquant | `flyctl secrets list --app streampulse-api` |
| `/health` sans `version` | image construite hors pipeline | redéployer depuis un tag `sha-…` de GHCR |
| Prod OK mais aucune trace | `OTEL_EXPORTER_OTLP_ENDPOINT` vide (défaut) | pointer un collecteur joignable depuis Fly |

---

## EN

### 0. The principle

The application is **never built by the host**. The `api-image` job publishes an image to GHCR, verifies it by booting it against a real Postgres, and the `deploy` job ships **that same image** (`flyctl deploy --image`). The artefact verified in CI and the artefact in production are byte-identical, not two builds that resemble each other.

As long as the `FLY_APP` repository variable is unset, the `deploy` job is **skipped**, not failed: we do not advertise a deployment we are not performing.

### 1. One-time setup

```bash
flyctl auth login
flyctl apps create streampulse-api
flyctl postgres create --name streampulse-db --region cdg
flyctl postgres attach streampulse-db --app streampulse-api   # injects DATABASE_URL

flyctl secrets set JWT_SECRET="$(openssl rand -base64 48)" --app streampulse-api

flyctl tokens create deploy --app streampulse-api
gh secret set FLY_API_TOKEN --repo Pec5A/streampulse
gh variable set FLY_APP --body streampulse-api --repo Pec5A/streampulse
```

Setting `FLY_APP` is what **enables** the deploy job. Before that, nothing deploys.

### 2. Normal deployment

Nothing to do: merging to `main` triggers `CD`. Follow the run in the Actions tab.

The job fails deliberately in two cases: the `fly.toml` `/health` check does not pass (Fly then keeps serving the previous version), or production does not report the deployed commit — a "successful" deployment still serving the old version is the failure mode nobody notices, so it is made loud.

### 3. Check what is actually running

```bash
curl -s https://streampulse-api.fly.dev/health
# {"status":"ok","version":"<tag>","commit":"<sha>"}
```

`commit` must match the SHA of the last green run. If it differs, the rollout did not switch — see §4.

### 4. Rollback

```bash
flyctl releases --app streampulse-api
flyctl deploy --app streampulse-api \
  --image ghcr.io/pec5a/streampulse-api:sha-<known-good-sha>
```

Rollback is **redeploying an earlier image**, not a `git revert`: the previous commit's image already exists on GHCR (CD builds one per push to `main`, tagged by long sha), so going back needs no build and takes one `flyctl deploy`.

### 5. Common incidents

| Symptom | Likely cause | Action |
|---|---|---|
| `deploy` job skipped | `FLY_APP` unset | `gh variable set FLY_APP` |
| `Error: no access token available` | `FLY_API_TOKEN` missing or expired | regenerate with `flyctl tokens create deploy` |
| Health check fails on deploy | `DATABASE_URL` or `JWT_SECRET` missing | `flyctl secrets list --app streampulse-api` |
| `/health` has no `version` | image built outside the pipeline | redeploy from a GHCR `sha-…` tag |
| Production fine but no traces | `OTEL_EXPORTER_OTLP_ENDPOINT` empty (default) | point at a collector reachable from Fly |
