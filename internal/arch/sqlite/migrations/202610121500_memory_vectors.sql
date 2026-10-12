-- Embeddings of memories for search by meaning, made lazily on search: a vector older than its memory or made by
-- another model is made again. Little-endian float32
create table if not exists memory_vectors
(
    memory_id   INTEGER  not null primary key,
    user_id     INTEGER  not null,
    model       TEXT     not null,
    vector      BLOB     not null,
    embedded_at DATETIME not null
);

CREATE INDEX IF NOT EXISTS memory_vectors_user ON memory_vectors (user_id, model);

CREATE TRIGGER IF NOT EXISTS memory_vectors_ad AFTER DELETE ON memories BEGIN
    DELETE FROM memory_vectors WHERE memory_id = old.id;
END;
