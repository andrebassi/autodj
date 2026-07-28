-- 016_artwork_url: external cover image URL per track (Spotify album image /
-- YouTube thumb). Empty for local files (their cover is the embedded picture,
-- extracted on demand). Persisting it lets GET /library/tracks/{id}/artwork
-- 302-redirect to the provider CDN instead of 404ing once a Spotify search
-- result is enqueued and dropped into the queue. The spotify catalog sets it
-- (albumImage()); importItems/toPlayable propagate it onto the library.Track.
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS artwork_url TEXT NOT NULL DEFAULT '';
