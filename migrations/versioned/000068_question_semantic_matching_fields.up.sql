-- Add redundant filter columns for question semantic matching results.
-- These mirror extraction_metadata.auto_processing fields for fast list filtering.
ALTER TABLE questions
  ADD COLUMN IF NOT EXISTS auto_tagging_status VARCHAR(16) NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS syllabus_checking_status VARCHAR(16) NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS syllabus_scope_result VARCHAR(16) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_questions_auto_tagging_status
  ON questions(auto_tagging_status);

CREATE INDEX IF NOT EXISTS idx_questions_syllabus_checking_status
  ON questions(syllabus_checking_status);

CREATE INDEX IF NOT EXISTS idx_questions_syllabus_scope_result
  ON questions(syllabus_scope_result);
