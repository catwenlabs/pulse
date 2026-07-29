CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;

CREATE FUNCTION pulse_fuzzy_contains(haystack text, needle text)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN char_length(needle) < 3 OR char_length(needle) > 64 THEN false
        ELSE EXISTS (
            SELECT 1
            FROM generate_series(
                1,
                greatest(char_length(haystack) - char_length(needle) + 1, 1)
            ) AS position
            WHERE levenshtein_less_equal(
                lower(needle),
                lower(substring(haystack FROM position FOR char_length(needle))),
                1
            ) <= 1
        )
    END
$$;
