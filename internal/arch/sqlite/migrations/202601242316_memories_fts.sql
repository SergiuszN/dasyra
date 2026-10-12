CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    name,
    category,
    content,
    content='memories',
    content_rowid='id'
);

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

DROP TABLE IF EXISTS memory_keywords;
DROP TABLE IF EXISTS keywords;
