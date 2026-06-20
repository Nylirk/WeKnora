DROP INDEX IF EXISTS uq_question_sets_tenant_kb_name_active;
ALTER TABLE question_sets DROP COLUMN IF EXISTS processing_stage;
