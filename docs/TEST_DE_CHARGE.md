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

## Résultats

Postgres 16 + API en conteneur (`docker compose`), diffusion synthétique à 128 kbit/s, 20 s de connexion.

| Auditeurs | Connectés | TTFB p50 | p95 | p99 | max | Heap au pic | Goroutines | Coût / auditeur | Auditeur le plus lent | Fuite |
|---|---|---|---|---|---|---|---|---|---|---|
| 50 | 50/50 | 54 ms | 99 ms | 104 ms | 106 ms | 11,4 MiB | 122 | 155 KiB | **100 %** | +0 |
| 200 | 200/200 | 59 ms | 104 ms | 108 ms | 108 ms | 20,5 MiB | 422 | 73 KiB | **100 %** | +0 |
| 500 | 500/500 | 59 ms | 114 ms | 197 ms | 257 ms | 30,9 MiB | 1021 | 23 KiB | **100 %** | +0 |
| 1000 | 1000/1000 | 61 ms | 108 ms | 117 ms | 160 ms | 38,5 MiB | 2021 | 12 KiB | **100 %** | +0 |

À 1000 auditeurs, 367 MiB ont été diffusés en 20 s, soit ~18 Mo/s sortants.

## Ce que les chiffres disent

**Aucun auditeur n'est affamé, à aucun palier.** L'auditeur le plus lent reçoit exactement 100 % du débit attendu, de 50 à 1000. C'est la propriété centrale du design — `Publish` non bloquant par `select/default` — et elle tient sous charge, pas seulement dans un test à 25.

**La mémoire ne monte pas linéairement, elle s'amortit.** Le coût par auditeur passe de 155 KiB à 50 auditeurs à 12 KiB à 1000. La raison : les tampons par abonné sont dimensionnés pour le pire cas (256 chunks × ~4 Ko ≈ 1 Mo, cf. ADR 0008), mais un auditeur qui suit le rythme garde son canal quasi vide. **Le pire cas théorique est ~1 Mo par auditeur ; le régime permanent mesuré est 12 KiB.** La différence, c'est la marge dont on dispose avant qu'un réseau dégradé ne la consomme.

**Aucune fuite.** Le delta de goroutines est de +0 aux quatre paliers : chaque goroutine d'abonnement se termine avec sa connexion. C'est ce que le `context.Context` du hub garantit, et c'est vérifié plutôt qu'affirmé.

**La latence reste plate.** Le p50 bouge de 54 à 61 ms entre 50 et 1000 auditeurs. Le p99 monte à 197 ms à 500 puis redescend à 117 ms à 1000 — c'est du bruit de fenêtre de montée en charge, pas une tendance.

**Dimensionnement.** 1000 auditeurs tiennent dans **38,5 MiB de heap**. La machine Fly `shared-cpu-1x` / 512 Mo du déploiement a donc une marge très large côté mémoire ; ce qui limitera en premier est la **bande passante sortante**, pas la RAM — 1000 auditeurs à 128 kbit/s représentent 16 Mo/s soutenus.

## Limites assumées

- **Client et serveur sur la même machine.** Ça mesure le hub, pas le réseau : ni perte, ni latence, ni congestion réelles. Les chiffres sont donc un plafond pour la partie serveur, pas une prédiction de production.
- **Un seul flux.** Le test charge un direct avec N auditeurs, ce que demande le sujet. N directs simultanés avec M auditeurs chacun est un autre profil, non couvert.
- **Le heap « après » dépasse parfois le pic** (41,2 contre 38,5 MiB à 1000). Ce n'est pas une fuite — les goroutines reviennent à leur base — mais un GC qui n'a pas encore tourné : le heap alloué reste réservé au processus.
- **Auditeur lent non simulé.** Tous les clients ici lisent à pleine vitesse. Le seuil d'éviction (64 pertes consécutives) n'est donc pas exercé ; il l'est par les tests unitaires du hub.

## Reproduire

```bash
JWT_SECRET=load-test-secret-at-least-32-characters docker compose up -d --build

cd backend
go run ./cmd/loadtest -listeners 1000 -duration 20s -bitrate 128 -ramp-up 8s
```

Le harnais crée son propre compte diffuseur et son propre direct : aucune donnée à préparer. Sans `/metrics` sur le build visé, il rapporte les mesures client et le dit explicitement plutôt que de prétendre avoir mesuré la mémoire.
