# Diagrams — StreamPulse

> French version: [`README.md`](README.md).
> RNCP criterion: **Ce3.6.1** ("a language allowing the components to be visualised in a standardised way").

**Mermaid** notation, rendered natively by GitHub: the diagrams are versioned text, reviewed in PRs and diffed like code. An exported image diverges from the code at the first refactor and nobody notices.

---

## 1. Layered architecture (component diagram)

The dependency rule is one-way: `domain` knows nobody, everybody knows `domain`.

```mermaid
flowchart TB
    subgraph transport["transport — HTTP"]
        R["router<br/>Go 1.22 ServeMux"]
        MW["middleware<br/>Auth · RequireAdmin · Metrics · Tracing"]
        H["handler<br/>Auth · User · Admin · Playlist · Stream · Track"]
    end

    subgraph application["application"]
        UC["usecase<br/>business rules and authorisation"]
        DTO["dto<br/>JSON contracts"]
    end

    subgraph domain["domain — no dependencies"]
        E["entity<br/>User · Playlist · Stream · Track"]
        RI["repository<br/>interfaces only"]
        SI["service<br/>interfaces only"]
    end

    subgraph infrastructure["infrastructure"]
        P["persistence<br/>pgx · PostgreSQL"]
        A["auth<br/>JWT · bcrypt"]
        ST["streaming<br/>Hub · Registry · ChatHub"]
        O["observability<br/>JSON slog · Prometheus · OTel"]
        C["config<br/>12-Factor, env only"]
    end

    R --> MW --> H --> UC
    H --> DTO
    UC --> E
    UC --> RI
    UC --> SI
    P -.implements.-> RI
    A -.implements.-> SI
    ST -.implements.-> SI
    MW --> O
    R --> C
```

**What the diagram makes visible**: no arrow leaves `domain`. Infrastructure implementations point at the domain's interfaces, never the other way round — which is what makes a use case testable with an in-memory repository, and lets local storage be swapped for S3 without touching business code.

## 2. Data model (entity-relationship)

```mermaid
erDiagram
    users ||--o{ playlists : "owns"
    users ||--o{ streams : "broadcasts"
    users ||--o{ tracks : "uploads"
    playlists ||--o{ playlist_tracks : "orders"

    users {
        uuid id PK
        text email UK
        text username UK
        text password "bcrypt hash"
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
        integer position "UNIQUE deferred, per playlist"
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
        text storage_key UK "opaque key, neither path nor URL"
        text filename "display only"
        text content_type
        bigint size_bytes
        timestamptz created_at
        timestamptz updated_at
    }
```

**Two decisions readable straight off the schema**:

- Every `ON DELETE CASCADE` points at `users`. That is what makes the GDPR account deletion (Y2) correct **without Y2 knowing these tables exist**: each slice declares its own cascade when it lands.
- `uq_playlist_tracks_position` is `DEFERRABLE INITIALLY DEFERRED`: reordering rewrites positions row by row inside a transaction, and uniqueness is only checked at `COMMIT` — otherwise the first write would collide with a position not yet moved.

## 3. Live streaming: one broadcaster, N listeners (sequence)

```mermaid
sequenceDiagram
    autonumber
    actor B as Broadcaster
    participant API as HTTP API
    participant UC as StreamUseCase
    participant REG as Registry
    participant HUB as Hub
    actor L1 as Listener 1
    actor LN as Listener N

    B->>API: POST /streams/{id}/publish
    API->>UC: StartLive(id, userID)
    UC->>UC: check stream ownership
    UC->>REG: Open(ctx, streamID)
    REG->>HUB: NewHub
    UC-->>API: status live

    L1->>API: GET /streams/{id}/listen
    API->>HUB: Subscribe
    HUB-->>L1: buffered channel (256 chunks)
    LN->>API: GET /streams/{id}/listen
    API->>HUB: Subscribe
    HUB-->>LN: buffered channel

    loop each audio chunk
        B->>API: chunked body
        API->>HUB: Publish(chunk)
        Note over HUB: select/default —<br/>non-blocking by construction
        HUB-->>L1: chunk
        HUB-->>LN: chunk
    end

    Note over HUB,LN: 64 CONSECUTIVE drops -> eviction.<br/>Counter resets on every successful send,<br/>so transient jitter does not disconnect.

    B->>API: end of body
    API->>UC: StopLive
    UC->>REG: Close(streamID)
    REG->>HUB: Close
    HUB-->>L1: end of stream
    HUB-->>LN: end of stream
```

**The non-obvious point**: the broadcaster's HTTP response is written **at the end**, not up front. A conforming HTTP client stops sending the body as soon as a response arrives — answering `200 OK` immediately cut the broadcast at the first chunk. Only authorisation refusals answer straight away, which is exactly what tells a rejected broadcaster to stop transmitting.

## 4. Stream lifecycle (state)

```mermaid
stateDiagram-v2
    [*] --> offline : POST /broadcast (creation)
    offline --> live : StartLive — hub and chat room opened
    live --> live : broadcaster reconnects<br/>(hub recycled, room preserved)
    live --> offline : StopLive — hub and room closed
    offline --> [*] : DELETE — cascades to listeners
```

**The asymmetry on the `live --> live` transition is deliberate.** The audio hub is fed by exactly one publisher connection: when the broadcaster reconnects it must be recycled, otherwise listeners stay attached to a hub nobody feeds any more. The chat room has no privileged participant — recycling it would eject the whole conversation on a two-second network drop.

## 5. Distributed trace of one request (sequence)

```mermaid
sequenceDiagram
    autonumber
    actor M as Flutter app
    participant MW as middleware.Tracing
    participant H as handler
    participant DB as PostgreSQL
    participant COL as OTel Collector
    participant T as Tempo
    participant G as Grafana

    M->>MW: HTTP + traceparent header
    MW->>MW: Extract — same trace, not a new one
    MW->>H: context carrying the span
    H->>DB: SQL query
    DB-->>H: rows
    H-->>MW: HTTP status
    MW->>MW: name = route template<br/>error only on 5xx
    MW-->>M: response
    MW--)COL: OTLP/gRPC export (batched)
    COL--)T: OTLP
    G->>T: query the trace
    Note over G: the same trace_id appears on<br/>every JSON log line
```

## 6. From PR to production (deployment)

```mermaid
flowchart LR
    PR["Pull request"] --> CI{"CI"}
    CI -->|"Go Quality"| GQ["vet · test -race · coverage"]
    CI -->|"Flutter Quality"| FQ["analyze · test"]
    CI -->|"Security"| SEC["gitleaks · trivy · govulncheck"]

    GQ & FQ & SEC --> REV["Cross-review<br/>never a self-merge"]
    REV --> MAIN["merge to main"]

    MAIN --> CD{"CD"}
    CD --> IMG["GHCR image<br/>long sha + semver tags"]
    IMG --> VERIF["real boot<br/>against a real Postgres"]
    VERIF --> DEP["flyctl deploy --image"]
    DEP --> HC{"/health check<br/>required by Fly"}
    HC -->|"ok"| PROD["production"]
    HC -->|"fails"| KEEP["traffic kept on<br/>the previous version"]
    PROD --> VER{"does production report<br/>the deployed commit?"}
    VER -->|"no"| FAIL["job fails"]

    CD --> APK["APK and web artefacts"]
```

**What the diagram encodes**: the deployed image is **the one that was verified**, not a second build that resembles it (`--image`, never a build by the host). Plus two guards after the fact — Fly only switches traffic if `/health` passes, and the job fails if production does not report the run's commit, because a "successful" deployment still serving the old version is the failure mode nobody notices.
