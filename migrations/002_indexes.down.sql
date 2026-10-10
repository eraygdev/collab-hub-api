-- ═══════════════════════════════════════════════════════════
-- 002 — Index'leri geri al
-- ═══════════════════════════════════════════════════════════

DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_user_id;
DROP INDEX IF EXISTS idx_project_categories_cat_project;
DROP INDEX IF EXISTS idx_project_stars_user_project;
DROP INDEX IF EXISTS idx_pc_pending;
DROP INDEX IF EXISTS idx_unique_project_user;
DROP INDEX IF EXISTS idx_pc_user_status;
DROP INDEX IF EXISTS idx_pc_project_status;
DROP INDEX IF EXISTS idx_projects_github_url_unique;
DROP INDEX IF EXISTS idx_projects_created_at;
DROP INDEX IF EXISTS idx_projects_author_id;
DROP INDEX IF EXISTS idx_users_username_lower;