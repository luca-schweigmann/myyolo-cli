package mysign

import (
	"encoding/json"
	"testing"
)

func TestRequiredParticipantCountProvenance(t *testing.T) {
	for _, input := range []string{`{}`, `{"TeilnehmerAnzahl":null}`, `{"TeilnehmerAnzahl":-1}`, `{"TeilnehmerAnzahl":1.5}`, `{"TeilnehmerAnzahl":"2"}`} {
		var session CourseSession
		if err := json.Unmarshal([]byte(input), &session); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	var session CourseSession
	if err := json.Unmarshal([]byte(`{"TeilnehmerAnzahl":0}`), &session); err != nil || !session.ParticipantCountValidated() {
		t.Fatalf("valid zero rejected: %v", err)
	}
	if (CourseSession{ParticipantCount: 0}).ParticipantCountValidated() {
		t.Fatal("default value acquired wire provenance")
	}
}

func TestDuplicateMemberSessionRejected(t *testing.T) {
	snapshot := Snapshot{CourseSessions: []CourseSession{{ID: "s"}}, Members: []Member{{ID: "m"}}, Attendance: []Attendance{{ID: "a", MemberID: "m", CourseSessionID: "s"}, {ID: "b", MemberID: "m", CourseSessionID: "s"}}}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("duplicate pair accepted")
	}
}

func TestRequiredRehaCollectionPresence(t *testing.T) {
	for _, payload := range []string{
		`{"KursBuchungen":[{"Id":"s","TeilnehmerAnzahl":3}]}`,
		`{"KursBuchungen":[{"Id":"s","TeilnehmerAnzahl":3}],"KursTeilnehmer":null}`,
		`{"KursBuchungen":[{"Id":"s","TeilnehmerAnzahl":3}],"KursTeilnehmer":false}`,
		`{"KursBuchungen":null,"KursTeilnehmer":[]}`,
		`{"KursTeilnehmer":[]}`,
	} {
		if _, err := Parse([]byte(payload)); err == nil {
			t.Fatalf("accepted incomplete collections: %s", payload)
		}
	}
	for _, payload := range []string{`{"KursBuchungen":[{"Id":"s","TeilnehmerAnzahl":3}],"KursTeilnehmer":[]}`, `{"KursBuchungen":[],"KursTeilnehmer":[]}`, `{"KursBuchungen":{},"KursTeilnehmer":{}}`} {
		snapshot, err := Parse([]byte(payload))
		if err != nil || !snapshot.RequiredCollectionsValidated() {
			t.Fatalf("explicit collections rejected: %v", err)
		}
	}
	if (Snapshot{}).RequiredCollectionsValidated() {
		t.Fatal("programmatic snapshot acquired provenance")
	}
}
