-- 002_category_check: pin the media category to the fixed domain set. Forward-
-- only; keeps the 'music' default from 001. Mirrors library.Categories() — the
-- domain is still the source of truth (Track.Validate rejects unknowns before a
-- write ever reaches here); this is defense in depth at the DB layer.
ALTER TABLE tracks
    ADD CONSTRAINT category_valid CHECK (category IN (
        'music','jingle','soundtrack','effect',
        'commercial','promo','sponsorship','extra'
    ));
