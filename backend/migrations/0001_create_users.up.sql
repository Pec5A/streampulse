CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT NOT NULL UNIQUE,
    username   TEXT NOT NULL UNIQUE,
    password   TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'broadcaster', 'admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_email ON users (email);
