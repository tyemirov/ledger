create extension if not exists pgcrypto;
create table if not exists accounts(
  account_id uuid primary key default gen_random_uuid(),
  user_id text not null unique,
  created_at timestamptz not null default now()
);

do $$
begin
  if not exists (select 1 from pg_type where typname = 'ledger_type') then
    create type ledger_type as enum ('grant','hold','reverse_hold','spend');
  end if;
end $$;

do $$
begin
  if not exists (select 1 from pg_type where typname = 'reservation_status') then
    create type reservation_status as enum ('active','captured','released');
  end if;
end $$;

create table if not exists ledger_entries(
  entry_id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(account_id),
  type ledger_type not null,
  amount_cents bigint not null,
  reservation_id text,
  idempotency_key text not null,
  expires_at timestamptz,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique(account_id, idempotency_key)
);

create index if not exists idx_ledger_account_created on ledger_entries(account_id, created_at desc);
create index if not exists idx_ledger_account_reservation on ledger_entries(account_id, reservation_id) where reservation_id is not null;
create index if not exists idx_ledger_active_holds on ledger_entries(account_id) where type='hold';

create table if not exists reservations(
  account_id uuid not null references accounts(account_id),
  reservation_id text not null,
  amount_cents bigint not null,
  status reservation_status not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (account_id, reservation_id)
);

create index if not exists idx_reservations_account_status on reservations(account_id, status);
