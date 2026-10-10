-- ═══════════════════════════════════════════════════════════
-- 004 — Seed Categories
-- ═══════════════════════════════════════════════════════════
-- Mevcut 23 kategoriyi ID'leriyle ekle.
-- ID'ler korunur çünkü projeler bu ID'lere referans veriyor.
-- Sequence'i de güncelle ki yeni eklenenler çakışmasın.
-- ═══════════════════════════════════════════════════════════

INSERT INTO categories (id, name, slug) VALUES
    (1,  'AI',                'ai'),
    (2,  'Python',            'python'),
    (3,  'Backend',           'backend'),
    (4,  'React',             'react'),
    (5,  'Go',                'go'),
    (7,  'Frontend',          'frontend'),
    (8,  'Fullstack',         'fullstack'),
    (9,  'Mobile',            'mobile'),
    (10, 'DevOps',            'devops'),
    (11, 'Security',          'security'),
    (13, 'Data Science',      'data-science'),
    (14, 'Blockchain',        'blockchain'),
    (15, 'Game Development',  'game-development'),
    (16, 'Open Source',       'open-source'),
    (17, 'Tool',              'tool'),
    (18, 'Library',           'library'),
    (19, 'API',               'api'),
    (20, 'Web3',              'web3'),
    (21, 'Cloud',             'cloud'),
    (29, 'Machine Learning',  'machine-learning'),
    (39, 'Database',          'database'),
    (40, 'Testing',           'testing'),
    (41, 'CLI',               'cli')
ON CONFLICT (id) DO NOTHING;

-- Sequence'i en büyük ID'den sonraya ayarla
SELECT setval('categories_id_seq', (SELECT MAX(id) FROM categories));