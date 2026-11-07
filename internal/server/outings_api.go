package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// outingInput is what the outing form (or another app) sends; nil = not sent.
type outingInput struct {
	RouteID      *int64         `json:"route_id"`
	Title        *string        `json:"title"`
	Activity     *string        `json:"activity"`
	Date         *string        `json:"date"`
	StartTime    *string        `json:"start_time"`
	People       []store.Person `json:"people"`
	Notes        *string        `json:"notes"`
	BackBy       *string        `json:"back_by"`
	Safety       *bool          `json:"safety"`
	ContactName  *string        `json:"contact_name"`
	ContactEmail *string        `json:"contact_email"`
	ContactRef   *string        `json:"contact_ref"`
	AlertAfter   *int           `json:"alert_after"`
	TripRef      *string        `json:"trip_ref"`
	TripName     *string        `json:"trip_name"`
	Rating       *int           `json:"rating"`
	Conditions   *string        `json:"conditions"`
	Report       *string        `json:"report"`
	SourceRef    *string        `json:"source_ref"`
	PackID       int64          `json:"pack_id"` // new outings: start the packing list from this one
}

func (in outingInput) apply(o *store.Outing, keys map[string]bool) string {
	if keys["route_id"] {
		if in.RouteID == nil || *in.RouteID <= 0 {
			o.RouteID = nil
		} else {
			o.RouteID = in.RouteID
		}
	}
	trimP(&o.Title, in.Title)
	if in.Activity != nil {
		if !oneOf(*in.Activity, store.Activities) {
			return "unknown activity"
		}
		o.Activity = *in.Activity
	}
	if in.Date != nil {
		if *in.Date == "" || !validDate(*in.Date) {
			return "the day is YYYY-MM-DD"
		}
		o.Date = *in.Date
	}
	if in.StartTime != nil {
		v := strings.TrimSpace(*in.StartTime)
		if v != "" && !reTime.MatchString(v) {
			return "the start time is HH:MM"
		}
		o.StartTime = v
	}
	if keys["people"] {
		var ps []store.Person
		for _, p := range in.People {
			p.Name, p.Email = clip(strings.TrimSpace(p.Name), 120), strings.ToLower(strings.TrimSpace(p.Email))
			if p.Name == "" && p.Email == "" {
				continue
			}
			ps = append(ps, p)
			if len(ps) >= 30 {
				break
			}
		}
		o.People = ps
	}
	if in.Notes != nil {
		o.Notes = clip(strings.TrimSpace(*in.Notes), 10000)
	}
	if in.BackBy != nil {
		v := strings.TrimSpace(*in.BackBy)
		if v != "" {
			if _, err := time.ParseInLocation("2006-01-02T15:04", v, time.Local); err != nil {
				return "back by is a day and a time"
			}
		}
		if v != o.BackBy {
			o.WarnedAt, o.AlertedAt, o.AlertError = "", "", ""
		}
		o.BackBy = v
	}
	if in.Safety != nil {
		o.Safety = *in.Safety
	}
	trimP(&o.ContactName, in.ContactName)
	if in.ContactEmail != nil {
		v := strings.ToLower(strings.TrimSpace(*in.ContactEmail))
		if v != "" && !reEmail.MatchString(v) {
			return "the contact's email address does not look right"
		}
		o.ContactEmail = v
	}
	trimP(&o.ContactRef, in.ContactRef)
	if in.AlertAfter != nil {
		if *in.AlertAfter < 0 || *in.AlertAfter > 24*60 {
			return "tell them 0 to 24 hours after the back-by time"
		}
		o.AlertAfter = *in.AlertAfter
	}
	trimP(&o.TripRef, in.TripRef)
	trimP(&o.TripName, in.TripName)
	if in.Rating != nil {
		if *in.Rating < 0 || *in.Rating > 5 {
			return "the rating is 0 to 5"
		}
		o.Rating = *in.Rating
	}
	if in.Conditions != nil {
		o.Conditions = clip(strings.TrimSpace(*in.Conditions), 4000)
	}
	if in.Report != nil {
		o.Report = clip(strings.TrimSpace(*in.Report), 20000)
	}
	if in.SourceRef != nil {
		o.SourceRef = clip(strings.TrimSpace(*in.SourceRef), 200)
	}
	if o.Safety && o.BackBy == "" {
		return "set a time to be back by, or switch off telling someone"
	}
	o.Title = clip(o.Title, 200)
	return ""
}

func (s *Server) handleListOutings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	f := store.OutingFilter{From: q.Get("from"), To: q.Get("to"), RouteID: int64(queryInt(r, "route_id")), Q: q.Get("q"), Limit: queryInt(r, "limit"), Desc: q.Get("order") == "desc"}
	if v := q.Get("status"); v != "" {
		f.Status = strings.Split(v, ",")
	}
	list, facts, err := s.store.ListOutings(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "outings")
		return
	}
	st := s.settingsOf(ctx, uid)
	for i, o := range list {
		decorateOuting(o, facts[i], st.PacePct)
	}
	writeJSON(w, http.StatusOK, list)
}

// outingPage is an outing with its route, the advice for the day, the sun
// and its links in the other apps.
func (s *Server) outingPage(ctx context.Context, uid, id int64) (map[string]any, error) {
	o, rf, err := s.store.GetOuting(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	c := s.clients(ctx, uid)
	decorateOuting(o, rf, c.st.PacePct)
	var r *store.Route
	if o.RouteID != nil {
		if r, err = s.store.GetRoute(ctx, uid, *o.RouteID); err == nil {
			decorateRoute(r, c.st.PacePct)
		} else {
			r = nil
		}
	}
	var weatherNote string
	if o.Status == store.OutingPlanned || o.Status == store.OutingOut {
		ctx2, cancel := context.WithTimeout(ctx, 12*time.Second)
		why, err := s.refreshWeather(ctx2, c, o, r, false)
		cancel()
		weatherNote = why
		if err != nil {
			weatherNote = err.Error()
		}
	}
	rise, set := sunFor(r, o.Date)
	links := map[string]string{}
	open := func(a pocketApp, path string) string {
		if st := c.status(ctx, a); st.Open != "" {
			return strings.TrimRight(st.Open, "/") + path
		}
		return ""
	}
	if o.CalRef != "" {
		links["calendar"] = open(appCalendar, "/event/"+o.CalRef)
	}
	if o.HealthRef != "" {
		links["health"] = open(appHealth, "/workouts")
	}
	if o.NoteRef != "" {
		links["notes"] = open(appNotes, "/note/"+o.NoteRef)
	}
	if o.TripRef != "" {
		links["travel"] = open(appTravel, "/trips/"+o.TripRef)
	}
	if o.ActualSource == "movement" {
		links["movement"] = open(appMovement, "/day/"+o.Date)
	}
	out := map[string]any{"outing": o, "route": r, "advice": adviceFor(o, r, c.st), "sunrise": rise, "sunset": set, "weather_note": weatherNote, "links": links,
		"suggest_back_by": suggestBackBy(o), "finish": ""}
	if f := finishAt(o); !f.IsZero() {
		out["finish"] = f.Format("15:04")
	}
	return out, nil
}

func (s *Server) handleGetOuting(w http.ResponseWriter, r *http.Request) {
	out, err := s.outingPage(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateOuting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	var in outingInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	o := &store.Outing{Activity: firstNonEmpty(st.DefaultActivity, "hike"), Status: store.OutingPlanned, Date: today(), AlertAfter: st.AlertAfter,
		Safety: st.SafetyDefault, ContactName: st.ContactName, ContactEmail: st.ContactEmail, ContactRef: st.ContactRef, Source: "manual"}
	if app := linkApp(ctx); app != "" {
		o.Source = app
		in.Safety = nil // another app never arms the "tell someone" part
		o.Safety = false
	}
	if !keys["back_by"] && !keys["safety"] {
		o.Safety = false // the default needs a back-by time to mean anything
	}
	if msg := in.apply(o, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if o.Source != "manual" && o.SourceRef != "" {
		if id, err := s.store.OutingBySource(ctx, uid, o.Source, o.SourceRef); err == nil {
			out, _ := s.outingPage(ctx, uid, id)
			writeJSON(w, http.StatusOK, out["outing"])
			return
		}
	}
	if o.RouteID != nil {
		rt, err := s.store.GetRoute(ctx, uid, *o.RouteID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "that route is not one of yours")
			return
		}
		if !keys["activity"] {
			o.Activity = rt.Activity
		}
	}
	if err := s.store.CreateOuting(ctx, uid, o); err != nil {
		s.fail(w, err, "plan outing")
		return
	}
	if in.PackID > 0 {
		_ = s.addPack(ctx, uid, o.ID, in.PackID)
	}
	out, err := s.outingPage(ctx, uid, o.ID)
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	full := out["outing"].(*store.Outing)
	bg := context.WithoutCancel(ctx)
	go s.syncOutingEvent(bg, uid, full, false)
	s.publish(ctx, uid, "trails.outing_planned", "outing:"+itoa(o.ID), s.outingEvent(full))
	if linkApp(ctx) != "" {
		writeJSON(w, http.StatusCreated, full)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) outingEvent(o *store.Outing) map[string]any {
	ev := map[string]any{"outing_id": o.ID, "title": outingTitle(o), "activity": o.Activity, "date": o.Date, "start_time": o.StartTime, "status": o.Status,
		"route": o.RouteName, "distance_m": o.DistanceM, "gain_m": o.GainM, "minutes": o.Minutes, "people": peopleNames(o.People), "back_by": o.BackBy,
		"url": s.link(fmt.Sprintf("/outings/%d", o.ID))}
	if o.RouteID != nil {
		ev["route_id"] = *o.RouteID
	}
	return ev
}

func (s *Server) handleUpdateOuting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, _, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	if app := linkApp(ctx); app != "" && o.Source != app {
		writeErr(w, http.StatusForbidden, "this outing was not planned by "+app)
		return
	}
	var in outingInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.SourceRef = nil
	if linkApp(ctx) != "" {
		in.Safety = nil
	}
	before := o.Date + o.StartTime + o.BackBy + fmt.Sprint(o.RouteID) + o.Title
	if msg := in.apply(o, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if keys["date"] {
		o.Weather = nil
	}
	if err := s.store.UpdateOuting(ctx, o); err != nil {
		s.fail(w, err, "save outing")
		return
	}
	out, err := s.outingPage(ctx, uid, o.ID)
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	full := out["outing"].(*store.Outing)
	if before != o.Date+o.StartTime+o.BackBy+fmt.Sprint(o.RouteID)+o.Title && (o.Status == store.OutingPlanned || o.Status == store.OutingOut) {
		go s.syncOutingEvent(context.WithoutCancel(ctx), uid, full, false)
	}
	if linkApp(ctx) != "" {
		writeJSON(w, http.StatusOK, full)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteOuting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, _, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	if app := linkApp(ctx); app != "" && o.Source != app {
		writeErr(w, http.StatusForbidden, "this outing was not planned by "+app)
		return
	}
	if o.CalRef != "" {
		s.syncOutingEvent(ctx, uid, o, true)
	}
	if err := s.store.DeleteOuting(ctx, uid, o.ID); err != nil {
		s.fail(w, err, "delete outing")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOutingStatus moves an outing along: out (I'm setting off), back
// (I'm back: done), done (it happened, marked afterwards), cancelled, or
// planned again.
func (s *Server) handleOutingStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, rf, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	c := s.clients(ctx, uid)
	decorateOuting(o, rf, c.st.PacePct)
	now := time.Now().UTC().Format(time.RFC3339)
	wasAlerted := o.AlertedAt != ""
	topic := ""
	switch chiParam(r, "action") {
	case "start":
		if o.Status == store.OutingDone || o.Status == store.OutingCancelled {
			writeErr(w, http.StatusBadRequest, "this outing is already over")
			return
		}
		o.Status, o.StartedAt, o.EndedAt = store.OutingOut, now, ""
		if o.Date != today() {
			o.Date = today()
		}
		topic = "trails.outing_started"
	case "back", "done":
		if o.Status == store.OutingCancelled {
			writeErr(w, http.StatusBadRequest, "this outing was called off")
			return
		}
		o.Status = store.OutingDone
		if chiParam(r, "action") == "back" {
			o.EndedAt = now
			if o.StartedAt == "" && o.StartTime != "" {
				if st := startAt(o); !st.IsZero() && st.Before(time.Now()) {
					o.StartedAt = st.UTC().Format(time.RFC3339)
				}
			}
		}
		topic = "trails.outing_done"
	case "cancel":
		o.Status = store.OutingCancelled
		topic = "trails.outing_cancelled"
	case "plan":
		o.Status, o.StartedAt, o.EndedAt, o.WarnedAt, o.AlertedAt, o.AlertError = store.OutingPlanned, "", "", "", "", ""
	default:
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.store.UpdateOuting(ctx, o); err != nil {
		s.fail(w, err, "save outing")
		return
	}
	bg := context.WithoutCancel(ctx)
	note := ""
	switch o.Status {
	case store.OutingCancelled:
		go s.syncOutingEvent(bg, uid, o, true)
	case store.OutingDone:
		// someone was told you were not back: tell them you are
		if wasAlerted && o.ContactEmail != "" {
			u := c.u
			body := fmt.Sprintf("Hello %s,\n\n%s is back from the %s (%s) — all good.\n\nThank you for keeping an eye out.", firstNonEmpty(o.ContactName, "there"),
				firstNonEmpty(u.Name, u.Email), strings.ToLower(activityWords[o.Activity]), outingTitle(o))
			if err := s.sendMail(ctx, c, o.ContactName, o.ContactEmail, firstNonEmpty(u.Name, u.Email)+" is back", body); err != nil {
				note = "Could not tell " + firstNonEmpty(o.ContactName, o.ContactEmail) + " you are back: " + err.Error() + ". Let them know yourself."
			} else {
				note = firstNonEmpty(o.ContactName, o.ContactEmail) + " was told you are back."
			}
		}
		go func() {
			ctx, cancel := context.WithTimeout(bg, 2*time.Minute)
			defer cancel()
			if c.st.MovementAuto || c.st.HealthLog {
				s.takeTrack(ctx, s.clients(ctx, uid), o.ID)
			}
		}()
	}
	if topic != "" {
		s.publish(ctx, uid, topic, "outing:"+itoa(o.ID), s.outingEvent(o))
	}
	out, err := s.outingPage(ctx, uid, o.ID)
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	out["note"] = note
	writeJSON(w, http.StatusOK, out)
}

// handleOutingWeather fetches the forecast again now.
func (s *Server) handleOutingWeather(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, rf, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	c := s.clients(ctx, uid)
	decorateOuting(o, rf, c.st.PacePct)
	var rt *store.Route
	if o.RouteID != nil {
		rt, _ = s.store.GetRoute(ctx, uid, *o.RouteID)
	}
	why, err := s.refreshWeather(ctx, c, o, rt, true)
	if err != nil {
		s.userFail(w, err, "weather")
		return
	}
	out, _ := s.outingPage(ctx, uid, o.ID)
	out["weather_note"] = firstNonEmpty(why, out["weather_note"].(string))
	writeJSON(w, http.StatusOK, out)
}

// handleSendPlan emails the plan to the safety contact now.
func (s *Server) handleSendPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	page, err := s.outingPage(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	o := page["outing"].(*store.Outing)
	rt, _ := page["route"].(*store.Route)
	var in struct {
		Message string `json:"message"`
		Preview bool   `json:"preview"`
	}
	_ = decode(r, &in)
	c := s.clients(ctx, uid)
	if o.ContactEmail == "" {
		writeErr(w, http.StatusBadRequest, "add who to tell (with an email address) first")
		return
	}
	body := s.planText(c.u, o, rt, c.st, in.Message)
	if o.Safety && o.AlertAfter >= 0 && o.BackBy != "" {
		body += fmt.Sprintf("If %s is not back by then, Pocket Trails will email you about %s after that time.", firstNonEmpty(c.u.Name, "they"), fmtMinutes(max(o.AlertAfter, 1)))
		body += "\n\n"
	}
	body += "Best regards,\n" + firstNonEmpty(c.u.Name, c.u.Email)
	subject := firstNonEmpty(c.u.Name, c.u.Email) + "'s plan: " + outingTitle(o) + ", " + niceDate(o.Date)
	if in.Preview {
		writeJSON(w, http.StatusOK, map[string]any{"subject": subject, "body": body, "to": o.ContactEmail})
		return
	}
	if err := s.sendMail(ctx, c, o.ContactName, o.ContactEmail, subject, body); err != nil {
		s.userFail(w, err, "send plan")
		return
	}
	o.UserID = uid
	o.PlanSentAt = time.Now().UTC().Format(time.RFC3339)
	_ = s.store.SetOutingRefs(ctx, o)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent_at": o.PlanSentAt})
}

// ---- the walked track ----

// handleTrackFromMovement takes the walked track from Pocket Movement now.
func (s *Server) handleTrackFromMovement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, _, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	var rt *store.Route
	if o.RouteID != nil {
		rt, _ = s.store.GetRoute(ctx, uid, *o.RouteID)
	}
	c := s.clients(ctx, uid)
	pts, err := s.movementTrack(ctx, c, o, rt)
	if err != nil {
		s.userFail(w, err, "movement")
		return
	}
	if err := s.compareOuting(ctx, uid, o, rt, pts, "movement"); err != nil {
		s.fail(w, err, "keep track")
		return
	}
	out, _ := s.outingPage(ctx, uid, o.ID)
	writeJSON(w, http.StatusOK, out)
}

// handleTrackUpload takes the walked track from a GPX (a watch, a phone app).
func (s *Server) handleTrackUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, _, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	var data []byte
	name := firstNonEmpty(r.URL.Query().Get("name"), "track.gpx")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(s.cfg.MaxBodyBytes); err != nil || len(r.MultipartForm.File["file"]) == 0 {
			writeErr(w, http.StatusBadRequest, "choose a GPX file")
			return
		}
		fh := r.MultipartForm.File["file"][0]
		name = fh.Filename
		if data, err = readPart(fh, s.cfg.MaxBodyBytes); err != nil {
			writeErr(w, http.StatusBadRequest, "the upload could not be read")
			return
		}
	} else if data, err = s.readBody(r); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too big")
		return
	}
	parsed, err := geo.ParseFile(name, data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var pts []store.Pt
	for _, p := range parsed {
		pts = append(pts, p.Track...)
	}
	var rt *store.Route
	if o.RouteID != nil {
		rt, _ = s.store.GetRoute(ctx, uid, *o.RouteID)
	}
	if err := s.compareOuting(ctx, uid, o, rt, geo.Thin(geo.Clean(pts), maxTrack), "gpx"); err != nil {
		s.fail(w, err, "keep track")
		return
	}
	out, _ := s.outingPage(ctx, uid, o.ID)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTrackDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, _, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	o.ActualTrack, o.Actual, o.ActualSource = nil, nil, ""
	if err := s.store.SetOutingActual(ctx, o); err != nil {
		s.fail(w, err, "forget track")
		return
	}
	out, _ := s.outingPage(ctx, uid, o.ID)
	writeJSON(w, http.StatusOK, out)
}

// handleTrackToRoute saves what you walked as a route of its own.
func (s *Server) handleTrackToRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, _, err := s.store.GetOuting(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	if len(o.ActualTrack) < 2 {
		writeErr(w, http.StatusBadRequest, "there is no walked track yet")
		return
	}
	st := s.settingsOf(ctx, uid)
	tr := make([]store.Pt, len(o.ActualTrack))
	for i, p := range o.ActualTrack {
		tr[i] = store.Pt{Lat: p.Lat, Lng: p.Lng, Ele: p.Ele}
	}
	rt := &store.Route{Name: outingTitle(o) + " (as walked)", Activity: o.Activity, Track: tr, Source: "outing", SourceRef: fmt.Sprintf("outing:%d", o.ID)}
	if have, err := s.store.RouteBySource(ctx, uid, "outing", rt.SourceRef); err == nil {
		writeJSON(w, http.StatusOK, have)
		return
	}
	prepRoute(rt)
	if err := s.store.CreateRoute(ctx, uid, rt); err != nil {
		s.fail(w, err, "create route")
		return
	}
	s.publish(ctx, uid, "trails.route_saved", "route:"+itoa(rt.ID), s.routeEvent(rt, st))
	writeJSON(w, http.StatusCreated, rt)
}

// handleOutingGPX downloads the walked track.
func (s *Server) handleOutingGPX(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	o, _, err := s.store.GetOuting(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	if len(o.ActualTrack) == 0 {
		writeErr(w, http.StatusNotFound, "there is no walked track")
		return
	}
	w.Header().Set("Content-Type", "application/gpx+xml")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName(outingTitle(o)+" "+o.Date)+`.gpx"`)
	_ = geo.WriteGPX(w, outingTitle(o), "", o.Activity, o.ActualTrack, nil)
}

// handleOutingMake puts an outing in another app: health, notes, tasks
// (gear to buy), calendar (now).
func (s *Server) handleOutingMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	page, err := s.outingPage(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "outing")
		return
	}
	o := page["outing"].(*store.Outing)
	rt, _ := page["route"].(*store.Route)
	o.UserID = uid
	c := s.clients(ctx, uid)
	var url string
	msg := ""
	switch chiParam(r, "app") {
	case "health":
		url, err = s.logHealth(ctx, c, o)
		msg = "In Pocket Health"
	case "notes":
		url, err = s.saveNote(ctx, c, o, rt)
		msg = "Saved in Pocket Notes"
	case "tasks":
		var n int
		n, url, err = s.gearTasks(ctx, c, o)
		msg = fmt.Sprintf("%d %s for Pocket Tasks", n, plural(n, "thing to buy", "things to buy"))
	case "calendar":
		if _, err = c.conn(ctx, appCalendar, scopeCalendarWrite); err == nil {
			c.st.CalendarSync = true
			s.syncOutingEvent(ctx, uid, o, false)
			if o.CalRef == "" {
				err = &userErr{"Pocket Calendar did not take the event"}
			}
			msg = "In Pocket Calendar"
		}
	default:
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.userFail(w, err, chiParam(r, "app"))
		return
	}
	out, _ := s.outingPage(ctx, uid, o.ID)
	out["message"], out["open_url"] = msg, url
	writeJSON(w, http.StatusOK, out)
}

// ---- what to pack ----

type itemInput struct {
	GearID     *int64  `json:"gear_id"`
	Name       *string `json:"name"`
	Category   *string `json:"category"`
	WeightG    *int    `json:"weight_g"`
	Qty        *int    `json:"qty"`
	Worn       *bool   `json:"worn"`
	Consumable *bool   `json:"consumable"`
	Packed     *bool   `json:"packed"`
	ToBuy      *bool   `json:"to_buy"`
	Position   *int    `json:"position"`
}

func (in itemInput) apply(it *store.OutingItem) string {
	trimP(&it.Name, in.Name)
	if in.Category != nil {
		if !oneOf(*in.Category, store.GearKinds) {
			return "unknown kind of gear"
		}
		it.Category = *in.Category
	}
	if in.WeightG != nil {
		if *in.WeightG < 0 || *in.WeightG > 100000 {
			return "the weight is 0 to 100 kg"
		}
		it.WeightG = *in.WeightG
	}
	if in.Qty != nil {
		if *in.Qty < 1 || *in.Qty > 999 {
			return "how many is 1 to 999"
		}
		it.Qty = *in.Qty
	}
	if in.Worn != nil {
		it.Worn = *in.Worn
	}
	if in.Consumable != nil {
		it.Consumable = *in.Consumable
	}
	if in.Packed != nil {
		it.Packed = *in.Packed
	}
	if in.ToBuy != nil {
		it.ToBuy = *in.ToBuy
	}
	if in.Position != nil {
		it.Position = *in.Position
	}
	if strings.TrimSpace(it.Name) == "" {
		return "name the thing to pack"
	}
	it.Name = clip(it.Name, 200)
	return ""
}

// addPack copies a packing list's gear onto an outing (what is there stays).
func (s *Server) addPack(ctx context.Context, uid, outingID, packID int64) error {
	p, err := s.store.GetPack(ctx, uid, packID)
	if err != nil {
		return err
	}
	for _, pi := range p.Items {
		if pi.Gear == nil {
			continue
		}
		g := pi.Gear
		id := g.ID
		if err := s.store.AddOutingItem(ctx, uid, &store.OutingItem{OutingID: outingID, GearID: &id, Name: g.Name, Category: g.Category, WeightG: g.WeightG, Qty: pi.Qty,
			Worn: g.Worn, Consumable: g.Consumable}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleAddItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	var in struct {
		itemInput
		PackID int64 `json:"pack_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.PackID > 0 {
		if err := s.addPack(ctx, uid, id, in.PackID); err != nil {
			s.fail(w, err, "add packing list")
			return
		}
	} else {
		it := &store.OutingItem{OutingID: id, Category: "other", Qty: 1}
		if in.GearID != nil && *in.GearID > 0 {
			g, err := s.store.GetGear(ctx, uid, *in.GearID)
			if err != nil {
				s.fail(w, err, "gear")
				return
			}
			it.GearID, it.Name, it.Category, it.WeightG, it.Worn, it.Consumable = &g.ID, g.Name, g.Category, g.WeightG, g.Worn, g.Consumable
		}
		if msg := in.itemInput.apply(it); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		if err := s.store.AddOutingItem(ctx, uid, it); err != nil {
			s.fail(w, err, "add to pack")
			return
		}
	}
	items, err := s.store.ListOutingItems(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "packing list")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	it, err := s.store.GetOutingItem(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "item")
		return
	}
	var in itemInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(it); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateOutingItem(ctx, it); err != nil {
		s.fail(w, err, "save item")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteOutingItem(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "remove item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePackAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Packed bool `json:"packed"`
	}
	_ = decode(r, &in)
	if err := s.store.PackAll(ctx, uid, idParam(r, "id"), in.Packed); err != nil {
		s.fail(w, err, "pack")
		return
	}
	items, err := s.store.ListOutingItems(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "packing list")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// jsonUnmarshal reads a stored JSON value.
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
