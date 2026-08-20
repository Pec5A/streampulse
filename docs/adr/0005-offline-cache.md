# 0005 — Mode offline : cache local des playlists (ticket S4, bonus)

## Statut
Accepté

## Contexte
S4 (bonus) : rendre les playlists consultables sans réseau.

## Décisions

### Cache read-through dans le repository, transparent pour les blocs
`PlaylistRepository.list()`/`get()` écrivent le résultat réseau dans un cache local, puis ne servent la dernière copie en cache **que sur un échec de connectivité** — précisément `SocketException`, `http.ClientException` ou `TimeoutException`. Toute autre erreur — serveur (`ApiException` 4xx/5xx) comme parsing — est **propagée**, jamais masquée par du cache périmé. Pour que ça tienne même sur un corps d'erreur non-JSON (page HTML d'un reverse proxy, panic en texte brut), `ApiClient` encapsule le `jsonDecode` du corps d'erreur et lève toujours une `ApiException` typée plutôt qu'une `FormatException`. Les blocs et écrans de S1 restent inchangés : l'offline est géré sous le repository.

### Stockage : `shared_preferences` (JSON)
Métadonnées structurées et légères (pas de fichiers média) → `shared_preferences` suffit ; pas de base embarquée (sqflite/Drift) à ce stade. `PlaylistModel`/`TrackModel` gagnent un `toJson()` symétrique de leur `fromJson`.

### Mutations non mises en cache
`create`/`addTrack`/`reorder`/`removeTrack` restent online-only : une file d'écriture offline (synchronisation différée) dépasse le périmètre du bonus.

## Conséquences
- La liste et les playlists déjà ouvertes restent affichables hors-ligne.
- Testable sans plateforme : `SharedPreferences.setMockInitialValues` + `MockClient` (http) pour simuler l'offline (`ClientException`/`SocketException`), une erreur serveur à corps JSON **et non-JSON**, et le cache-on-success.
- **Limite connue** : le cache n'est pas scopé par utilisateur (clés globales `cache.playlists`/`cache.playlist.$id`) et `logout()` ne le vide pas — sur un appareil partagé, un utilisateur pourrait voir en cache les playlists du précédent. Acceptable pour un bonus (contenu peu sensible) ; à durcir (`cache.clear()` au logout + scope par utilisateur) si le besoin apparaît.
- Cette PR est **empilée sur S1** (#16) — à rebaser sur `main` après le merge de S1.
