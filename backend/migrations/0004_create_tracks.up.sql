CREATE TABLE tracks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT NOT NULL,
    artist       TEXT NOT NULL DEFAULT '',
    -- Opaque key into the storage port, not a path and not a URL: the host
    -- name must never end up baked into a row.
    storage_key  TEXT NOT NULL UNIQUE,
    -- Original client filename, kept for display and the download header
    -- only. Never used to build a filesystem path.
    filename     TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL,
    size_bytes   BIGINT NOT NULL CHECK (size_bytes >= 0),
    uploader_id  UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The broadcaster screen lists "my tracks" on every open.
CREATE INDEX idx_tracks_uploader ON tracks (uploader_id);
CREATE INDEX idx_tracks_created ON tracks (created_at DESC);

-- ON DELETE CASCADE keeps the RGPD account deletion (ticket Y2) correct
-- without Y2 having to know this table exists. Note the stored objects
-- themselves are NOT removed by this cascade — see ADR 0009.
