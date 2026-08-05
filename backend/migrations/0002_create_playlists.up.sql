-- Playlists and their ordered tracks (ticket S1).
-- Tracks are playlist-owned (not a shared catalog) so this slice stays
-- independent of the upload/streaming features; source_url is an optional
-- forward-compatible pointer to that media.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS playlists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(80) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_public   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_playlists_owner ON playlists (owner_id);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    playlist_id      UUID NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    title            VARCHAR(200) NOT NULL,
    artist           VARCHAR(200) NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    source_url       TEXT NOT NULL DEFAULT '',
    position         INTEGER NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Deferred so ReorderTracks can rewrite positions row-by-row inside a
    -- single transaction: uniqueness is enforced at COMMIT, not mid-rewrite.
    CONSTRAINT uq_playlist_tracks_position UNIQUE (playlist_id, position)
        DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX IF NOT EXISTS idx_playlist_tracks_playlist ON playlist_tracks (playlist_id, position);
