CREATE VIRTUAL TABLE IF NOT EXISTS conversations_fts USING fts5(
    role,
    content,
    content='conversations',
    content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS conversations_fts_ai AFTER INSERT ON conversations BEGIN
    INSERT INTO conversations_fts(rowid, role, content)
    VALUES (new.id, new.role, new.content);
END;

CREATE TRIGGER IF NOT EXISTS conversations_fts_ad AFTER DELETE ON conversations BEGIN
    INSERT INTO conversations_fts(conversations_fts, rowid, role, content)
    VALUES ('delete', old.id, old.role, old.content);
END;

CREATE TRIGGER IF NOT EXISTS conversations_fts_au AFTER UPDATE ON conversations BEGIN
    INSERT INTO conversations_fts(conversations_fts, rowid, role, content)
    VALUES ('delete', old.id, old.role, old.content);
    INSERT INTO conversations_fts(rowid, role, content)
    VALUES (new.id, new.role, new.content);
END;

INSERT INTO conversations_fts(conversations_fts) VALUES ('rebuild');
