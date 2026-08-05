# 0005 — Mode offline : cache local des playlists (ticket S4, bonus)

## Statut
Accepté

## Contexte
S4 (bonus) : rendre les playlists consultables sans réseau.

## Décisions

### Cache read-through dans le repository, transparent pour les blocs
`PlaylistRepository.list()`/`get()` écrivent le résultat réseau dans un cache local, puis — en cas d'**échec réseau uniquement** — servent la dernière copie en cache. Une vraie erreur API (`ApiException`, 4xx/5xx) est **propagée**, jamais masquée par du cache périmé. Les blocs et écrans de S1 restent inchangés : l'offline est géré sous le repository.

### Stockage : `shared_preferences` (JSON)
Métadonnées structurées et légères (pas de fichiers média) → `shared_preferences` suffit ; pas de base embarquée (sqflite/Drift) à ce stade. `PlaylistModel`/`TrackModel` gagnent un `toJson()` symétrique de leur `fromJson`.

### Mutations non mises en cache
`create`/`addTrack`/`reorder`/`removeTrack` restent online-only : une file d'écriture offline (synchronisation différée) dépasse le périmètre du bonus.

## Conséquences
- La liste et les playlists déjà ouvertes restent affichables hors-ligne.
- Testable sans plateforme : `SharedPreferences.setMockInitialValues` + `MockClient` (http) pour simuler l'offline, une vraie erreur serveur, et le cache-on-success.
- Cette PR est **empilée sur S1** (#16) — à rebaser sur `main` après le merge de S1.
