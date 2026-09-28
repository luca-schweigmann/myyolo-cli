package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
)

type AdminRehaSession struct {
	PeriodStatus       string  `json:"period_status"`
	ParticipationBasis string  `json:"participation_basis"`
	StableSessionID    string  `json:"stable_session_id"`
	StableCourseID     string  `json:"stable_course_id"`
	Date               string  `json:"date"`
	Planner            *string `json:"planner"`
	ObservedAt         string  `json:"observed_at"`
	Registered         int     `json:"registered"`
	AttendanceMarked   int     `json:"attendance_marked"`
	SignedAttendance   int     `json:"signed_attendance"`
	Participated       *int    `json:"participated"`
	Cancelled          *int    `json:"cancelled"`
	Capacity           *int    `json:"capacity"`
	Completeness       string  `json:"completeness"`
	CancellationStatus string  `json:"cancellation_status"`
}
type AdminRehaSessionReport struct {
	SchemaVersion    string             `json:"schema_version"`
	Source           string             `json:"source"`
	RequestedFrom    string             `json:"requested_from"`
	RequestedTo      string             `json:"requested_to"`
	AsOf             string             `json:"as_of"`
	Timezone         string             `json:"timezone"`
	Coverage         string             `json:"coverage"`
	ExpectedSessions *int               `json:"expected_sessions"`
	MissingSessions  *int               `json:"missing_sessions"`
	Sessions         []AdminRehaSession `json:"sessions"`
}

// AdminRehaSessions reads native Admin observations only. It never joins by
// a name/time to mySIGN, never claims an account/location not carried by the DB,
// and defines management participation as source-marked attendance plus signature.
// Cancellation is kept independently unknown; it is never guessed or subtracted.
func (store *ReadOnlyStore) AdminRehaSessions(ctx context.Context, from, to, asOf time.Time) (AdminRehaSessionReport, error) {
	if store == nil || store.db == nil {
		return AdminRehaSessionReport{}, errors.New("read-only database is not open")
	}
	if from.Location().String() != rehaTimezone || to.Location().String() != rehaTimezone || from.After(to) || asOf.IsZero() {
		return AdminRehaSessionReport{}, errors.New("invalid Admin Reha date bounds")
	}
	rows, err := store.db.QueryContext(ctx, `SELECT request_path,observed_at,page_json FROM admin_observations WHERE route='capability:course-session' ORDER BY run_id`)
	if err != nil {
		return AdminRehaSessionReport{}, errors.New("Admin session observation archive unavailable")
	}
	defer rows.Close()
	byID := map[string]AdminRehaSession{}
	capability, _ := readcatalog.Lookup("course-session")
	for rows.Next() {
		var requestPath, observedAt, raw string
		if err := rows.Scan(&requestPath, &observedAt, &raw); err != nil {
			return AdminRehaSessionReport{}, err
		}
		u, err := url.ParseRequestURI(requestPath)
		if err != nil || u.Path != capability.Path || u.IsAbs() {
			return AdminRehaSessionReport{}, errors.New("invalid Admin session request provenance")
		}
		q := u.Query()
		for k, v := range q {
			if (k != "Kurs" && k != "Datum" && k != "defaultMode") || len(v) != 1 {
				return AdminRehaSessionReport{}, errors.New("ambiguous Admin session request identity")
			}
		}
		var day time.Time
		for _, layout := range []string{"2006-01-02", "02.01.2006", "2.1.2006"} {
			day, err = time.ParseInLocation(layout, q.Get("Datum"), from.Location())
			if err == nil {
				break
			}
		}
		if err != nil {
			return AdminRehaSessionReport{}, errors.New("invalid Admin session date")
		}
		params := map[string]string{"course-id": q.Get("Kurs"), "date": day.Format("2006-01-02")}
		if q.Get("defaultMode") != "" {
			params["planner"] = q.Get("defaultMode")
		}
		if _, err := readcatalog.Build("course-session", params); err != nil {
			return AdminRehaSessionReport{}, errors.New("invalid Admin course identity")
		}
		if day.Before(from) || day.After(to) {
			continue
		}
		stamp, err := parseDataTimestamp(observedAt)
		if err != nil {
			return AdminRehaSessionReport{}, errors.New("invalid Admin session observation timestamp")
		}
		if stamp.After(asOf) {
			continue
		}
		var page admin.Page
		if json.Unmarshal([]byte(raw), &page) != nil || page.Metadata.Route != "capability:course-session" {
			return AdminRehaSessionReport{}, errors.New("invalid Admin session archive")
		}
		f := page.CourseSessionFacts
		// Old text-only archives carry no proof of image-based states.
		if f == nil {
			return AdminRehaSessionReport{}, errors.New("Admin session archive lacks typed roster facts; reparse private source evidence")
		}
		if f.Completeness != "validated_observed_roster" || f.Registered < 0 || f.AttendanceMarked < 0 || f.SignedAttendance < 0 || f.SignedAttendance > f.AttendanceMarked || f.AttendanceMarked > f.Registered || f.Cancelled != nil || f.Capacity != nil || f.CancellationStatus != "unknown_not_provided" {
			return AdminRehaSessionReport{}, errors.New("invalid Admin roster fact contract")
		}
		if page.CourseSessionIdentity == nil || page.CourseSessionIdentity.BookingID == "" || page.CourseSessionIdentity.Date != day.Format("2006-01-02") {
			return AdminRehaSessionReport{}, errors.New("missing or conflicting native Admin booking identity")
		}
		id := adminRehaPseudonym("session", page.CourseSessionIdentity.BookingID)
		var planner *string
		if q.Get("defaultMode") != "" {
			v := q.Get("defaultMode")
			planner = &v
		}
		item := AdminRehaSession{StableSessionID: id, StableCourseID: adminRehaPseudonym("course", q.Get("Kurs")), Date: day.Format("2006-01-02"), Planner: planner, ObservedAt: stamp.UTC().Format(time.RFC3339Nano), Registered: f.Registered, AttendanceMarked: f.AttendanceMarked, SignedAttendance: f.SignedAttendance, Completeness: f.Completeness, CancellationStatus: f.CancellationStatus}
		participated := f.SignedAttendance
		item.Participated = &participated
		item.ParticipationBasis = "same_native_roster_row:anwesend_haken.png+unterschrift_gruen.png"
		item.PeriodStatus = "provisional"
		localToday := asOf.In(from.Location()).Format("2006-01-02")
		if item.Date < localToday {
			item.PeriodStatus = "past_local_day"
		}
		old, exists := byID[id]
		if exists && (old.Date != item.Date || old.StableCourseID != item.StableCourseID) {
			return AdminRehaSessionReport{}, errors.New("conflicting native Admin booking course relationship")
		}
		oldStamp, _ := parseDataTimestamp(old.ObservedAt)
		if !exists || stamp.After(oldStamp) {
			byID[id] = item
		}
	}
	if err := rows.Err(); err != nil {
		return AdminRehaSessionReport{}, err
	}
	result := AdminRehaSessionReport{SchemaVersion: "admin-reha-sessions.v1", Source: "myyolo-admin", RequestedFrom: from.Format("2006-01-02"), RequestedTo: to.Format("2006-01-02"), AsOf: asOf.UTC().Format(time.RFC3339Nano), Timezone: rehaTimezone, Coverage: "observed_native_sessions_only", Sessions: []AdminRehaSession{}}
	for _, row := range byID {
		result.Sessions = append(result.Sessions, row)
	}
	sort.Slice(result.Sessions, func(i, j int) bool {
		if result.Sessions[i].Date != result.Sessions[j].Date {
			return result.Sessions[i].Date < result.Sessions[j].Date
		}
		return result.Sessions[i].StableSessionID < result.Sessions[j].StableSessionID
	})
	return result, nil
}
func adminRehaPseudonym(kind string, parts ...string) string {
	sum := sha256.Sum256([]byte("myyolo-admin-reha-" + kind + "-v1\x00" + strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
