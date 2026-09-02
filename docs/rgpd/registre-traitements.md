# Registre des activités de traitement — StreamPulse (RGPD art. 30)

> **Responsable de traitement** : équipe StreamPulse (projet pédagogique RNCP Bloc 3).
> Ce registre documente les traitements de données à caractère personnel **cibles** de la plateforme. Support **A3.6**.
> Dernière mise à jour : 2026-08-05.

> **Statut de livraison** : l'**auth** et l'**admin** sont sur `main`. Les **playlists** (S1, PR #16) et l'**export/suppression RGPD** (Y2, PR #14) sont **en cours de review**, pas encore mergés — signalés ci-dessous. Le registre décrit le traitement prévu ; il sera confirmé « en production » au fil des merges.

---

## 1. Comptes utilisateurs & authentification *(sur `main`)*

| Élément | Détail |
|---|---|
| **Finalité** | Créer/authentifier un compte ; gérer les rôles (`user` / `broadcaster` / `admin`). |
| **Base légale** | Exécution du service demandé par la personne (art. 6.1.b RGPD). |
| **Personnes concernées** | Utilisateurs inscrits. |
| **Données** | Email, nom d'utilisateur, mot de passe **haché bcrypt** (jamais en clair, jamais renvoyé aux clients), rôle, dates création/màj. |
| **Destinataires** | Aucun tiers. Accès administrateur interne réservé au rôle `admin` (endpoints protégés `RequireAuth` + `RequireAdmin`). |
| **Conservation** | Jusqu'à suppression du compte par la personne (voir §4). |
| **Transferts hors UE** | Aucun. |
| **Sécurité** | bcrypt ; JWT signé HS256 (secret ≥ 32 car., hors code) ; HTTPS ; moindre privilège ; scans CI (gitleaks / trivy / govulncheck). |

## 2. Contenus — playlists & pistes *(S1 — en review, PR #16)*

| Élément | Détail |
|---|---|
| **Finalité** | Permettre à l'utilisateur d'organiser ses playlists et leur file d'attente. |
| **Base légale** | Exécution du service (art. 6.1.b). |
| **Données** | `owner_id` (rattachement à l'utilisateur), nom, description, visibilité ; pistes : titre, artiste, durée, URL source, position. |
| **Conservation** | Supprimées en cascade à la suppression du compte propriétaire (`ON DELETE CASCADE`). |
| **Sécurité** | Contrôle d'ownership sur toute mutation ; playlists privées invisibles aux tiers. |

## 3. Journaux techniques

| Élément | Détail |
|---|---|
| **Finalité** | Exploitation, sécurité, débogage. |
| **Données** | Logs applicatifs (identifiant utilisateur issu du JWT, horodatage, route) — pas d'autres données personnelles. |
| **Conservation** | Durée d'exploitation courte, rotation. |

> _Streaming live & upload de fichiers (tickets K1/K2, PR #22/#23 en cours) : à intégrer à ce registre lorsqu'ils seront livrés (données : flux, fichiers audio, `broadcaster_id`)._

## 4. Droits des personnes

> Les mécanismes d'**export** et d'**effacement** ci-dessous sont fournis par le ticket **Y2** (PR #14, en review) — opérationnels une fois mergé.

| Droit | Mise en œuvre |
|---|---|
| **Accès / portabilité** | Export des données personnelles (fonctionnalité **Y2**). |
| **Effacement** | Suppression du compte avec cascade (**Y2**) → users + playlists/pistes liées. |
| **Rectification** | Modification du profil / des playlists depuis l'app. |
| **Exercice** | Depuis l'application, ou par demande à l'équipe StreamPulse. |

## 5. Principes appliqués
- **Minimisation** : aucune donnée sensible (art. 9) collectée ; strict nécessaire.
- **Sécurité par conception** : mot de passe haché et exclu des réponses ; rôles et endpoints admin protégés ; secrets scannés en CI (voir `docs/runbooks/security-incidents.md`).
- **Traçabilité** : ce registre est versionné avec le code et mis à jour à chaque nouveau traitement.
