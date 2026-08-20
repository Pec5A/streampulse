# 0002 — Playlists : tracks possédées et réordonnancement transactionnel (ticket S1)

## Statut
Accepté

## Contexte
S1 demande un CRUD de playlists + tracks avec une **file d'attente réordonnable de façon transactionnelle**. Deux questions de conception : (1) d'où viennent les tracks, (2) comment garantir un réordonnancement atomique et cohérent.

## Décisions

### Tracks possédées par la playlist (pas de catalogue partagé)
Le repo de référence modélise les tracks comme des entités indépendantes (uploadées) reliées aux playlists par une table de jointure. Ici, l'upload (K2) n'est pas encore construit, et un catalogue `tracks` partagé aurait une propriété ambiguë (CODEOWNERS n'attribue que les chemins `playlist/` à SamyZ). J'ai donc choisi des tracks **possédées par la playlist** : une table `playlist_tracks` (`id, playlist_id, title, artist, duration_seconds, source_url, position`) dont chaque ligne est une entrée de la file. `source_url` reste un pointeur optionnel vers le média que le streaming/upload fourniront plus tard.

→ Slice **auto-suffisante** : testable et déployable sans dépendre d'un autre ticket, et sans collision de table avec le futur `tracks` de K2.

### Réordonnancement transactionnel + contrainte `UNIQUE ... DEFERRABLE`
`UNIQUE (playlist_id, position)` garantit qu'aucune playlist n'a deux tracks à la même position. Mais réécrire les positions ligne par ligne crée des doublons **intermédiaires**. La contrainte est donc **`DEFERRABLE INITIALLY DEFERRED`** : elle n'est vérifiée qu'au `COMMIT`. `ReorderTracks` ouvre une transaction, réaffecte chaque position (`0..n-1`), et ne valide qu'à la fin — l'agencement final est unique, les états intermédiaires tolérés.

Alternatives écartées :
- **Pas de contrainte d'unicité** (approche de la référence) : plus simple, mais autorise des positions dupliquées et des files incohérentes.
- **Positions temporaires négatives** : plus de code pour le même résultat.

### Port repository + fake in-memory pour les tests
Comme pour l'auth (ADR 0001), `internal/infrastructure/persistence` n'a pas de tests unitaires (nécessite un vrai Postgres). Les règles métier (validation, ownership, permutation du reorder) sont testées au niveau **usecase** avec un fake in-memory qui applique les mêmes invariants (positions contiguës, reorder = permutation). L'adapter pgx réel est couvert par un **test d'intégration** `//go:build integration` (`playlist_repository_integration_test.go`) qui, si `DATABASE_URL` est défini, inverse une file de 3 tracks — ce qui force précisément les collisions de positions que seule la contrainte différée autorise.

### Mobile : flutter_bloc + reorder optimiste
Conforme à l'ADR 0001 (choix `flutter_bloc` à suivre par les tickets mobile). `PlaylistDetailBloc` applique le déplacement localement pour un retour instantané (glisser-déposer via `ReorderableListView`), persiste via l'API, puis réémet l'ordre canonique renvoyé par le serveur ; en cas d'échec il affiche l'erreur et recharge.

## Conséquences
- Le futur ticket upload (K2) introduira un vrai catalogue média ; relier une entrée de playlist à un média se fera via `source_url` (ou un futur `media_id`) sans casser ce schéma.
- L'adapter pgx du reorder n'est pas exécuté par la CI (pas de Postgres) : le test d'intégration doit être lancé manuellement — `DATABASE_URL=... go test -tags integration ./internal/infrastructure/persistence/` — à intégrer à la CI via un service Postgres quand le ticket CI (Y3/S3) le permettra.
