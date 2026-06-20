-- Add processing_stage column for import pipeline status tracking.
ALTER TABLE question_sets ADD COLUMN IF NOT EXISTS processing_stage VARCHAR(32) NOT NULL DEFAULT '';

-- Prevent duplicate classification names within the same knowledge base.
-- Partial index excludes soft-deleted rows so the same name can be reused
-- after the old classification is deleted.
CREATE UNIQUE INDEX IF NOT EXISTS uq_question_sets_tenant_kb_name_active
    ON question_sets (tenant_id, knowledge_base_id, name)
    WHERE deleted_at IS NULL;
