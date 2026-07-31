# Setup & Workflow — Guide pour KaysZ et SamyZ

> Ce guide explique comment **vous** (Kays, Samy) clonez le repo, configurez votre identité Git, signez vos commits, et ouvrez vos PRs.
> **Important** : Yassir vous a ajoutés comme collaborators. Vos commits doivent partir de **vos machines** avec **votre** identité — sinon ils ne comptent pas pour vous au RNCP.

---

## 1. Pré-requis (une seule fois)

### Outils
```bash
# macOS
brew install git gh go flutter gpg

# Linux (Debian/Ubuntu)
sudo apt install git gnupg
# Go 1.26+ : https://go.dev/dl/
# Flutter : https://docs.flutter.dev/get-started/install
# gh : https://cli.github.com/
```

### Authentification GitHub CLI
```bash
gh auth login
# Choisir : GitHub.com → HTTPS → authenticate via browser
```

---

## 2. Identité Git — CRUCIAL

**Configure dans le repo** (pas en global, pour ne pas mélanger avec d'autres projets) :

```bash
git clone https://github.com/Pec5A/streampulse.git
cd streampulse

# REMPLACE par TES infos GitHub
git config user.name "KaysZ"                  # ou "SamyZ"
git config user.email "ton-email@github.com"  # email PRIMARY de ton compte GitHub
```

> ⚠️ L'email doit être celui que tu as dans **GitHub → Settings → Emails**. Sinon les commits n'apparaîtront pas comme étant les tiens (avatar absent, pas comptabilisés dans tes contributions).
>
> Si tu utilises l'email de privacy `xxx+username@users.noreply.github.com`, ça marche aussi.

Vérifie :
```bash
git config user.name && git config user.email
```

---

## 3. Signature des commits — OBLIGATOIRE (exigence PDF)

Le cahier des charges page 5 exige : *"Repository GitHub (**commits signés**, README complet, répartition des tâches)"*.

→ Sans signature, ton commit affiche **"Unverified"** rouge sur GitHub. Le jury le verra.

### Option A : Signature SSH (le plus simple en 2026)

```bash
# Génère une clé SSH dédiée signing (si tu n'en as pas)
ssh-keygen -t ed25519 -C "streampulse-signing" -f ~/.ssh/id_signing

# Ajoute la clé publique à GitHub :
# Settings → SSH and GPG keys → New SSH key
# Type : Signing Key
cat ~/.ssh/id_signing.pub  # copie ce contenu

# Configure Git pour signer en SSH dans CE repo
git config gpg.format ssh
git config user.signingkey ~/.ssh/id_signing.pub
git config commit.gpgsign true
git config tag.gpgsign true
```

### Option B : Signature GPG

```bash
# Génère une clé GPG
gpg --full-generate-key
# → ECC, "default" curve, 0 (jamais expirée), tes nom/email GitHub, mot de passe

# Liste tes clés et copie l'ID
gpg --list-secret-keys --keyid-format=long
# sec   ed25519/ABCD1234EFGH5678 ...

# Exporte la clé publique
gpg --armor --export ABCD1234EFGH5678 | pbcopy  # macOS
# → colle dans GitHub Settings → SSH and GPG keys → New GPG key

# Configure Git
git config user.signingkey ABCD1234EFGH5678
git config commit.gpgsign true
git config tag.gpgsign true
```

### Vérifier que ça marche
```bash
git commit --allow-empty -m "test: verify signing"
git log --show-signature -1
# → doit afficher "Good signature from..."
```

Sur GitHub, ton commit doit afficher un badge **"Verified"** vert.

---

## 4. Workflow PR — étape par étape

### a. Choisis une tâche

Va dans `docs/team/briefs/<ton-prenom>/` et choisis un fichier `.md` non encore fait.
Chaque brief contient : **objectif**, **fichiers à toucher**, **critères d'acceptation**, **critère RNCP couvert**, **points défense oral**.

### b. Crée ta branche

Convention : `<type>/<ton-prenom-lowercase>-<scope-court>`

```bash
git checkout main
git pull origin main
git checkout -b test/kaysz-stream-usecase
```

Types autorisés : `feat`, `fix`, `test`, `ci`, `docs`, `ops`, `chore`, `refactor`.

### c. Code + commits incrémentaux

```bash
# Édite les fichiers...

# Commits Conventional Commits, signés automatiquement
git add <fichiers>
git commit -m "test(stream): add integration tests for CreateStream use case"

# Plusieurs petits commits valent mieux qu'un gros (l'historique te défendra à l'oral)
```

### d. Avant de push : checklist

```bash
# Backend
cd backend
go test ./... -cover
go vet ./...
golangci-lint run

# Mobile
cd ../mobile
flutter test
dart analyze
```

Si rouge → corrige avant de push.

### e. Push + ouverture PR

```bash
git push -u origin test/kaysz-stream-usecase

# Ouvre la PR depuis ton compte
gh pr create --base main \
  --title "test(stream): integration tests for CreateStream use case" \
  --body-file - <<'EOF'
## Brief
docs/team/briefs/kaysz/02-tests-stream-usecase.md

## Critère RNCP couvert
A3.2 / Ce3.2.1, Ce3.2.3

## Résumé
- Setup testcontainers pour PostgreSQL
- 12 cas de test (golden path + 8 erreurs + 3 edge cases)
- Coverage du package `application/stream` : 0% → 87%

## Démo
- `cd backend && go test ./internal/application/stream/... -cover`

## Checklist
- [x] Tests passent
- [x] Coverage cible atteinte (≥ 80%)
- [x] golangci-lint clean
- [x] Brief référencé
- [x] Commits signés (badge Verified)
EOF
```

### f. Review → merge

- Yassir (et/ou l'autre membre selon CODEOWNERS) review.
- Tu corriges les remarques avec de nouveaux commits sur la branche (pas de force push pendant la review).
- CI verte + 1 approval → squash merge sur `main` par le reviewer (jamais self-merge).

---

## 5. Comment lire un brief

Chaque brief suit ce format :

```markdown
# <ID> — <Titre>

## Métadonnées
- **Owner** : KaysZ
- **Critère RNCP** : A3.2 / Ce3.2.1
- **Phase** : 2 (tests jusqu'à 80%)
- **Branche** : `test/kaysz-stream-usecase`
- **Effort estimé** : 4-6h

## Objectif
<contexte + ce qu'on veut atteindre>

## Fichiers à toucher
- `backend/internal/application/stream/create.go` (lecture)
- `backend/internal/application/stream/create_test.go` (création)
- `backend/pkg/testhelper/postgres.go` (extension)

## Critères d'acceptation
- [ ] X cas de test (liste détaillée)
- [ ] Coverage du package ≥ 80%
- [ ] Pas de mock de la DB (testcontainers)
- [ ] CI verte

## Démo / commande de validation
\`\`\`bash
cd backend && go test ./internal/application/stream/... -cover -v
\`\`\`

## Points défense oral RNCP
- Q: "Pourquoi testcontainers et pas un mock ?"
  R: "Mock divergence prod = bug masqué. Testcontainers garantit qu'on teste contre la vraie BDD que prod utilise (Postgres 16). Coût : 2s de boot, gain : confiance réelle."
- Q: "Comment tu as choisi tes cas de test ?"
  R: "Critère Ce3.2.1 du PDF : tests basés sur cas d'utilisation + besoins fonctionnels. J'ai dérivé 1 happy path, N erreurs métier (couvre les ports du domain), 3 edge cases (timeout, contention, payload trop gros)."
```

→ Tu réponds aux Q&A pendant la PR review pour t'entraîner. Ils te ressortiront mot pour mot à l'oral.

---

## 6. Pièges à éviter

| Piège | Conséquence | Comment éviter |
|---|---|---|
| Commit avec mauvais email | Pas attribué à toi sur GitHub | Vérifier `git config user.email` dans CE repo |
| Commit non signé | Badge "Unverified" rouge → jury suspecte | Tester `git log --show-signature` après chaque commit |
| Force push pendant review | Perdre les commentaires inline du reviewer | Ne push --force qu'après approval, jamais avant |
| Brancher depuis une autre branche que `main` | Conflits + diff pollué | `git checkout main && git pull` AVANT `git checkout -b ...` |
| Self-merge | Bloqué par branch protection (à activer) | Toujours demander review |
| PR géante | Reviewer perd le fil + diff non défendable | Découper en PRs ≤ 400 LOC |

---

## 7. Demande d'aide

- Question technique → ouvre une **issue** avec template `feature` ou `bug`.
- Bloqué sur un brief → commente sur l'issue qui le tracke ou ping en draft PR avec `WIP:` dans le titre.
- Pair-programming bienvenu (et c'est légitime) : c'est de l'entraide d'équipe, pas de la fraude.

---

## 8. Avant la soutenance

Pour ton entretien individuel 20 min, prépare :

1. **Ta liste de PRs mergées** (`gh pr list --author @me --state merged`)
2. **Pour chacune** : 30s de pitch (problème → solution → critère RNCP couvert)
3. **Anticiper les Q&A** (section "Points défense oral" de chaque brief)
4. **Connaître par cœur** : Clean Architecture, Riverpod 2.x, OTEL traces, ton ADR si tu en as écrit un.

Bon courage 🚀
