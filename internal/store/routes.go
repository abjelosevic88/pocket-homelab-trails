package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

const routeCols = `id, user_id, name, activity, difficulty, region, description, tags, %s, drawn, distance_m, gain_m, loss_m,
  min_ele, max_ele, minutes_set, is_loop, start_lat, start_lng, end_lat, end_lng, start_name, link_url, favourite, archived,
  source, source_ref, created_at, updated_at, preview`

func routeSelect(full bool) string {
	if full {
		return "SELECT " + strings.Replace(routeCols, "%s", "track, controls", 1) + " FROM routes"
	}
	return "SELECT " + strings.Replace(routeCols, "%s", "'[]', '[]'", 1) + " FROM routes"
}

func scanRoute(sc interface{ Scan(...any) error }) (*Route, error) {
	var r Route
	var tags, track, controls, preview string
	var minE, maxE, sLat, sLng, eLat, eLng sql.NullFloat64
	var loop, fav, arch int
	if err := sc.Scan(&r.ID, &r.UserID, &r.Name, &r.Activity, &r.Difficulty, &r.Region, &r.Description, &tags, &track, &controls, &r.Drawn,
		&r.DistanceM, &r.GainM, &r.LossM, &minE, &maxE, &r.MinutesSet, &loop, &sLat, &sLng, &eLat, &eLng, &r.StartName, &r.LinkURL, &fav, &arch,
		&r.Source, &r.SourceRef, &r.CreatedAt, &r.UpdatedAt, &preview); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(tags), &r.Tags)
	if r.Tags == nil {
		r.Tags = []string{}
	}
	_ = json.Unmarshal([]byte(track), &r.Track)
	_ = json.Unmarshal([]byte(controls), &r.Controls)
	_ = json.Unmarshal([]byte(preview), &r.Preview)
	r.MinEle, r.MaxEle = nf(minE), nf(maxE)
	r.StartLat, r.StartLng, r.EndLat, r.EndLng = nf(sLat), nf(sLng), nf(eLat), nf(eLng)
	r.Loop, r.Favourite, r.Archived = loop == 1, fav == 1, arch == 1
	r.Points = len(r.Track)
	return &r, nil
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

func nfv(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// RouteFilter narrows the route list.
type RouteFilter struct {
	Activity string
	Q        string
	Archived bool // the archived ones instead
	IDs      []int64
}

// ListRoutes lists one person's routes, newest change first, without the
// full tracks (the preview line is there).
func (s *Store) ListRoutes(ctx context.Context, uid int64, f RouteFilter) ([]*Route, error) {
	q := routeSelect(false) + ` WHERE user_id = ? AND archived = ?`
	args := []any{uid, boolInt(f.Archived)}
	if f.Activity != "" {
		q += ` AND activity = ?`
		args = append(args, f.Activity)
	}
	if t := strings.TrimSpace(f.Q); t != "" {
		q += ` AND (name LIKE ? OR region LIKE ? OR description LIKE ? OR tags LIKE ? OR start_name LIKE ?)`
		like := "%" + t + "%"
		args = append(args, like, like, like, like, like)
	}
	if len(f.IDs) > 0 {
		q += ` AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(f.IDs)), ",") + `)`
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	q += ` ORDER BY favourite DESC, updated_at DESC`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Route{}
	for rows.Next() {
		r, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.fillRouteUse(ctx, uid, out)
}

// fillRouteUse adds how often each route was done and when it is next.
func (s *Store) fillRouteUse(ctx context.Context, uid int64, rs []*Route) error {
	if len(rs) == 0 {
		return nil
	}
	by := map[int64]*Route{}
	for _, r := range rs {
		by[r.ID] = r
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT route_id,
	    SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END),
	    COALESCE(MAX(CASE WHEN status = 'done' THEN date END), ''),
	    COALESCE(MIN(CASE WHEN status IN ('planned','out') AND date >= ? THEN date END), '')
	  FROM outings WHERE user_id = ? AND route_id IS NOT NULL GROUP BY route_id`, Today(), uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		var last, next string
		if err := rows.Scan(&id, &n, &last, &next); err != nil {
			return err
		}
		if r := by[id]; r != nil {
			r.Done, r.LastDone, r.Next = n, last, next
		}
	}
	return rows.Err()
}

// GetRoute is one route with its track and spots.
func (s *Store) GetRoute(ctx context.Context, uid, id int64) (*Route, error) {
	r, err := scanRoute(s.DB.QueryRowContext(ctx, routeSelect(true)+` WHERE user_id = ? AND id = ?`, uid, id))
	if err != nil {
		return nil, err
	}
	if r.Waypoints, err = s.ListWaypoints(ctx, uid, id); err != nil {
		return nil, err
	}
	return r, s.fillRouteUse(ctx, uid, []*Route{r})
}

// RouteBySource finds a route another app (or an import) made.
func (s *Store) RouteBySource(ctx context.Context, uid int64, source, ref string) (*Route, error) {
	var id int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM routes WHERE user_id = ? AND source = ? AND source_ref = ?`, uid, source, ref).Scan(&id); err != nil {
		return nil, notFound(err)
	}
	return s.GetRoute(ctx, uid, id)
}

func routeArgs(r *Route) []any {
	if r.Tags == nil {
		r.Tags = []string{}
	}
	return []any{r.Name, r.Activity, r.Difficulty, r.Region, r.Description, toJSON(r.Tags), toJSON(nonNilPts(r.Track)), toJSON(nonNilCtl(r.Controls)), toJSON(nonNilCtl(r.Preview)), r.Drawn,
		r.DistanceM, r.GainM, r.LossM, nfv(r.MinEle), nfv(r.MaxEle), r.MinutesSet, boolInt(r.Loop), nfv(r.StartLat), nfv(r.StartLng), nfv(r.EndLat), nfv(r.EndLng),
		r.StartName, r.LinkURL, boolInt(r.Favourite), boolInt(r.Archived), r.Source, r.SourceRef}
}

func nonNilPts(p []Pt) []Pt {
	if p == nil {
		return []Pt{}
	}
	return p
}

func nonNilCtl(c [][]float64) [][]float64 {
	if c == nil {
		return [][]float64{}
	}
	return c
}

// CreateRoute adds a route.
func (s *Store) CreateRoute(ctx context.Context, uid int64, r *Route) error {
	r.UserID = uid
	if r.Source == "" {
		r.Source = "manual"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO routes (user_id, name, activity, difficulty, region, description, tags, track, controls, preview, drawn,
	  distance_m, gain_m, loss_m, min_ele, max_ele, minutes_set, is_loop, start_lat, start_lng, end_lat, end_lng, start_name, link_url, favourite, archived, source, source_ref)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{uid}, routeArgs(r)...)...)
	if err != nil {
		return conflict(err)
	}
	r.ID, _ = res.LastInsertId()
	r.CreatedAt, r.UpdatedAt = Now(), Now()
	return nil
}

// UpdateRoute saves a route.
func (s *Store) UpdateRoute(ctx context.Context, r *Route) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE routes SET name = ?, activity = ?, difficulty = ?, region = ?, description = ?, tags = ?, track = ?, controls = ?, preview = ?, drawn = ?,
	  distance_m = ?, gain_m = ?, loss_m = ?, min_ele = ?, max_ele = ?, minutes_set = ?, is_loop = ?, start_lat = ?, start_lng = ?, end_lat = ?, end_lng = ?,
	  start_name = ?, link_url = ?, favourite = ?, archived = ?, source = ?, source_ref = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		append(routeArgs(r), Now(), r.UserID, r.ID)...)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRoute removes a route (its outings stay, without the route).
func (s *Store) DeleteRoute(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM routes WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- waypoints ----

func scanWaypoint(sc interface{ Scan(...any) error }) (*Waypoint, error) {
	var w Waypoint
	var ele sql.NullFloat64
	if err := sc.Scan(&w.ID, &w.UserID, &w.RouteID, &w.Kind, &w.Name, &w.Note, &w.Lat, &w.Lng, &ele, &w.AtM, &w.PlaceRef, &w.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	w.Ele = nf(ele)
	return &w, nil
}

const wpCols = `id, user_id, route_id, kind, name, note, lat, lng, ele, at_m, place_ref, created_at`

// ListWaypoints lists a route's spots from the start.
func (s *Store) ListWaypoints(ctx context.Context, uid, routeID int64) ([]*Waypoint, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+wpCols+` FROM waypoints WHERE user_id = ? AND route_id = ? ORDER BY at_m, id`, uid, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Waypoint{}
	for rows.Next() {
		w, err := scanWaypoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// AllWaypoints lists every spot of a person (search, the map).
func (s *Store) AllWaypoints(ctx context.Context, uid int64) ([]*Waypoint, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+wpCols+` FROM waypoints WHERE user_id = ? ORDER BY route_id, at_m`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Waypoint{}
	for rows.Next() {
		w, err := scanWaypoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWaypoint is one spot.
func (s *Store) GetWaypoint(ctx context.Context, uid, id int64) (*Waypoint, error) {
	return scanWaypoint(s.DB.QueryRowContext(ctx, `SELECT `+wpCols+` FROM waypoints WHERE user_id = ? AND id = ?`, uid, id))
}

// CreateWaypoint adds a spot (the route must be the person's).
func (s *Store) CreateWaypoint(ctx context.Context, uid int64, w *Waypoint) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM routes WHERE user_id = ? AND id = ?`, uid, w.RouteID).Scan(&n); err != nil || n == 0 {
		return ErrNotFound
	}
	w.UserID = uid
	res, err := s.DB.ExecContext(ctx, `INSERT INTO waypoints (user_id, route_id, kind, name, note, lat, lng, ele, at_m, place_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, w.RouteID, w.Kind, w.Name, w.Note, w.Lat, w.Lng, nfv(w.Ele), w.AtM, w.PlaceRef)
	if err != nil {
		return err
	}
	w.ID, _ = res.LastInsertId()
	w.CreatedAt = Now()
	return nil
}

// UpdateWaypoint saves a spot.
func (s *Store) UpdateWaypoint(ctx context.Context, w *Waypoint) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE waypoints SET kind = ?, name = ?, note = ?, lat = ?, lng = ?, ele = ?, at_m = ?, place_ref = ? WHERE user_id = ? AND id = ?`,
		w.Kind, w.Name, w.Note, w.Lat, w.Lng, nfv(w.Ele), w.AtM, w.PlaceRef, w.UserID, w.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWaypoint removes a spot.
func (s *Store) DeleteWaypoint(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM waypoints WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
