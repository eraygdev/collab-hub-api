-- ═══════════════════════════════════════════════════════════
-- 003 — Cleanup geri alma
-- ═══════════════════════════════════════════════════════════

-- description → TEXT
ALTER TABLE projects
    ALTER COLUMN description TYPE TEXT;

-- created_at / updated_at → timestamp
ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

ALTER TABLE projects
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

-- stars kolonu geri
ALTER TABLE projects ADD COLUMN IF NOT EXISTS stars INTEGER DEFAULT 0;

-- google_id geri
ALTER TABLE users ADD COLUMN IF NOT EXISTS google_id VARCHAR(64) UNIQUE;

-- password_hash geri
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255);

-- duplicate index geri
CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique
    ON users (username);