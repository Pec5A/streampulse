# 0001 — Architecture de l'authentification (ticket Y1)

## Statut
Accepté

## Contexte
Il fallait choisir : l'algorithme de signature JWT, le routeur HTTP côté backend, et la gestion d'état côté mobile pour la fonctionnalité d'authentification.

## Décisions

### JWT en HS256 (pas RS256)
StreamPulse est une API unique consommée par une seule app mobile — pas de scénario multi-services où un tiers aurait besoin de vérifier un token sans détenir le secret. RS256 (paire clé publique/privée) apporte de la complexité (gestion des clés, rotation) sans bénéfice ici. HS256 avec un secret partagé unique (`JWT_SECRET`, ≥32 caractères, jamais en dur) suffit.

### Routeur HTTP : `net/http` stdlib (pas de framework)
Go 1.22+ a ajouté le routage par méthode+motif (`mux.HandleFunc("POST /api/v1/auth/register", ...)`) directement dans `net/http`. Pour la taille de cette API, ajouter Gin/Chi/Echo n'apporte rien qu'on n'ait déjà, et augmente la surface de dépendances (donc d'attaque, et de maintenance). Réévaluer si le nombre de routes/middlewares grossit significativement.

### Persistance : `database/sql` + pilote `pgx` (pas d'ORM)
SQL brut, explicite, sans couche d'abstraction supplémentaire à apprendre/maintenir. Le nombre de requêtes reste faible pour ce projet.

### Mobile : `flutter_bloc`
Séparation claire événements → état, testable unitairement sans widget (voir `test/features/auth/bloc/auth_bloc_test.dart`, 5 tests avec `bloc_test`/`mocktail`, zéro rendu d'UI nécessaire). Choix à suivre par les autres tickets mobile (K1, S1...) pour la cohérence de l'app.

## Conséquences
- Le secret JWT doit être distribué de façon sécurisée en production (variable d'environnement injectée par la plateforme d'hébergement, jamais committée).
- `internal/infrastructure/persistence` n'a pas de tests unitaires (nécessite une vraie base Postgres) — à couvrir via testcontainers dans un ticket dédié si le temps le permet.
