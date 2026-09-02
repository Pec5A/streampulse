# StreamPulseAuthHighLatency

**Sévérité** : warning · **Domaine** : technique · **Règle** : `deployments/prometheus/alerts.yml`

## C'est quoi
Le 95e percentile (p95) du temps de réponse sur `/api/v1/auth/*` dépasse 1 seconde, pendant au moins 5 minutes d'affilée. Contrairement aux deux autres alertes (déclenchement rapide, 2 min), celle-ci a une fenêtre plus longue exprès : la latence bouge naturellement avec le trafic, un pic de 30 secondes ne doit pas réveiller quelqu'un.

Requête exacte :
```promql
histogram_quantile(0.95,
  sum(rate(streampulse_http_request_duration_seconds_bucket{path=~".*/api/v1/auth/.*"}[5m])) by (le)
) > 1
```

## Causes possibles
1. **Coût du bcrypt** — `auth.NewBcryptHasher()` (ADR 0001) est volontairement lent (c'est tout l'intérêt du hashing de mot de passe), mais un coût mal calibré ou une charge CPU élevée sur la machine peut le faire déraper.
2. **DB lente** — requêtes `FindByID`/recherche par email sans index adapté, ou Postgres sous charge.
3. **Voisin bruyant sur l'hôte compose** — en environnement de démo (une seule machine pour api+postgres+prometheus+grafana), un autre conteneur qui consomme le CPU ralentit tout le monde. Moins pertinent une fois sur une vraie infra (ticket K3).
4. **Pool de connexions DB saturé** — les requêtes attendent une connexion libre avant même de s'exécuter.

## Comment vérifier
1. Regarder si la latence DB seule est en cause ou si c'est le calcul bcrypt :
   ```bash
   docker compose exec postgres psql -U streampulse -d streampulse -c "SELECT * FROM pg_stat_activity WHERE state = 'active';"
   ```
2. Vérifier la charge CPU de l'hôte/des conteneurs :
   ```bash
   docker stats --no-stream
   ```
3. Comparer p50/p95/p99 sur le dashboard Grafana (`deployments/grafana/dashboards/streampulse-auth.json`, panneaux techniques) — un p50 normal avec un p95 qui explose pointe vers quelques requêtes lentes isolées (queue de connexions ?) plutôt qu'un ralentissement général.

## Quoi faire
- **CPU saturé par un voisin** → en démo locale, réduire le nombre de conteneurs actifs en parallèle. En vraie infra, c'est un problème pour K3 (limits/requests Kubernetes).
- **DB lente** → vérifier les index sur `users(email)` (déjà présent, voir `migrations/0001_create_users.up.sql`) ; si de nouvelles requêtes sont ajoutées plus tard sans index, c'est la première chose à suspecter.
- **Pool saturé** → même remède que pour `alert-auth-high-error-rate.md` : regarder `pg_stat_activity`, ajuster `SetMaxOpenConns` si justifié par du trafic réel mesuré, pas en préventif.
- **Rien d'anormal trouvé** → probable variance normale à faible trafic (un seul appel lent suffit à faire bouger un p95 avec peu de volume) ; comparer le volume absolu avant de creuser plus loin.
