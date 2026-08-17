# Convention de protection de `main` (ticket S3)

> Objectif : garantir l'intégrité de `main` — tout code y arrive via une PR revue, CI verte et commits signés. Support pour **A3.1** (intégration des changements) et **A3.5** (opérations continues).

## Règles sur `main`

1. **Pas de push direct** — toute modification passe par une PR.
2. **CI verte obligatoire** — checks requis : `Go Quality`, `Flutter Quality`, `Secret Detection`, `Dependency Vulnerabilities`.
3. **1 review d'un CODEOWNER** — un membre ne merge jamais sa propre PR (voir `.github/CODEOWNERS`).
4. **Commits signés** — badge « Verified » exigé (cahier des charges p.5, voir `docs/team/setup.md`).
5. **Pas de force-push** pendant la review.
6. **Historique linéaire** — squash merge.

## Application (owner du repo)

Sur un repo **privé en plan gratuit**, la protection de branche n'est pas forçable techniquement : ces règles sont une **convention d'équipe**. Dès que le plan le permet (ou repo public), les appliquer :

```bash
gh api -X PUT repos/Pec5A/streampulse/branches/main/protection --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "checks": [
      {"context": "Go Quality"},
      {"context": "Flutter Quality"},
      {"context": "Secret Detection"},
      {"context": "Dependency Vulnerabilities"}
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "require_code_owner_reviews": true
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "required_linear_history": true
}
JSON

# Exiger des commits signés sur main (endpoint séparé) :
gh api -X POST repos/Pec5A/streampulse/branches/main/protection/required_signatures
```

> `required_signatures` refuse tout commit non signé sur `main` — aligné avec l'exigence RNCP.
