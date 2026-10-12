-- Global settings of the admin panel, JSON by name: the check-in plan
create table if not exists settings
(
    name  VARCHAR(64) not null primary key,
    value TEXT        not null
);
