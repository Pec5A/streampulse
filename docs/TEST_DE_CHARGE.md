# Test de charge — diffusion 1 vers N

> Version anglaise : [`TEST_DE_CHARGE.en.md`](TEST_DE_CHARGE.en.md).
> Harnais : [`backend/cmd/loadtest`](../backend/cmd/loadtest/main.go).
> Comble le manque §4.3 du plan de tests.

## La question posée

Le sujet demande de « prouver que le serveur peut encaisser N auditeurs simultanés avec une consommation mémoire minimale ». Le test bout-en-bout à 25 auditeurs prouve la **correction** — tout le monde reçoit tous les chunks. C'est une autre question. La correction ne dit rien de la latence avant le premier son sous charge, ni de la mémoire que coûte un auditeur de plus, ni du comportement quand un abonné décroche.

## Pourquoi un programme Go et pas k6

L'objet mesuré est un **corps HTTP chunké de longue durée**. Un outil de charge HTTP généraliste mesure des allers-retours requête/réponse ; ici la requête ne se termine jamais, et ce qui compte est le flux d'octets à l'intérieur. Go permet en plus au harnais de lire le `/metrics` du serveur : la mémoire est mesurée **dans le processus testé**, pas devinée de l'extérieur.

## Ce qui est mesuré

| Métrique | Pourquoi elle |
|---|---|
| Auditeurs connectés / demandés | un refus de connexion est le premier symptôme de saturation |
| Temps jusqu'au premier octet audio (p50/p95/p99) | ce qu'un vrai auditeur vit comme « combien de temps avant d'entendre quelque chose » |
| Octets reçus par l'auditeur **le plus lent** | le design affirme qu'un abonné lent ne peut pas être affamé par ses pairs — c'est le chiffre qui le vérifie |
| Heap au **pic** de charge | ce que la machine doit contenir au pire moment, pas en moyenne |
| Delta de goroutines après déconnexion | la vérification de fuite : chaque abonnement doit se terminer avec sa connexion |

## Deux profils, deux questions différentes

Le sujet en pose deux, et elles ne se répondent pas l'une l'autre :

- « prouver que le serveur peut encaisser **N auditeurs simultanés** » → dominé par le coût **par abonné** ;
- « combien nous coûte en CPU le streaming de **100 flux simultanés** ? » → dominé par le coût **fixe par hub**.

Le harnais mesure les deux (`-streams`, `-listeners` par flux).

## Résultats

Postgres 16 + API en conteneur (`docker compose`, build durci), diffusion synthétique à 128 kbit/s, 20 s de connexion. **API redémarrée avant chaque campagne** — sans ça le heap non collecté du run précédent fausse la mesure de départ, au point de produire un coût négatif.

### Profil A — 1 flux, beaucoup d'auditeurs

| Auditeurs | Connectés | TTFB p50 | p99 | Heap au pic | Goroutines | Mémoire / auditeur | CPU soutenu | Auditeur le plus lent | Fuite |
|---|---|---|---|---|---|---|---|---|---|
| 1000 | 1000/1000 | 62 ms | 142 ms | 47,0 MiB | 2022 | 45,9 KiB | **0,31 cœur** | **100 %** | +0 |

### Profil B — beaucoup de flux

| Flux | Auditeurs/flux | TTFB p50 | p99 | Heap au pic | Goroutines | Mémoire / flux | CPU soutenu | CPU / flux | Fuite |
|---|---|---|---|---|---|---|---|---|---|
| 100 | 1 | 39 ms | 47 ms | 22,2 MiB | 320 | 203 KiB | 0,09 cœur | **0,9 mcore** | +0 |
| 100 | 5 (500 au total) | 78 ms | 140 ms | 40,9 MiB | 1121 | 399 KiB | **0,19 cœur** | **1,9 mcore** | +3 |

## La réponse à la question du sujet

> « Combien nous coûte en CPU le streaming de 100 flux simultanés ? »

**0,19 cœur soutenu** — 19 % d'un seul cœur — pour 100 flux et 500 auditeurs, dans 41 MiB de heap. Soit **environ 2 millicores et 400 KiB par flux**.

Autrement dit : une machine à 1 cœur tient ce profil avec 80 % de marge CPU, et la mémoire n'est pas le facteur limitant. Ce qui borne en premier reste la **bande passante sortante** — 500 auditeurs à 128 kbit/s représentent 8 Mo/s soutenus, et 100 flux entrants 1,6 Mo/s de plus.

## Ce que les chiffres disent

**Aucun auditeur n'est affamé, dans aucun profil.** L'auditeur le plus lent reçoit 100 % du débit attendu à 1000 auditeurs sur un flux comme à 500 auditeurs répartis sur 100 flux. C'est la propriété centrale du design — `Publish` non bloquant par `select/default` — et elle tient dans les deux régimes.

**Le coût par flux est faible mais pas nul.** Un hub coûte ~200 KiB et ~1 mcore à vide, ~400 KiB avec 5 auditeurs. À 100 flux, le coût fixe des hubs (20 MiB) devient comparable à celui des abonnés — c'est exactement l'inversion que le profil A ne pouvait pas montrer.

**La latence s'améliore quand la charge se répartit.** TTFB p99 à 47 ms sur 100 flux × 1 auditeur contre 142 ms sur 1 flux × 1000. Un hub qui diffuse à 1 abonné n'a pas de file d'attente à écouler ; à 1000, la boucle de diffusion devient le point chaud.

**Aucune fuite.** Delta de goroutines +0 sur les deux premiers profils. Le +3 du troisième est un artefact de mesure et non une fuite : l'échantillon « avant » a été pris juste après un redémarrage, à 16 goroutines, alors que la base au repos est de 19-20 — les trois goroutines « en trop » sont celles que le serveur crée normalement en sortant de son démarrage à froid.

## Limites assumées

- **Client et serveur sur la même machine.** Ça mesure le hub, pas le réseau : ni perte, ni latence, ni congestion réelles. Les chiffres sont donc un plafond pour la partie serveur, pas une prédiction de production.
- **Pas de montée en charge au-delà de 100 flux.** Le coût par flux est mesuré à 100, pas extrapolé au-delà : la relation pourrait cesser d'être linéaire quand le nombre de goroutines de diffusion dépasse ce que l'ordonnanceur absorbe sans coût.
- **Le heap « après » dépasse parfois le pic** (41,2 contre 38,5 MiB à 1000). Ce n'est pas une fuite — les goroutines reviennent à leur base — mais un GC qui n'a pas encore tourné : le heap alloué reste réservé au processus.
- **Auditeur lent non simulé.** Tous les clients ici lisent à pleine vitesse. Le seuil d'éviction (64 pertes consécutives) n'est donc pas exercé ; il l'est par les tests unitaires du hub.

## Reproduire

```bash
JWT_SECRET=load-test-secret-at-least-32-characters docker compose up -d --build

cd backend
# Profil A — 1 flux, N auditeurs
go run ./cmd/loadtest -streams 1 -listeners 1000 -duration 20s -ramp-up 8s \
  -metrics-token local-scrape-token

# Profil B — 100 flux simultanés
go run ./cmd/loadtest -streams 100 -listeners 5 -duration 20s -ramp-up 5s \
  -metrics-token local-scrape-token
```

**Redémarrer l'API entre deux campagnes** (`docker compose restart api`) : le heap non collecté du run précédent fausse l'échantillon de départ. Le harnais refuse d'ailleurs d'afficher un coût quand le delta est négatif, plutôt que d'imprimer un chiffre faux et crédible.

Le harnais crée son propre compte diffuseur et son propre direct : aucune donnée à préparer. Sans `/metrics` sur le build visé, il rapporte les mesures client et le dit explicitement plutôt que de prétendre avoir mesuré la mémoire.
