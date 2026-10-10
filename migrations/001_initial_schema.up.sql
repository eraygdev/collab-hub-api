-- ═══════════════════════════════════════════════════════════
-- 001 — Initial Schema
-- ═══════════════════════════════════════════════════════════
-- Mevcut Neon şemasının birebir kopyası.
-- Sonraki migration'lar (002, 003...) düzeltme ve temizlik yapar.
-- ═══════════════════════════════════════════════════════════

-- ─────────────────────────────────────────────────────────
-- FONKSİYON: update_updated_at_column
-- updated_at kolonunu otomatik NOW() yapar (trigger için)
-- ─────────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ─────────────────────────────────────────────────────────
-- USERS
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id              SERIAL PRIMARY KEY,
    username        VARCHAR(30)  NOT NULL UNIQUE,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   VARCHAR(255),
    github_id       INTEGER UNIQUE,
    google_id       VARCHAR(64)  UNIQUE,
    avatar_url      TEXT,
    bio             TEXT,
    avatar_id       VARCHAR(50),
    avatar_type     VARCHAR(20)  DEFAULT 'preset',
    is_premium      BOOLEAN      DEFAULT false,
    premium_until   TIMESTAMPTZ,
    created_at      TIMESTAMP    DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────
-- PROJECTS
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS projects (
    id               SERIAL PRIMARY KEY,
    title            VARCHAR(80)  NOT NULL,
    description      TEXT         NOT NULL,
    long_description TEXT,
    author_id        INTEGER      REFERENCES users(id) ON DELETE SET NULL,
    image_url        TEXT,
    github_url       TEXT,
    demo_url         TEXT,
    stars            INTEGER      DEFAULT 0,
    status           VARCHAR(50)  DEFAULT 'Online',
    max_contributors INTEGER      NOT NULL DEFAULT 10,
    created_at       TIMESTAMP    DEFAULT NOW(),
    updated_at       TIMESTAMP    DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────
-- CATEGORIES
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS categories (
    id    SERIAL PRIMARY KEY,
    name  VARCHAR(50) NOT NULL UNIQUE,
    slug  VARCHAR(50) NOT NULL UNIQUE
);

-- ─────────────────────────────────────────────────────────
-- PROJECT_CATEGORIES (pivot)
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS project_categories (
    project_id  INTEGER NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, category_id)
);

-- ─────────────────────────────────────────────────────────
-- PROJECT_CONTRIBUTORS
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS project_contributors (
    id          SERIAL PRIMARY KEY,
    project_id  INTEGER     NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     INTEGER     NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    message     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trigger — updated_at otomatik güncellensin
DROP TRIGGER IF EXISTS set_updated_at ON project_contributors;
CREATE TRIGGER set_updated_at
    BEFORE UPDATE ON project_contributors
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ─────────────────────────────────────────────────────────
-- PROJECT_STARS
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS project_stars (
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

-- ─────────────────────────────────────────────────────────
-- AUDIT_LOGS
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS audit_logs (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER      REFERENCES users(id) ON DELETE SET NULL,
    action      VARCHAR(50)  NOT NULL,
    entity_type VARCHAR(50),
    entity_id   INTEGER,
    ip_address  VARCHAR(45),
    user_agent  TEXT,
    created_at  TIMESTAMPTZ  DEFAULT NOW()
);