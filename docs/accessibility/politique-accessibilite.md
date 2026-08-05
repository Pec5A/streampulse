# Politique d'accessibilité — StreamPulse

> Engagement d'accessibilité de l'application mobile StreamPulse. Support **A3.6** (ticket **S2**).
> Référentiel cible : **WCAG 2.1 niveau AA** / RGAA. Dernière mise à jour : 2026-08-05.

## Engagement
Rendre l'application utilisable par tous — y compris avec un lecteur d'écran (TalkBack / VoiceOver), une navigation au clavier ou par commutateur, et sur différentes tailles d'écran (téléphone, tablette).

## Mesures implémentées (S2)

| Axe | Mise en œuvre |
|---|---|
| **Lecteur d'écran** | `Semantics` explicites — ex. cartes de stats admin fusionnées en un seul énoncé (« 42 Utilisateurs ») via `Semantics` + `ExcludeSemantics` ; `ListTile` natifs (titre/sous-titre/action déjà exposés) ; options de dialogue focusables (`SimpleDialogOption` + `Semantics(selected:)`). |
| **Cibles tactiles** | 48 dp minimum via `MaterialTapTargetSize.padded` posé au niveau du **thème** → s'applique à toute l'app, écrans d'auth compris. |
| **Contraste** | Palette Material 3 (`ColorScheme.fromSeed`) : paires de couleurs conformes AA par défaut, en clair comme en sombre. |
| **Responsive** | Helper `core/layout/` (breakpoints, largeur de contenu plafonnée `ContentWidth`, grilles `maxCrossAxisExtent`) → adaptation téléphone / tablette. |
| **Retour utilisateur** | États de chargement explicites (indicateurs) ; erreurs textuelles (SnackBar) ; boutons « Réessayer ». |

## Périmètre
- **Couvert** : écrans admin et playlists (mes tickets S1/S2), + tout l'app via le thème (cibles 48 dp, contraste).
- **À poursuivre** : audit lecteur d'écran écran par écran ; libellés `Semantics` sur 100 % des contrôles interactifs ; tests avec des utilisateurs en situation de handicap ; gestion du redimensionnement de police (`textScaleFactor`).

## Limites connues
Cette politique couvre le travail livré à date ; elle n'a pas encore fait l'objet d'un audit d'accessibilité externe.

## Signalement
Tout problème d'accessibilité peut être remonté via une **issue GitHub** (template `bug`) ou directement à l'équipe StreamPulse.
