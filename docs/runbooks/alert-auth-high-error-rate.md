# StreamPulseAuthHighErrorRate

**Sévérité** : critical · **Domaine** : technique · **Règle** : `deployments/prometheus/alerts.yml`

## C'est quoi
Plus de 5% des requêtes vers `/api/v1/auth/*` ont renvoyé un code 5xx sur une fenêtre de 5 minutes, pendant au moins 2 minutes d'affilée. Contrairement à l'alerte métier (échecs de connexion), celle-ci ne regarde que les **codes HTTP** via le middleware technique (`middleware.Metrics`, voir ADR 0002) — elle se déclenche même si personne n'essaie de se connecter avec un mauvais mot de passe, juste parce que le serveur plante.

Requête exacte :
```promql
sum(rate(streampulse_http_requests_total{path=~".*/api/v1/auth/.*", status=~"5.."}[5m]))
/
sum(rate(streampulse_http_requests_total{path=~".*/api/v1/auth/.*"}[5m]))
> 0.05
```

## Causes possibles
1. **Postgres down ou inaccessible** — c'est exactement le bug trouvé pendant le travail sur ADR 0004 : sans les migrations, chaque appel réel plantait en 500. Un problème similaire (DB down, pool épuisé, mauvaise `DATABASE_URL`) redonnerait ce symptôme.
2. **Migrations non appliquées** — voir `persistence.Migrate` dans `main.go` ; si elle échoue au démarrage, l'API ne démarre même pas (fatal), donc ce cas précis serait plutôt une absence totale de trafic. Mais une migration future mal écrite pourrait laisser le schéma dans un état intermédiaire cassé.
3. **Bug de code introduit par un futur commit** sur `AuthUseCase`/`UserRepository`.
4. **Pool de connexions DB épuisé** sous forte charge (`db.SetMaxOpenConns(10)` dans `persistence.Open` — 10 connexions max, un pic de trafic peut suffire à saturer).

## Comment vérifier
1. Logs de l'API :
   ```bash
   docker compose logs api --tail 100
   ```
2. État de Postgres :
   ```bash
   docker compose exec postgres pg_isready -U streampulse
   ```
3. Reproduire en local avec le smoke test (`scripts/smoke-test.sh`) — il exerce exactement `/register` et `/login`, donc il donnera le même 500 si le problème est reproductible.
4. Vérifier que les migrations sont bien passées :
   ```bash
   docker compose exec postgres psql -U streampulse -d streampulse -c "SELECT * FROM schema_migrations;"
   ```

## Quoi faire
- **Postgres down** → `docker compose restart postgres`, attendre le healthcheck, relancer le smoke test.
- **Pool épuisé** → vérifier le nombre de connexions actives (`SELECT count(*) FROM pg_stat_activity;`), augmenter `SetMaxOpenConns` si le trafic réel le justifie (pas juste "au cas où" — voir la règle KISS/YAGNI du projet).
- **Migration cassée** → ne jamais éditer un fichier `*.up.sql` déjà appliqué en prod ; écrire une nouvelle migration corrective (`0002_fix_....up.sql`).
- **Bug de code** → rollback du dernier déploiement (`git revert` + redeploy) le temps d'investiguer, plutôt que de debugger en prod.
