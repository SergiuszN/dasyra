-- repeat_interval was free text written by the model, now it is a list of weekdays like "mon,wed"
UPDATE calendar_events SET repeat_interval = 'wed,sun' WHERE repeat_interval = 'every Wednesday, Sunday';
UPDATE calendar_events SET repeat_interval = 'mon,fri' WHERE repeat_interval = 'every Monday, Friday';
UPDATE calendar_events SET repeat_interval = 'wed' WHERE repeat_interval = 'every Wednesday';
UPDATE calendar_events SET repeat_interval = 'mon,tue,wed,thu,fri' WHERE repeat_interval LIKE '%будн%' OR lower(repeat_interval) LIKE '%weekday%';
UPDATE calendar_events SET repeat_interval = 'mon,tue,wed,thu,fri,sat,sun' WHERE repeat_interval IN ('daily', 'every day') OR repeat_interval LIKE 'каждый день%';

-- Anything else repeats weekly on the weekday it started
UPDATE calendar_events
SET repeat_interval = CASE strftime('%w', start_time)
    WHEN '0' THEN 'sun' WHEN '1' THEN 'mon' WHEN '2' THEN 'tue' WHEN '3' THEN 'wed'
    WHEN '4' THEN 'thu' WHEN '5' THEN 'fri' ELSE 'sat' END
WHERE repeat_interval IS NOT NULL
  AND trim(replace(replace(replace(replace(replace(replace(replace(repeat_interval,
      'mon', ''), 'tue', ''), 'wed', ''), 'thu', ''), 'fri', ''), 'sat', ''), 'sun', ''), ',') != '';

-- Occurrences that already passed are not reminded about
UPDATE calendar_events SET reminded_at = datetime('now', 'localtime') WHERE repeat_interval IS NOT NULL;
