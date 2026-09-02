-- 0003 (not 0002): ticket S1 (playlists, PR #16) already claims 0002. Two
-- files sharing a prefix would make an ordered migration runner ambiguous,
-- so K1 takes the next free number.

CREATE TABLE streams (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    broadcaster_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('live', 'offline')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Listeners browse live streams on every app open; broadcasters list their
-- own. Both are the hot read paths.
CREATE INDEX idx_streams_status ON streams (status);
CREATE INDEX idx_streams_broadcaster ON streams (broadcaster_id);

-- ON DELETE CASCADE above is what makes the RGPD account deletion (ticket Y2)
-- remove a user's streams along with the account, without Y2 having to know
-- this table exists.
