package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
	_ "modernc.org/sqlite"
)

func TestCreateSnapshotUsesReadOnlySourceAndBindsExternalEvidence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "snapshot.scope.json")

	result, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != schemaVersion || result.SnapshotSHA256 == "" {
		t.Fatalf("snapshot result = %#v", result)
	}
	for _, path := range []string{outputPath, receiptPath} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
			t.Fatalf("%s mode = %o, want %o", filepath.Base(path), got, want)
		}
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptForSnapshot(receipt, outputPath, "point-gerlingen"); err != nil {
		t.Fatal(err)
	}
	if receipt.Evidence.Kind != "independent-local-receipt" || receipt.Provenance != "external_verified_scope" {
		t.Fatalf("receipt lost external evidence: %#v", receipt)
	}
	if err := VerifySnapshotHash(outputPath, receipt.SnapshotSHA256); err != nil {
		t.Fatal(err)
	}
	if err := VerifySnapshotHash(outputPath, strings.Repeat("a", sha256.Size*2)); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong snapshot hash error = %v", err)
	}

	readonly, err := OpenReadOnly(ctx, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	from := mustBerlinDate(t, "2026-07-28")
	to := mustBerlinDate(t, "2026-07-29")
	report, err := readonly.RehaSessions(ctx, receipt.Source, receipt.Location, from, to, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), receipt.SnapshotSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) != 2 || report.ScopeBasis != "external_verified_scope" {
		t.Fatalf("report = %#v", report)
	}
	serialized, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	assertAggregateJSONOmitsPrivateData(t, serialized)
}

func TestCreateSnapshotRejectsBareLocationAssertion(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "bare-scope.json")
	if err := os.WriteFile(scopePath, []byte(`{"version":1,"source_db":"`+sourcePath+`","source":"mysign","location":"point-gerlingen"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := CreateSnapshot(context.Background(), sourcePath, filepath.Join(dir, "snapshot.sqlite"), scopePath, filepath.Join(dir, "receipt.json"))
	if err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("bare scope error = %v", err)
	}
}

func TestCreateSnapshotRequiresBoundEvidenceReceipt(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, scopePath, evidencePath string)
	}{
		{
			name: "missing evidence file",
			mutate: func(t *testing.T, _, evidencePath string) {
				if err := os.Remove(evidencePath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "evidence symlink",
			mutate: func(t *testing.T, _, evidencePath string) {
				target := evidencePath + ".target"
				data, err := os.ReadFile(evidencePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(evidencePath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, evidencePath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong evidence hash",
			mutate: func(t *testing.T, scopePath, _ string) {
				var scope RehaScopeInput
				readJSONFile(t, scopePath, &scope)
				scope.Evidence.SHA256 = strings.Repeat("a", sha256.Size*2)
				writeJSONFile(t, scopePath, scope)
			},
		},
		{
			name: "scope content mismatch",
			mutate: func(t *testing.T, scopePath, evidencePath string) {
				var scope RehaScopeInput
				readJSONFile(t, scopePath, &scope)
				evidence := RehaEvidenceReceipt{
					Version:    1,
					SourceDB:   scope.SourceDB,
					Source:     "mysign",
					Location:   "point-ditzingen",
					VerifiedAt: scope.Evidence.VerifiedAt,
					Kind:       "independent-local-receipt",
				}
				data, err := json.Marshal(evidence)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, '\n')
				if err := os.WriteFile(evidencePath, data, 0o600); err != nil {
					t.Fatal(err)
				}
				scope.Evidence.SHA256 = digestBytes(data)
				writeJSONFile(t, scopePath, scope)
			},
		},
		{
			name: "permissive evidence mode",
			mutate: func(t *testing.T, _, evidencePath string) {
				if err := os.Chmod(evidencePath, 0o640); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unknown evidence field",
			mutate: func(t *testing.T, scopePath, evidencePath string) {
				var scope RehaScopeInput
				readJSONFile(t, scopePath, &scope)
				data := []byte(`{"version":1,"source_db":"` + scope.SourceDB + `","source":"mysign","location":"` + scope.Location + `","verified_at":"` + scope.Evidence.VerifiedAt + `","kind":"independent-local-receipt","unexpected":"private"}`)
				data = append(data, '\n')
				if err := os.WriteFile(evidencePath, data, 0o600); err != nil {
					t.Fatal(err)
				}
				scope.Evidence.SHA256 = digestBytes(data)
				writeJSONFile(t, scopePath, scope)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "source.sqlite")
			prepareRehaSource(t, sourcePath)
			scopePath := filepath.Join(dir, "scope.json")
			writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
			evidencePath := scopePath + ".evidence.json"
			test.mutate(t, scopePath, evidencePath)
			outputPath := filepath.Join(dir, "snapshot.sqlite")
			receiptPath := filepath.Join(dir, "receipt.json")
			if _, err := CreateSnapshot(context.Background(), sourcePath, outputPath, scopePath, receiptPath); err == nil {
				t.Fatal("unbound evidence unexpectedly accepted")
			}
			for _, path := range []string{outputPath, receiptPath} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("artifact %s unexpectedly exists: %v", filepath.Base(path), err)
				}
			}
		})
	}
}

func TestCreateSnapshotIncludesCommittedAndExcludesUncommittedWALRows(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	writer, err := sql.Open("sqlite", sqliteDSN(sourcePath))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)
	if _, err := writer.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `
		INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,description,room_name,time_formatted,participant_count,updated_at)
		VALUES('mysign','committed-wal','2026-07-30T10:00:00+02:00','30.07.2026','synthetic','synthetic','10:00 - 10:45',3,'2026-07-30T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	beforeHash, err := fileSHA256(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeMode, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := writer.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,description,room_name,time_formatted,participant_count,updated_at)
		VALUES('mysign','uncommitted-wal','2026-07-31T10:00:00+02:00','31.07.2026','synthetic','synthetic','10:00 - 10:45',4,'2026-07-31T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	afterHash, err := fileSHA256(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	afterMode, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if afterHash != beforeHash || afterMode.Mode().Perm() != beforeMode.Mode().Perm() {
		t.Fatalf("snapshot changed source bytes or mode: before %s/%o after %s/%o", beforeHash, beforeMode.Mode().Perm(), afterHash, afterMode.Mode().Perm())
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(ctx, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	var count int
	if err := readonly.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM course_sessions WHERE myyolo_id IN ('committed-wal','uncommitted-wal')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("snapshot WAL row count = %d, want 1", count)
	}
}

func TestOpenReadOnlyRejectsUnknownSchemaAndDoesNotCreateMissingSource(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.sqlite")
	if _, err := OpenReadOnly(context.Background(), missing); err == nil {
		t.Fatal("missing source unexpectedly opened")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing source stat error = %v", err)
	}
	for _, test := range []struct {
		name      string
		statement string
	}{
		{name: "old schema", statement: "PRAGMA user_version=4"},
		{name: "new schema", statement: "PRAGMA user_version=7"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(test.name, " ", "-")+".sqlite")
			prepareRehaSource(t, path)
			db, err := sql.Open("sqlite", sqliteDSN(path))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.statement); err != nil {
				db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenReadOnly(context.Background(), path); err == nil || !strings.Contains(err.Error(), "schema version") {
				t.Fatalf("schema error = %v", err)
			}
		})
	}
}

func TestCreateSnapshotAndOpenReadOnlyAcceptSchema6(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source-v6.sqlite")
	prepareRehaSource(t, sourcePath)

	db, err := sql.Open("sqlite", sqliteDSN(sourcePath))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE UNIQUE INDEX attendance_source_member_session_unique ON attendance(source, member_id, course_session_id)`,
		`PRAGMA user_version=6`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "snapshot.scope.json")
	result, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 6 {
		t.Fatalf("snapshot schema version = %d, want 6", result.SchemaVersion)
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SourceSchema != 6 {
		t.Fatalf("receipt schema version = %d, want 6", receipt.SourceSchema)
	}
	if err := ValidateReceiptForSnapshot(receipt, outputPath, "point-gerlingen"); err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(ctx, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotSchema int
	if err := readonly.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&snapshotSchema); err != nil {
		readonly.Close()
		t.Fatal(err)
	}
	if snapshotSchema != 6 {
		readonly.Close()
		t.Fatalf("snapshot schema version = %d, want 6", snapshotSchema)
	}
	report, err := readonly.RehaSessions(
		ctx,
		receipt.Source,
		receipt.Location,
		mustBerlinDate(t, "2026-07-28"),
		mustBerlinDate(t, "2026-07-29"),
		mustUTC(t, "2026-07-30T00:00:00Z"),
		receipt.SnapshotSHA256,
	)
	if err != nil {
		readonly.Close()
		t.Fatal(err)
	}
	if len(report.Sessions) == 0 || report.Location != "point-gerlingen" {
		readonly.Close()
		t.Fatalf("schema-v6 report = %#v", report)
	}
	serialized, err := json.Marshal(report)
	if err != nil {
		readonly.Close()
		t.Fatal(err)
	}
	assertAggregateJSONOmitsPrivateData(t, serialized)
	if err := readonly.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSnapshotRefusesExistingOutputAndReceipt(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(outputPath, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSnapshot(context.Background(), sourcePath, outputPath, scopePath, receiptPath); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	if err := os.Remove(outputPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSnapshot(context.Background(), sourcePath, outputPath, scopePath, receiptPath); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing receipt error = %v", err)
	}
}

func TestCreateSnapshotReservesDestinationAndCleansOwnedPartials(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if err := os.Symlink("somewhere-else", outputPath); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSnapshot(context.Background(), sourcePath, outputPath, scopePath, receiptPath); err == nil {
		t.Fatal("symlink destination unexpectedly accepted")
	}
	if info, err := os.Lstat(outputPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination symlink was changed: info=%v err=%v", info, err)
	}
	if _, err := os.Lstat(receiptPath); !os.IsNotExist(err) {
		t.Fatalf("receipt reservation was not cleaned: %v", err)
	}

	if err := os.Remove(outputPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source-bad.sqlite"), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	badSource := filepath.Join(dir, "source-bad.sqlite")
	badScope := filepath.Join(dir, "bad-scope.json")
	writeRehaScope(t, badScope, badSource, "point-gerlingen")
	badOutput := filepath.Join(dir, "bad-snapshot.sqlite")
	badReceipt := filepath.Join(dir, "bad-receipt.json")
	if _, err := CreateSnapshot(context.Background(), badSource, badOutput, badScope, badReceipt); err == nil {
		t.Fatal("invalid source unexpectedly snapshotted")
	}
	for _, path := range []string{badOutput, badReceipt} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("run-owned partial %s remains: %v", filepath.Base(path), err)
		}
	}
}

func TestRehaSessionsKeepsNeutralAttendanceCandidatesAndNormalizesDataAsOf(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareConflictSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenReadOnly(ctx, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report, err := db.RehaSessions(
		ctx,
		receipt.Source,
		receipt.Location,
		mustBerlinDate(t, "2026-07-28"),
		mustBerlinDate(t, "2026-07-28"),
		mustUTC(t, "2026-07-28T09:00:00Z"),
		receipt.SnapshotSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) != 1 {
		t.Fatalf("sessions = %#v", report.Sessions)
	}
	row := report.Sessions[0]
	if row.AttendanceRows != 2 || row.AttendedFlagTrue != 1 || row.CancelledFlagTrue != 1 ||
		row.AttendedNotCancelled != 0 || row.NonAttendedNotCancelledCandidate != 1 ||
		row.PrescriptionLinkedRows != 0 || row.PrescriptionUnlinkedRows != 2 || row.ContradictoryFlags != 1 {
		t.Fatalf("session counts = %#v", row)
	}
	if report.DataAsOf != "2026-07-28T08:30:00Z" {
		t.Fatalf("data_as_of = %q", report.DataAsOf)
	}
}

func TestRehaSessionsRejectsNegativeParticipantCountAndInvalidDataTimestamp(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name  string
		query string
		want  string
	}{
		{name: "negative participant", query: `UPDATE course_sessions SET participant_count=-1 WHERE myyolo_id='session-1'`, want: "negative participant"},
		{name: "invalid timestamp", query: `UPDATE course_sessions SET updated_at='raw-private-timestamp' WHERE myyolo_id='session-1'`, want: "invalid data timestamp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "source.sqlite")
			prepareConflictSource(t, sourcePath)
			db, err := sql.Open("sqlite", sqliteDSN(sourcePath))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, test.query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			scopePath := filepath.Join(dir, "scope.json")
			writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
			outputPath := filepath.Join(dir, "snapshot.sqlite")
			receiptPath := filepath.Join(dir, "receipt.json")
			if _, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath); err != nil {
				t.Fatal(err)
			}
			receipt, err := ReadSnapshotReceipt(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			readonly, err := OpenReadOnly(ctx, outputPath)
			if err != nil {
				t.Fatal(err)
			}
			defer readonly.Close()
			_, err = readonly.RehaSessions(
				ctx,
				receipt.Source,
				receipt.Location,
				mustBerlinDate(t, "2026-07-28"),
				mustBerlinDate(t, "2026-07-28"),
				mustUTC(t, "2026-07-28T09:00:00Z"),
				receipt.SnapshotSHA256,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSnapshotRejectsPostSnapshotWALEvenWhenMainHashIsUnchanged(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash, err := fileSHA256(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash != receipt.SnapshotSHA256 {
		t.Fatalf("snapshot hash before mutation = %s, receipt = %s", beforeHash, receipt.SnapshotSHA256)
	}
	writer, err := sql.Open("sqlite", sqliteDSN(outputPath))
	if err != nil {
		t.Fatal(err)
	}
	writer.SetMaxOpenConns(1)
	defer writer.Close()
	if _, err := writer.ExecContext(ctx, `PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `
		INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,description,room_name,time_formatted,participant_count,updated_at)
		VALUES('mysign','post-snapshot-wal','2026-07-29T10:00:00+02:00','29.07.2026','tampered','tampered','10:00 - 10:45',1,'2026-07-29T08:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	afterHash, err := fileSHA256(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if afterHash != beforeHash {
		t.Fatalf("post-snapshot WAL changed main hash: before %s after %s", beforeHash, afterHash)
	}
	if !pathExists(outputPath + "-wal") {
		t.Fatal("post-snapshot WAL sidecar was not created")
	}
	if err := VerifySnapshotHash(outputPath, receipt.SnapshotSHA256); err == nil {
		t.Fatal("snapshot with unbound WAL sidecar unexpectedly verified")
	}
	if _, err := OpenReadOnly(ctx, outputPath); err == nil {
		t.Fatal("snapshot with unbound WAL sidecar unexpectedly opened")
	}
}

func TestRehaSessionsRejectsNullAttendanceFlags(t *testing.T) {
	ctx := context.Background()
	for _, nullableFlag := range []string{"attended", "signed", "cancelled"} {
		t.Run(nullableFlag, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "nullable.sqlite")
			db, err := sql.Open("sqlite", sqliteDSN(path))
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			_, err = db.ExecContext(ctx, `
				CREATE TABLE course_sessions(
					source TEXT, myyolo_id TEXT, date_iso TEXT, date_formatted TEXT,
					time_formatted TEXT, participant_count INTEGER, ends_at_utc TEXT, updated_at TEXT
				);
				CREATE TABLE attendance(
					source TEXT, myyolo_id TEXT, course_session_id TEXT, member_id TEXT,
					prescription_id TEXT, attended INTEGER, signed INTEGER, cancelled INTEGER, updated_at TEXT
				);
				PRAGMA user_version=5;
				INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,time_formatted,participant_count,ends_at_utc,updated_at)
				VALUES('mysign','session-null','2026-07-28T08:00:00Z','28.07.2026','10:00 - 10:45',1,'2026-07-28T08:45:00Z','2026-07-28T08:30:00Z');`)
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			values := map[string]string{"attended": "0", "signed": "0", "cancelled": "0"}
			markSyntheticObservation(t, db)
			values[nullableFlag] = "NULL"
			if _, err := db.ExecContext(ctx, `
				INSERT INTO attendance(source,myyolo_id,course_session_id,attended,signed,cancelled,updated_at)
				VALUES('mysign','attendance-null','session-null',`+values["attended"]+`,`+values["signed"]+`,`+values["cancelled"]+`,'2026-07-28T08:20:00Z')`); err != nil {
				db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			readonly, err := OpenReadOnly(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer readonly.Close()
			_, err = readonly.RehaSessions(
				ctx,
				"mysign",
				"point-gerlingen",
				mustBerlinDate(t, "2026-07-28"),
				mustBerlinDate(t, "2026-07-28"),
				mustUTC(t, "2026-07-28T09:00:00Z"),
				strings.Repeat("a", 64),
			)
			if err == nil || !strings.Contains(err.Error(), "invalid") {
				t.Fatalf("NULL %s flag error = %v", nullableFlag, err)
			}
		})
	}
}

func TestBerlinLocationUsesDSTRulesAndConvertsUTCDateCrossing(t *testing.T) {
	location, err := berlinLocation()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := location.String(), rehaTimezone; got != want {
		t.Fatalf("timezone = %q, want %q", got, want)
	}
	for _, test := range []struct {
		name       string
		value      string
		wantDate   string
		wantOffset int
	}{
		{
			name:       "spring before transition",
			value:      "2026-03-29T00:30:00Z",
			wantDate:   "2026-03-29",
			wantOffset: 3600,
		},
		{
			name:       "spring after transition",
			value:      "2026-03-29T01:30:00Z",
			wantDate:   "2026-03-29",
			wantOffset: 7200,
		},
		{
			name:       "fall before transition",
			value:      "2026-10-24T23:30:00Z",
			wantDate:   "2026-10-25",
			wantOffset: 7200,
		},
		{
			name:       "fall after transition",
			value:      "2026-10-25T01:30:00Z",
			wantDate:   "2026-10-25",
			wantOffset: 3600,
		},
		{
			name:       "UTC midnight crossing",
			value:      "2026-03-28T23:30:00Z",
			wantDate:   "2026-03-29",
			wantOffset: 3600,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseSessionDate(test.value, "", location)
			if err != nil {
				t.Fatal(err)
			}
			if got := parsed.Format("2006-01-02"); got != test.wantDate {
				t.Fatalf("local date = %q, want %q", got, test.wantDate)
			}
			_, offset := parsed.Zone()
			if offset != test.wantOffset {
				t.Fatalf("UTC offset = %d, want %d", offset, test.wantOffset)
			}
		})
	}
}

func TestRehaSessionsUsesInclusiveBerlinDatesAndDistinctSourceIDs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	db, err := Open(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	markSyntheticObservation(t, db.db)
	for _, session := range []struct {
		id      string
		dateISO string
		time    string
	}{
		{id: "outside-before", dateISO: "2026-03-28T20:30:00Z", time: "21:30 - 22:15"},
		{id: "spring-cross-midnight", dateISO: "2026-03-28T23:30:00Z", time: "00:30 - 01:15"},
		{id: "spring-after", dateISO: "2026-03-29T01:30:00Z", time: "03:30 - 04:15"},
		{id: "same-looking-a", dateISO: "2026-03-29T01:30:00Z", time: "03:30 - 04:15"},
		{id: "same-looking-b", dateISO: "2026-03-29T01:30:00Z", time: "03:30 - 04:15"},
		{id: "fall-before", dateISO: "2026-10-24T23:30:00Z", time: "01:30 - 02:15"},
		{id: "fall-after", dateISO: "2026-10-25T01:30:00Z", time: "02:30 - 03:15"},
		{id: "outside-after", dateISO: "2026-10-25T23:30:00Z", time: "01:30 - 02:15"},
	} {
		if _, err := db.db.ExecContext(ctx, `
			INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,time_formatted,participant_count,updated_at)
			VALUES('mysign',?,?,?,?,0,'2026-09-05T00:00:00Z')`, session.id, session.dateISO, "synthetic", session.time); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := CreateSnapshot(ctx, sourcePath, outputPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(ctx, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	report, err := readonly.RehaSessions(
		ctx,
		receipt.Source,
		receipt.Location,
		mustBerlinDate(t, "2026-03-29"),
		mustBerlinDate(t, "2026-10-25"),
		mustUTC(t, "2026-11-01T00:00:00Z"),
		receipt.SnapshotSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(report.Sessions), 6; got != want {
		t.Fatalf("sessions in inclusive range = %d, want %d: %#v", got, want, report.Sessions)
	}
	if got, want := report.Coverage.SourceSessions, 8; got != want {
		t.Fatalf("source sessions = %d, want %d", got, want)
	}
	if got, want := report.Coverage.SessionsInRange, 6; got != want {
		t.Fatalf("sessions in range = %d, want %d", got, want)
	}
	if got, want := report.Coverage.ObservedFrom, "2026-03-29"; got != want {
		t.Fatalf("observed from = %q, want %q", got, want)
	}
	if got, want := report.Coverage.ObservedTo, "2026-10-25"; got != want {
		t.Fatalf("observed to = %q, want %q", got, want)
	}
	seen := make(map[string]RehaSessionRow, len(report.Sessions))
	for _, row := range report.Sessions {
		seen[row.StableSessionID] = row
		if row.Date != "2026-03-29" && row.Date != "2026-10-25" {
			t.Fatalf("out-of-range local date included: %#v", row)
		}
	}
	for _, id := range []string{"same-looking-a", "same-looking-b"} {
		stableID := stableSessionID("mysign", id)
		if _, ok := seen[stableID]; !ok {
			t.Fatalf("session %q missing from report", id)
		}
	}
	if stableSessionID("mysign", "same-looking-a") == stableSessionID("mysign", "same-looking-b") {
		t.Fatal("distinct source IDs produced the same stable session ID")
	}
}

func markSyntheticObservation(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE reha_import_observation(singleton INTEGER PRIMARY KEY, observed_at TEXT, validation TEXT); INSERT INTO reha_import_observation VALUES(1,'2026-07-28T08:30:00Z','required_nonnegative_integer_and_collections_v1')`); err != nil {
		t.Fatal(err)
	}
}

func prepareConflictSource(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	markSyntheticObservation(t, db.db)
	_, err = db.db.ExecContext(ctx, `
		INSERT INTO members(source,myyolo_id,member_number,first_name,last_name,updated_at)
		VALUES('mysign','member-1','synthetic-1','Synthetic','One','2026-07-28T08:00:00Z'),
		      ('mysign','member-2','synthetic-2','Synthetic','Two','2026-07-28T08:00:00Z')`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	_, err = db.db.ExecContext(ctx, `
		INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,description,room_name,time_formatted,ends_at_utc,participant_count,updated_at)
		VALUES('mysign','session-1','2026-07-28T08:00:00+02:00','28.07.2026','synthetic course','synthetic room','10:00 - 10:45','2026-07-28T08:45:00Z',2,'2026-07-28T08:30:00Z')`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	_, err = db.db.ExecContext(ctx, `
		INSERT INTO attendance(source,myyolo_id,member_id,course_session_id,attended,signed,cancelled,updated_at)
		VALUES('mysign','attendance-conflict','member-1','session-1',1,0,1,'2026-07-28T08:20:00Z'),
		      ('mysign','attendance-absent','member-2','session-1',0,0,0,'2026-07-28T08:25:00Z')`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func prepareRehaSource(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportMySign(context.Background(), snapshot, "synthetic"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeRehaScope(t *testing.T, path, sourcePath, location string) {
	t.Helper()
	canonical, err := filepath.Abs(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath, err := filepath.Abs(path + ".evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	evidence := RehaEvidenceReceipt{
		Version:    1,
		SourceDB:   canonical,
		Source:     "mysign",
		Location:   location,
		VerifiedAt: "2026-09-05T00:00:00Z",
		Kind:       "independent-local-receipt",
	}
	evidenceData, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidenceData = append(evidenceData, '\n')
	if err := os.WriteFile(evidencePath, evidenceData, 0o600); err != nil {
		t.Fatal(err)
	}
	evidenceHash, err := fileSHA256(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	scope := RehaScopeInput{
		Version:    1,
		SourceDB:   canonical,
		Source:     "mysign",
		Location:   location,
		Provenance: "external_verified_scope",
		Evidence: RehaScopeEvidence{
			Kind:       "independent-local-receipt",
			Reference:  evidencePath,
			SHA256:     evidenceHash,
			VerifiedAt: "2026-09-05T00:00:00Z",
			SourceDB:   canonical,
			Source:     "mysign",
			Location:   location,
		},
	}
	data, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustBerlinDate(t *testing.T, value string) time.Time {
	t.Helper()
	location, err := berlinLocation()
	if err != nil {
		t.Fatal(err)
	}
	date, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

func mustUTC(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
