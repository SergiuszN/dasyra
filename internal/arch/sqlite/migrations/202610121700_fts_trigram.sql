-- Full-text search in any language: the trigram tokenizer finds words as substrings without stemming or stop words
-- (fakturę and faktura share "faktur", German compounds contain their parts), diacritics ignored.
DROP TABLE IF EXISTS memories_fts;
CREATE VIRTUAL TABLE memories_fts USING fts5(
    name,
    category,
    content,
    content='memories',
    content_rowid='id',
    tokenize='trigram remove_diacritics 1'
);
INSERT INTO memories_fts(memories_fts) VALUES ('rebuild');

DROP TABLE IF EXISTS conversations_fts;
CREATE VIRTUAL TABLE conversations_fts USING fts5(
    role,
    content,
    content='conversations',
    content_rowid='id',
    tokenize='trigram remove_diacritics 1'
);
INSERT INTO conversations_fts(conversations_fts) VALUES ('rebuild');

DROP TABLE IF EXISTS notes_fts;
CREATE VIRTUAL TABLE notes_fts USING fts5(
    title,
    tags,
    content,
    content='notes',
    content_rowid='id',
    tokenize='trigram remove_diacritics 1'
);
INSERT INTO notes_fts(notes_fts) VALUES ('rebuild');
