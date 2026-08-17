# 0007 — Smoke test de déploiement, auto-migrations, alertes auth (A3.4/A3.5)

## Statut
Accepté

## Contexte
Le découpage d'équipe (`docs/team/plan.md`) confie A3.4 (déploiement continu) à KaysZ et A3.5 (opérations continues/alertes) à SamyZ — aucun des deux n'est sur ma tranche (identité & confiance). Or l'évaluation RNCP est individuelle : chaque candidat passe 20 minutes seul face au jury, et un seul critère "non acquis" invalide le bloc entier *pour lui*. Sans artefact personnel sur A3.4/A3.5, je n'aurais rien de concret à montrer si le jury pose la question — indépendamment de ce que KaysZ ou SamyZ auront construit de leur côté.

Le choix n'était donc pas "refaire K3/S3 en entier" (doublon inutile, et hors de ma tranche fonctionnelle), mais construire un artefact scopé à ce que je possède déjà (le service auth) qui démontre concrètement ces deux compétences.

## Décisions

### A3.4 : un smoke test de déploiement en CI, pas juste un `docker-compose.yml` qui existe
Un fichier compose qui "a la bonne syntaxe" ne prouve rien sur le critère Ce3.4.1 ("mise en production automatique... sans intervention manuelle"). `scripts/smoke-test.sh` + le job CI `deploy-smoke-test` font vraiment `docker compose up`, puis appellent `/health`, `/api/v1/auth/register`, `/api/v1/auth/login` et `/metrics` contre le service démarré, et font échouer le pipeline si l'un de ces appels rate.

Ce choix a immédiatement payé : le premier run a révélé que les migrations SQL (`migrations/0001_create_users.up.sql`) n'étaient jamais appliquées automatiquement — un déploiement neuf plantait au premier appel réel avec un 500. Sans ce test, ce bug serait resté invisible jusqu'à la démo devant le jury.

### Migrations auto-appliquées au démarrage, pas un outil CLI séparé
Plutôt qu'ajouter une dépendance externe (`golang-migrate`, `goose`), un petit runner maison (`backend/internal/infrastructure/persistence/migrate.go`) lit les `*.up.sql` embarqués via `embed.FS` (`backend/migrations/migrations.go`) et les applique dans une table `schema_migrations`, une transaction par fichier. Cohérent avec le choix déjà pris dans l'ADR 0001 de rester sur `database/sql` sans couche d'abstraction supplémentaire. L'API applique ses propres migrations au démarrage — `docker compose up` devient un vrai déploiement en une commande.

### A3.5 : des règles d'alerte Prometheus scopées à l'auth, pas l'alerting complet du système
Trois règles dans `deployments/prometheus/alerts.yml`, toutes limitées à mon périmètre (`/api/v1/auth/*` et les compteurs métier auth) :
- `StreamPulseAuthHighFailureRate` (métier) : plus de 50% d'échecs de login sur 5 min — signal de credential stuffing ou de service dégradé, pas juste "il y a des erreurs".
- `StreamPulseAuthHighErrorRate` (technique) : taux de 5xx sur les endpoints auth.
- `StreamPulseAuthHighLatency` (technique) : p95 > 1s sur les endpoints auth.

Volontairement, pas d'Alertmanager ni de routing de notifications (email/Slack) : ça reste le périmètre du ticket S3 (SamyZ), qui construira l'alerting transverse à tout le système. Ici, ce sont des règles d'évaluation Prometheus — visibles dans `/alerts`, exploitables par n'importe quel Alertmanager branché dessus plus tard, sans dépendre de l'implémentation de S3 pour exister.

## Un bug trouvé en vérifiant, pas en supposant
Les règles d'alerte utilisaient d'abord `path=~"/api/v1/auth.*"`. En testant contre le stack réel (`docker compose up` + requêtes réelles), le label `path` du middleware technique (ADR 0005) s'est révélé être `"POST /api/v1/auth/login"` — la méthode HTTP est préfixée dans la valeur du label, pas juste dans le label `method` séparé. Le premier regex ne matchait donc jamais rien. Corrigé en `path=~".*/api/v1/auth/.*"`, revérifié contre Prometheus (`/api/v1/rules`, `health: "ok"`, séries non vides). Sans exécution réelle du stack, cette règle serait restée "présente dans le fichier" mais silencieusement inopérante — invisible à la simple lecture du YAML.

## Conséquences
- Le job `deploy-smoke-test` ajoute ~1-2 minutes au pipeline CI ; jugé acceptable vu ce qu'il a déjà attrapé une fois.
- Les migrations auto-appliquées supposent un seul processus API qui démarre à la fois (pas de verrou distribué). Suffisant pour ce projet (une seule instance en docker-compose) ; à revoir si K3 déploie plusieurs replicas en parallèle au démarrage.
- Les alertes définies ici n'envoient aucune notification tant que S3 (Alertmanager) n'est pas mergé — elles sont visibles dans l'UI Prometheus mais "silencieuses" en pratique. C'est un choix assumé de scope, pas un oubli.
