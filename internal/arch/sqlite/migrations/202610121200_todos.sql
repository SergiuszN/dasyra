-- The user's to-do list: day is the user's calendar date it is planned for ('' for no day), done_at server time
create table if not exists todos
(
    id         INTEGER  not null primary key,
    user_id    INTEGER  not null,
    text       TEXT     not null,
    day        TEXT     not null default '',
    done_at    DATETIME,
    created_at DATETIME not null
);

CREATE INDEX IF NOT EXISTS todos_user ON todos (user_id, done_at);
