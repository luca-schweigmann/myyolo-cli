package mysign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// ID accepts the numeric and string identifiers both observed in classic
// ASP/JavaScript payloads while keeping the database key stable as text.
type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*id = ""
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*id = ID(text)
		return nil
	}

	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("identifier must be string or number: %w", err)
	}
	*id = ID(number.String())
	return nil
}

type Bool bool

func (value *Bool) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return fmt.Errorf("boolean must not be null")
	}

	var boolean bool
	if err := json.Unmarshal(data, &boolean); err == nil {
		*value = Bool(boolean)
		return nil
	}

	var number int
	if err := json.Unmarshal(data, &number); err == nil && (number == 0 || number == 1) {
		*value = Bool(number == 1)
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		parsed, err := strconv.ParseBool(text)
		if err == nil {
			*value = Bool(parsed)
			return nil
		}
		if text == "0" || text == "1" {
			*value = Bool(text == "1")
			return nil
		}
	}

	return fmt.Errorf("boolean must be true/false or 0/1")
}

// Text normalizes enum-like source fields that mySIGN emits as either JSON
// numbers or strings depending on server/version.
type Text string

func (value *Text) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*value = ""
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*value = Text(text)
		return nil
	}

	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("text value must be string, number or null: %w", err)
	}
	*value = Text(number.String())
	return nil
}

type Snapshot struct {
	NextRequestToken string `json:"nextRequestToken"`
	CourseSessions   []CourseSession
	Attendance       []Attendance
	Members          []Member
	Prescriptions    []Prescription
}

type wireSnapshot struct {
	NextRequestToken string                    `json:"nextRequestToken"`
	CourseSessions   collection[CourseSession] `json:"KursBuchungen"`
	Attendance       collection[Attendance]    `json:"KursTeilnehmer"`
	Members          collection[Member]        `json:"Mitglieder"`
	Prescriptions    collection[Prescription]  `json:"Verordnungen"`
}

type collection[T any] []T

func (items *collection[T]) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty collection")
	}
	if bytes.Equal(trimmed, []byte("null")) {
		*items = []T{}
		return nil
	}

	var list []T
	if trimmed[0] == '[' {
		if err := json.Unmarshal(data, &list); err != nil {
			return fmt.Errorf("decode collection array: %w", err)
		}
		*items = list
		return nil
	}

	if trimmed[0] != '{' {
		return fmt.Errorf("collection must be array or object map")
	}
	var keyed map[string]T
	if err := json.Unmarshal(data, &keyed); err != nil {
		return fmt.Errorf("decode collection object map: %w", err)
	}

	keys := make([]string, 0, len(keyed))
	for key := range keyed {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	list = make([]T, 0, len(keys))
	for _, key := range keys {
		item := keyed[key]
		if getItemID(item) == "" {
			setItemID(&item, ID(key))
		}
		list = append(list, item)
	}
	*items = list
	return nil
}

func getItemID[T any](item T) ID {
	switch value := any(item).(type) {
	case CourseSession:
		return value.ID
	case Attendance:
		return value.ID
	case Member:
		return value.ID
	case Prescription:
		return value.ID
	default:
		return ""
	}
}

func setItemID[T any](item *T, id ID) {
	switch value := any(item).(type) {
	case *CourseSession:
		value.ID = id
	case *Attendance:
		value.ID = id
	case *Member:
		value.ID = id
	case *Prescription:
		value.ID = id
	}
}

func Parse(data []byte) (Snapshot, error) {
	var wire wireSnapshot
	if err := json.Unmarshal(data, &wire); err != nil {
		return Snapshot{}, fmt.Errorf("decode mySIGN snapshot: %w", err)
	}

	snapshot := Snapshot{
		NextRequestToken: wire.NextRequestToken,
		CourseSessions:   []CourseSession(wire.CourseSessions),
		Attendance:       []Attendance(wire.Attendance),
		Members:          []Member(wire.Members),
		Prescriptions:    []Prescription(wire.Prescriptions),
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (snapshot Snapshot) Validate() error {
	if len(snapshot.CourseSessions) == 0 &&
		len(snapshot.Attendance) == 0 &&
		len(snapshot.Members) == 0 &&
		len(snapshot.Prescriptions) == 0 {
		return fmt.Errorf("mySIGN snapshot contains no recognized collections")
	}

	sessionIDs := make(map[ID]struct{}, len(snapshot.CourseSessions))
	for _, session := range snapshot.CourseSessions {
		if session.ID == "" {
			return fmt.Errorf("course session without Id")
		}
		if _, exists := sessionIDs[session.ID]; exists {
			return fmt.Errorf("duplicate course session Id %s", session.ID)
		}
		sessionIDs[session.ID] = struct{}{}
	}

	memberIDs := make(map[ID]struct{}, len(snapshot.Members))
	memberNumbers := make(map[string]ID, len(snapshot.Members))
	for _, member := range snapshot.Members {
		if member.ID == "" {
			return fmt.Errorf("member without Id")
		}
		if _, exists := memberIDs[member.ID]; exists {
			return fmt.Errorf("duplicate member Id %s", member.ID)
		}
		memberIDs[member.ID] = struct{}{}
		if member.MemberNumber != "" {
			if previous, exists := memberNumbers[member.MemberNumber]; exists {
				return fmt.Errorf(
					"duplicate member number assigned to %s and %s",
					previous,
					member.ID,
				)
			}
			memberNumbers[member.MemberNumber] = member.ID
		}
	}

	prescriptionIDs := make(map[ID]struct{}, len(snapshot.Prescriptions))
	for _, prescription := range snapshot.Prescriptions {
		if prescription.ID == "" {
			return fmt.Errorf("prescription without Id")
		}
		if _, exists := prescriptionIDs[prescription.ID]; exists {
			return fmt.Errorf("duplicate prescription Id %s", prescription.ID)
		}
		prescriptionIDs[prescription.ID] = struct{}{}
	}

	attendanceIDs := make(map[ID]struct{}, len(snapshot.Attendance))
	for _, attendance := range snapshot.Attendance {
		if attendance.ID == "" {
			return fmt.Errorf("attendance row without Id")
		}
		if attendance.MemberID == "" || attendance.CourseSessionID == "" {
			return fmt.Errorf("attendance %s lacks MitgliedId or KursBuchungId", attendance.ID)
		}
		if _, exists := attendanceIDs[attendance.ID]; exists {
			return fmt.Errorf("duplicate attendance Id %s", attendance.ID)
		}
		attendanceIDs[attendance.ID] = struct{}{}
		if _, exists := memberIDs[attendance.MemberID]; !exists {
			return fmt.Errorf("attendance %s references unknown member %s", attendance.ID, attendance.MemberID)
		}
		if _, exists := sessionIDs[attendance.CourseSessionID]; !exists {
			return fmt.Errorf(
				"attendance %s references unknown course session %s",
				attendance.ID,
				attendance.CourseSessionID,
			)
		}
		if attendance.PrescriptionID != "" {
			if _, exists := prescriptionIDs[attendance.PrescriptionID]; !exists {
				return fmt.Errorf(
					"attendance %s references unknown prescription %s",
					attendance.ID,
					attendance.PrescriptionID,
				)
			}
		}
	}
	return nil
}

type CourseSession struct {
	ID                    ID     `json:"Id"`
	DateISO               string `json:"DatumISO8601"`
	DateFormatted         string `json:"DatumFormatted"`
	Description           string `json:"Beschreibung"`
	RoomName              string `json:"RaumName"`
	TimeFormatted         string `json:"VonBisFormatted"`
	ParticipantCount      int    `json:"TeilnehmerAnzahl"`
	CurrentWeek           Bool   `json:"IstInAktuellerWoche"`
	SignatureAvailableISO string `json:"DatumUnterschriftAbISO8601"`
}

type Attendance struct {
	ID                     ID     `json:"Id"`
	MemberID               ID     `json:"MitgliedId"`
	CourseSessionID        ID     `json:"KursBuchungId"`
	PrescriptionID         ID     `json:"VerordnungId"`
	Signed                 Bool   `json:"HatUnterschrift"`
	TrainingPreviousDay    Bool   `json:"TrainingAmVortag"`
	SignedManually         Bool   `json:"UnterschriftManuell"`
	Attended               Bool   `json:"Teilgenommen"`
	Cancelled              Bool   `json:"Storniert"`
	ManualMemberTime       Bool   `json:"IstManuelleMitgliedzeit"`
	CurrentWeek            Bool   `json:"IstInAktuellerWoche"`
	Date                   string `json:"Datum"`
	DateFormatted          string `json:"DatumFormatted"`
	TimeFormatted          string `json:"VonBisFormatted"`
	Course                 string `json:"Kurs"`
	RemainingUnits         int    `json:"RestEinheiten"`
	Type                   Text   `json:"Type"`
	PrescriptionType       Text   `json:"VerordnungTyp"`
	ManualPrescriptionType Text   `json:"ManuellerTerminVerordnungType"`
}

func (item *Attendance) UnmarshalJSON(data []byte) error {
	type attendanceAlias Attendance
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, field := range []string{
		"Id",
		"MitgliedId",
		"KursBuchungId",
		"HatUnterschrift",
		"Teilgenommen",
		"Storniert",
	} {
		if _, exists := raw[field]; !exists {
			return fmt.Errorf("attendance field %s is required", field)
		}
	}

	var decoded attendanceAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*item = Attendance(decoded)
	return nil
}

type Member struct {
	ID           ID     `json:"Id"`
	LastName     string `json:"Name"`
	FirstName    string `json:"Vorname"`
	MemberNumber string `json:"MitgliedsNummer"`
}

type Prescription struct {
	ID               ID     `json:"Id"`
	EndText          string `json:"EndeText"`
	TreatmentCount   int    `json:"AnzahlBehandlungen"`
	WeeklyTreatments int    `json:"AnzahlBehandlungenWoechentlich"`
	Visits           int    `json:"Besuche"`
}
