-- Add system-managed metadata columns to knowledge_bases.
-- parent_knowledge_base_id: links a child KB (e.g., hidden syllabus KB) to its parent (e.g., question_bank KB).
-- purpose: describes the system role (e.g., "question_bank_syllabus"); NULL for normal user KBs.
-- visibility: "visible" (default) or "hidden" — controls whether the KB appears in normal listings.
-- system_managed: true when the KB was auto-created by the system and should be protected from manual operations.
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS parent_knowledge_base_id VARCHAR(36);
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS purpose VARCHAR(64);
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS visibility VARCHAR(16) NOT NULL DEFAULT 'visible';
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS system_managed BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_parent ON knowledge_bases(parent_knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_purpose ON knowledge_bases(purpose);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility ON knowledge_bases(visibility);
