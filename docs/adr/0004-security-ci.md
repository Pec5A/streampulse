# 0004 — Scan de sécurité en CI et protection de branche (ticket S3)

## Statut
Accepté

## Contexte
S3 vise le durcissement sécurité de la chaîne : scan de secrets + dépendances vulnérables + SAST, une convention de protection de branche, et un runbook d'incident.

## Note de périmètre (Y3 / S3) — importante
Le ticket **Y3** (Yassir, PR #15, non mergée) a déjà ajouté au `ci.yml` : **gosec**, staticcheck, un plancher de couverture, et une stack observabilité (Prometheus/Grafana + **alertes et runbooks auth**). Le plan liste d'ailleurs gosec dans Y3 **et** S3. Pour éviter la duplication et les conflits, ce ticket S3 se limite à ce qui n'est **pas** couvert par #15 :

- **trivy + govulncheck** (vulnérabilités des dépendances) — absents de #15, dans un workflow **séparé** `security.yml`.
- **convention de protection de branche** (doc).
- **runbook d'incident sécurité** (scans) — distinct des runbooks d'*alertes auth* de #15.

La partie **alertes Prometheus/Alertmanager** de l'intitulé S3 est **déjà couverte par #15** → répartition à trancher explicitement avec Yassir pour la défense RNCP (voir la PR).

## Décisions

### Workflow `security.yml` séparé (pas d'ajout au `ci.yml`)
`ci.yml` est en cours de remaniement lourd par #15. Isoler les scans de dépendances dans un workflow dédié évite un conflit de merge et sépare proprement « qualité » et « sécurité ». Checks : `govulncheck` (base de vulnérabilités Go officielle, ne signale que les fonctions vulnérables réellement atteignables) + `trivy fs` (CVE des modules, `--severity HIGH,CRITICAL --ignore-unfixed`).

### Scanners à jour, pas figés
Les bases de vulnérabilités (trivy, govulncheck) sont récupérées au runtime : un scan peut passer au rouge quand une CVE est **nouvellement divulguée** sur une dépendance. C'est **voulu** — un rouge appelle une action (voir runbook), ce n'est pas un test instable.

### Protection de branche : convention documentée
Repo privé en plan gratuit → protection non forçable techniquement ; documentée comme convention (CI verte + 1 review CODEOWNER + commits signés + pas de force-push + squash), avec les commandes `gh api` prêtes à l'activation.

## Conséquences
- Couverture sécurité combinée (une fois #15 mergé) : **gitleaks** (secrets) + **gosec** (SAST) + **trivy** + **govulncheck** (dépendances). Pas de duplication.
- La répartition Y3/S3 sur gosec et l'alerting doit être tranchée en équipe pour la défense RNCP (qui présente quoi).
