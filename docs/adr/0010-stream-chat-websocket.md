# 0010 — Chat texte en direct par stream (ticket Y4, bonus)

## Statut
Accepté

## Contexte
Le cahier des charges liste explicitement en fonctionnalité bonus : *"Websockets : Chat en direct entre les auditeurs d'un même flux"*. Cette fonctionnalité existait déjà dans une itération précédente du projet (`Pec5A/streampulse-reference`, PR #54) — utilisée ici uniquement comme référence pour comprendre l'approche (design du hub, choix d'authentification), pas copiée : le framework HTTP de cette référence (`gin`) est différent de celui de ce repo (stdlib `net/http`), donc l'intégration réelle a dû être réécrite contre le vrai code de K1 (`Hub`/`Registry`, `RequireAuthWS`, `stream_handler.go`), pas contre la référence.

## Décisions

### `ChatHub` calqué sur `Hub`, mais N-vers-N au lieu de 1-vers-N
Le hub audio (`hub.go`, ticket K1) diffuse un flux depuis un diffuseur vers N auditeurs. Le chat a besoin de l'inverse : n'importe quel participant peut publier, et tous les participants (y compris l'émetteur) reçoivent chaque message. `ChatHub` reprend exactement le même design de concurrence (canal par participant, `select ... default` non-bloquant, fermeture via contexte annulé) mais avec une boucle `Publish` qui écrit vers tous les abonnés plutôt qu'un seul émetteur qui écrit et N qui lisent.

### Pas de mécanisme d'éviction, contrairement à `Hub`
`Hub` évince un auditeur après 64 pertes consécutives (ADR 0008) parce qu'un auditeur audio bloqué gaspille de la mémoire indéfiniment tant qu'il n'est pas détecté. Le chat n'a pas ce problème à la même échelle : un participant qui décroche perd au pire `ChatBuffer` (32) petits messages JSON avant que sa connexion WebSocket elle-même ne finisse par expirer côté transport. Ajouter un mécanisme d'éviction dédié n'aurait rien apporté qui ne soit pas déjà couvert par la fermeture normale de connexion.

### Cycle de vie du salon lié à celui du stream, pas géré séparément
`StreamUseCase.StartLive` ouvre maintenant le hub audio **et** le salon de chat ensemble ; `StopLive` et `Delete` ferment les deux. Un salon de chat qui survivrait à la fin du stream (ou l'inverse) serait un état incohérent difficile à diagnostiquer. Garder les deux dans la même méthode garantit qu'aucun futur appelant ne peut ouvrir l'un sans l'autre.

### `JoinChat` n'exige pas la propriété du stream, contrairement à `StartLive`
Publier de l'audio est réservé au diffuseur (ou à un admin) — c'est `authorise()` dans `stream_usecase.go`. Rejoindre le chat est ouvert à n'importe quel utilisateur authentifié, la même ouverture que `Listen` pour l'audio (qui, lui, ne demande même pas de compte). Le chat a seulement besoin d'une identité pour attribuer les messages à quelqu'un — pas d'être propriétaire du stream. `JoinChat` est donc une méthode séparée de `StartLive`/`StopLive`, sans passer par `authorise()`.

### Le pseudo affiché est résolu côté serveur, jamais envoyé par le client
`JoinChat` va chercher le `Username` réel via `UserRepository.FindByID` au moment de la connexion, une fois. Le message entrant du client (`dto.ChatIncoming`) ne contient qu'un champ `text` — aucun champ `username` ou `user_id` n'est jamais lu depuis la frame WebSocket entrante. Un client malveillant ne peut donc pas usurper l'identité affichée d'un autre participant : le pseudo vient de la session authentifiée, jamais de la charge utile.

### JWT en paramètre de requête sur cette route aussi, via `RequireAuthWS`
Même contrainte que `PublishWS` (voir ADR 0008) : un navigateur ne peut pas fixer d'en-tête sur une requête d'upgrade WebSocket. Le chat réutilise `middleware.RequireAuthWS` tel quel, sans dupliquer sa logique — c'est exactement pour ce genre de deuxième cas d'usage que ce middleware a été isolé de `RequireAuth` dans K1 plutôt que d'être fusionné dedans.

### Message trop long : rejeté avec une frame d'erreur, pas tronqué silencieusement
Un message qui dépasse `maxChatMessageLen` (500 caractères, comptés en runes via `utf8.RuneCountInString` pour ne pas pénaliser les caractères multi-octets) reçoit une frame `{"error": "..."}` renvoyée uniquement à l'émetteur, et n'est pas diffusé. La connexion reste ouverte — c'est un rejet du message, pas une erreur fatale du protocole. Tronquer silencieusement (l'approche de la référence) a été écartée : un utilisateur qui voit son message coupé sans le savoir n'a aucun moyen de comprendre pourquoi son interlocuteur ne reçoit qu'une phrase à moitié écrite.

### Portée de ce ticket : backend seulement
L'interface mobile du chat n'est pas incluse ici, cohérent avec la référence elle-même qui documentait déjà ce report ("Interface mobile pas incluse ici — à faire dans un ticket séparé si le temps le permet"). L'issue #10 de ce repo ne mentionne que `ChatHub`/`ChatRegistry` + endpoint WS + tests — le backend est la fonctionnalité livrable et défendable à l'oral en l'état ; l'écran Flutter resterait un ticket bonus distinct si le temps le permet d'ici la soutenance.

## Conséquences
- Le salon de chat n'a pas de compteur Prometheus dédié (contrairement à ce que faisait la référence avec `streampulse_chat_messages_total`) : le package `internal/infrastructure/observability` n'existe pas encore sur la branche K1 au moment de ce ticket (il arrive avec le ticket Y3, PR #15, pas encore mergé dans `main` à ce stade). À ajouter une fois Y3 et K1 réconciliés sur `main`.
- Cette PR est empilée sur K1 (`feat/kaysz-streaming-hub`, PR #22, approuvée mais pas encore mergée) puisque le chat dépend structurellement du hub audio et de son registre. À rebaser sur `main` une fois K1 mergée, suivant la même mécanique que les autres PR empilées de ce projet.
- Comme `Hub`, `ChatRegistry` est un état en mémoire, par processus : un déploiement multi-répliques (ticket K3) devrait router un participant vers la même instance que le reste de son salon — même limitation déjà actée pour l'audio dans l'ADR 0008, pas un nouveau problème introduit ici.
