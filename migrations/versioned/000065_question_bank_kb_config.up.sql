ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS question_bank_config JSONB NOT NULL DEFAULT '{}';
