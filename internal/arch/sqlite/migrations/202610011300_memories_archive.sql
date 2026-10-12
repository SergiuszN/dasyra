-- Every deleted or overwritten memory version lands here, so memory upkeep can never lose data
create table if not exists memories_archive
(
    id          INTEGER  not null,
    user_id     INTEGER  not null,
    name        TEXT     not null,
    category    VARCHAR(255) not null,
    content     LONGTEXT not null,
    created_at  DATETIME,
    updated_at  DATETIME,
    archived_at DATETIME not null,
    reason      VARCHAR(16) not null
);

CREATE INDEX IF NOT EXISTS memories_archive_user_id ON memories_archive (user_id, archived_at);
