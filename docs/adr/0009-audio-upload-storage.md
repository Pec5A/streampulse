# 0009 — Upload et stockage des fichiers audio (ticket K2)

## Statut
Accepté

## Contexte

K2 demande l'upload de fichiers audio avec « stockage local, interface prête
pour S3, validation taille/type », et une interface diffuseur côté mobile.
Trois questions à trancher : où vivent les octets, à quoi fait-on confiance
dans une requête d'upload, et ce que « démarrer un stream » veut dire dans
l'app.

## Décisions

### Un port `Storage` de quatre méthodes, pas un chemin de fichier

`internal/domain/storage` définit `Save` / `Open` / `Delete` / `Exists`.
Aucune signature ne laisse fuiter un `os.File` ni un chemin : passer à S3
revient à écrire un second type qui satisfait l'interface, sans toucher un
seul use case.

`Open` renvoie un `io.ReadSeekCloser` et non un simple `io.ReadCloser`. Ce
n'est pas gratuit : c'est ce qui permet à la couche HTTP de répondre aux
requêtes `Range` via `http.ServeContent`, donc au lecteur mobile de **se
déplacer dans une piste** au lieu de la retélécharger depuis le début. C'est
le pendant direct du seek implémenté en K1, qui n'avait jusqu'ici aucune
source à laquelle s'appliquer (un direct n'a pas de passé). Une implémentation
S3 satisfait le même contrat avec des GET ranged.

### Écriture temporaire puis `rename`, pas d'écriture directe

`Local.Save` écrit dans un fichier temporaire, `fsync`, puis `rename` — qui
est atomique sur un même système de fichiers. Une connexion coupée en plein
upload ne laisse donc **aucun objet partiel** : un fichier à moitié écrit
qu'un auditeur pourrait streamer comme s'il était complet est pire que pas de
fichier du tout. Testé par `TestLocal_FailedUploadLeavesNoObject`.

### La clé de stockage est un UUID généré côté serveur

Le nom de fichier envoyé par le client n'est **jamais** utilisé pour
construire un chemin : il est conservé uniquement pour l'affichage et pour
l'en-tête `Content-Disposition`. La clé est un UUID plus l'extension déduite
du type déclaré.

`Local.resolve` refuse par ailleurs structurellement toute clé qui n'est pas
un nom simple d'un seul segment (pas d'absolu, pas de `/` ni `\`, pas de `..`,
pas d'octet nul), puis revérifie que le chemin résolu est bien sous la
racine. Les clés sont générées côté serveur aujourd'hui, mais une frontière de
sécurité ne doit pas dépendre de la bonne foi de l'appelant.

### On ne fait confiance ni au type déclaré, ni à la taille déclarée

**Le type déclaré ne sert qu'à choisir l'extension.** Il est validé contre une
liste d'audio acceptés, puis les 512 premiers octets réels sont reniflés.

Mais — et c'est le point non évident — **on renifle pour *rejeter*, pas pour
*autoriser***. `http.DetectContentType` ne connaît qu'une poignée de
conteneurs : un MP3 sans tag ID3, un AAC ou un FLAC ressortent tous en
`application/octet-stream`. Un allowlist sur le reniflage rejetterait donc des
fichiers audio parfaitement valides. Ce qui compte réellement pour la sécurité,
c'est que le contenu ne soit pas du **contenu actif** (HTML, SVG, XML, JS,
PDF) — et c'est précisément ce que le renifleur détecte de façon fiable.

Défense en profondeur côté service : l'audio est renvoyé avec le type validé à
l'upload (jamais un type reniflé), plus `X-Content-Type-Options: nosniff` et
une CSP `default-src 'none'; sandbox`. Même si un fichier hostile passait, le
navigateur ne l'exécuterait pas.

**La taille est comptée pendant le streaming**, jamais lue dans un
`Content-Length` : un en-tête est une déclaration, pas un fait. Double
barrière : `http.MaxBytesReader` coupe au niveau transport pendant que le
corps arrive, et le use case compte les octets réellement écrits.

### La ligne est la source de vérité, l'objet suit

- Si l'insertion en base échoue après l'écriture, **l'objet est supprimé** :
  un objet sans ligne est un déchet que plus rien ne référencera jamais.
- À la suppression, **la ligne part en premier** : en cas d'échec sur l'objet
  on se retrouve avec un fichier orphelin (récupérable, invisible des
  utilisateurs) plutôt qu'une ligne pointant dans le vide, qui casserait
  l'affichage de tout le catalogue.
- Une ligne dont l'objet a disparu est servie en **404, pas en 500** : c'est
  la réponse honnête, ce n'est pas une erreur serveur.

### Pas d'URL publique en base

Le repo de référence stockait une `file_url` en base. Cela grave le nom d'hôte
dans les données : changer d'environnement, ou passer derrière un CDN,
obligerait à réécrire les lignes. On stocke la clé opaque et l'`audio_url` est
construite au moment de la lecture. La clé, elle, n'est jamais exposée dans les
réponses.

### Mobile : « diffuser » veut dire diffuser une piste, pas capter le micro

Le ticket demande « démarrer/arrêter un stream, uploader un track » — pas la
capture micro. Le diffuseur choisit donc une de ses pistes et la met à
l'antenne, comme une webradio. La capture micro reste une évolution naturelle
(K4) : elle se brancherait sur le même `BroadcastTransport` en changeant
uniquement la source.

**Le flux est cadencé** (`pacedSource`). Sans ça, le fichier partirait dans la
socket aussi vite que le réseau l'accepte : un morceau de quatre minutes serait
terminé en deux secondes et un auditeur arrivant juste après ne trouverait
plus rien. La cadence est une approximation assumée (16 Kio/s ≈ 128 kbps) ;
un vrai calage lirait le bitrate du conteneur.

### Chaque dépendance plateforme derrière un port

`AudioFilePicker` devant `file_picker`, `BroadcastTransport` devant le POST
chunké — même raisonnement qu'`AudioEngine` en K1. Résultat : la machine à
états du diffuseur, y compris « la source audio est introuvable, est-ce qu'on
a bien coupé la diffusion ? », est couverte par des tests unitaires sans
device ni serveur.

## Conséquences

- Nouvelle variable d'environnement `STORAGE_PATH` (défaut `./uploads`).
- Le stockage local suppose **un seul nœud** : deux répliques ne partageraient
  pas leurs fichiers. Même limite que le registre de streams en K1, et même
  point de découplage — c'est exactement ce que l'implémentation S3 résoudra,
  à brancher en K3 si le déploiement passe à plusieurs répliques.
- `tracks.uploader_id` est en `ON DELETE CASCADE` : la suppression de compte
  RGPD (Y2) enlève les lignes sans que Y2 connaisse cette table. **Les objets
  stockés ne sont pas supprimés par ce cascade** — il faudra soit un balayage
  périodique des orphelins, soit un hook applicatif dans la suppression de
  compte. À trancher avec Yassir quand nos deux tickets seront sur `main`.
- Deux nouvelles dépendances mobile : `file_picker`, `http_parser`.
- `internal/infrastructure/persistence/track_repository.go` n'a pas de tests
  unitaires (il faut un vrai Postgres) — même limite que les autres
  repositories, déjà actée dans les ADR 0001 et 0008.
