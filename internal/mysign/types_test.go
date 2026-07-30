package mysign

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSupportsArraysAndKeyedObjects(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(snapshot.CourseSessions), 2; got != want {
		t.Fatalf("sessions = %d, want %d", got, want)
	}
	if got, want := len(snapshot.Members), 2; got != want {
		t.Fatalf("members = %d, want %d", got, want)
	}
	if got := string(snapshot.Members[0].ID); got != "501" {
		t.Fatalf("first keyed member ID = %q, want 501", got)
	}
	if !bool(snapshot.Attendance[0].Attended) {
		t.Fatal("numeric boolean was not parsed")
	}
}

func TestParseRejectsUnknownPayload(t *testing.T) {
	if _, err := Parse([]byte(`{"login":true}`)); err == nil {
		t.Fatal("expected empty/unknown payload to fail closed")
	}
}

func TestParseRejectsMissingAttendanceMetric(t *testing.T) {
	payload := `{
		"KursBuchungen":[{"Id":1}],
		"Mitglieder":[{"Id":2}],
		"Verordnungen":[],
		"KursTeilnehmer":[{
			"Id":3,
			"MitgliedId":2,
			"KursBuchungId":1,
			"HatUnterschrift":false,
			"Storniert":false
		}]
	}`
	if _, err := Parse([]byte(payload)); err == nil {
		t.Fatal("expected missing Teilgenommen to fail closed")
	}
}

func TestParseRejectsUnknownAttendanceReference(t *testing.T) {
	payload := `{
		"KursBuchungen":[{"Id":1}],
		"Mitglieder":[{"Id":2}],
		"Verordnungen":[],
		"KursTeilnehmer":[{
			"Id":3,
			"MitgliedId":999,
			"KursBuchungId":1,
			"HatUnterschrift":false,
			"Teilgenommen":false,
			"Storniert":false
		}]
	}`
	if _, err := Parse([]byte(payload)); err == nil {
		t.Fatal("expected unknown member reference to fail closed")
	}
}

func TestParseAcceptsNullOptionalCollection(t *testing.T) {
	payload := `{
		"KursBuchungen":[{"Id":1}],
		"Mitglieder":[],
		"Verordnungen":null,
		"KursTeilnehmer":[]
	}`
	if _, err := Parse([]byte(payload)); err != nil {
		t.Fatalf("optional null collection was rejected: %v", err)
	}
}
