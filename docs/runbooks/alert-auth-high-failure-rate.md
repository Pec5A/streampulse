# StreamPulseAuthHighFailureRate

**Sévérité** : warning · **Domaine** : métier · **Règle** : `deployments/prometheus/alerts.yml`

## C'est quoi
Plus de 50% des tentatives de connexion (`POST /api/v1/auth/login`) ont échoué (`result != "success"`) sur une fenêtre de 5 minutes, pendant au moins 2 minutes d'affilée. C'est une alerte **métier** : elle regarde le *résultat* des connexions (mot de passe faux, compte inexistant...), pas le code HTTP — un serveur qui répond correctement 401 à un mauvais mot de passe n'est pas "en panne" techniquement, mais peut quand même signaler un problème produit ou une attaque.

Requête exacte :
```promql
sum(rate(streampulse_auth_logins_total{result!="success"}[5m]))
/
sum(rate(streampulse_auth_logins_total[5m]))
> 0.5
```

## Causes possibles
1. **Credential stuffing / brute force** — quelqu'un teste des couples email/mot de passe en masse.
2. **Mauvais déploiement mobile** — une release de l'app envoie un payload cassé (mauvais format de mot de passe, bug de hashing côté client).
3. **Panne DB en amont** — `AuthUseCase.Login` renvoie `result="error"` sur les erreurs internes, pas seulement `invalid_credentials` ; un Postgres indisponible fait grimper le même compteur.
4. **Faible volume** — avec peu de trafic (ex. démo, dev), quelques échecs suffisent à dépasser 50% ; vérifier le volume absolu avant de paniquer.

## Comment vérifier
1. Regarder le volume absolu, pas juste le ratio :
   ```promql
   sum(increase(streampulse_auth_logins_total[5m]))
   ```
   Si c'est 3 tentatives dont 2 ratées, ce n'est pas une vraie anomalie — ajuster mentalement la lecture de l'alerte au volume réel.
2. Distinguer `invalid_credentials` de `error` :
   ```promql
   sum by (result) (rate(streampulse_auth_logins_total[5m]))
   ```
   Si c'est `error` qui domine → panne technique, voir `alert-auth-high-error-rate.md`. Si c'est `invalid_credentials` → suspicion de brute force ou de bug client.
3. Regarder la diversité des IP/comptes visés si les logs le permettent (pas encore instrumenté ici — limite connue, voir ADR 0004).

## Quoi faire
- **Si `error` domine** → traiter comme une panne technique (voir le runbook error-rate) : `docker compose logs api`, vérifier Postgres.
- **Si `invalid_credentials` domine et volume élevé** → suspicion de brute force. Ce repo n'a pas encore de rate-limiting sur `/auth/login` (dette connue, à traiter avec S3 — sécurité). Solution immédiate manuelle : surveiller, pas de blocage automatique disponible pour l'instant.
- **Si volume faible** → probable faux positif, ne rien faire, noter dans le canal d'équipe si ça se reproduit souvent (le seuil 50%/2min pourrait être mal calibré pour un environnement à faible trafic comme la démo).
