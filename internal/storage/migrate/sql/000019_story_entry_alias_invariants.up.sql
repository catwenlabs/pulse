CREATE OR REPLACE FUNCTION pulse_validate_entry_membership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    entry_uuid uuid;
BEGIN
    IF TG_TABLE_NAME = 'entries' THEN
        IF TG_OP = 'DELETE' THEN
            entry_uuid := OLD.id;
        ELSE
            entry_uuid := NEW.id;
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        entry_uuid := OLD.entry_id;
    ELSE
        entry_uuid := NEW.entry_id;
    END IF;

    -- An Entry that is being deleted no longer needs a membership. This also
    -- allows a final Entry and its Story membership to be removed atomically.
    IF EXISTS (SELECT 1 FROM entries WHERE id = entry_uuid)
       AND (SELECT count(*) FROM story_entries WHERE entry_id = entry_uuid) <> 1 THEN
        RAISE EXCEPTION 'Entry % must belong to exactly one Story', entry_uuid;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS entries_story_membership_trigger ON entries;

CREATE CONSTRAINT TRIGGER entries_story_membership_trigger
AFTER INSERT OR UPDATE ON entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION pulse_validate_entry_membership();

DROP TRIGGER IF EXISTS story_entries_entry_membership_trigger ON story_entries;

CREATE CONSTRAINT TRIGGER story_entries_entry_membership_trigger
AFTER INSERT OR UPDATE OR DELETE ON story_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION pulse_validate_entry_membership();

CREATE OR REPLACE FUNCTION pulse_validate_story_alias()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.alias_id = NEW.canonical_story_id
       OR EXISTS (
           SELECT 1
           FROM story_aliases
           WHERE alias_id = NEW.canonical_story_id
       ) THEN
        RAISE EXCEPTION 'Story alias % must point directly to a live canonical Story', NEW.alias_id;
    END IF;
    RETURN NEW;
END
$$;

CREATE CONSTRAINT TRIGGER story_alias_invariant_trigger
AFTER INSERT OR UPDATE ON story_aliases
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION pulse_validate_story_alias();
