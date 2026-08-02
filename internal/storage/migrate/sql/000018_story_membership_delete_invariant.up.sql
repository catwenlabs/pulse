CREATE OR REPLACE FUNCTION pulse_validate_story_membership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        PERFORM pulse_validate_story(OLD.story_id);
    END IF;
    IF TG_OP <> 'DELETE' THEN
        PERFORM pulse_validate_story(NEW.story_id);
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS story_entries_invariant_trigger ON story_entries;

CREATE CONSTRAINT TRIGGER story_entries_invariant_trigger
AFTER INSERT OR UPDATE OR DELETE ON story_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION pulse_validate_story_membership();
