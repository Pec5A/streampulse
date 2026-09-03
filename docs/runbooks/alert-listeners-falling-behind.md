# StreamPulseListenersFallingBehind

**Sévérité** : warning · **Domaine** : métier · **Règle** : `deployments/prometheus/alerts.yml`

## C'est quoi
Plus de 3 auditeurs ont été déconnectés en 10 minutes parce qu'ils n'arrivaient pas à suivre le rythme de la diffusion, et ça dure depuis au moins 5 minutes.

C'est une alerte **métier**, et c'est tout l'intérêt : côté technique, rien ne bouge. Un auditeur évincé voit sa requête `GET /api/v1/streams/{id}/listen` se terminer normalement, avec un 200. Ni le taux de 5xx ni la latence HTTP ne réagissent. Le seul endroit où ça se voit, c'est ici.

Requête exacte :
```promql
sum(increase(streampulse_listener_evictions_total[10m])) > 3
```

Le seuil est un **compte**, pas un taux. Un auditeur lent sur une connexion pourrie, c'est la vie ; une poignée en dix minutes, c'est le hub qui lâche une audience qu'il n'arrive plus à servir.

## Le mécanisme, en deux phrases
Chaque auditeur a une file d'attente bornée (`Hub.Subscribe`). Quand un chunk arrive et que sa file est pleine, le chunk est jeté et `streampulse_listener_chunks_dropped_total` monte. Au bout de `maxDrops` chunks jetés d'affilée, l'auditeur est détaché et `streampulse_listener_evictions_total` monte. Les chutes précèdent donc toujours les évictions — c'est le signal avancé.

## Causes possibles
1. **Le débit du diffuseur dépasse ce que l'audience absorbe** — un fichier à haut bitrate mis à l'antenne. Le panneau « Débit de diffusion » le montre directement.
2. **L'hôte de l'API est saturé** — le fan-out ne tourne plus assez vite, tout le monde prend du retard en même temps (regarder si les évictions arrivent en rafale plutôt qu'étalées).
3. **Un seul auditeur pathologique** — mobile en 3G qui se reconnecte en boucle. Peut à lui seul dépasser le seuil : vérifier le volume avant de conclure à une panne.
4. **Faible volume** — en démo, 4 évictions ne veulent pas dire grand-chose. Comme pour l'alerte auth, lire le seuil à l'aune du trafic réel.

## Comment vérifier
1. Regarder si les chutes précèdent les évictions (mécanisme normal) ou si les évictions arrivent seules (plus suspect) :
   ```promql
   rate(streampulse_listener_chunks_dropped_total[5m])
   rate(streampulse_listener_evictions_total[5m])
   ```
2. Rapporter à l'audience réelle — 3 évictions sur 4 auditeurs et 3 sur 400, ce n'est pas la même histoire :
   ```promql
   streampulse_active_listeners
   ```
3. Regarder le débit au même instant :
   ```promql
   rate(streampulse_broadcast_bytes_total[1m])
   ```
   S'il a bondi juste avant les évictions → cause n°1.
4. Les métriques ne portent **aucune étiquette par flux** (choix assumé, cardinalité — voir ADR 0005). Pour savoir *quel* direct est en cause, passer par les traces dans Tempo : l'identifiant de flux y est disponible, gratuitement.

## Quoi faire
- **Débit anormalement haut** → c'est le diffuseur, pas l'infra. Le contenu mis à l'antenne est trop lourd ; côté produit, c'est un sujet de limite de bitrate à l'upload (pas encore implémenté, dette connue).
- **Évictions en rafale sur tous les auditeurs** → regarder la charge de l'hôte (`docker stats`, `docker compose logs api`). Le fan-out est dans le chemin chaud de `Hub.Publish` ; s'il ralentit, tout le monde tombe ensemble.
- **Un seul auditeur, volume faible** → faux positif, ne rien faire. Si ça se répète, le seuil de 3 est probablement mal calibré pour ce niveau de trafic.
- **Rien d'anormal ailleurs** → noter dans le canal d'équipe. Cette alerte est neuve ; son seuil n'a pas encore été confronté à du trafic réel.
