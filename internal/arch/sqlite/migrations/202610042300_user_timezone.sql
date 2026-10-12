-- Calendar times are the user's wall-clock time in this zone, message timestamps stay in the server zone
ALTER TABLE users ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';
