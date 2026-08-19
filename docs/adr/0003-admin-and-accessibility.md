# 0003 — Console admin et accessibilité (ticket S2)

## Statut
Accepté

## Contexte
S2 ajoute une console d'administration (rôles, liste des utilisateurs, statistiques) réservée aux admins, et demande une **passe d'accessibilité** sur l'app + un layout **responsive**.

## Décisions

### Contrôle d'accès : middleware `RequireAdmin` chaîné après `RequireAuth`
Plutôt que de re-vérifier le rôle dans chaque handler, un middleware `RequireAdmin` lit le rôle injecté dans le contexte par `RequireAuth` et renvoie `403` si ≠ `admin`. Chaînage : `RequireAuth → RequireAdmin → handler`. → Gate centralisée, testable isolément, réutilisable pour toute future route admin.

### Repository : extension du repo user sans toucher au ticket auth
Les requêtes admin (`ListUsers`, `CountUsersByRole`) sont ajoutées au type `*persistence.UserRepository` dans un **fichier séparé** (`user_admin_queries.go`) + une interface `AdminUserRepository` propre à S2, plutôt que d'étendre l'interface `UserRepository` de Y1 — ce qui aurait cassé le fake de test de l'auth. L'usecase admin dépend de la nouvelle interface ; le type pgx la satisfait structurellement.

### Stats scopées aux users
Le snapshot ne compte que la table `users` (total + par rôle) — la seule disponible dans cette tranche. Extensible (streams, playlists…) quand les autres tickets seront mergés. Honnête et sans dépendance.

### Accessibilité
- **Semantics** : cartes de stats fusionnées en un seul nœud lisible (« 42 Utilisateurs ») via `Semantics` + `ExcludeSemantics` sur le visuel décoratif ; `ListTile` natifs pour la liste d'utilisateurs (déjà exposés au lecteur d'écran) ; dialogue de rôle en `SimpleDialogOption` (focusable) + `Semantics(selected:)`.
- **Cibles tactiles** : `MaterialTapTargetSize.padded` (48 dp) au niveau du **thème** → s'applique à toute l'app, y compris les écrans d'auth existants, sans les modifier.
- **Responsive** : helper `core/layout/` (`Breakpoints`, `ContentWidth`, `context.isTablet`) ; grille de stats en `maxCrossAxisExtent` (le nombre de colonnes s'adapte) + largeur de contenu plafonnée sur tablette.
- **Contraste** : Material 3 (`ColorScheme.fromSeed`) fournit des paires de couleurs conformes par défaut.

### Mobile : `flutter_bloc` (cf. ADR 0001)
`AdminBloc` charge stats + users et gère le changement de rôle (recharge après succès, message d'erreur sinon).

## Conséquences
- La console admin n'est atteignable depuis l'accueil que pour les utilisateurs de rôle `admin` (défense en profondeur : l'UI masque l'entrée, le backend l'impose via `RequireAdmin`).
- La passe a11y portée par le thème profite aux écrans existants sans toucher aux fichiers d'autres tickets.
- Le **registre RGPD** (autre volet de A3.6) reste à produire en phase documentation (S-doc).
