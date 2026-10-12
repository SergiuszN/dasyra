-- Notes are things the user asked to keep as they are (recipes, gift ideas, places), filed in collections
create table if not exists notes
(
    id         INTEGER primary key,
    user_id    INTEGER  not null,
    collection TEXT     not null,
    title      TEXT     not null,
    tags       TEXT     not null default '',
    content    TEXT     not null,
    source     TEXT     not null default '',
    created_at DATETIME not null,
    updated_at DATETIME not null
);

CREATE INDEX IF NOT EXISTS notes_user_collection ON notes (user_id, collection, updated_at);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
    title,
    tags,
    content,
    content='notes',
    content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS notes_fts_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, title, tags, content) VALUES (new.id, new.title, new.tags, new.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_fts_ad AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, tags, content) VALUES ('delete', old.id, old.title, old.tags, old.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_fts_au AFTER UPDATE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, tags, content) VALUES ('delete', old.id, old.title, old.tags, old.content);
    INSERT INTO notes_fts(rowid, title, tags, content) VALUES (new.id, new.title, new.tags, new.content);
END;

-- Every overwritten or deleted note version lands here, like memories_archive
create table if not exists notes_archive
(
    id          INTEGER  not null,
    user_id     INTEGER  not null,
    collection  TEXT     not null,
    title       TEXT     not null,
    tags        TEXT     not null,
    content     TEXT     not null,
    source      TEXT     not null,
    created_at  DATETIME,
    updated_at  DATETIME,
    archived_at DATETIME not null,
    reason      VARCHAR(16) not null
);

CREATE INDEX IF NOT EXISTS notes_archive_user_id ON notes_archive (user_id, archived_at);
