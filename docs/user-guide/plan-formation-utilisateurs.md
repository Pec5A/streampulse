# Plan de formation utilisateurs — StreamPulse

> Périmètre actuel : identité (inscription/connexion) et RGPD (export/suppression de compte), les seules fonctionnalités mergées à ce jour. Ce document sera étendu au fur et à mesure que playlists (S1), streaming live (K1) et diffuseur (K2) arrivent — voir `docs/team/plan.md` pour le calendrier.

## Objectif
Donner à chaque profil d'utilisateur ce dont il a besoin pour utiliser StreamPulse de façon autonome, y compris les personnes en situation de handicap, sans supposer de compétence technique préalable.

## Profils utilisateurs et besoins de formation

| Profil | Ce qu'il/elle doit apprendre | Format |
|---|---|---|
| **Anonyme** | Ce qui est accessible sans compte, comment créer un compte | Écran d'accueil avec CTA clair "Créer un compte" / "Se connecter" |
| **User standard** | Inscription, connexion, gestion de son compte (export/suppression de données) | Parcours in-app guidé au premier lancement (voir ci-dessous) |
| **Diffuseur** | Idem User standard + (à venir : lancer/arrêter un flux, ticket K2) | Idem, complété quand K2 sera mergé |
| **Admin** | Idem User standard + (à venir : gestion des utilisateurs, ticket S2) | Idem, complété quand S2 sera mergé |

## Parcours de formation — inscription/connexion (fonctionnalité actuelle)

1. **Découverte** : écran d'accueil explique en une phrase ce qu'est StreamPulse, avant de demander quoi que ce soit.
2. **Inscription** : formulaire email/username/mot de passe, avec message d'erreur explicite si le mot de passe fait moins de 8 caractères (pas juste "invalide") — voir `auth_handler.go`, message `"email, username and a password of at least 8 characters are required"`.
3. **Connexion** : formulaire email/mot de passe, message générique en cas d'échec ("identifiants invalides") — volontairement sans préciser si c'est l'email ou le mot de passe qui est faux, pour ne pas confirmer à un attaquant qu'un email existe (compromis sécurité/pédagogie assumé, voir ADR 0001).
4. **Gestion du compte (RGPD)** : depuis l'écran profil, deux actions explicites — "Télécharger mes données" (export JSON, article 15) et "Supprimer mon compte" (article 17, action irréversible et immédiate, avec une confirmation avant validation côté mobile).

## Accessibilité — adaptation à la diversité du public

Le module auth/RGPD mobile est construit avec `flutter_bloc` (ADR 0001), ce qui permet de séparer la logique de l'affichage — utile pour les adaptations ci-dessous sans dupliquer le code métier :

- **Déficience visuelle** : tous les champs de formulaire (`login`/`register`) doivent porter un `Semantics label` explicite pour les lecteurs d'écran (VoiceOver/TalkBack) — ex. "Champ email, requis" plutôt que le seul placeholder visuel. Contraste des messages d'erreur conforme WCAG AA (à valider par une passe dédiée, ticket S2 — accessibilité globale de l'app).
- **Déficience motrice** : cibles tactiles ≥ 44×44px sur les boutons "Se connecter"/"S'inscrire"/"Supprimer mon compte" ; navigation clavier complète pour la version web du player (cf. ticket K1, hors périmètre auth).
- **Déficience cognitive** : messages d'erreur en langage simple, une seule action par écran (pas de formulaire d'inscription + connexion sur le même écran), confirmation explicite avant l'action irréversible de suppression de compte.
- **Déficience auditive** : sans objet pour l'auth/RGPD (aucun contenu audio dans ce périmètre) — sera pertinent pour le player audio (K1) : prévoir des indicateurs visuels d'état (lecture/pause/buffering) qui ne dépendent pas du son.

## Ce qui manque encore (honnêteté du périmètre)
- Pas de tutoriel vidéo ou de mode "premier lancement" pas-à-pas implémenté — seulement les écrans eux-mêmes avec labels/erreurs clairs.
- La passe d'accessibilité complète (contrastes réels mesurés, tests avec lecteur d'écran) est le ticket S2, pas encore mergée.
- Ce plan devra être complété avec les parcours streaming (K1), diffuseur (K2), playlists (S1) et admin (S2) au fur et à mesure de leur merge.
