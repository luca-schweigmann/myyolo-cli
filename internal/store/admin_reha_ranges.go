package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
	"sort"
	"time"
)

type AdminRehaRangeSession struct {
	PlannerRegistered      *int     `json:"planner_registered"`
	PlannerCancelled       *int     `json:"planner_cancelled"`
	PlannerFree            *int     `json:"planner_free"`
	PlannerLinked          *int     `json:"planner_linked"`
	PlannerObservedAt      *string  `json:"planner_observed_at"`
	StableSessionID        string   `json:"stable_session_id"`
	StableCourseID         *string  `json:"stable_course_id"`
	CourseLabel            string   `json:"course_label"`
	Date                   string   `json:"date"`
	StartTime              string   `json:"start_time"`
	EndTime                string   `json:"end_time"`
	Registered             *int     `json:"registered"`
	Participated           *int     `json:"participated"`
	NotAttended            *int     `json:"not_attended"`
	Cancelled              int      `json:"cancelled"`
	SignedParticipated     *int     `json:"signed_participated"`
	Capacity               *int     `json:"capacity"`
	RegistrationStatus     string   `json:"registration_status"`
	AttendanceStatus       string   `json:"attendance_status"`
	RegisteredObservedAt   []string `json:"registered_observed_at"`
	AttendanceObservedAt   *string  `json:"attendance_observed_at"`
	CancellationObservedAt string   `json:"cancellation_observed_at"`
	SignedObservedAt       *string  `json:"signed_observed_at"`
	PeriodStatus           string   `json:"period_status"`
}
type AdminRehaRangeWindow struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Completeness string `json:"completeness"`
}
type AdminRehaRangeReport struct {
	SchemaVersion         string                  `json:"schema_version"`
	Source                string                  `json:"source"`
	RequestedFrom         string                  `json:"requested_from"`
	RequestedTo           string                  `json:"requested_to"`
	AsOf                  string                  `json:"as_of"`
	Timezone              string                  `json:"timezone"`
	SessionInventory      string                  `json:"session_inventory"`
	ExpectedSessions      *int                    `json:"expected_sessions"`
	MissingSessions       *int                    `json:"missing_sessions"`
	Windows               []AdminRehaRangeWindow  `json:"source_windows"`
	Sessions              []AdminRehaRangeSession `json:"sessions"`
	CompletedSessionCount int                     `json:"completed_session_count"`
	AverageRegistered     *float64                `json:"average_registered"`
	AverageParticipated   *float64                `json:"average_participated"`
}
type rangeObservation struct {
	Facts admin.CourseRangeFacts
	Stamp time.Time
}

// AdminRehaRanges combines validated source windows by exact native booking ID.
// A missing classification row leaves only that metric (and dependent totals)
// unknown; missing windows and conflicting identities still fail validation.
func (s *ReadOnlyStore) AdminRehaRanges(ctx context.Context, from, to, asOf time.Time) (AdminRehaRangeReport, error) {
	fail := func() (AdminRehaRangeReport, error) {
		return AdminRehaRangeReport{}, errors.New("Admin range source proof incomplete or conflicting")
	}
	if s == nil || s.db == nil || from.Location().String() != rehaTimezone || to.Location().String() != rehaTimezone || from.After(to) || asOf.IsZero() {
		return fail()
	}
	report := AdminRehaRangeReport{SchemaVersion: "admin-reha-ranges.v1", Source: "myyolo-admin", RequestedFrom: from.Format("2006-01-02"), RequestedTo: to.Format("2006-01-02"), AsOf: asOf.UTC().Format(time.RFC3339Nano), Timezone: rehaTimezone, SessionInventory: "observed_native_bookings_calendar_completeness_unknown", Windows: []AdminRehaRangeWindow{}, Sessions: []AdminRehaRangeSession{}}
	rows, e := s.db.QueryContext(ctx, `SELECT route,request_path,request_body,observed_at,page_json FROM admin_observations WHERE route IN ('capability:course-attended','capability:course-not-attended','capability:course-cancelled') ORDER BY run_id`)
	if e != nil {
		return fail()
	}
	defer rows.Close()
	windows := map[string]map[string]rangeObservation{}
	for rows.Next() {
		var route, path, body, stamp, raw string
		if rows.Scan(&route, &path, &body, &stamp, &raw) != nil {
			return fail()
		}
		observed, e := parseDataTimestamp(stamp)
		if e != nil {
			return fail()
		}
		if observed.After(asOf) {
			continue
		}
		var page admin.Page
		if json.Unmarshal([]byte(raw), &page) != nil || page.Metadata.Route != route || page.CourseRangeFacts == nil {
			return fail()
		}
		f := page.CourseRangeFacts
		if route != "capability:course-"+f.Kind || f.Completeness != "validated_complete_range" {
			return fail()
		}
		request, e := readcatalog.Build("course-"+f.Kind, map[string]string{"from": f.From, "to": f.To})
		if e != nil || request.Path != path || request.Body != body {
			return fail()
		}
		if f.To < report.RequestedFrom || f.From > report.RequestedTo {
			continue
		}
		key := f.From + "/" + f.To
		if windows[key] == nil {
			windows[key] = map[string]rangeObservation{}
		}
		old, ok := windows[key][f.Kind]
		if !ok || observed.After(old.Stamp) {
			windows[key][f.Kind] = rangeObservation{*f, observed}
		}
	}
	if rows.Err() != nil {
		return fail()
	}
	rows.Close()
	keys := make([]string, 0, len(windows))
	for k := range windows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	covered := map[string]bool{}
	byID := map[string]AdminRehaRangeSession{}
	for _, key := range keys {
		w := windows[key]
		if len(w) != 3 {
			return fail()
		}
		a, ok := w["attended"]
		if !ok {
			return fail()
		}
		n, ok := w["not-attended"]
		if !ok {
			return fail()
		}
		c, ok := w["cancelled"]
		if !ok {
			return fail()
		}
		start, _ := time.ParseInLocation("2006-01-02", a.Facts.From, from.Location())
		end, _ := time.ParseInLocation("2006-01-02", a.Facts.To, from.Location())
		for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
			if day.Before(from) || day.After(to) {
				continue
			}
			d := day.Format("2006-01-02")
			if covered[d] {
				return fail()
			}
			covered[d] = true
		}
		report.Windows = append(report.Windows, AdminRehaRangeWindow{a.Facts.From, a.Facts.To, "three_validated_source_classifications"})
		native := map[string]admin.CourseRangeRow{}
		counts := map[string]map[string]int{}
		for _, kind := range []string{"attended", "not-attended", "cancelled"} {
			seen := map[string]bool{}
			for _, r := range w[kind].Facts.Rows {
				if r.BookingID == "" || r.Count < 0 || r.Date < a.Facts.From || r.Date > a.Facts.To || seen[r.BookingID] {
					return fail()
				}
				seen[r.BookingID] = true
				if old, ok := native[r.BookingID]; ok && (old.Date != r.Date || old.CourseLabel != r.CourseLabel || old.StartTime != r.StartTime || old.EndTime != r.EndTime) {
					return fail()
				}
				native[r.BookingID] = r
				if counts[r.BookingID] == nil {
					counts[r.BookingID] = map[string]int{}
				}
				counts[r.BookingID][kind] = r.Count
			}
		}
		for booking, r := range native {
			if r.Date < report.RequestedFrom || r.Date > report.RequestedTo {
				continue
			}
			id := adminRehaPseudonym("session", booking)
			if _, ok := byID[id]; ok {
				return fail()
			}
			count := counts[booking]
			// Missing classification rows are unknown for this exact session.
			// Keep the other proven sessions; never turn an absent row into zero.
			var registered, participated, notAttended *int
			var attendanceStamp *string
			registeredStamps := []string{}
			if value, ok := count["attended"]; ok {
				participated = &value
				stamp := a.Stamp.UTC().Format(time.RFC3339Nano)
				attendanceStamp = &stamp
			}
			if value, ok := count["not-attended"]; ok {
				notAttended = &value
			}
			if participated != nil && notAttended != nil {
				value := *participated + *notAttended
				registered = &value
				registeredStamps = []string{a.Stamp.UTC().Format(time.RFC3339Nano), n.Stamp.UTC().Format(time.RFC3339Nano)}
			}
			item := AdminRehaRangeSession{StableSessionID: id, CourseLabel: r.CourseLabel, Date: r.Date, StartTime: r.StartTime, EndTime: r.EndTime, Registered: registered, Participated: participated, NotAttended: notAttended, Cancelled: count["cancelled"], RegistrationStatus: "complete_range_attended_plus_not_attended", AttendanceStatus: "admin_attendance_classification", RegisteredObservedAt: registeredStamps, AttendanceObservedAt: attendanceStamp, CancellationObservedAt: c.Stamp.UTC().Format(time.RFC3339Nano), PeriodStatus: "provisional"}
			if registered == nil {
				item.RegistrationStatus = "partial_range_classifications"
			}
			if item.Date < asOf.In(from.Location()).Format("2006-01-02") {
				item.PeriodStatus = "past_local_day"
			}
			byID[id] = item
		}
	}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if !covered[day.Format("2006-01-02")] {
			return fail()
		}
	}
	// Exact booking identity is the sole permitted bridge to detail signatures and
	// recurring course identity. Legacy detail archives cannot establish this.
	inventory, expected, e := s.adminPlannerInventory(ctx, from, to, asOf)
	if e != nil {
		return fail()
	}
	report.ExpectedSessions = expected
	if expected != nil {
		report.SessionInventory = "validated_week_planner_inventory_positive_booking_crosswalk_partial"
	}
	details, e := s.AdminRehaSessions(ctx, from, to, asOf)
	if e != nil {
		return fail()
	}
	for _, d := range details.Sessions {
		item, ok := byID[d.StableSessionID]
		planner := ""
		if d.Planner != nil {
			planner = *d.Planner
		}
		proof, hasPlanner := inventory[plannerKey(d.StableCourseID, d.Date, planner)]
		if !ok {
			if !hasPlanner || proof.Row.Registered != 0 {
				continue
			}
			if d.Registered != 0 || d.SignedAttendance != 0 || d.AttendanceMarked != 0 {
				return fail()
			}
			item = AdminRehaRangeSession{StableSessionID: d.StableSessionID, CourseLabel: proof.Row.CourseLabel, Date: d.Date, StartTime: proof.Row.StartTime, EndTime: proof.Row.EndTime, Registered: rangeInt(0), Participated: rangeInt(0), NotAttended: rangeInt(0), Cancelled: proof.Row.Cancelled, RegistrationStatus: "explicit_zero_planner_and_empty_native_roster", AttendanceStatus: "explicit_empty_native_roster", RegisteredObservedAt: []string{proof.Stamp, d.ObservedAt}, AttendanceObservedAt: &d.ObservedAt, CancellationObservedAt: proof.Stamp, PeriodStatus: d.PeriodStatus}
		}
		if hasPlanner {
			r := proof.Row
			item.Capacity = &r.Capacity
			item.PlannerRegistered = &r.Registered
			item.PlannerCancelled = &r.Cancelled
			item.PlannerFree = &r.Free
			item.PlannerLinked = &r.Linked
			stamp := proof.Stamp
			item.PlannerObservedAt = &stamp
		}
		if item.Date != d.Date {
			return fail()
		}
		course, sign, stamp := d.StableCourseID, d.SignedAttendance, d.ObservedAt
		item.StableCourseID = &course
		item.SignedParticipated = &sign
		item.SignedObservedAt = &stamp
		byID[d.StableSessionID] = item
	}
	sumRegistered, sumParticipated, knownRegistered, knownParticipated := 0, 0, 0, 0
	for _, item := range byID {
		report.Sessions = append(report.Sessions, item)
		if item.PeriodStatus == "past_local_day" {
			report.CompletedSessionCount++
			if item.Registered != nil {
				sumRegistered += *item.Registered
				knownRegistered++
			}
			if item.Participated != nil {
				sumParticipated += *item.Participated
				knownParticipated++
			}
		}
	}
	sort.Slice(report.Sessions, func(i, j int) bool {
		a, b := report.Sessions[i], report.Sessions[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.StartTime != b.StartTime {
			return a.StartTime < b.StartTime
		}
		return a.StableSessionID < b.StableSessionID
	})
	if knownRegistered > 0 {
		value := float64(sumRegistered) / float64(knownRegistered)
		report.AverageRegistered = &value
	}
	if knownParticipated > 0 {
		value := float64(sumParticipated) / float64(knownParticipated)
		report.AverageParticipated = &value
	}
	return report, nil
}

func rangeInt(value int) *int { return &value }
