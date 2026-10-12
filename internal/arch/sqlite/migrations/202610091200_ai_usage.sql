-- Every paid AI call (OpenRouter chat and transcription) with its tokens and cost in USD, for the admin panel.
-- user_id is null for calls not made on a user's behalf
create table if not exists ai_usage
(
    id                 INTEGER      not null primary key autoincrement,
    user_id            INTEGER,
    purpose            VARCHAR(32)  not null,
    model              VARCHAR(128) not null,
    prompt_tokens      INTEGER      not null default 0,
    cached_tokens      INTEGER      not null default 0,
    cache_write_tokens INTEGER      not null default 0,
    completion_tokens  INTEGER      not null default 0,
    reasoning_tokens   INTEGER      not null default 0,
    audio_seconds      REAL         not null default 0,
    cost               REAL         not null default 0,
    created_at         DATETIME     not null
);

CREATE INDEX IF NOT EXISTS ai_usage_created_at ON ai_usage (created_at);
CREATE INDEX IF NOT EXISTS ai_usage_user_created_at ON ai_usage (user_id, created_at);
