ALTER TABLE calendar_events ADD COLUMN reminded_at DATETIME;

-- Empty strings came from models filling optional fields, an empty repeat_interval made events look recurring
UPDATE calendar_events SET repeat_interval = NULL WHERE repeat_interval = '';
UPDATE calendar_events SET location = NULL WHERE location = '';

-- Events that already passed are not reminded about
UPDATE calendar_events SET reminded_at = datetime('now', 'localtime') WHERE start_time <= datetime('now', 'localtime');

CREATE INDEX IF NOT EXISTS calendar_events_reminders ON calendar_events (reminded_at, start_time);
