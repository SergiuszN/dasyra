-- Prices of models at providers that do not report the cost (USD per million tokens, per minute of audio),
-- set in the admin panel. A call priced from them is marked cost_estimated
create table if not exists ai_prices
(
    provider_id INTEGER      not null,
    model       VARCHAR(128) not null,
    input       REAL         not null default 0,
    cache_read  REAL         not null default 0,
    cache_write REAL         not null default 0,
    output      REAL         not null default 0,
    per_minute  REAL         not null default 0,
    primary key (provider_id, model)
);

alter table ai_usage add column cost_estimated INTEGER not null default 0;
