# Runbook — Incidents de scan sécurité / Security scan incidents (ticket S3)

Bilingue **FR / EN**. Réponse aux alertes des scanners de la CI : gitleaks (secrets), trivy + govulncheck (dépendances vulnérables), gosec (SAST).

---

## FR

### 1. `Secret Detection` rouge — gitleaks a détecté un secret
1. **Ne pas merger.** Repérer le secret (commit / fichier / ligne dans les logs du job).
2. **Vrai secret** → le considérer **compromis** : le révoquer/roter immédiatement à la source (clé API, token, mot de passe DB), le sortir du code (variable d'environnement). S'il est déjà poussé, réécrire l'historique (`git filter-repo`) puis roter la valeur.
3. **Faux positif** (secret de test) → commentaire `// gitleaks:allow` sur la ligne, ou entrée justifiée dans `.gitleaksignore` (fingerprint).

### 2. `Dependency Vulnerabilities` rouge — trivy / govulncheck
1. Lire la CVE et la version corrigée dans les logs.
2. `cd backend && go get <module>@<version-corrigée> && go mod tidy`, puis relancer les tests.
3. **Pas de correctif** (`unfixed`) → évaluer l'exploitabilité : `govulncheck` indique si la fonction vulnérable est réellement appelée. Non appelée → risque faible, documenter. Appelée → mitiger (contournement, désactivation) et suivre.
4. **Ne jamais** downgrader un scanner pour « faire passer » : un scan rouge = action requise, pas un test instable.

### 3. `Go Quality` rouge sur gosec — finding SAST
1. Lire la règle (ex. G401 crypto faible, G104 erreur ignorée).
2. Corriger le code. Faux positif justifié → annotation `// #nosec Gxxx <raison>` sur la ligne.

---

## EN

### 1. `Secret Detection` red — gitleaks found a secret
1. **Do not merge.** Locate the secret (commit / file / line in the job logs).
2. **Real secret** → treat it as **compromised**: revoke/rotate it at the source immediately (API key, token, DB password), move it to an environment variable. If already pushed, rewrite history (`git filter-repo`) then rotate.
3. **False positive** (test secret) → inline `// gitleaks:allow`, or a justified `.gitleaksignore` fingerprint.

### 2. `Dependency Vulnerabilities` red — trivy / govulncheck
1. Read the CVE and fixed version in the logs.
2. `cd backend && go get <module>@<fixed> && go mod tidy`, then re-run tests.
3. **Unfixed** → assess exploitability: `govulncheck` reports whether the vulnerable function is actually reachable. Not reachable → low risk, document. Reachable → mitigate and track.
4. **Never** downgrade a scanner to turn CI green: a red scan means action is needed, not a flaky test.

### 3. `Go Quality` red on gosec — SAST finding
1. Read the rule (e.g. G401 weak crypto, G104 unhandled error).
2. Fix the code. Justified false positive → `// #nosec Gxxx <reason>` on the line.
