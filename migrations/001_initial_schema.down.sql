-- ═══════════════════════════════════════════════════════════
-- 001 — Geri alma (ters sırayla)
-- ═══════════════════════════════════════════════════════════

DROP TRIGGER IF EXISTS set_updated_at ON project_contributors;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS project_stars;
DROP TABLE IF EXISTS project_contributors;
DROP TABLE IF EXISTS project_categories;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS update_updated_at_column();