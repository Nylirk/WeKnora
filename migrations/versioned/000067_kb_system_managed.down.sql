ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS parent_knowledge_base_id;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS purpose;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS visibility;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS system_managed;
