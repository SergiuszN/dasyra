-- Embeddings of memories, messages and notes in one table, made lazily on search and nightly. Little-endian float32
create table if not exists vectors
(
    kind        VARCHAR(16) not null,
    record_id   INTEGER     not null,
    user_id     INTEGER     not null,
    model       TEXT        not null,
    vector      BLOB        not null,
    embedded_at DATETIME    not null,
    primary key (kind, record_id)
);

CREATE INDEX IF NOT EXISTS vectors_user ON vectors (user_id, kind, model);

INSERT OR IGNORE INTO vectors (kind, record_id, user_id, model, vector, embedded_at)
SELECT 'memory', memory_id, user_id, model, vector, embedded_at FROM memory_vectors;
DROP TRIGGER IF EXISTS memory_vectors_ad;
DROP TABLE IF EXISTS memory_vectors;

CREATE TRIGGER IF NOT EXISTS vectors_memory_ad AFTER DELETE ON memories BEGIN
    DELETE FROM vectors WHERE kind = 'memory' AND record_id = old.id;
END;
CREATE TRIGGER IF NOT EXISTS vectors_message_ad AFTER DELETE ON conversations BEGIN
    DELETE FROM vectors WHERE kind = 'message' AND record_id = old.id;
END;
CREATE TRIGGER IF NOT EXISTS vectors_note_ad AFTER DELETE ON notes BEGIN
    DELETE FROM vectors WHERE kind = 'note' AND record_id = old.id;
END;
