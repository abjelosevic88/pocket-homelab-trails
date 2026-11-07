package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

const outingCols = `o.id, o.user_id, o.route_id, o.title, o.activity, o.date, o.start_time, o.status, o.people, o.notes,
  o.back_by, o.safety, o.contact_name, o.contact_email, o.contact_ref, o.alert_after, o.plan_sent_at, o.warned_at, o.alerted_at, o.alert_error,
  o.started_at, o.ended_at, o.weather, o.trip_ref, o.trip_name, o.cal_ref, o.rating, o.conditions, o.report, %s, o.actual, o.actual_source,
  o.health_ref, o.note_ref, o.source, o.source_ref, o.created_at, o.updated_at,
  COALESCE(r.name, ''), COALESCE(r.distance_m, 0), COALESCE(r.gain_m, 0), COALESCE(r.loss_m, 0), COALESCE(r.minutes_set, 0), COALESCE(r.activity, ''), r.start_lat, r.start_lng,
  (SELECT COUNT(1) FROM outing_items i WHERE i.outing_id = o.id), (SELECT COUNT(1) FROM outing_items i WHERE i.outing_id = o.id AND i.packed = 1)`

func outingSelect(full bool) string {
	tr := "'[]'"
	if full {
		tr = "o.actual_track"
	}
	return "SELECT " + strings.Replace(outingCols, "%s", tr, 1) + " FROM outings o LEFT JOIN routes r ON r.id = o.route_id AND r.user_id = o.user_id"
}

// RouteFacts are the route numbers an outing list needs to work out times.
type RouteFacts struct {
	LossM      float64
	MinutesSet int
	Activity   string
}

func scanOuting(sc interface{ Scan(...any) error }) (*Outing, RouteFacts, error) {
	var o Outing
	var rf RouteFacts
	var route sql.NullInt64
	var people, weather, track, actual string
	var safety int
	var sLat, sLng sql.NullFloat64
	if err := sc.Scan(&o.ID, &o.UserID, &route, &o.Title, &o.Activity, &o.Date, &o.StartTime, &o.Status, &people, &o.Notes,
		&o.BackBy, &safety, &o.ContactName, &o.ContactEmail, &o.ContactRef, &o.AlertAfter, &o.PlanSentAt, &o.WarnedAt, &o.AlertedAt, &o.AlertError,
		&o.StartedAt, &o.EndedAt, &weather, &o.TripRef, &o.TripName, &o.CalRef, &o.Rating, &o.Conditions, &o.Report, &track, &actual, &o.ActualSource,
		&o.HealthRef, &o.NoteRef, &o.Source, &o.SourceRef, &o.CreatedAt, &o.UpdatedAt,
		&o.RouteName, &o.DistanceM, &o.GainM, &rf.LossM, &rf.MinutesSet, &rf.Activity, &sLat, &sLng, &o.ItemCount, &o.PackedCount); err != nil {
		return nil, rf, notFound(err)
	}
	if route.Valid {
		id := route.Int64
		o.RouteID = &id
	}
	o.StartLat, o.StartLng = nf(sLat), nf(sLng)
	o.Safety = safety == 1
	_ = json.Unmarshal([]byte(people), &o.People)
	if o.People == nil {
		o.People = []Person{}
	}
	if weather != "" && json.Valid([]byte(weather)) {
		o.Weather = json.RawMessage(weather)
	}
	if actual != "" && json.Valid([]byte(actual)) {
		o.Actual = json.RawMessage(actual)
	}
	_ = json.Unmarshal([]byte(track), &o.ActualTrack)
	return &o, rf, nil
}

// OutingFilter narrows the outing list.
type OutingFilter struct {
	From, To string // dates, inclusive
	Status   []string
	RouteID  int64
	TripRef  string
	Q        string
	Limit    int
	Desc     bool
}

// ListOutings lists one person's outings (without the walked tracks).
func (s *Store) ListOutings(ctx context.Context, uid int64, f OutingFilter) ([]*Outing, []RouteFacts, error) {
	q := outingSelect(false) + ` WHERE o.user_id = ?`
	args := []any{uid}
	if f.From != "" {
		q += ` AND o.date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND o.date <= ?`
		args = append(args, f.To)
	}
	if len(f.Status) > 0 {
		q += ` AND o.status IN (` + strings.TrimSuffix(strings.Repeat("?,", len(f.Status)), ",") + `)`
		for _, st := range f.Status {
			args = append(args, st)
		}
	}
	if f.RouteID > 0 {
		q += ` AND o.route_id = ?`
		args = append(args, f.RouteID)
	}
	if f.TripRef != "" {
		q += ` AND o.trip_ref = ?`
		args = append(args, f.TripRef)
	}
	if t := strings.TrimSpace(f.Q); t != "" {
		like := "%" + t + "%"
		q += ` AND (o.title LIKE ? OR o.notes LIKE ? OR o.conditions LIKE ? OR o.report LIKE ? OR o.people LIKE ? OR r.name LIKE ?)`
		args = append(args, like, like, like, like, like, like)
	}
	if f.Desc {
		q += ` ORDER BY o.date DESC, o.start_time DESC, o.id DESC`
	} else {
		q += ` ORDER BY o.date, o.start_time, o.id`
	}
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []*Outing{}
	facts := []RouteFacts{}
	for rows.Next() {
		o, rf, err := scanOuting(rows)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, o)
		facts = append(facts, rf)
	}
	return out, facts, rows.Err()
}

// GetOuting is one outing with what to pack and its walked track.
func (s *Store) GetOuting(ctx context.Context, uid, id int64) (*Outing, RouteFacts, error) {
	o, rf, err := scanOuting(s.DB.QueryRowContext(ctx, outingSelect(true)+` WHERE o.user_id = ? AND o.id = ?`, uid, id))
	if err != nil {
		return nil, rf, err
	}
	o.Items, err = s.ListOutingItems(ctx, uid, id)
	return o, rf, err
}

// OutingBySource finds an outing another app made.
func (s *Store) OutingBySource(ctx context.Context, uid int64, source, ref string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM outings WHERE user_id = ? AND source = ? AND source_ref = ?`, uid, source, ref).Scan(&id)
	return id, notFound(err)
}

func outingArgs(o *Outing) []any {
	if o.People == nil {
		o.People = []Person{}
	}
	var route any
	if o.RouteID != nil && *o.RouteID > 0 {
		route = *o.RouteID
	}
	return []any{route, o.Title, o.Activity, o.Date, o.StartTime, o.Status, toJSON(o.People), o.Notes,
		o.BackBy, boolInt(o.Safety), o.ContactName, o.ContactEmail, o.ContactRef, o.AlertAfter, o.PlanSentAt, o.WarnedAt, o.AlertedAt, o.AlertError,
		o.StartedAt, o.EndedAt, string(o.Weather), o.TripRef, o.TripName, o.CalRef, o.Rating, o.Conditions, o.Report, toJSON(nonNilPts(o.ActualTrack)),
		string(o.Actual), o.ActualSource, o.HealthRef, o.NoteRef, o.Source, o.SourceRef}
}

// CreateOuting adds an outing.
func (s *Store) CreateOuting(ctx context.Context, uid int64, o *Outing) error {
	o.UserID = uid
	if o.Source == "" {
		o.Source = "manual"
	}
	if o.RouteID != nil {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM routes WHERE user_id = ? AND id = ?`, uid, *o.RouteID).Scan(&n); err != nil || n == 0 {
			return ErrNotFound
		}
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO outings (user_id, route_id, title, activity, date, start_time, status, people, notes,
	  back_by, safety, contact_name, contact_email, contact_ref, alert_after, plan_sent_at, warned_at, alerted_at, alert_error,
	  started_at, ended_at, weather, trip_ref, trip_name, cal_ref, rating, conditions, report, actual_track, actual, actual_source, health_ref, note_ref, source, source_ref)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{uid}, outingArgs(o)...)...)
	if err != nil {
		return conflict(err)
	}
	o.ID, _ = res.LastInsertId()
	return nil
}

// UpdateOuting saves an outing (not its packing list).
func (s *Store) UpdateOuting(ctx context.Context, o *Outing) error {
	if o.RouteID != nil {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM routes WHERE user_id = ? AND id = ?`, o.UserID, *o.RouteID).Scan(&n); err != nil || n == 0 {
			return ErrNotFound
		}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE outings SET route_id = ?, title = ?, activity = ?, date = ?, start_time = ?, status = ?, people = ?, notes = ?,
	  back_by = ?, safety = ?, contact_name = ?, contact_email = ?, contact_ref = ?, alert_after = ?, plan_sent_at = ?, warned_at = ?, alerted_at = ?, alert_error = ?,
	  started_at = ?, ended_at = ?, weather = ?, trip_ref = ?, trip_name = ?, cal_ref = ?, rating = ?, conditions = ?, report = ?, actual_track = ?, actual = ?, actual_source = ?,
	  health_ref = ?, note_ref = ?, source = ?, source_ref = ?, updated_at = ? WHERE user_id = ? AND id = ?`, append(outingArgs(o), Now(), o.UserID, o.ID)...)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOuting removes an outing and its packing list.
func (s *Store) DeleteOuting(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM outings WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- what to pack ----

const itemCols = `id, user_id, outing_id, gear_id, name, category, weight_g, qty, worn, consumable, packed, to_buy, task_ref, position`

func scanItem(sc interface{ Scan(...any) error }) (*OutingItem, error) {
	var it OutingItem
	var gear sql.NullInt64
	var worn, cons, packed, buy int
	if err := sc.Scan(&it.ID, &it.UserID, &it.OutingID, &gear, &it.Name, &it.Category, &it.WeightG, &it.Qty, &worn, &cons, &packed, &buy, &it.TaskRef, &it.Position); err != nil {
		return nil, notFound(err)
	}
	if gear.Valid {
		g := gear.Int64
		it.GearID = &g
	}
	it.Worn, it.Consumable, it.Packed, it.ToBuy = worn == 1, cons == 1, packed == 1, buy == 1
	return &it, nil
}

// ListOutingItems is an outing's packing list, in order.
func (s *Store) ListOutingItems(ctx context.Context, uid, outingID int64) ([]*OutingItem, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM outing_items WHERE user_id = ? AND outing_id = ? ORDER BY position, id`, uid, outingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*OutingItem{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetOutingItem is one thing on a packing list.
func (s *Store) GetOutingItem(ctx context.Context, uid, id int64) (*OutingItem, error) {
	return scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemCols+` FROM outing_items WHERE user_id = ? AND id = ?`, uid, id))
}

// AddOutingItem puts something on an outing's packing list (once per piece
// of gear: adding it again does nothing).
func (s *Store) AddOutingItem(ctx context.Context, uid int64, it *OutingItem) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM outings WHERE user_id = ? AND id = ?`, uid, it.OutingID).Scan(&n); err != nil || n == 0 {
		return ErrNotFound
	}
	if it.GearID != nil {
		var have int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM outing_items WHERE user_id = ? AND outing_id = ? AND gear_id = ?`, uid, it.OutingID, *it.GearID).Scan(&have)
		if have > 0 {
			return nil
		}
		var mine int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM gear WHERE user_id = ? AND id = ?`, uid, *it.GearID).Scan(&mine); err != nil || mine == 0 {
			return ErrNotFound
		}
	}
	if it.Position == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM outing_items WHERE user_id = ? AND outing_id = ?`, uid, it.OutingID).Scan(&it.Position)
	}
	it.UserID = uid
	res, err := s.DB.ExecContext(ctx, `INSERT INTO outing_items (user_id, outing_id, gear_id, name, category, weight_g, qty, worn, consumable, packed, to_buy, task_ref, position)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, it.OutingID, nullI(it.GearID), it.Name, it.Category, it.WeightG, max(it.Qty, 1),
		boolInt(it.Worn), boolInt(it.Consumable), boolInt(it.Packed), boolInt(it.ToBuy), it.TaskRef, it.Position)
	if err != nil {
		return err
	}
	it.ID, _ = res.LastInsertId()
	return nil
}

// UpdateOutingItem saves one thing on a packing list.
func (s *Store) UpdateOutingItem(ctx context.Context, it *OutingItem) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE outing_items SET name = ?, category = ?, weight_g = ?, qty = ?, worn = ?, consumable = ?, packed = ?, to_buy = ?, task_ref = ?, position = ?
	  WHERE user_id = ? AND id = ?`, it.Name, it.Category, it.WeightG, max(it.Qty, 1), boolInt(it.Worn), boolInt(it.Consumable), boolInt(it.Packed), boolInt(it.ToBuy), it.TaskRef, it.Position, it.UserID, it.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOutingItem takes something off a packing list.
func (s *Store) DeleteOutingItem(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM outing_items WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PackAll ticks (or unticks) everything on an outing's list.
func (s *Store) PackAll(ctx context.Context, uid, outingID int64, packed bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE outing_items SET packed = ? WHERE user_id = ? AND outing_id = ?`, boolInt(packed), uid, outingID)
	return err
}

// SetOutingWeather keeps the forecast on an outing.
func (s *Store) SetOutingWeather(ctx context.Context, uid, id int64, weather string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE outings SET weather = ? WHERE user_id = ? AND id = ?`, weather, uid, id)
	return err
}

// SetOutingRefs saves what an outing has in the other apps (and the
// safety marks), without touching the rest.
func (s *Store) SetOutingRefs(ctx context.Context, o *Outing) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE outings SET cal_ref = ?, health_ref = ?, note_ref = ?, plan_sent_at = ?, warned_at = ?, alerted_at = ?, alert_error = ?
	  WHERE user_id = ? AND id = ?`, o.CalRef, o.HealthRef, o.NoteRef, o.PlanSentAt, o.WarnedAt, o.AlertedAt, o.AlertError, o.UserID, o.ID)
	return err
}

// SetOutingActual keeps the walked track and how it compared.
func (s *Store) SetOutingActual(ctx context.Context, o *Outing) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE outings SET actual_track = ?, actual = ?, actual_source = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		toJSON(nonNilPts(o.ActualTrack)), string(o.Actual), o.ActualSource, Now(), o.UserID, o.ID)
	return err
}

// SafetyOutings are the outings someone might need to be told about:
// safety on, a back-by time, not back yet and nobody told yet.
func (s *Store) SafetyOutings(ctx context.Context, from string) ([]*Outing, error) {
	rows, err := s.DB.QueryContext(ctx, outingSelect(false)+` WHERE o.safety = 1 AND o.back_by <> '' AND o.back_by >= ? AND o.status IN ('planned','out') AND o.alerted_at = ''`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Outing
	for rows.Next() {
		o, _, err := scanOuting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
