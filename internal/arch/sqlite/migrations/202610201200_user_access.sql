-- A user turned off in the admin panel keeps their chat and data, Dasyra stops answering and writing to them
ALTER TABLE users ADD COLUMN disabled_at DATETIME;
-- Whom an invite link was made for, set in the admin panel
ALTER TABLE invites ADD COLUMN note TEXT NOT NULL DEFAULT '';
