# Diagrammes — StreamPulse

> Version anglaise : [`README.en.md`](README.en.md).
> Critère RNCP visé : **Ce3.6.1** (« langage qui permet de visualiser les composantes de façon standardisée »).

Notation **Mermaid**, rendue nativement par GitHub : les diagrammes sont du texte versionné, relu en PR et diffé comme du code. Une image exportée diverge du code au premier refactor sans que personne le voie.

---

## 1. Architecture en couches (diagramme de composants)

La règle de dépendance est unidirectionnelle : `domain` ne connaît personne, tout le monde connaît `domain`.

```mermaid
flowchart TB
    subgraph transport["transport — HTTP"]
        R["router<br/>ServeMux Go 1.22"]
        MW["middleware<br/>Auth · RequireAdmin · Metrics · Tracing"]
        H["handler<br/>Auth · User · Admin · Playlist · Stream · Track"]
    end

    subgraph application["application"]
        UC["usecase<br/>regles metier et autorisations"]
        DTO["dto<br/>contrats JSON"]
    end

    subgraph domain["domain — aucune dependance"]
        E["entity<br/>User · Playlist · Stream · Track"]
        RI["repository<br/>interfaces uniquement"]
        SI["service<br/>interfaces uniquement"]
    end

    subgraph infrastructure["infrastructure"]
        P["persistence<br/>pgx · PostgreSQL"]
        A["auth<br/>JWT · bcrypt"]
        ST["streaming<br/>Hub · Registry · ChatHub"]
        O["observability<br/>slog JSON · Prometheus · OTel"]
        C["config<br/>12-Factor, env uniquement"]
    end

    R --> MW --> H --> UC
    H --> DTO
    UC --> E
    UC --> RI
    UC --> SI
    P -.implemente.-> RI
    A -.implemente.-> SI
    ST -.implemente.-> SI
    MW --> O
    R --> C
```

**Ce que le diagramme rend visible** : aucune flèche ne part de `domain`. Les implémentations d'infrastructure pointent vers les interfaces du domaine, jamais l'inverse — c'est ce qui permet de tester un usecase avec un dépôt en mémoire, et de remplacer le stockage local par S3 sans toucher au métier.

## 2. Modèle de données (entité-association)

```mermaid
erDiagram
    users ||--o{ playlists : "possede"
    users ||--o{ streams : "diffuse"
    users ||--o{ tracks : "televerse"
    playlists ||--o{ playlist_tracks : "ordonne"

    users {
        uuid id PK
        text email UK
        text username UK
        text password "hash bcrypt"
        text role "user | broadcaster | admin"
        timestamptz created_at
        timestamptz updated_at
    }

    playlists {
        uuid id PK
        uuid owner_id FK "ON DELETE CASCADE"
        varchar name
        text description
        boolean is_public
        timestamptz created_at
        timestamptz updated_at
    }

    playlist_tracks {
        uuid id PK
        uuid playlist_id FK "ON DELETE CASCADE"
        varchar title
        varchar artist
        integer duration_seconds
        text source_url
        integer position "UNIQUE differe par playlist"
        timestamptz created_at
    }

    streams {
        uuid id PK
        uuid broadcaster_id FK "ON DELETE CASCADE"
        text title
        text description
        text status "live | offline"
        timestamptz created_at
        timestamptz updated_at
    }

    tracks {
        uuid id PK
        uuid uploader_id FK "ON DELETE CASCADE"
        text title
        text artist
        text storage_key UK "cle opaque, ni chemin ni URL"
        text filename "affichage seulement"
        text content_type
        bigint size_bytes
        timestamptz created_at
        timestamptz updated_at
    }
```

**Deux décisions lisibles directement sur le schéma** :

- Tous les `ON DELETE CASCADE` pointent vers `users`. C'est ce qui rend la suppression de compte RGPD (Y2) correcte **sans que Y2 connaisse l'existence de ces tables** : chaque tranche déclare sa propre cascade quand elle arrive.
- `uq_playlist_tracks_position` est `DEFERRABLE INITIALLY DEFERRED` : le réordonnancement réécrit les positions ligne par ligne dans une transaction, et l'unicité n'est vérifiée qu'au `COMMIT` — sinon la première écriture entrerait en collision avec une position pas encore déplacée.

## 3. Diffusion live : un diffuseur, N auditeurs (séquence)

```mermaid
sequenceDiagram
    autonumber
    actor B as Diffuseur
    participant API as API HTTP
    participant UC as StreamUseCase
    participant REG as Registry
    participant HUB as Hub
    actor L1 as Auditeur 1
    actor LN as Auditeur N

    B->>API: POST /streams/{id}/publish
    API->>UC: StartLive(id, userID)
    UC->>UC: verifie la propriete du direct
    UC->>REG: Open(ctx, streamID)
    REG->>HUB: NewHub
    UC-->>API: statut live

    L1->>API: GET /streams/{id}/listen
    API->>HUB: Subscribe
    HUB-->>L1: canal bufferise (256 chunks)
    LN->>API: GET /streams/{id}/listen
    API->>HUB: Subscribe
    HUB-->>LN: canal bufferise

    loop chaque chunk audio
        B->>API: corps chunke
        API->>HUB: Publish(chunk)
        Note over HUB: select/default —<br/>non bloquant par construction
        HUB-->>L1: chunk
        HUB-->>LN: chunk
    end

    Note over HUB,LN: 64 pertes CONSECUTIVES -> eviction.<br/>Compteur remis a zero a chaque envoi reussi,<br/>donc une gigue passagere ne deconnecte pas.

    B->>API: fin du corps
    API->>UC: StopLive
    UC->>REG: Close(streamID)
    REG->>HUB: Close
    HUB-->>L1: fin de flux
    HUB-->>LN: fin de flux
```

**Le point non évident** : la réponse HTTP du diffuseur est écrite **à la fin**, pas au début. Un client HTTP conforme arrête d'envoyer le corps dès qu'une réponse arrive — répondre `200 OK` d'emblée coupait la diffusion au premier chunk. Seuls les refus d'autorisation répondent immédiatement, ce qui est précisément ce qui dit à un diffuseur refusé d'arrêter d'émettre.

## 4. Cycle de vie d'un direct (états)

```mermaid
stateDiagram-v2
    [*] --> offline : POST /broadcast (creation)
    offline --> live : StartLive — hub et salon de chat ouverts
    live --> live : reconnexion du diffuseur<br/>(hub recycle, salon conserve)
    live --> offline : StopLive — hub et salon fermes
    offline --> [*] : DELETE — cascade sur les auditeurs
```

**L'asymétrie de la transition `live --> live` est délibérée.** Le hub audio est alimenté par exactement une connexion de publication : quand le diffuseur se reconnecte, il faut le recycler, sinon les auditeurs restent accrochés à un hub que plus personne n'alimente. Le salon de chat, lui, n'a aucun participant privilégié — le recycler éjecterait toute la conversation à la première coupure réseau de deux secondes.

## 5. Trace distribuée d'une requête (séquence)

```mermaid
sequenceDiagram
    autonumber
    actor M as App Flutter
    participant MW as middleware.Tracing
    participant H as handler
    participant DB as PostgreSQL
    participant COL as OTel Collector
    participant T as Tempo
    participant G as Grafana

    M->>MW: HTTP + en-tete traceparent
    MW->>MW: Extract — meme trace, pas une nouvelle
    MW->>H: contexte porteur du span
    H->>DB: requete SQL
    DB-->>H: lignes
    H-->>MW: statut HTTP
    MW->>MW: nom = template de route<br/>erreur seulement si 5xx
    MW-->>M: reponse
    MW--)COL: export OTLP/gRPC (par lots)
    COL--)T: OTLP
    G->>T: consultation de la trace
    Note over G: le meme trace_id figure dans<br/>chaque ligne de log JSON
```

## 6. De la PR à la production (déploiement)

```mermaid
flowchart LR
    PR["Pull request"] --> CI{"CI"}
    CI -->|"Go Quality"| GQ["vet · test -race · couverture"]
    CI -->|"Flutter Quality"| FQ["analyze · test"]
    CI -->|"Security"| SEC["gitleaks · trivy · govulncheck"]

    GQ & FQ & SEC --> REV["Review croisee<br/>jamais de self-merge"]
    REV --> MAIN["merge sur main"]

    MAIN --> CD{"CD"}
    CD --> IMG["image GHCR<br/>tag sha long + semver"]
    IMG --> VERIF["demarrage reel<br/>contre un vrai Postgres"]
    VERIF --> DEP["flyctl deploy --image"]
    DEP --> HC{"check /health<br/>exige par Fly"}
    HC -->|"ok"| PROD["production"]
    HC -->|"echec"| KEEP["trafic garde sur<br/>la version precedente"]
    PROD --> VER{"la prod annonce-t-elle<br/>le commit deploye ?"}
    VER -->|"non"| FAIL["job en echec"]

    CD --> APK["artefacts APK et web"]
```

**Ce que le diagramme encode** : l'image déployée est **celle qui a été vérifiée**, pas un second build qui lui ressemble (`--image`, jamais un build par l'hébergeur). Et deux garde-fous après coup — Fly ne bascule le trafic que si `/health` passe, et le job échoue si la production ne rapporte pas le commit du run, parce qu'un déploiement « réussi » qui sert encore l'ancienne version est le mode de panne qu'on ne voit pas.
