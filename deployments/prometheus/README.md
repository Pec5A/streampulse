# Prometheus — cibles de scrape

Deux cibles, et une seule marche sans configuration.

| Cible | Marche sur un clone frais | Ce qu'elle observe |
|---|---|---|
| `streampulse-api` | **oui** | L'API lancée par `docker compose` sur cette machine |
| `streampulse-api-prod` | non | Le backend déployé sur Render |

## Pourquoi la cible production apparaît `down` chez vous

Son jeton de scrape est un vrai secret de production, généré par Render. Il
n'est pas dans le dépôt : il est lu dans `deployments/prometheus/prod-token`,
un fichier que `.gitignore` exclut.

Sans ce fichier, Docker crée un **dossier** à sa place au montage, et le
scrape échoue avec `is a directory` dans <http://localhost:9090/targets>.
C'est visible et explicite, pas silencieux — mais ça ne casse rien d'autre :
Prometheus démarre, la cible locale scrape, les tableaux de bord affichent
les données de votre machine.

## Pour observer la production

1. Dashboard Render → service `streampulse-api` → Environment → copier `METRICS_TOKEN`
2. L'écrire **sans retour à la ligne** :

```bash
printf '%s' 'LE_JETON' > deployments/prometheus/prod-token
chmod 600 deployments/prometheus/prod-token
docker compose restart prometheus
```

3. Vérifier : <http://localhost:9090/targets> doit montrer les deux cibles `up`.

Si le montage avait déjà créé un dossier, le supprimer d'abord :

```bash
docker compose stop prometheus
rm -rf deployments/prometheus/prod-token
```

## Distinguer les deux sources dans Grafana

Les panneaux ne filtrent pas sur `job`, donc les deux sources se superposent
quand les deux cibles sont `up`. Pour n'en voir qu'une, ajouter `{job="streampulse-api-prod"}`
à la requête du panneau, ou arrêter l'autre : `docker compose stop api`.
