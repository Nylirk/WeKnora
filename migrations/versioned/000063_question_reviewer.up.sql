-- Add reviewer tracking columns to questions table
ALTER TABLE questions ADD COLUMN IF NOT EXISTS reviewed_by VARCHAR(36) NOT NULL DEFAULT '';
ALTER TABLE questions ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
