CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);



-- ---------- Pocket Trails: everything below is per person ----------
-- Distances in metres, heights in metres, weights in grams, times in
-- minutes or seconds; the UI converts to miles/feet/ounces when asked.

-- A route: a hike, a walk, a run or a ride, drawn or brought from a GPX.
-- track is [[lat, lng, ele|null], …] (thinned to a few thousand points);
-- controls are the points a drawn route was drawn through.
CREATE TABLE routes (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL DEFAULT '',
  activity    TEXT NOT NULL DEFAULT 'hike',   -- hike | walk | run | bike | gravel | mtb | ski | other
  difficulty  TEXT NOT NULL DEFAULT '',       -- '' = worked out | easy | moderate | hard | expert
  region      TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  tags        TEXT NOT NULL DEFAULT '[]',
  track       TEXT NOT NULL DEFAULT '[]',
  controls    TEXT NOT NULL DEFAULT '[]',
  preview     TEXT NOT NULL DEFAULT '[]',     -- a thin line for lists and the map
  drawn       TEXT NOT NULL DEFAULT '',       -- '' (not drawn) | straight | paths
  distance_m  REAL NOT NULL DEFAULT 0,
  gain_m      REAL NOT NULL DEFAULT 0,
  loss_m      REAL NOT NULL DEFAULT 0,
  min_ele     REAL,
  max_ele     REAL,
  minutes_set INTEGER NOT NULL DEFAULT 0,     -- the person's own time; 0 = worked out
  is_loop     INTEGER NOT NULL DEFAULT 0,
  start_lat   REAL,
  start_lng   REAL,
  end_lat     REAL,
  end_lng     REAL,
  start_name  TEXT NOT NULL DEFAULT '',       -- the trailhead or car park
  link_url    TEXT NOT NULL DEFAULT '',       -- where it came from (Komoot, AllTrails…)
  favourite   INTEGER NOT NULL DEFAULT 0,
  archived    INTEGER NOT NULL DEFAULT 0,
  source      TEXT NOT NULL DEFAULT 'manual', -- manual | gpx | drawn | movement | <app slug>
  source_ref  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX routes_user ON routes(user_id, archived, updated_at);
CREATE UNIQUE INDEX routes_source ON routes(user_id, source, source_ref) WHERE source_ref <> '';

-- Along a route: water, a hut, a summit, the car park, a tricky bit.
-- at_m is how far along the route it is.
CREATE TABLE waypoints (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  route_id  INTEGER NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
  kind      TEXT NOT NULL DEFAULT 'other',
  name      TEXT NOT NULL DEFAULT '',
  note      TEXT NOT NULL DEFAULT '',
  lat       REAL NOT NULL,
  lng       REAL NOT NULL,
  ele       REAL,
  at_m      REAL NOT NULL DEFAULT 0,
  place_ref TEXT NOT NULL DEFAULT '',          -- the same spot in Pocket Places
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX waypoints_route ON waypoints(user_id, route_id, at_m);

-- Your gear: what it is and what it weighs.
CREATE TABLE gear (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          TEXT NOT NULL DEFAULT '',
  category      TEXT NOT NULL DEFAULT 'other',
  weight_g      INTEGER NOT NULL DEFAULT 0,
  worn          INTEGER NOT NULL DEFAULT 0,     -- on you, not in the pack
  consumable    INTEGER NOT NULL DEFAULT 0,     -- food, water, fuel
  note          TEXT NOT NULL DEFAULT '',
  inventory_ref TEXT NOT NULL DEFAULT '',       -- the item in Pocket Inventory
  archived      INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX gear_user ON gear(user_id, archived, category, name);

-- A packing list to start from ("Day hike in summer", "Hut to hut").
CREATE TABLE packs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  activity   TEXT NOT NULL DEFAULT '',        -- '' = any
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX packs_user ON packs(user_id, name);

CREATE TABLE pack_items (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pack_id INTEGER NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  gear_id INTEGER NOT NULL REFERENCES gear(id) ON DELETE CASCADE,
  qty     INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (pack_id, gear_id)
);
CREATE INDEX pack_items_user ON pack_items(user_id, pack_id);

-- An outing: a route on a day (planned, out now, done or called off), who
-- comes, the weather, the time to be back by and who to tell, and
-- afterwards how it went — with the track you really walked.
CREATE TABLE outings (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  route_id      INTEGER REFERENCES routes(id) ON DELETE SET NULL,
  title         TEXT NOT NULL DEFAULT '',
  activity      TEXT NOT NULL DEFAULT 'hike',
  date          TEXT NOT NULL,                   -- YYYY-MM-DD
  start_time    TEXT NOT NULL DEFAULT '',        -- HH:MM
  status        TEXT NOT NULL DEFAULT 'planned', -- planned | out | done | cancelled
  people        TEXT NOT NULL DEFAULT '[]',      -- [{name, email, ref}]
  notes         TEXT NOT NULL DEFAULT '',
  -- safety
  back_by       TEXT NOT NULL DEFAULT '',        -- local 'YYYY-MM-DDTHH:MM'
  safety        INTEGER NOT NULL DEFAULT 0,      -- tell someone if not back
  contact_name  TEXT NOT NULL DEFAULT '',
  contact_email TEXT NOT NULL DEFAULT '',
  contact_ref   TEXT NOT NULL DEFAULT '',
  alert_after   INTEGER NOT NULL DEFAULT 60,     -- minutes after back_by
  plan_sent_at  TEXT NOT NULL DEFAULT '',
  warned_at     TEXT NOT NULL DEFAULT '',        -- you were reminded
  alerted_at    TEXT NOT NULL DEFAULT '',        -- the contact was told
  alert_error   TEXT NOT NULL DEFAULT '',
  started_at    TEXT NOT NULL DEFAULT '',
  ended_at      TEXT NOT NULL DEFAULT '',
  -- the day
  weather       TEXT NOT NULL DEFAULT '',        -- JSON snapshot
  trip_ref      TEXT NOT NULL DEFAULT '',        -- Pocket Travel trip
  trip_name     TEXT NOT NULL DEFAULT '',
  cal_ref       TEXT NOT NULL DEFAULT '',        -- the event in Pocket Calendar
  -- afterwards
  rating        INTEGER NOT NULL DEFAULT 0,
  conditions    TEXT NOT NULL DEFAULT '',        -- trail conditions, for next time
  report        TEXT NOT NULL DEFAULT '',
  actual_track  TEXT NOT NULL DEFAULT '[]',      -- [[lat, lng, ele|null, unix], …]
  actual        TEXT NOT NULL DEFAULT '',        -- JSON: the comparison
  actual_source TEXT NOT NULL DEFAULT '',        -- '' | movement | gpx
  health_ref    TEXT NOT NULL DEFAULT '',
  note_ref      TEXT NOT NULL DEFAULT '',
  source        TEXT NOT NULL DEFAULT 'manual',
  source_ref    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX outings_user ON outings(user_id, date, status);
CREATE INDEX outings_route ON outings(user_id, route_id, date);
CREATE UNIQUE INDEX outings_source ON outings(user_id, source, source_ref) WHERE source_ref <> '';

-- What to pack for one outing (copied from a packing list, then ticked).
CREATE TABLE outing_items (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  outing_id INTEGER NOT NULL REFERENCES outings(id) ON DELETE CASCADE,
  gear_id   INTEGER REFERENCES gear(id) ON DELETE SET NULL,
  name      TEXT NOT NULL DEFAULT '',
  category  TEXT NOT NULL DEFAULT 'other',
  weight_g  INTEGER NOT NULL DEFAULT 0,
  qty       INTEGER NOT NULL DEFAULT 1,
  worn      INTEGER NOT NULL DEFAULT 0,
  consumable INTEGER NOT NULL DEFAULT 0,
  packed    INTEGER NOT NULL DEFAULT 0,
  to_buy    INTEGER NOT NULL DEFAULT 0,
  task_ref  TEXT NOT NULL DEFAULT '',          -- "buy it" in Pocket Tasks
  position  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX outing_items_outing ON outing_items(user_id, outing_id, position);
