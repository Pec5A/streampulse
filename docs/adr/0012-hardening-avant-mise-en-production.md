# 0012 — Durcissement avant mise en production (ticket Y5)

## Statut
Accepté

## Contexte
La chaîne de déploiement (ticket K3) rend le service **publiquement joignable**. Une revue menée juste avant de l'activer a trouvé quatre écarts avec les exigences explicites du sujet — « sécurité des flux (TLS), protection contre l'injection et **sécurisation des endpoints sensibles (admin/metrics)** » et « zéro *hardcoding* » :

1. **Aucun CORS**, alors que la CD publie un artefact `flutter build web`. Une page servie depuis une autre origine voit chacune de ses requêtes bloquée par le navigateur : l'artefact est livré mais inutilisable.
2. **Aucune limite de débit**, nulle part. `POST /api/v1/auth/login` accepte un nombre illimité de tentatives ; bcrypt rend chaque essai lent, rien ne les rendait rares.
3. **`/metrics` ouvert.** Le commentaire dans le code le justifiait ainsi : *« acceptable for now since nothing here is deployed publicly yet (ticket K3) »*. Le ticket K3 est précisément celui qui déploie — la justification expirait au merge suivant.
4. **`JWTExpiration` codée en dur** à 24 h, dans le fichier dont l'en-tête affirme que rien ne l'est.

## Décision

### CORS : liste blanche exacte, jamais de reflet
L'origine reçue est comparée à une liste configurée par `CORS_ALLOWED_ORIGINS`. Elle n'est **jamais renvoyée telle quelle**. Refléter l'`Origin` est la façon classique d'obtenir une API qui fait confiance à tout site du web : c'est silencieusement équivalent à `*`, sauf que — contrairement à `*` — ça fonctionne aussi avec des identifiants, ce qui est exactement ce qui le rend dangereux.

Liste vide = **aucun en-tête CORS**. C'est le bon défaut pour une API dont le seul client est une application native : l'accès navigateur est un choix explicite, jamais implicite.

Le préflight est répondu par le middleware et non transmis : le mux renverrait `405` sur un `OPTIONS` d'une route déclarée `POST`, et le navigateur lirait ce 405 comme un refus.

### Limite de débit : en mémoire, par IP, assumée comme telle
20 requêtes/minute par IP sur `/auth/*` (`AUTH_RATE_LIMIT_PER_MINUTE`). Délibérément **en mémoire et par instance** : un limiteur partagé (Redis) est la bonne réponse dès qu'il y a plus d'une machine — avec N instances la limite effective est N fois celle-ci. Choisir la version simple maintenant est un arbitrage, pas un oubli : ça supprime l'attaque triviale aujourd'hui sans ajouter une dépendance externe dont le projet n'a pas besoin par ailleurs.

**L'identification du client passe par `TRUSTED_PROXY_HOPS`**, et les deux options naïves sont fausses en sens inverse :

- *Toujours utiliser `RemoteAddr`.* Derrière un proxy qui termine TLS — ce que fait tout PaaS — cette adresse est celle du proxy, identique pour tout le monde. Le quota s'applique alors à **tous les utilisateurs réunis** : avec une limite de 20/min, la 21e tentative de connexion de la minute échoue quel que soit son auteur. Une première version de cet ADR affirmait que c'était « conservateur » et que « ça ne sous-compte personne ». C'était l'inverse : mettre tous les clients dans un seul seau les **sur-compte** chacun, et transforme la protection en déni de service auto-infligé dès que quelques personnes se connectent en même temps. Défaut relevé par @monkeyDkz en revue.
- *Toujours faire confiance à `X-Forwarded-For`.* N'importe quel client peut envoyer cet en-tête, donc un attaquant se fabrique un quota neuf à chaque requête en le faisant varier. Le limiteur devient décoratif.

**Compter depuis la droite** est ce qui rend l'en-tête utilisable : un attaquant peut préfixer des entrées, mais ne peut pas supprimer celles que l'infrastructure ajoute après les siennes. Sauter exactement `TRUSTED_PROXY_HOPS` entrées depuis la fin tombe donc sur l'adresse que le proxy de confiance le plus proche a réellement observée. Le nombre de sauts est une donnée de déploiement — 0 en local, 1 derrière un load balancer de PaaS — et vaut 0 par défaut : un quota partagé est mauvais, un quota falsifiable est pire.

La table est balayée à chaque requête pour oublier les clients silencieux. Sans ça elle grandit d'une entrée par IP distincte ayant jamais touché le service et ne rétrécit jamais — la même forme de croissance non bornée que le problème de cardinalité des labels Prometheus, atteinte par un autre chemin.

### `/metrics` : jeton porteur, et refus de démarrer sans lui
`/metrics` n'est pas un endpoint anodin : il publie la table des routes, les volumes par route, les distributions de latence et le profil mémoire du processus. C'est une carte de l'application remise à qui la demande.

Protégé par un jeton (`METRICS_TOKEN`), comparé en **temps constant** — une comparaison octet par octet livre le jeton un caractère à la fois à qui accepte de mesurer. La réponse en cas d'échec est **404 et non 401** : un scanner non authentifié n'apprend rien, pas même que l'endpoint existe.

`config.Load` **refuse de démarrer** si `ENVIRONMENT` n'est pas `development` et que le jeton est absent. Échouer bruyamment au boot est le seul moyen que ça ne puisse pas être oublié le jour où ça commence à compter — c'est-à-dire le jour de la mise en ligne.

### En-têtes de sécurité
`nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, et une CSP `default-src 'none'` — l'API ne renvoie que du JSON, donc interdire toute source est exact et pas seulement restrictif.

HSTS **uniquement hors développement** : envoyé depuis un serveur local en HTTP simple, il épinglerait `localhost` en HTTPS dans le navigateur du développeur, pour un an, et pour tous les projets utilisant ce port.

### `JWT_EXPIRATION` par variable d'environnement
Défaut 24 h, validé (durée Go, strictement positive). Au-delà du 12-Factor, ce paramètre a une conséquence de sécurité : c'est la durée pendant laquelle un jeton reste utilisable **après** la suppression du compte qu'il désigne — un opérateur doit pouvoir la raccourcir sans reconstruire l'image.

## Conséquences
- **La stack locale exerce le chemin de production.** `METRICS_TOKEN` est posé dans le compose et Prometheus le présente, alors que le développement le tolérerait vide. Un garde-fou prouvé uniquement par des tests unitaires est un garde-fou découvert cassé en production.
- **Piège Prometheus** : sa configuration **n'interpole pas** les variables d'environnement. Un `${METRICS_TOKEN}` y serait envoyé littéralement et chaque scrape renverrait 404 — en silence, puisqu'un scrape en échec ne se voit que comme un trou dans les courbes. La valeur locale est donc littérale et commentée comme telle ; un déploiement fournit la sienne via `credentials_file`.
- **Couplage avec le déploiement** : avec `ENVIRONMENT=production`, l'API **ne démarre pas** sans `METRICS_TOKEN`. Le secret doit exister avant le premier déploiement, sinon le check de santé échoue et Fly conserve la version précédente — comportement voulu, mais qui doit figurer dans le runbook.
- **Non traité** : révocation de jeton (une liste de révocation demanderait un stockage partagé), limite de débit distribuée, et jeton d'admin distinct de celui des métriques.

## Vérifié
Stack complète démarrée, résultats bruts :

```
/metrics sans token          404
/metrics avec token          200 (66 séries)
en-têtes                     nosniff, DENY, no-referrer, CSP présents
CORS origine autorisée       Allow-Origin + Vary: Origin
CORS origine refusée         0 en-tête CORS
rate limit /auth/login       20 x 401 puis 429
cible Prometheus             health=up
```
