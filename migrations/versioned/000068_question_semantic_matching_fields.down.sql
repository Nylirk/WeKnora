DROP INDEX IF EXISTS idx_questions_syllabus_scope_result;
DROP INDEX IF EXISTS idx_questions_syllabus_checking_status;
DROP INDEX IF EXISTS idx_questions_auto_tagging_status;

ALTER TABLE questions
  DROP COLUMN IF EXISTS syllabus_scope_result,
  DROP COLUMN IF EXISTS syllabus_checking_status,
  DROP COLUMN IF EXISTS auto_tagging_status;
