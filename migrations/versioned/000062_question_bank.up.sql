CREATE TABLE question_sets (
    id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL, description TEXT NOT NULL DEFAULT '',
    source_type VARCHAR(32) NOT NULL DEFAULT 'manual',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    question_count INTEGER NOT NULL DEFAULT 0,
    generation_config JSONB NOT NULL DEFAULT '{}',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CHECK (source_type IN ('manual', 'imported', 'generated')),
    CHECK (status IN ('active', 'archived', 'pending'))
);
CREATE INDEX idx_question_sets_tenant ON question_sets (tenant_id, knowledge_base_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_question_sets_kb ON question_sets (knowledge_base_id) WHERE deleted_at IS NULL;

CREATE TABLE questions (
    id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL,
    question_set_id VARCHAR(36) NOT NULL REFERENCES question_sets(id),
    question_type VARCHAR(64) NOT NULL DEFAULT 'single_choice',
    schema_version VARCHAR(16) NOT NULL DEFAULT 'v1',
    stem_text TEXT NOT NULL DEFAULT '',
    question_body JSONB NOT NULL DEFAULT '{}',
    answer_text TEXT NOT NULL DEFAULT '',
    answer_body JSONB NOT NULL DEFAULT '{}',
    analysis_text TEXT NOT NULL DEFAULT '',
    grading_rubric JSONB NOT NULL DEFAULT '{}',
    difficulty VARCHAR(16) NOT NULL DEFAULT 'medium',
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    knowledge_points JSONB NOT NULL DEFAULT '[]',
    tags JSONB NOT NULL DEFAULT '[]',
    source_knowledge_id VARCHAR(36) NOT NULL DEFAULT '',
    evidence_chunk_ids JSONB NOT NULL DEFAULT '[]',
    source_payload JSONB NOT NULL DEFAULT '{}',
    extraction_metadata JSONB NOT NULL DEFAULT '{}',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CHECK (difficulty IN ('easy', 'medium', 'hard')),
    CHECK (status IN ('draft', 'reviewed', 'rejected'))
);
CREATE INDEX idx_questions_set ON questions (tenant_id, question_set_id, sort_order ASC) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_type ON questions (tenant_id, question_set_id, question_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_difficulty ON questions (tenant_id, question_set_id, difficulty) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_status ON questions (tenant_id, question_set_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_knowledge_points ON questions USING gin (knowledge_points) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_tags ON questions USING gin (tags) WHERE deleted_at IS NULL;
