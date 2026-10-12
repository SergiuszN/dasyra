-- Check-ins stop after two unanswered ones; replies and reminders (digests) are assistant rows too and must not count
ALTER TABLE conversations ADD COLUMN check_in INTEGER NOT NULL DEFAULT 0;

-- Older check-ins have no flag: they are the assistant rows sent in the first minute of a cron hour (9, 12, 15, 18, 21)
-- that do not answer a user message from the last minutes
UPDATE conversations SET check_in = 1
WHERE role = 'assistant'
  AND strftime('%M', created_at) = '00'
  AND CAST(strftime('%H', created_at) AS INTEGER) IN (9, 12, 15, 18, 21)
  AND NOT EXISTS (
      SELECT 1 FROM conversations u
      WHERE u.user_id = conversations.user_id AND u.role = 'user'
        AND u.created_at < conversations.created_at
        AND u.created_at >= datetime(conversations.created_at, '-10 minutes')
  );
