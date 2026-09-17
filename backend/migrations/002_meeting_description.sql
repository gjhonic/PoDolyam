ALTER TABLE meetings ADD COLUMN description text NOT NULL DEFAULT '';

CREATE OR REPLACE FUNCTION protect_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state <> 'draft' AND
   (NEW.snapshot IS DISTINCT FROM OLD.snapshot OR NEW.bill IS DISTINCT FROM OLD.bill
    OR NEW.title IS DISTINCT FROM OLD.title OR NEW.description IS DISTINCT FROM OLD.description
    OR NEW.meeting_date IS DISTINCT FROM OLD.meeting_date OR NEW.venue IS DISTINCT FROM OLD.venue
    OR NEW.owner_id IS DISTINCT FROM OLD.owner_id OR NEW.state='draft'
    OR (OLD.state='closed' AND NEW.state<>'closed')) THEN
   RAISE EXCEPTION 'Зафиксированный расчёт неизменяем';
 END IF;
 RETURN NEW;
END $$;
