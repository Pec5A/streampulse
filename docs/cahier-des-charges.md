# Cahier des charges — StreamPulse

> Version anglaise : [`cahier-des-charges.en.md`](cahier-des-charges.en.md).
> Projet Semestriel 5A Tech Lead — S2, Bloc 3 RNCP 38822, École .decode.
> Équipe : Yassir Sabbar ([@JASSBR](https://github.com/JASSBR)), KaysZ ([@monkeyDkz](https://github.com/monkeyDkz)), SamyZ ([@SamyNikaia](https://github.com/SamyNikaia)).

---

## 1. Contexte et objectif

L'industrie du streaming en direct exige des infrastructures capables de diffuser un flux vers de nombreux destinataires avec une latence faible et une consommation maîtrisée. StreamPulse est une plateforme de **diffusion audio en temps réel** : un *diffuseur* émet un flux, N *auditeurs* le reçoivent simultanément.

L'objectif pédagogique n'est pas la fonctionnalité seule mais la **façon dont elle est industrialisée** : résilience, observabilité, automatisation. Le projet est adossé au Bloc 3 du RNCP 38822 (« Piloter la mise en production des solutions logicielles et leur évolution »).

## 2. Périmètre

### Dans le périmètre

La diffusion audio en direct est la fonctionnalité centrale ; tout le reste existe pour la rendre utilisable et exploitable.

| Domaine | Contenu |
|---|---|
| Identité | inscription, connexion, rafraîchissement de session, rôles |
| Diffusion | création d'un direct, publication d'un flux, écoute par N auditeurs |
| Contenu | playlists avec file d'attente ordonnée, upload de pistes audio |
| Administration | gestion des rôles, liste des utilisateurs, statistiques globales |
| Conformité | export des données personnelles, suppression de compte |
| Exploitation | métriques, traces, logs corrélés, alertes, tableaux de bord |
| Livraison | intégration et déploiement continus, mise en production |

### Hors périmètre, assumé

- **Transcodage adaptatif** : les octets audio sont multiplexés tels quels, jamais réencodés. Le format est décidé par le diffuseur.
- **Recommandation** : aucun algorithme de suggestion.
- **Diffusion multi-instances** : le hub de diffusion est en mémoire, donc lié à un processus. Passer à plusieurs instances demanderait un bus externe — décision documentée dans l'ADR 0008, pas un oubli.
- **Révocation de jetons** : un JWT reste valide jusqu'à son expiration, y compris après suppression du compte. Sa durée est configurable pour borner cette fenêtre (ADR 0012).
- **Paiement, abonnement, modération de contenu.**

## 3. Rôles et droits

| Rôle | Peut |
|---|---|
| **Anonyme** | consulter la liste des directs, écouter un direct |
| **Utilisateur** | + gérer son compte, créer et ordonner des playlists, exporter ou supprimer ses données |
| **Diffuseur** | + créer un direct, publier un flux, téléverser des pistes |
| **Admin** | + lister les utilisateurs, changer les rôles, consulter les statistiques globales |

L'écoute est **publique par choix** : exiger un compte pour écouter une radio en direct serait une friction sans contrepartie, et cela réduit la surface d'attaque authentifiée.

## 4. Exigences fonctionnelles (user stories)

Chaque story porte ses critères d'acceptation. Les tests qui les vérifient sont nommés dans le [cahier de recette](CAHIER_DE_RECETTE.md).

### 4.1 Identité et compte

**US-01 — En tant que visiteur, je veux créer un compte, afin de pouvoir écouter et créer des playlists.**
- 201 et compte créé ; mot de passe stocké haché (bcrypt), jamais en clair
- 409 si l'email existe déjà, sans créer de second compte
- 400 si le mot de passe est trop court

**US-02 — En tant qu'utilisateur, je veux me connecter, afin de retrouver mes contenus.**
- 200 et jeton exploitable sur les routes protégées
- 401 identique que l'email soit inconnu ou le mot de passe faux : l'API ne doit pas révéler quels comptes existent
- au-delà de 20 tentatives par minute et par IP : 429

**US-03 — En tant qu'utilisateur, je veux prolonger ma session sans ressaisir mes identifiants.**
- 200 et nouveau jeton ; 401 sans jeton valide

### 4.2 Diffusion en direct

**US-04 — En tant que diffuseur, je veux ouvrir un direct, afin que des auditeurs puissent me rejoindre.**
- 201, statut `live`, le direct apparaît dans la liste des directs
- seul le propriétaire peut publier dessus ou l'arrêter

**US-05 — En tant qu'auditeur, je veux écouter un direct sans créer de compte.**
- 200 et flux chunké, sans authentification
- le flux démarre en moins d'une seconde en conditions nominales

**US-06 — En tant qu'exploitant, je veux que N auditeurs reçoivent le même flux sans se gêner.**
- un auditeur lent ne ralentit ni le diffuseur ni ses pairs
- un auditeur réellement mort est évincé après 64 pertes **consécutives**, pas dès la première : une gigue réseau ne doit pas déconnecter un client sain
- **vérifié** : 1000 auditeurs simultanés, l'auditeur le plus lent reçoit 100 % du débit attendu ([test de charge](TEST_DE_CHARGE.md))

**US-07 — En tant que diffuseur sur navigateur, je veux publier depuis une page web.**
- un navigateur ne peut pas streamer un corps de requête HTTP : une route WebSocket dédiée existe pour ce cas

### 4.3 Contenu

**US-08 — En tant qu'utilisateur, je veux organiser mes morceaux en playlists.**
- CRUD complet, restreint au propriétaire (403 sinon)
- une playlist privée n'est pas visible par un tiers

**US-09 — En tant qu'utilisateur, je veux réordonner ma file d'attente.**
- le nouvel ordre est persisté en une transaction
- un ordre qui n'est pas une permutation exacte est rejeté sans changement partiel
- retirer une piste ne laisse pas de trou dans les positions

**US-10 — En tant que diffuseur, je veux téléverser un fichier audio.**
- upload atomique : un envoi interrompu ne laisse ni fichier final ni temporaire
- tout contenu qui n'est pas binaire est rejeté — l'audio n'est jamais du texte
- la lecture supporte les requêtes `Range` (reprise en cours de morceau)

### 4.4 Administration

**US-11 — En tant qu'admin, je veux gérer les rôles et voir l'activité.**
- liste des utilisateurs, changement de rôle, statistiques globales
- 403 pour un utilisateur non-admin, 401 pour un anonyme

**US-12 — En tant qu'utilisateur malvoyant, je veux naviguer l'application au lecteur d'écran.**
- libellés `Semantics`, cibles tactiles ≥ 48 dp, contrastes conformes AA
- politique détaillée : [`accessibility/politique-accessibilite.md`](accessibility/politique-accessibilite.md)

### 4.5 Conformité RGPD

**US-13 — En tant qu'utilisateur, je veux récupérer mes données personnelles.**
- export JSON complet, **sans** le hash de mot de passe — l'absence du champ est structurelle, pas un filtre à l'exécution

**US-14 — En tant qu'utilisateur, je veux supprimer mon compte.**
- 204 et effacement en cascade (playlists, directs, pistes)
- une seconde suppression reste un succès : un rejeu réseau ne doit pas produire d'erreur serveur
- l'identifiant vient **du jeton**, jamais de l'URL : agir sur le compte d'autrui est impossible par construction

### 4.6 Exploitation

**US-15 — En tant qu'exploitant, je veux distinguer une panne technique d'un problème métier.**
- métriques séparées : erreurs 5xx d'un côté, échecs de connexion et déconnexions de l'autre
- panneaux distincts sur le tableau de bord

**US-16 — En tant qu'exploitant, je veux suivre une requête de bout en bout.**
- trace continue de l'application mobile jusqu'à la base
- chaque ligne de log émise dans une requête porte son `trace_id`

**US-17 — En tant qu'exploitant, je veux savoir quelle version tourne.**
- `/health` renvoie la version et le commit du binaire déployé

## 5. Exigences non fonctionnelles

### 5.1 Performance — mesurée, pas estimée

| Exigence | Cible | Mesuré |
|---|---|---|
| Auditeurs simultanés sur un direct | « N » (non chiffré au sujet) | **1000**, sans perte |
| Latence avant le premier octet audio | < 1 s | **p99 = 117 ms** |
| Mémoire sous charge | « minimale » | **38,5 MiB** pour 1000 auditeurs, 12 KiB/auditeur |
| Fuite de ressources | aucune | **delta goroutines +0** après déconnexion |

Détails et limites assumées : [`TEST_DE_CHARGE.md`](TEST_DE_CHARGE.md).

### 5.2 Sécurité — schéma

| Couche | Mesure |
|---|---|
| Transport | HTTPS forcé en production, HSTS hors développement |
| Authentification | JWT signé, expiration configurable ; mots de passe bcrypt |
| Autorisation | identifiant issu du jeton et jamais de l'URL sur les routes personnelles ; propriété vérifiée dans les cas d'usage, pas dans le transport |
| Injection | requêtes paramétrées exclusivement, aucune concaténation SQL |
| Abus | limite de débit par IP sur les routes d'authentification |
| Endpoints sensibles | `/metrics` derrière un jeton comparé en temps constant ; l'API refuse de démarrer en production sans ce jeton |
| Navigateur | CORS en liste blanche exacte, `nosniff`, `X-Frame-Options: DENY`, CSP `default-src 'none'` |
| Chaîne d'approvisionnement | gitleaks, trivy et govulncheck bloquants sur chaque PR |

Justifications et arbitrages : [ADR 0012](adr/0012-hardening-avant-mise-en-production.md).

### 5.3 Qualité

- Couverture de tests **≥ 80 %**, appliquée par la CI (mesure actuelle : 82,6 %)
- `go vet`, `staticcheck`, `gosec` sans avertissement ; `flutter analyze` sans issue
- Détection de course (`-race`) sur toute la suite
- Stratégie détaillée : [`PLAN_DE_TESTS.md`](PLAN_DE_TESTS.md)

### 5.4 Exploitabilité

- Logs **JSON** sur la sortie standard, corrélés aux traces
- Métriques Prometheus, traces OpenTelemetry, tableaux de bord Grafana
- Alertes sur taux d'erreur et latence, chacune avec son runbook FR/EN
- Configuration **exclusivement par variables d'environnement** (12-Factor) : aucune valeur codée en dur

### 5.5 Accessibilité

L'application respecte les critères AA (contrastes, cibles tactiles, lecteur d'écran). La **documentation** l'est aussi : structure par titres, tableaux plutôt que schémas seuls, et chaque diagramme accompagné d'un texte qui en énonce le contenu — un diagramme non décrit est inaccessible à un lecteur d'écran.

## 6. Architecture

**Backend Go** en Clean Architecture : `domain` (aucune dépendance) ← `application` ← `infrastructure` / `transport`. La règle de dépendance est unidirectionnelle et visible sur le [diagramme de composants](diagrams/README.md#1-architecture-en-couches-diagramme-de-composants).

**Mobile Flutter** organisé par fonctionnalité, gestion d'état BLoC, chaque feature portant `bloc/`, `models/`, `repositories/`, `screens/`.

**Diffusion** : un hub pub/sub en mémoire par direct, alimenté par une goroutine et distribuant sur des canaux bufferisés, avec `context.Context` pour l'annulation. C'est ce qui permet la non-famine et l'absence de fuite mesurées au §5.1.

Les décisions structurantes et leurs alternatives écartées sont dans [`adr/`](adr/).

## 7. Modèle de données

Cinq tables : `users`, `playlists`, `playlist_tracks`, `streams`, `tracks`. Toutes les clés étrangères vers `users` sont en `ON DELETE CASCADE` — c'est ce qui rend l'effacement RGPD correct sans que le code de suppression connaisse l'existence de ces tables.

Diagramme entité-association et particularités (notamment la contrainte de position différée pour le réordonnancement) : [diagramme du modèle de données](diagrams/README.md#2-modèle-de-données-entité-association).

## 8. Contraintes

| Contrainte | Origine | Traitement |
|---|---|---|
| Go pour le backend, Flutter pour le mobile | sujet | respecté |
| Clean Architecture / DDD | sujet | règle de dépendance vérifiable sur le diagramme |
| Testabilité unitaire ≥ 80 % | sujet | porte bloquante en CI |
| 12-Factor, zéro *hardcoding* | sujet | toute la configuration par variables d'environnement |
| Conteneurisation multi-stage | sujet | image `alpine`, utilisateur non-root, healthcheck |
| RGPD | réglementaire | export, effacement en cascade, [registre des traitements](rgpd/registre-traitements.md) |
| Commits signés | convention d'équipe | badge « Verified » sur l'ensemble de l'historique |
| Hébergement sans coût | contrainte projet | plan gratuit, avec ses limites documentées dans le runbook |

## 9. Organisation

Découpage en **tranches verticales** : chacun livre une fonctionnalité complète — backend, mobile, tests, instrumentation, ADR — plutôt qu'une couche horizontale. Chacun peut ainsi défendre du code qu'il a personnellement écrit, ce que l'évaluation individuelle du Bloc 3 exige.

| Membre | Tranche |
|---|---|
| 🟦 Yassir | Identité et confiance : authentification, RGPD, observabilité, sécurité, livraison, chat |
| 🟧 KaysZ | Streaming et média : hub de diffusion, lecteur mobile, upload |
| 🟪 SamyZ | Contenu et plateforme : playlists, administration, accessibilité, scans de sécurité |

Règles : une PR par ticket, review croisée, jamais d'auto-merge, CI verte avant fusion. Détail dans [`team/plan.md`](team/plan.md).

## 10. Livrables

| Livrable | Où |
|---|---|
| Code source | ce dépôt |
| Documentation technique FR/EN | [`docs/`](.) |
| Décisions d'architecture | [`docs/adr/`](adr/) |
| Diagrammes standardisés | [`docs/diagrams/`](diagrams/) |
| Plan de tests et cahier de recette | [`PLAN_DE_TESTS.md`](PLAN_DE_TESTS.md), [`CAHIER_DE_RECETTE.md`](CAHIER_DE_RECETTE.md) |
| Plan de formation utilisateurs | [`user-guide/`](user-guide/) |
| Runbooks d'exploitation | [`docs/runbooks/`](runbooks/) |
| Application déployée | voir le runbook de mise en production |

## 11. Limites assumées et suites

Ce que le projet **ne prétend pas** faire, et pourquoi :

- **Une seule instance.** Le hub de diffusion est en mémoire ; la limite de débit aussi. Passer à N instances demande un bus de messages et un limiteur partagé. Les deux sont écrits dans le code, à l'endroit exact où la contrainte s'applique.
- **Mesures sur une seule machine.** Le test de charge fait tourner client et serveur sur le même hôte : les chiffres sont un plafond côté serveur, pas une prédiction de production.
- **Profil « N directs simultanés » non mesuré.** Le sujet demande N auditeurs sur un flux ; c'est ce qui est prouvé. Le coût de N flux est dominé par d'autres facteurs et reste à mesurer.
- **Instrumentation jusqu'à la couche HTTP.** Descendre les traces jusqu'aux requêtes SQL est la suite logique.
