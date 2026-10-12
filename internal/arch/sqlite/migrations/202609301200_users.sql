create table if not exists users
(
    id               INTEGER primary key,
    name             TEXT,
    telegram_chat_id INTEGER unique,
    profile          TEXT,
    created_at       DATETIME default CURRENT_TIMESTAMP
);

create table if not exists invites
(
    code       VARCHAR(32) primary key,
    user_id    INTEGER,
    created_at DATETIME default CURRENT_TIMESTAMP,
    used_at    DATETIME
);

-- Owner account: all existing data belongs to it, telegram_chat_id is bound on first contact from TELEGRAM_CHAT_ID,
-- the empty profile starts onboarding
INSERT INTO users (id, name, telegram_chat_id, profile)
VALUES (1, NULL, NULL, NULL);

ALTER TABLE memories ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE conversations ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE calendar_events ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS memories_user_id ON memories (user_id);
CREATE INDEX IF NOT EXISTS conversations_user_id_created_at ON conversations (user_id, created_at);
CREATE INDEX IF NOT EXISTS conversations_user_id_telegram_message_id ON conversations (user_id, telegram_message_id);
CREATE INDEX IF NOT EXISTS calendar_events_user_id ON calendar_events (user_id);

-- Restore memories FTS triggers (missing in some databases) and reindex
CREATE TRIGGER IF NOT EXISTS memories_fts_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, name, category, content)
    VALUES (new.id, new.name, new.category, new.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_fts_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, name, category, content)
    VALUES('delete', old.id, old.name, old.category, old.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_fts_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, name, category, content)
    VALUES('delete', old.id, old.name, old.category, old.content);
    INSERT INTO memories_fts(rowid, name, category, content)
    VALUES (new.id, new.name, new.category, new.content);
END;

INSERT INTO memories_fts(memories_fts) VALUES('rebuild');
