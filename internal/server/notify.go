package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// ---- the back-by time ----

// safetyChecks runs every minute. For an outing with "tell someone" on and
// a back-by time: at the time, a message to you ("are you back?"); after
// the wait the person chose, an email through Pocket Mail to your contact
// with the plan (and the trails.overdue event, for Pocket Flows). Once
// each. "I'm back" stops it, and tells the contact when they were told.
func (s *Server) safetyChecks(ctx context.Context) {
	if s.cfg.DemoData {
		return
	}
	now := time.Now()
	from := now.Add(-24 * time.Hour).Format("2006-01-02T15:04")
	outs, err := s.store.SafetyOutings(ctx, from)
	if err != nil {
		return
	}
	for _, o := range outs {
		back := backByTime(o)
		if back.IsZero() || now.Before(back) {
			continue
		}
		c := s.clients(ctx, o.UserID)
		if c.u == nil {
			continue
		}
		if o.WarnedAt == "" {
			o.WarnedAt = now.UTC().Format(time.RFC3339)
			_ = s.store.SetOutingRefs(ctx, o)
			body := fmt.Sprintf("You planned to be back from %s by %s.", outingTitle(o), back.Format("15:04"))
			if o.ContactEmail != "" {
				body += fmt.Sprintf(" Tap “I'm back” in Pocket Trails, or %s will be emailed at %s.", firstNonEmpty(o.ContactName, o.ContactEmail), back.Add(time.Duration(o.AlertAfter)*time.Minute).Format("15:04"))
			}
			s.pushLevel(ctx, o.UserID, c.st, "Are you back?", body, s.link(fmt.Sprintf("/outings/%d", o.ID)), "warning")
		}
		if now.Before(back.Add(time.Duration(o.AlertAfter) * time.Minute)) {
			continue
		}
		o.AlertedAt = now.UTC().Format(time.RFC3339)
		o.AlertError = ""
		var r *store.Route
		if o.RouteID != nil {
			if rt, err := s.store.GetRoute(ctx, o.UserID, *o.RouteID); err == nil {
				decorateRoute(rt, c.st.PacePct)
				r = rt
			}
		}
		if o.ContactEmail != "" {
			name := firstNonEmpty(c.u.Name, c.u.Email)
			intro := fmt.Sprintf("%s planned to be back by %s and has not said they are back yet. This may be nothing (a long lunch, a flat phone battery), but you may want to try to reach them.", name, back.Format("15:04"))
			body := s.planText(c.u, o, r, c.st, intro) + "This message was sent by Pocket Trails because " + name + " asked it to."
			if err := s.sendMail(ctx, c, o.ContactName, o.ContactEmail, name+" is not back yet from "+outingTitle(o), body); err != nil {
				o.AlertError = err.Error()
			}
		} else {
			o.AlertError = "nobody to email"
		}
		_ = s.store.SetOutingRefs(ctx, o)
		ev := s.outingEvent(o)
		ev["contact_name"], ev["alert_error"] = o.ContactName, o.AlertError
		if r != nil && r.StartLat != nil {
			ev["start_lat"], ev["start_lng"] = *r.StartLat, *r.StartLng
		}
		s.publish(ctx, o.UserID, "trails.overdue", "outing:"+itoa(o.ID), ev)
		msg := firstNonEmpty(o.ContactName, o.ContactEmail) + " was emailed that you are not back yet."
		if o.AlertError != "" {
			msg = "Could not email " + firstNonEmpty(o.ContactName, o.ContactEmail, "your contact") + ": " + o.AlertError + "."
		}
		s.pushLevel(ctx, o.UserID, c.st, "Not back yet: "+outingTitle(o), msg+" Tap “I'm back” when you are.", s.link(fmt.Sprintf("/outings/%d", o.ID)), "failure")
	}
}

// ---- the evening before and the morning of ----

// pushes runs every minute; at each person's evening hour it says what is
// on tomorrow (weather, sunset, what is not packed yet), at their morning
// hour what is on today; and it tells the box (trails.outing_today).
func (s *Server) pushes(ctx context.Context) {
	now := time.Now()
	if now.Minute() > 10 {
		return
	}
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil {
			continue
		}
		if now.Hour() == st.NotifyHour && s.once(ctx, fmt.Sprintf("morning:%d:%s", uid, today())) {
			s.outingMessage(ctx, uid, st, today(), st.NotifyMorning)
		}
		if now.Hour() == st.EveningHour && s.once(ctx, fmt.Sprintf("evening:%d:%s", uid, today())) {
			s.outingMessage(ctx, uid, st, store.AddDays(today(), 1), st.NotifyEvening)
		}
	}
}

// outingMessage builds the message about one day's outings (and sends it
// when asked). It returns the text.
func (s *Server) outingMessage(ctx context.Context, uid int64, st store.Settings, date string, send bool) string {
	outs, _, err := s.store.ListOutings(ctx, uid, store.OutingFilter{From: date, To: date, Status: []string{store.OutingPlanned}})
	if err != nil || len(outs) == 0 {
		return ""
	}
	var lines []string
	for _, o := range outs {
		page, err := s.outingPage(ctx, uid, o.ID)
		if err != nil {
			continue
		}
		full := page["outing"].(*store.Outing)
		if date == today() {
			s.publish(ctx, uid, "trails.outing_today", "outing:"+itoa(o.ID)+":"+date, s.outingEvent(full))
		}
		line := outingTitle(full)
		if full.StartTime != "" {
			line += " at " + full.StartTime
		}
		var w dayWeather
		if len(full.Weather) > 0 && json.Unmarshal(full.Weather, &w) == nil && w.Date == date {
			line += fmt.Sprintf(": %s", strings.ToLower(firstNonEmpty(w.Condition, wmoWords(w.Code))))
			if w.TMax != nil && w.TMin != nil {
				line += fmt.Sprintf(", %.0f–%.0f°", *w.TMin, *w.TMax)
			}
		}
		if set, _ := page["sunset"].(string); set != "" {
			line += ", sunset " + set
		}
		line += "."
		for _, a := range page["advice"].([]advice) {
			if a.Level != "info" {
				line += " " + a.Text
			}
		}
		if full.ItemCount > 0 && full.PackedCount < full.ItemCount {
			line += fmt.Sprintf(" %d of %d things packed.", full.PackedCount, full.ItemCount)
		}
		lines = append(lines, line)
	}
	body := strings.Join(lines, "\n")
	if send && body != "" {
		title := "Tomorrow outdoors"
		if date == today() {
			title = "Outdoors today"
		}
		s.push(ctx, uid, st, title, body, s.link("/"))
	}
	return body
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	return s.pushLevel(ctx, uid, st, title, body, link, "info")
}

func (s *Server) pushLevel(ctx context.Context, uid int64, st store.Settings, title, body, link, level string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: level, Tags: []string{"mountain"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Trails", Body: notifyTestBody, Priority: "info", Tags: []string{"mountain"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckNow builds tomorrow's (or today's) message now, and sends it when asked.
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	date := store.AddDays(today(), 1)
	if r.URL.Query().Get("day") == "today" {
		date = today()
	}
	body := s.outingMessage(ctx, uid, st, date, queryBool(r, "send"))
	if body == "" {
		body = "No outing planned for that day."
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": body})
}

const notifyTestBody = "Notifications work. The evening before an outing (and that morning) a note gives the weather, the sunset and what is not packed yet; and if you set a time to be back by, you are asked when it passes."
