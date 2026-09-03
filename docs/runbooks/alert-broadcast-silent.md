# StreamPulseBroadcastSilent

**Sévérité** : warning · **Domaine** : métier · **Règle** : `deployments/prometheus/alerts.yml`

## C'est quoi
Au moins un flux est marqué **en direct** depuis 10 minutes sans qu'un seul octet d'audio ait été publié. Des auditeurs peuvent être connectés — ils écoutent le silence.

C'est l'angle mort classique d'une supervision purement technique : le hub est en parfaite santé, il n'a simplement rien à transmettre. Aucun 5xx, aucune latence, aucune erreur dans les logs. L'application se croit en train de diffuser.

Requête exacte :
```promql
streampulse_active_streams > 0
and
rate(streampulse_broadcast_bytes_total[5m]) == 0
```

## Pourquoi `for: 10m` et pas moins
Le seuil protège contre une confusion : un diffuseur qui garderait la connexion de publication ouverte sans envoyer d'octets produit exactement la même signature qu'un direct mort. Dix minutes est le pari — au-delà, ce n'est plus une hésitation, c'est un direct abandonné.

> Vérifié sur le client actuel : côté mobile, arrêter la diffusion ou changer de piste ferme le corps de la requête de publication, ce qui déclenche `StopLive` et repasse le flux hors ligne. Le client d'aujourd'hui ne produit donc **pas** de direct silencieux volontaire — tout déclenchement de cette alerte est anormal. Le `for` long reste néanmoins justifié : il couvre les stalls réseau transitoires, et un futur client qui maintiendrait la connexion pendant une pause.

## Causes possibles
1. **Upload du diffuseur mort sans fermer la connexion** — le cas le plus courant. TCP n'a pas encore rendu la main, le serveur attend des octets qui ne viendront jamais.
2. **Pause longue** — un diffuseur qui laisse l'antenne ouverte en s'absentant. Bénin, mais visible pour les auditeurs.
3. **Ligne du diffuseur coupée** — mobile passé en tunnel, Wi-Fi tombé. Même signature que le cas 1.
4. **Fuite d'état** — un hub resté ouvert alors que la session de diffusion est terminée. Ce serait un vrai bug côté serveur : `StreamUseCase.StopLive` est appelé dans un `defer` des handlers de publication, il ne devrait pas y avoir de hub orphelin. Si c'est ça, c'est le cas grave.

## Comment vérifier
1. Confirmer que le compteur est réellement plat, et depuis quand :
   ```promql
   increase(streampulse_broadcast_bytes_total[15m])
   ```
   Zéro sur 15 minutes ferme la question.
2. Y a-t-il du monde qui subit le silence ?
   ```promql
   streampulse_active_listeners
   ```
   Zéro auditeur → gênant mais sans impact. Des auditeurs → impact réel.
3. Demander à l'API la liste des directs, et vérifier qu'elle correspond à ce que la métrique dit :
   ```bash
   curl -s localhost:8080/api/v1/streams/live
   ```
   Un flux listé ici depuis longtemps, dont plus personne ne parle → candidat au hub orphelin (cause 4).
4. Les métriques n'ont **aucune étiquette par flux** (ADR 0005) : pour identifier le direct fautif, croiser avec `/api/v1/streams/live` ci-dessus, ou les traces Tempo où l'identifiant de flux est présent.

## Quoi faire
- **Diffuseur injoignable, auditeurs présents** → couper le direct côté API pour libérer les auditeurs plutôt que de les laisser sur du silence : `DELETE /api/v1/streams/{id}` (propriétaire ou admin). `Registry.Close` détache les auditeurs et absorbe les compteurs du hub.
- **Pause assumée** → ne rien faire. Si ça se produit souvent, remonter le `for`.
- **Hub orphelin suspecté (cause 4)** → c'est un bug, pas un incident d'exploitation. Vérifier dans les logs que `StopLive` a bien tourné pour ce flux (`docker compose logs api | grep <stream_id>`), et ouvrir un ticket. Le redémarrage de l'API remet tout à plat (`Registry.CloseAll`) mais efface la preuve — capturer les logs avant.
- **Aucun auditeur, aucune urgence** → noter et surveiller. Cette alerte est neuve, son seuil n'a pas encore vu de trafic réel.
