-- ═══════════════════════════════════════════════════════════
-- 003 — Cleanup
-- ═══════════════════════════════════════════════════════════
-- Kullanılmayan kolonları sil, tip tutarsızlıklarını düzelt,
-- duplicate index'i temizle.
-- ═══════════════════════════════════════════════════════════

-- ─────────────────────────────────────────────────────────
-- 1. Duplicate UNIQUE index (users.username)
-- users_username_key ve users_username_unique aynı şey.
-- Standart olan users_username_key (PostgreSQL auto-isim) kalır.
-- ─────────────────────────────────────────────────────────
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_unique;

-- ─────────────────────────────────────────────────────────
-- 2. USERS — kullanılmayan kolonlar
-- ─────────────────────────────────────────────────────────
-- password_hash: GitHub OAuth kullanıyoruz, email/şifre login yok
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;

-- google_id: Google OAuth yok
ALTER TABLE users DROP COLUMN IF EXISTS google_id;

-- ─────────────────────────────────────────────────────────
-- 3. PROJECTS — kullanılmayan kolon
-- ─────────────────────────────────────────────────────────
-- stars: kod artık project_stars tablosundan COUNT yapıyor
ALTER TABLE projects DROP COLUMN IF EXISTS stars;

-- ─────────────────────────────────────────────────────────
-- 4. Tip tutarsızlıkları
-- ─────────────────────────────────────────────────────────

-- projects.created_at / updated_at → timestamptz
-- (USING clause ile eski timestamp'i timestamptz'e çevir)
ALTER TABLE projects
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- users.created_at → timestamptz
ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- projects.description → VARCHAR(150)
-- (mevcut max 134 karakter, güvenli)
ALTER TABLE projects
    ALTER COLUMN description TYPE VARCHAR(150);