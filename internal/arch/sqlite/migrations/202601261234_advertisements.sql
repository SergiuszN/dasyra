create table if not exists advertisements (
    id          INTEGER primary key,
    url         TEXT     not null unique,
    is_seen     BOOLEAN  default 0,
    description TEXT,
    parameters  TEXT,
    price       INTEGER,
    localisation VARCHAR(255),
    created_at  DATETIME default CURRENT_TIMESTAMP
);
