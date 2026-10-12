-- Tool calls behind each Dasyra reply, so a follow-up question can reuse them instead of searching again. Kept for 2 days
create table if not exists tool_calls
(
    id              INTEGER  not null primary key autoincrement,
    user_id         INTEGER  not null,
    conversation_id INTEGER  not null,
    tool            VARCHAR(64) not null,
    args            TEXT     not null,
    result          LONGTEXT not null,
    created_at      DATETIME not null
);

CREATE INDEX IF NOT EXISTS tool_calls_user_conversation ON tool_calls (user_id, conversation_id);
