CREATE TABLE question_vector_indexes (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    question_set_id VARCHAR(36) NOT NULL,
    question_id VARCHAR(36) NOT NULL,
    embedding_model_id VARCHAR(36) NOT NULL,
    retriever_engine_type VARCHAR(50) NOT NULL,
    index_mode VARCHAR(32) NOT NULL DEFAULT 'question_prompt',
    content_hash VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    error_message TEXT NOT NULL DEFAULT '',
    indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_question_vector_index_target UNIQUE (
        tenant_id, question_id, embedding_model_id, retriever_engine_type, index_mode
    ),
    CONSTRAINT chk_question_vector_index_status CHECK (
        status IN ('pending', 'indexing', 'indexed', 'failed', 'deleted')
    )
);

CREATE INDEX idx_question_vector_indexes_tenant ON question_vector_indexes (tenant_id);
CREATE INDEX idx_question_vector_indexes_kb ON question_vector_indexes (knowledge_base_id);
CREATE INDEX idx_question_vector_indexes_set ON question_vector_indexes (question_set_id);
CREATE INDEX idx_question_vector_indexes_question ON question_vector_indexes (question_id);
CREATE INDEX idx_question_vector_indexes_status ON question_vector_indexes (status);
