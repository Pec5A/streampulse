# 0008 — Architecture de la diffusion live (ticket K1)

## Statut
Accepté

> Numéro 0008 : les ADR 0002 à 0007 sont déjà pris par des PR ouvertes
> (playlists, admin, sécurité CI, observabilité, RGPD, smoke test). K1 prend
> le premier numéro libre pour rester mergeable dans n'importe quel ordre.

## Contexte

K1 demande qu'un *broadcaster* diffuse un flux audio en direct vers N
*listeners* simultanés, avec une consommation mémoire maîtrisée, depuis une
app mobile et depuis un navigateur. Il fallait décider : comment transporter
l'audio, comment le distribuer en mémoire, et comment le lire côté Flutter.

## Décisions

### Fan-out en mémoire : un hub par stream, une goroutine par listener

Le `Hub` (`internal/infrastructure/streaming/hub.go`) tient une map
`listenerID -> chan []byte`. Le broadcaster appelle `Publish`, qui recopie le
chunk **une fois** puis le pousse dans chaque canal en `select ... default`.

Trois propriétés en découlent :

1. **`Publish` ne bloque jamais.** Un auditeur lent ne peut pas ralentir le
   diffuseur ni ses pairs — c'est le `default` qui le garantit.
2. **La mémoire est bornée par construction** : `listeners × 256 chunks × ~4 Ko`,
   soit ~1 Mo par auditeur au pire. Rien ne croît sans plafond.
3. **Pas de fuite de goroutine** : fermer le hub ferme tous les canaux, donc
   chaque handler d'écoute sort de sa boucle. Un test le vérifie
   (`TestHub_DoesNotLeakGoroutines`).

**Pourquoi une copie par publication et pas par auditeur** : le handler du
diffuseur réutilise un seul buffer de lecture. Sans copie, un auditeur lent
lirait un tableau déjà réécrit par la lecture suivante. Une copie par publish
(et non par auditeur) rend le chunk immuable pour tout le monde au coût
minimal. Testé par `TestHub_PublishCopiesTheCallersBuffer`.

### Éviction après 64 chunks consécutifs perdus, pas au premier

L'implémentation de référence détachait un auditeur dès le premier chunk
perdu. C'est trop agressif : un pic de latence réseau de quelques centaines de
millisecondes déconnecte un auditeur parfaitement sain.

Ici le compteur de pertes est **consécutif** et remis à zéro à chaque envoi
réussi. Une gigue passagère se résorbe seule ; seul un auditeur réellement
mort (TCP fermé, app suspendue) atteint 64 pertes d'affilée et est évincé pour
libérer sa mémoire.

### Le hub ne connaît ni Prometheus ni les logs

Le hub expose `Stats()` (auditeurs, octets diffusés, chunks perdus, évictions)
et rien d'autre. C'est la couche transport — ou le collecteur Prometheus du
ticket Y3 — qui décide quoi enregistrer.

Conséquence directe : le package se teste avec la seule bibliothèque standard,
et il ne dépend pas du package `observability` introduit par une PR encore non
mergée.

### Registre par processus (limite assumée)

`Registry` indexe les hubs vivants du processus courant. Un déploiement
multi-répliques exigerait donc que les auditeurs atteignent la même réplique
que leur diffuseur (sticky sessions), ou un vrai bus (Redis pub/sub, NATS).

C'est un choix assumé pour l'échelle du projet : la complexité d'un bus
externe n'est pas justifiée ici, et le point de découplage est identifié si
elle le devient — `Registry` est déjà l'unique porte d'entrée.

### Deux chemins de publication : HTTP chunké **et** WebSocket

- `POST /api/v1/streams/{id}/publish` : corps de requête en
  `Transfer-Encoding: chunked`, lu en continu. C'est le chemin des clients
  natifs.
- `GET /api/v1/streams/{id}/publish/ws` : trames binaires WebSocket. **Un
  navigateur ne peut pas streamer un corps de requête HTTP** — c'est la seule
  raison d'être de ce second chemin.

Les deux alimentent le même hub, donc les auditeurs ne voient aucune
différence.

### Le handler `publish` ne répond pas 200 avant la fin

Point non évident, découvert en écrivant le test de bout en bout : un client
HTTP conforme **arrête d'envoyer le corps de la requête dès qu'une réponse
arrive** (`http.Transport` de Go le fait). Acquitter la publication tout de
suite coupait donc la diffusion au premier chunk.

La réponse est désormais le *résultat* de la session (octets diffusés), écrite
une fois la diffusion terminée. Les échecs d'autorisation (401/403/404)
répondent toujours immédiatement — c'est justement ce qui dit à un diffuseur
refusé d'arrêter d'émettre.

### Le JWT en query string, uniquement sur la route WebSocket

L'API WebSocket des navigateurs ne permet pas d'ajouter d'en-tête à la requête
d'upgrade. `middleware.RequireAuthWS` accepte donc le token soit dans
`Authorization`, soit dans `?token=`.

C'est strictement moins bon qu'un en-tête : une URL finit dans les logs
d'accès, les logs de proxy et l'historique du navigateur. La mitigation est le
**confinement** : ce middleware n'est monté que sur cette route, toutes les
autres routes authentifiées restent en en-tête seul. Un test le verrouille
(`TestRequireAuth_DoesNotAcceptAQueryToken`) — s'il casse un jour, c'est que
toute l'API s'est mise à accepter des credentials dans l'URL.

Autres atténuations : tokens à durée de vie courte (24 h), TLS obligatoire en
production, et la valeur n'est jamais journalisée par nos handlers.

### `ListLive` réconcilie la base et le registre

Si l'API redémarre pendant une diffusion, la ligne reste `live` en base alors
que le hub a disparu. `ListLive` filtre donc sur `registry.IsLive`, pour ne
jamais proposer à un auditeur un direct qui renverrait 404 au moment du tap.

### Mobile : un port `AudioEngine` devant `just_audio`

`AudioPlayer` de `just_audio` passe par des method channels : un bloc qui en
dépend n'est testable qu'avec un vrai moteur derrière. Le player dépend donc
d'une interface `AudioEngine` (~10 méthodes), implémentée dans un seul fichier
par `JustAudioEngine`.

Bénéfice concret : **toute la machine à états — y compris la gestion des
interruptions, la partie la plus pénible à reproduire sur un vrai téléphone —
est couverte par des tests unitaires rapides** (appel entrant qui met en
pause, reprise seulement si l'OS le demande, casque débranché). 24 tests, sans
device.

### Un seul objet pour l'arrière-plan et pour l'UI

`StreamAudioHandler` est à la fois un `BaseAudioHandler` (côté OS : écran
verrouillé, notification) et un `AudioEngine` (côté app : le bloc). Une pause
depuis l'écran verrouillé et une pause depuis l'app empruntent donc exactement
le même chemin — il n'y a pas deux machines à états à garder synchronisées.

### Le seek est refusé sur un direct, pas caché

Un direct n'a pas de passé où revenir : `duration` est `null`, `canSeek` est
faux, et la barre est remplacée par un indicateur « Direct ». Le contrôle
reste là où l'utilisateur l'attend et s'explique, au lieu de disparaître.

Le seek reste implémenté et testé (avec bornage aux deux extrémités) : il
servira aux pistes enregistrées du ticket K2.

## Conséquences

- `internal/infrastructure/persistence/stream_repository.go` n'a pas de tests
  unitaires — il faut un vrai Postgres. Même limite que `user_repository.go`,
  déjà notée dans l'ADR 0001 ; à couvrir par testcontainers dans un ticket
  dédié.
- Une nouvelle dépendance backend : `github.com/coder/websocket` (minimale,
  pas de transitives).
- Trois nouvelles dépendances mobile : `just_audio`, `audio_service`,
  `audio_session`, plus la configuration plateforme associée (service et
  permissions Android, `UIBackgroundModes: audio` sur iOS).
- La diffusion multi-répliques est hors périmètre tant que le registre est en
  mémoire (voir plus haut).
