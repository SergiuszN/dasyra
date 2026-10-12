create table if not exists keywords
(
    id         INTEGER primary key,
    keyword    VARCHAR(125) not null,
    created_at DATETIME default CURRENT_TIMESTAMP
);

create table if not exists memories
(
    id         INTEGER primary key,
    name       TEXT         not null,
    category   VARCHAR(255) not null,
    content    LONGTEXT     not null,
    created_at DATETIME default CURRENT_TIMESTAMP,
    updated_at DATETIME default CURRENT_TIMESTAMP
);

create table if not exists memory_keywords
(
    memory_id  INTEGER not null,
    keyword_id INTEGER not null,
    primary key (memory_id, keyword_id),
    foreign key (memory_id) references memories (id) on delete cascade,
    foreign key (keyword_id) references keywords (id) on delete cascade
);

create table if not exists conversations
(
    id                  INTEGER primary key,
    role                TEXT not null,
    content             TEXT not null,
    telegram_message_id INTEGER,
    created_at          DATETIME default CURRENT_TIMESTAMP
);

create table calendar_events
(
    id                  INTEGER primary key,
    event               TEXT     not null,
    start_time          DATETIME not null,
    end_time            DATETIME,
    location            TEXT,
    repeat_interval     TEXT,
    recurrence_end_date DATETIME
);



