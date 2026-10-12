ALTER TABLE advertisements ADD COLUMN is_recommended BOOLEAN DEFAULT 0;
ALTER TABLE advertisements ADD COLUMN confidence REAL;
ALTER TABLE advertisements ADD COLUMN comment TEXT;
ALTER TABLE advertisements DROP COLUMN is_seen;
