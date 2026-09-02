# 0011 — Traces OpenTelemetry et logs JSON corrélés (ticket Y3, suite)

## Statut
Accepté

## Contexte
Deux exigences explicites du sujet n'étaient couvertes par aucune branche du dépôt :

- « Instrumentation du code pour générer des **traces distribuées**. Un étudiant doit pouvoir suivre le cheminement d'une requête de l'application mobile jusqu'à la base de données. »
- « **Abandon du format texte simple** au profit de logs structurés permettant une analyse automatisée (via Loki ou Elasticsearch). »

L'ADR 0005 avait mis en place les métriques Prometheus et le dashboard. Il manquait donc deux des trois signaux d'observabilité, et surtout le lien entre eux : une métrique dit *qu'il y a* un problème, une trace dit *où*, un log dit *quoi*. Sans corrélation, on a trois outils et aucune enquête.

## Décision

### 1. Traces : OTLP vers un collecteur, jamais vers un backend directement
`observability.InitTracing` installe un `TracerProvider` qui exporte en OTLP/gRPC vers un **OpenTelemetry Collector**, lui-même exportant vers **Tempo**. Le backend ne connaît donc qu'un endpoint et un protocole ; changer Tempo pour Jaeger, ajouter de l'échantillonnage ou dupliquer vers un second backend est une modification de `deployments/otel-collector/config.yaml`, pas un redéploiement de l'API.

Trois choix méritent d'être justifiés :

- **Le propagateur W3C est installé même quand le tracing est désactivé.** C'est lui qui lit l'en-tête `traceparent`. Sans lui, une requête déjà tracée en amont (l'app Flutter) arriverait ici comme une trace neuve et sans lien — la trace serait cassée précisément chez celui qui l'a désactivée par commodité.
- **Échantillonnage `ParentBased`.** Si la décision d'échantillonner a été prise en amont, on l'honore. Un ratio décidé indépendamment à chaque étage produit des trous au milieu de la trace qu'on est en train de suivre.
- **`shutdown` obligatoire et avec son propre contexte.** Le *batch span processor* garde les spans terminés en mémoire jusqu'au prochain envoi ; tuer le process sans flush jette les spans des dernières secondes — exactement celles qu'on veut après un incident. Le contexte de flush n'est pas dérivé de celui de l'arrêt, sinon il est déjà annulé au moment où on s'en sert.

### 2. Le nom du span est le **template de route**, jamais l'URL
`GET /api/v1/playlists/{id}`, pas `/api/v1/playlists/3f2b…`. Même raison que le label Prometheus : une URL brute crée une opération distincte par identifiant jamais ouvert, ce qui rend les traces ingroupables et fait exploser la cardinalité côté backend de traces. Les requêtes qui ne matchent aucune route retombent sur le libellé constant `<unmatched>`, pour la même raison de cardinalité bornée que dans `middleware.Metrics`.

Seules les **5xx** marquent le span en erreur. Un 401 ou un 404 est le serveur qui fonctionne ; les compter comme des erreurs ferait suivre au taux d'erreur le comportement des clients au lieu de la santé du service — c'est-à-dire la confusion métier/technique que l'ADR 0005 existe pour éviter.

### 3. Logs : JSON partout, avec `trace_id` à la racine
`observability.NewLogger` renvoie un `slog` en handler JSON, **y compris en développement** : un format exercé uniquement en production est un format découvert cassé en production. Seul le niveau change (debug en local, info ailleurs).

Le handler enveloppe le handler JSON pour estampiller chaque ligne émise dans un span avec `trace_id` et `span_id`. C'est ce qui joint les trois signaux : un pic de latence sur un panel Grafana → ouverture de la trace → le même `trace_id` collé dans Loki rend exactement les lignes de log de cette requête.

## Conséquences
- **Contrainte connue et testée** : `slog` imbrique sous le groupe ouvert tout attribut ajouté pendant `Handle`, identifiants de corrélation compris. Un logger construit avec `WithGroup` émettrait `http.trace_id` au lieu d'un `trace_id` racine, que le lien log→trace de Grafana ne trouverait pas. L'API ne groupe donc jamais sur le logger racine, et `TestNewLogger_GroupNestsCorrelationIdsAwayFromTheRoot` verrouille ce comportement : le changer demande une décision délibérée avec un test rouge, pas un accident invisible jusqu'au prochain incident.
- Le tracing est **désactivé par défaut** (`OTEL_EXPORTER_OTLP_ENDPOINT` vide). L'API doit rester démarrable avec un simple Postgres ; rendre le collecteur obligatoire ferait de la stack d'observabilité un prérequis pour tout développeur.
- Un ratio d'échantillonnage invalide fait **échouer le démarrage** plutôt que de dégrader silencieusement à 0 : un service en apparence sain qui ne produit aucune trace ne se découvre que le jour où on en a besoin.
- La chaîne `docker compose` gagne deux services (`otel-collector`, `tempo`) et une datasource Grafana. L'uid de la datasource Prometheus est désormais épinglé — sans ça Grafana en génère un aléatoire et le lien trace→métrique de Tempo cesse de résoudre au premier `docker compose up` suivant.
- L'instrumentation s'arrête à la couche HTTP. Descendre les spans jusqu'aux requêtes SQL (`otelpgx`) est la suite logique et n'est pas faite ici.
