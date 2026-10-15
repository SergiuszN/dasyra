-- The recipe server (Mealie) each user's Dasyra works with, connected in the admin panel. No row = no cookbook tools
create table if not exists cookbooks
(
    user_id    INTEGER      not null primary key references users (id),
    base_url   VARCHAR(255) not null,
    public_url VARCHAR(255) not null default '',
    api_token  TEXT         not null,
    updated_at DATETIME     not null
);
