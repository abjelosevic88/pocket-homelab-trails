# API

Everything the web app does goes through `/api/v1`, with a session cookie or `Authorization: Bearer ptr_…` (Settings → API & shortcuts). Distances and heights are in **metres**, weights in **grams**, times in minutes (or seconds where named `_s`). A track is a list of `[lat, lng, ele|null]` (walked tracks add a unix time). Every row belongs to the person signed in; another person's ids answer 404.

## Routes
| | |
|---|---|
| `GET /routes` (`?activity=hike&q=…&archived=1`) | Routes without their tracks: numbers, time at your speed (`minutes`), difficulty (`level`), a thin `preview` line, how often done and when next |
| `GET /routes/{id}` | `{route, outings}`: the track, the spots along it (`waypoints`, with `at_m`), the outings on it (`?plain=1`: the route only) |
| `POST /routes` | `{name, activity, track, waypoints?, controls?, drawn?, source_ref?, fill_heights?}` — numbers are worked out; a `source_ref` seen before answers the same route |
| `PUT /routes/{id}` · `DELETE /routes/{id}` | change (a new `track` re-measures and re-places the spots) · delete (its outings stay) |
| `POST /routes/import` | GPX, KML, TCX or GeoJSON: a multipart form with `file` parts, or the file as the body with `?name=walk.gpx`; `?activity=` overrides; the same file twice → the same routes |
| `POST /routes/{id}/reverse` | the other way round |
| `GET /routes/{id}/gpx` | GPX 1.1 with the spots |
| `GET /routes/{id}/places` | your Pocket Places within 1.5 km of the way |
| `POST /routes/draw` | `{controls: [[lat,lng],…], mode: straight\|paths, activity}` → the track with heights and its numbers (nothing saved) |
| `POST /routes/from-movement` | `{start_at, end_at, name?, activity?}` → a route from what Pocket Movement recorded |
| `GET /movement/day?date=` | the day's recorded tracks in Pocket Movement |
| `POST /routes/{id}/waypoints` · `PUT /waypoints/{id}` · `DELETE /waypoints/{id}` | spots: `{kind, name, note, lat, lng}` (kinds: water hut food shelter camp summit viewpoint lake parking transport junction danger photo other) |
| `POST /waypoints/{id}/place` | save a spot in Pocket Places |
| `GET /lookup/places?q=` | find a mountain, a village, a car park (OpenStreetMap) |

## Outings
| | |
|---|---|
| `GET /outings` (`?status=planned,out&from=&to=&route_id=&q=&order=desc&limit=`) | outings with their route's numbers and packing counts |
| `GET /outings/{id}` | `{outing, route, advice, sunrise, sunset, finish, weather_note, links, suggest_back_by}` — the forecast is fetched when older than an hour |
| `POST /outings` | `{route_id?, date, start_time?, title?, activity?, people?, notes?, back_by?: "YYYY-MM-DDTHH:MM", safety?, contact_name?, contact_email?, alert_after?, trip_ref?, pack_id?, source_ref?}` |
| `PUT /outings/{id}` · `DELETE /outings/{id}` | change (rating, conditions, report too) · delete |
| `POST /outings/{id}/status/{start\|back\|done\|cancel\|plan}` | setting off, I'm back, it happened, called off, planned again |
| `POST /outings/{id}/weather` | the forecast again now |
| `POST /outings/{id}/send-plan` | `{message?, preview?}` emails the plan to the contact through Pocket Mail |
| `POST /outings/{id}/track/movement` · `POST /outings/{id}/track` · `DELETE /outings/{id}/track` | the walked track from Pocket Movement · from a file · forget it |
| `POST /outings/{id}/track/route` · `GET /outings/{id}/gpx` | the walked track as a new route · as GPX |
| `POST /outings/{id}/make/{health\|notes\|tasks\|calendar}` | into Pocket Health, Pocket Notes, gear to buy into Pocket Tasks, the event into Pocket Calendar |
| `POST /outings/{id}/items` · `POST /outings/{id}/items/all` · `PUT /items/{id}` · `DELETE /items/{id}` | the packing list: `{gear_id}`, `{pack_id}` or `{name, category, weight_g}`; `{packed}` for all |

## Gear
| | |
|---|---|
| `GET /gear` · `POST /gear` · `PUT /gear/{id}` · `DELETE /gear/{id}` | `{name, category, weight_g, worn, consumable, note}` |
| `GET /packs` · `POST /packs` · `PUT /packs/{id}` · `DELETE /packs/{id}` | packing lists `{name, activity, items: [{gear_id, qty}]}` with their weights |
| `POST /gear/import` | a LighterPack CSV (or any CSV with Name, Weight, Unit, Category) → gear and a packing list |
| `POST /gear/from-inventory` | `{items: [{id, name}]}` from Pocket Inventory |

## Overviews
| | |
|---|---|
| `GET /today` · `GET /summary` | Today's page · the small picture (out now, next, the year) for Homepage, Pocket Day and AI |
| `GET /stats?year=` | the year: totals, by month, by activity, summits, the longest |
| `GET /day/{YYYY-MM-DD}` | the outings of a day, for a journal |
| `GET /search?q=` | routes, outings, gear and spots |

## Pocket Core links

Another app acting for you with a `pcx_` token reaches only: `trails:read` — `GET summary, today, stats, routes, routes/{id}, routes/{id}/gpx, outings, outings/{id}, gear, packs, search, day/{date}`; `trails:write` — `POST /outings`, `POST /routes`, `PUT`/`DELETE` of the outings and routes it added itself. It never arms “tell someone”.

## Everything else
`/auth/*`, `/settings`, `/tokens`, `/users`, `/apps`, `/export.json`, `/export.csv`, `/import` (a Pocket Trails export), `/backup` (admins), `/ai` (`{want: pack|report|describe, outing_id|route_id}`), `/healthz`, `/metrics`.
