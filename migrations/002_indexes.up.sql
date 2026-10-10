-- ═══════════════════════════════════════════════════════════
-- 002 — Indexes
-- ═══════════════════════════════════════════════════════════
-- Neon'da mevcut olan tüm index'ler.
-- Primary key index'leri otomatik oluşur, eklemeye gerek yok.
-- ═══════════════════════════════════════════════════════════

-- ─── USERS ───
CREATE INDEX IF NOT EXISTS idx_users_username_lower
    ON users (LOWER(username));

-- ─── PROJECTS ───
CREATE INDEX IF NOT EXISTS idx_projects_author_id
    ON projects (author_id);

CREATE INDEX IF NOT EXISTS idx_projects_created_at
    ON projects (created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_github_url_unique
    ON projects (LOWER(github_url));

-- ─── PROJECT_CONTRIBUTORS ───
CREATE INDEX IF NOT EXISTS idx_pc_project_status
    ON project_contributors (project_id, status);

CREATE INDEX IF NOT EXISTS idx_pc_user_status
    ON project_contributors (user_id, status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_project_user
    ON project_contributors (project_id, user_id);

CREATE INDEX IF NOT EXISTS idx_pc_pending
    ON project_contributors (project_id, created_at DESC)
    WHERE status = 'pending';

-- ─── PROJECT_STARS ───
CREATE INDEX IF NOT EXISTS idx_project_stars_user_project
    ON project_stars (user_id, project_id);

-- ─── PROJECT_CATEGORIES ───
CREATE INDEX IF NOT EXISTS idx_project_categories_cat_project
    ON project_categories (category_id, project_id);

-- ─── AUDIT_LOGS ───
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id
    ON audit_logs (user_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action
    ON audit_logs (action);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
    ON audit_logs (created_at DESC);