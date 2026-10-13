-- Check-ins and reminders got their own roles: they start with what the chat was set to
insert or ignore into ai_roles (role, provider_id, model, effort)
select 'dasyra_check_in', provider_id, model, effort from ai_roles where role = 'dasyra';
insert or ignore into ai_roles (role, provider_id, model, effort)
select 'dasyra_reminder', provider_id, model, effort from ai_roles where role = 'dasyra';
