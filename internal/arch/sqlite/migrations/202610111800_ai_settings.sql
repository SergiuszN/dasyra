-- AI providers with their keys and which provider and model each role runs on, both edited in the admin panel.
-- A role without a row keeps its defaults from llm/models.go, provider_id null is OpenRouter with the key from .env
create table if not exists ai_providers
(
    id         INTEGER      not null primary key autoincrement,
    name       VARCHAR(64)  not null,
    kind       VARCHAR(16)  not null,
    base_url   VARCHAR(255) not null default '',
    api_key    TEXT         not null default '',
    created_at DATETIME     not null
);

create table if not exists ai_roles
(
    role        VARCHAR(32)  not null primary key,
    provider_id INTEGER,
    model       VARCHAR(128) not null default '',
    effort      VARCHAR(16)  not null default ''
);
