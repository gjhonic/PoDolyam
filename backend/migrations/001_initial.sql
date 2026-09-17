CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 email text UNIQUE NOT NULL,
 password_salt bytea NOT NULL CHECK (octet_length(password_salt)=16),
 password_hash bytea NOT NULL CHECK (octet_length(password_hash)=32),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash)=32),
 user_id uuid REFERENCES users(id) ON DELETE CASCADE,
 csrf text NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE TABLE rate_limits (
 key bytea PRIMARY KEY,
 count integer NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE TABLE meetings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id),
 title text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
 meeting_date date NOT NULL,
 venue text NOT NULL DEFAULT '',
 currency text NOT NULL DEFAULT 'RUB' CHECK (currency='RUB'),
 state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','finalized','closed')),
 version integer NOT NULL DEFAULT 1 CHECK (version>0),
 bill jsonb NOT NULL,
 snapshot jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((state='draft' AND snapshot IS NULL) OR (state<>'draft' AND snapshot IS NOT NULL))
);
CREATE INDEX meetings_owner ON meetings(owner_id,created_at DESC);
CREATE TABLE links (
 token_hash bytea PRIMARY KEY,
 meeting_id uuid NOT NULL REFERENCES meetings(id),
 participant_id text NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE payments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 meeting_id uuid NOT NULL REFERENCES meetings(id),
 participant_id text NOT NULL,
 amount bigint NOT NULL CHECK (amount>0 AND amount<=100000000000),
 created_at timestamptz NOT NULL DEFAULT now(),
 cancelled_at timestamptz,
 cancellation_reason text,
 CHECK ((cancelled_at IS NULL AND cancellation_reason IS NULL) OR
        (cancelled_at IS NOT NULL AND length(cancellation_reason) BETWEEN 1 AND 500))
);
CREATE INDEX payments_meeting ON payments(meeting_id);
-- Снимок защищён и на уровне БД: обычный UPDATE не должен переписать историю.
CREATE FUNCTION protect_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state <> 'draft' AND
   (NEW.snapshot IS DISTINCT FROM OLD.snapshot OR NEW.bill IS DISTINCT FROM OLD.bill
    OR NEW.title IS DISTINCT FROM OLD.title OR NEW.meeting_date IS DISTINCT FROM OLD.meeting_date
    OR NEW.venue IS DISTINCT FROM OLD.venue OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
    OR NEW.state='draft' OR (OLD.state='closed' AND NEW.state<>'closed')) THEN
   RAISE EXCEPTION 'Зафиксированный расчёт неизменяем';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER meetings_immutable BEFORE UPDATE ON meetings FOR EACH ROW EXECUTE FUNCTION protect_snapshot();
