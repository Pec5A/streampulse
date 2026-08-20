# Politique d'accessibilité — StreamPulse

> Engagement d'accessibilité de l'application mobile StreamPulse. Support **A3.6** (ticket **S2**).
> Référentiel **visé** : WCAG 2.1 niveau AA / RGAA (non encore audité — voir Limites connues). Dernière mise à jour : 2026-08-05.

## Engagement
Rendre l'application utilisable par tous — y compris avec un lecteur d'écran (TalkBack / VoiceOver), une navigation au clavier ou par commutateur, et sur différentes tailles d'écran (téléphone, tablette).

## Mesures implémentées (S2)

| Axe | Mise en œuvre |
|---|---|
| **Lecteur d'écran** | `Semantics` explicites — ex. cartes de stats admin fusionnées en un seul énoncé (« 42 Utilisateurs ») via `Semantics` + `ExcludeSemantics` ; `ListTile` natifs (titre/sous-titre/action déjà exposés) ; options de dialogue focusables (`SimpleDialogOption` + `Semantics(selected:)`). |
| **Cibles tactiles** | `MaterialTapTargetSize.padded` au niveau du **thème** → cibles d'au moins 48 dp pour les composants Material standard. Ne contraint pas les widgets custom, à vérifier au cas par cas. |
| **Contraste** | Palette Material 3 (`ColorScheme.fromSeed`) : **vise** des paires conformes AA (défaut Material 3, en clair comme en sombre) — non vérifié par un audit formel. |
| **Responsive** | Helper `core/layout/` (breakpoints, largeur de contenu plafonnée `ContentWidth`, grilles `maxCrossAxisExtent`) → adaptation téléphone / tablette. |
| **Retour utilisateur** | États de chargement explicites (indicateurs) ; erreurs textuelles (SnackBar) ; boutons « Réessayer ». |

## Périmètre
- **Couvert** : écran admin (S2, sur `main`) ; l'écran playlists suit la même approche (S1, en review — PR #16). Les réglages de thème (cibles tactiles, contraste) s'appliquent à toute l'app.
- **À poursuivre** : audit lecteur d'écran écran par écran ; libellés `Semantics` sur 100 % des contrôles interactifs ; contraintes de taille sur les widgets custom ; tests avec des utilisateurs en situation de handicap ; gestion du redimensionnement de police (`textScaleFactor`).

## Limites connues
Cette politique couvre le travail livré à date ; les objectifs WCAG 2.1 AA sont **visés** mais n'ont pas encore fait l'objet d'un audit d'accessibilité externe.

## Signalement
Tout problème d'accessibilité peut être remonté via une **issue GitHub** (template `bug`) ou directement à l'équipe StreamPulse.
