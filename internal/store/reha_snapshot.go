package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	sqlite "modernc.org/sqlite"
)

// CreateSnapshot makes a transaction-consistent SQLite online backup from a
// pre-existing supported Reha source. The source is opened with mode=ro and
// query_only=1; only the destination database and receipt are written.
func CreateSnapshot(
	ctx context.Context,
	sourcePath string,
	outputPath string,
	scopeInputPath string,
	receiptPath string,
) (SnapshotResult, error) {
	sourceAbs, err := absolutePath(sourcePath, "source database")
	if err != nil {
		return SnapshotResult{}, err
	}
	outputAbs, err := absolutePath(outputPath, "snapshot database")
	if err != nil {
		return SnapshotResult{}, err
	}
	scopeAbs, err := absolutePath(scopeInputPath, "scope input")
	if err != nil {
		return SnapshotResult{}, err
	}
	receiptAbs, err := absolutePath(receiptPath, "receipt")
	if err != nil {
		return SnapshotResult{}, err
	}
	if sourceAbs == outputAbs {
		return SnapshotResult{}, errors.New("source database and snapshot database must differ")
	}
	if outputAbs == receiptAbs || sourceAbs == receiptAbs {
		return SnapshotResult{}, errors.New("receipt path must differ from both databases")
	}
	for _, base := range []string{outputAbs, sourceAbs} {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			sidecar := base + suffix
			if receiptAbs == sidecar || sourceAbs == sidecar || outputAbs == sidecar {
				return SnapshotResult{}, errors.New("snapshot paths must not overlap SQLite sidecars")
			}
		}
	}

	scopeBytes, err := os.ReadFile(scopeAbs)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("read scope input: %w", err)
	}
	scope, err := parseScopeInput(scopeBytes)
	if err != nil {
		return SnapshotResult{}, err
	}
	if scoped, err := absolutePath(scope.SourceDB, "scope source database"); err != nil {
		return SnapshotResult{}, err
	} else if scoped != sourceAbs {
		return SnapshotResult{}, errors.New("scope input source database does not match --source-db")
	}
	if !isCanonicalAbsolute(scope.SourceDB) {
		return SnapshotResult{}, errors.New("scope input source_db must be canonical absolute")
	}
	if scoped, err := absolutePath(scope.Evidence.SourceDB, "scope evidence source database"); err != nil {
		return SnapshotResult{}, err
	} else if scoped != sourceAbs {
		return SnapshotResult{}, errors.New("scope evidence source database does not match --source-db")
	}
	if err := validateEvidenceReceipt(scope.Evidence, scope.Source, sourceAbs, scope.Location); err != nil {
		return SnapshotResult{}, err
	}
	sourceInfo, err := os.Stat(sourceAbs)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("stat source database: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return SnapshotResult{}, errors.New("source database is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(outputAbs), 0o700); err != nil {
		return SnapshotResult{}, fmt.Errorf("create snapshot directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(receiptAbs), 0o700); err != nil {
		return SnapshotResult{}, fmt.Errorf("create receipt directory: %w", err)
	}
	// Reserve both output paths before opening SQLite. O_EXCL makes the
	// reservation race-safe and rejects symlinks/competing files; mode 0600 is
	// in force before SQLite can write even the first database page.
	if err := rejectSnapshotSidecars(outputAbs); err != nil {
		return SnapshotResult{}, err
	}
	receiptFile, err := reservePrivateFile(receiptAbs, "receipt")
	if err != nil {
		return SnapshotResult{}, err
	}
	outputFile, err := reservePrivateFile(outputAbs, "snapshot database")
	if err != nil {
		_ = receiptFile.Close()
		removeOwnedPath(receiptFile)
		return SnapshotResult{}, err
	}
	success := false
	defer func() {
		_ = outputFile.Close()
		_ = receiptFile.Close()
		if !success {
			removeSnapshotArtifacts(outputFile)
			removeOwnedPath(receiptFile)
		}
	}()

	started := time.Now().UTC()
	sourceDB, err := sql.Open("sqlite", readOnlyDSN(sourceAbs))
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("open source database read-only: %w", err)
	}
	sourceDB.SetMaxOpenConns(1)
	conn, err := sourceDB.Conn(ctx)
	if err != nil {
		_ = sourceDB.Close()
		return SnapshotResult{}, fmt.Errorf("acquire source database connection: %w", err)
	}
	schema, err := validateReadOnlySchema(ctx, conn)
	if err != nil {
		_ = conn.Close()
		_ = sourceDB.Close()
		return SnapshotResult{}, err
	}
	if err := onlineBackup(ctx, conn, outputAbs); err != nil {
		_ = conn.Close()
		_ = sourceDB.Close()
		return SnapshotResult{}, err
	}
	if err := conn.Close(); err != nil {
		_ = sourceDB.Close()
		return SnapshotResult{}, fmt.Errorf("close source database: %w", err)
	}
	if err := sourceDB.Close(); err != nil {
		return SnapshotResult{}, fmt.Errorf("close source database pool: %w", err)
	}
	if err := outputFile.Sync(); err != nil {
		return SnapshotResult{}, fmt.Errorf("sync snapshot database: %w", err)
	}
	if err := outputFile.Close(); err != nil {
		return SnapshotResult{}, fmt.Errorf("close snapshot database: %w", err)
	}
	if err := protectSQLiteFiles(outputAbs); err != nil {
		return SnapshotResult{}, err
	}
	if err := rejectSnapshotSidecars(outputAbs); err != nil {
		return SnapshotResult{}, err
	}
	snapshotHash, err := fileSHA256(outputAbs)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("hash snapshot database: %w", err)
	}
	completed := time.Now().UTC()
	receipt := SnapshotReceipt{
		Version:          rehaReceiptVersion,
		Source:           scope.Source,
		Location:         scope.Location,
		Provenance:       scope.Provenance,
		Evidence:         scope.Evidence,
		SourceDB:         sourceAbs,
		SnapshotDB:       outputAbs,
		SourceSchema:     schema,
		SnapshotSHA256:   snapshotHash,
		ScopeInputSHA256: digestBytes(scopeBytes),
		Method:           rehaSnapshotMethod,
		StartedAt:        started.Format(time.RFC3339Nano),
		CompletedAt:      completed.Format(time.RFC3339Nano),
	}
	if err := writePrivateJSON(receiptFile.File, receipt); err != nil {
		return SnapshotResult{}, err
	}
	if err := receiptFile.Close(); err != nil {
		return SnapshotResult{}, fmt.Errorf("close receipt: %w", err)
	}
	success = true
	return SnapshotResult{
		Status:         "created",
		Source:         scope.Source,
		Location:       scope.Location,
		SourceDB:       sourceAbs,
		SnapshotDB:     outputAbs,
		Receipt:        receiptAbs,
		SchemaVersion:  schema,
		SnapshotSHA256: snapshotHash,
		Method:         rehaSnapshotMethod,
	}, nil
}

func onlineBackup(ctx context.Context, source *sql.Conn, outputPath string) error {
	return source.Raw(func(driverConn any) error {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite driver does not expose online backup")
		}
		backup, err := backuper.NewBackup(sqliteFileURI(outputPath))
		if err != nil {
			return fmt.Errorf("create online backup: %w", err)
		}
		for {
			more, stepErr := backup.Step(-1)
			if stepErr != nil {
				_ = backup.Finish()
				return fmt.Errorf("copy online backup: %w", stepErr)
			}
			if !more {
				break
			}
			select {
			case <-ctx.Done():
				_ = backup.Finish()
				return ctx.Err()
			default:
			}
		}
		if err := backup.Finish(); err != nil {
			return fmt.Errorf("finish online backup: %w", err)
		}
		return nil
	})
}

func readOnlyDSN(path string) string {
	return sqliteFileURI(path) +
		"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
}

func immutableReadOnlyDSN(path string) string {
	return sqliteFileURI(path) +
		"?mode=ro&immutable=1&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
}

func validateReadOnlySchema(ctx context.Context, conn *sql.Conn) (int, error) {
	var schema int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&schema); err != nil {
		return 0, fmt.Errorf("read database schema version: %w", err)
	}
	if !supportedRehaSchema(schema) {
		return 0, fmt.Errorf("database schema version %d is not supported; expected %d or %d", schema, rehaMinSchema, rehaMaxSchema)
	}
	var integrity string
	if err := conn.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil {
		return 0, fmt.Errorf("check database integrity: %w", err)
	}
	if integrity != "ok" {
		return 0, fmt.Errorf("database integrity check failed: %s", integrity)
	}
	var queryOnly int
	if err := conn.QueryRowContext(ctx, `PRAGMA query_only`).Scan(&queryOnly); err != nil {
		return 0, fmt.Errorf("check read-only query mode: %w", err)
	}
	if queryOnly != 1 {
		return 0, errors.New("database is not in query-only mode")
	}
	return schema, nil
}

// OpenReadOnly validates the private snapshot against the exact supported
// Reha schema versions without
// invoking Store.Open (which intentionally creates/migrates/chmods databases).
func OpenReadOnly(ctx context.Context, path string) (*ReadOnlyStore, error) {
	absolute, err := absolutePath(path, "database")
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("database is not a regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("database file mode must be 0600, got %o", info.Mode().Perm())
	}
	if err := rejectSnapshotSidecars(absolute); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", immutableReadOnlyDSN(absolute))
	if err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("acquire database connection: %w", err)
	}
	if _, err := validateReadOnlySchema(ctx, conn); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, err
	}
	if err := conn.Close(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("close validation connection: %w", err)
	}
	return &ReadOnlyStore{db: db, path: absolute}, nil
}

func (store *ReadOnlyStore) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func parseScopeInput(data []byte) (RehaScopeInput, error) {
	var input RehaScopeInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return RehaScopeInput{}, fmt.Errorf("decode scope input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return RehaScopeInput{}, errors.New("scope input contains trailing JSON")
		}
		return RehaScopeInput{}, fmt.Errorf("decode scope input: %w", err)
	}
	if input.Version != rehaReceiptVersion {
		return RehaScopeInput{}, fmt.Errorf("scope input version %d is not supported", input.Version)
	}
	if input.Source != "mysign" {
		return RehaScopeInput{}, errors.New("scope input must name exactly the mysign source")
	}
	if !validScopeScalar(input.Location, 256) || strings.TrimSpace(input.Location) != input.Location {
		return RehaScopeInput{}, errors.New("scope input location is missing or invalid")
	}
	if !validScopeScalar(input.Provenance, 4096) {
		return RehaScopeInput{}, errors.New("scope input provenance is missing or invalid")
	}
	if input.Provenance != "external_verified_scope" {
		return RehaScopeInput{}, errors.New("scope input provenance must be external_verified_scope")
	}
	if !isCanonicalAbsolute(input.SourceDB) || !isCanonicalAbsolute(input.Evidence.SourceDB) {
		return RehaScopeInput{}, errors.New("scope input source_db must be canonical absolute")
	}
	if err := validateScopeEvidence(input.Evidence, input.Source, input.SourceDB, input.Location); err != nil {
		return RehaScopeInput{}, err
	}
	return input, nil
}

func ReadSnapshotReceipt(path string) (SnapshotReceipt, error) {
	absolute, err := absolutePath(path, "scope receipt")
	if err != nil {
		return SnapshotReceipt{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return SnapshotReceipt{}, fmt.Errorf("stat scope receipt: %w", err)
	}
	if !info.Mode().IsRegular() {
		return SnapshotReceipt{}, errors.New("scope receipt is not a regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return SnapshotReceipt{}, fmt.Errorf("scope receipt mode must be 0600, got %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return SnapshotReceipt{}, fmt.Errorf("read scope receipt: %w", err)
	}
	var receipt SnapshotReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return SnapshotReceipt{}, fmt.Errorf("decode scope receipt: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return SnapshotReceipt{}, errors.New("scope receipt contains trailing JSON")
		}
		return SnapshotReceipt{}, fmt.Errorf("decode scope receipt: %w", err)
	}
	if err := validateReceipt(receipt); err != nil {
		return SnapshotReceipt{}, err
	}
	if err := validateEvidenceReceipt(receipt.Evidence, receipt.Source, receipt.SourceDB, receipt.Location); err != nil {
		return SnapshotReceipt{}, err
	}
	return receipt, nil
}

func VerifySnapshotHash(path, expected string) error {
	if !validLowerHex(expected, sha256.Size*2) {
		return errors.New("snapshot hash is invalid")
	}
	absolute, err := absolutePath(path, "report database")
	if err != nil {
		return err
	}
	if err := rejectSnapshotSidecars(absolute); err != nil {
		return err
	}
	actual, err := fileSHA256(absolute)
	if err != nil {
		return fmt.Errorf("hash snapshot database: %w", err)
	}
	if actual != expected {
		return errors.New("snapshot hash does not match scope receipt")
	}
	return nil
}

func ValidateReceiptForSnapshot(receipt SnapshotReceipt, dbPath, location string) error {
	if err := validateReceipt(receipt); err != nil {
		return err
	}
	dbAbs, err := absolutePath(dbPath, "report database")
	if err != nil {
		return err
	}
	if receipt.SnapshotDB != dbAbs {
		return errors.New("scope receipt snapshot database does not match --db")
	}
	if receipt.Location != location {
		return errors.New("--location does not match scope receipt")
	}
	return nil
}

func validateReceipt(receipt SnapshotReceipt) error {
	if receipt.Version != rehaReceiptVersion {
		return fmt.Errorf("scope receipt version %d is not supported", receipt.Version)
	}
	if receipt.Source != "mysign" {
		return errors.New("scope receipt source must be exactly mysign")
	}
	if !validScopeScalar(receipt.Location, 256) || strings.TrimSpace(receipt.Location) != receipt.Location || !validScopeScalar(receipt.Provenance, 4096) {
		return errors.New("scope receipt location or provenance is invalid")
	}
	if receipt.Provenance != "external_verified_scope" {
		return errors.New("scope receipt provenance must be external_verified_scope")
	}
	if !isCanonicalAbsolute(receipt.SourceDB) || !isCanonicalAbsolute(receipt.SnapshotDB) {
		return errors.New("scope receipt database paths are required")
	}
	if receipt.SourceDB == receipt.SnapshotDB {
		return errors.New("scope receipt source and snapshot databases must differ")
	}
	if !validLowerHex(receipt.SnapshotSHA256, sha256.Size*2) ||
		!validLowerHex(receipt.ScopeInputSHA256, sha256.Size*2) {
		return errors.New("scope receipt hashes are invalid")
	}
	if !supportedRehaSchema(receipt.SourceSchema) {
		return fmt.Errorf("scope receipt schema version %d is not supported", receipt.SourceSchema)
	}
	if receipt.Method != rehaSnapshotMethod {
		return errors.New("scope receipt snapshot method is not supported")
	}
	if err := validateScopeEvidence(receipt.Evidence, receipt.Source, receipt.SourceDB, receipt.Location); err != nil {
		return fmt.Errorf("scope receipt evidence: %w", err)
	}
	for _, timestamp := range []string{receipt.StartedAt, receipt.CompletedAt} {
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			return errors.New("scope receipt timestamps must be RFC3339")
		}
	}
	return nil
}

func validateScopeEvidence(
	evidence RehaScopeEvidence,
	source string,
	sourceDB string,
	location string,
) error {
	if evidence.Kind != "independent-local-receipt" {
		return errors.New("scope evidence kind must be independent-local-receipt")
	}
	if !isCanonicalAbsolute(evidence.Reference) {
		return errors.New("scope evidence reference must be a canonical absolute path")
	}
	if !validLowerHex(evidence.SHA256, sha256.Size*2) {
		return errors.New("scope evidence sha256 is invalid")
	}
	if _, err := time.Parse(time.RFC3339, evidence.VerifiedAt); err != nil {
		return errors.New("scope evidence verified_at must be RFC3339")
	}
	if evidence.Source != source || evidence.Source != "mysign" {
		return errors.New("scope evidence source does not match scope")
	}
	if evidence.SourceDB != sourceDB || evidence.Location != location {
		return errors.New("scope evidence does not exactly match scope")
	}
	return nil
}

func validateEvidenceReceipt(
	evidence RehaScopeEvidence,
	source string,
	sourceDB string,
	location string,
) error {
	info, err := os.Lstat(evidence.Reference)
	if err != nil {
		return errors.New("scope evidence receipt is unavailable")
	}
	if !info.Mode().IsRegular() {
		return errors.New("scope evidence receipt must be a regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return errors.New("scope evidence receipt mode must be 0600")
	}
	data, err := os.ReadFile(evidence.Reference)
	if err != nil {
		return errors.New("scope evidence receipt cannot be read")
	}
	if digestBytes(data) != evidence.SHA256 {
		return errors.New("scope evidence receipt hash does not match")
	}
	receipt, err := parseEvidenceReceipt(data)
	if err != nil {
		return err
	}
	if receipt.Version != rehaReceiptVersion {
		return errors.New("scope evidence receipt version is not supported")
	}
	if receipt.Source != source || receipt.Source != "mysign" {
		return errors.New("scope evidence receipt source does not match scope")
	}
	if !isCanonicalAbsolute(receipt.SourceDB) || receipt.SourceDB != sourceDB {
		return errors.New("scope evidence receipt source database does not match scope")
	}
	if !validScopeScalar(receipt.Location, 256) || strings.TrimSpace(receipt.Location) != receipt.Location || receipt.Location != location {
		return errors.New("scope evidence receipt location does not match scope")
	}
	if _, err := time.Parse(time.RFC3339, receipt.VerifiedAt); err != nil || receipt.VerifiedAt != evidence.VerifiedAt {
		return errors.New("scope evidence receipt timestamp does not match scope")
	}
	if receipt.Kind != evidence.Kind || receipt.Kind != "independent-local-receipt" {
		return errors.New("scope evidence receipt kind does not match scope")
	}
	return nil
}

func parseEvidenceReceipt(data []byte) (RehaEvidenceReceipt, error) {
	var receipt RehaEvidenceReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return RehaEvidenceReceipt{}, errors.New("scope evidence receipt JSON is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return RehaEvidenceReceipt{}, errors.New("scope evidence receipt JSON contains trailing data")
	}
	return receipt, nil
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, runeValue := range value {
		if !((runeValue >= '0' && runeValue <= '9') || (runeValue >= 'a' && runeValue <= 'f')) {
			return false
		}
	}
	return true
}

func validScopeScalar(value string, maxLen int) bool {
	if value == "" || len(value) > maxLen {
		return false
	}
	for _, runeValue := range value {
		if unicode.IsControl(runeValue) {
			return false
		}
	}
	return true
}

func absolutePath(path, label string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	return filepath.Clean(absolute), nil
}

func isCanonicalAbsolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

type privateFileReservation struct {
	*os.File
	path string
	info os.FileInfo
}

func reservePrivateFile(path, label string) (*privateFileReservation, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%s already exists; refusing to overwrite", label)
		}
		return nil, fmt.Errorf("create %s: %w", label, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("protect %s: %w", label, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("inspect %s: %w", label, err)
	}
	return &privateFileReservation{File: file, path: path, info: info}, nil
}

func rejectSnapshotSidecars(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return errors.New("snapshot has unsupported SQLite sidecar state")
		} else if !os.IsNotExist(err) {
			return errors.New("snapshot sidecar state cannot be inspected")
		}
	}
	return nil
}

func removeSnapshotArtifacts(reservation *privateFileReservation) {
	if reservation == nil || !ownsReservedPath(reservation) {
		return
	}
	path := reservation.path
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(path + suffix)
	}
}

func removeOwnedPath(reservation *privateFileReservation) {
	if reservation == nil || !ownsReservedPath(reservation) {
		return
	}
	_ = os.Remove(reservation.path)
}

func ownsReservedPath(reservation *privateFileReservation) bool {
	if reservation == nil || reservation.info == nil {
		return false
	}
	info, err := os.Lstat(reservation.path)
	return err == nil && os.SameFile(reservation.info, info)
}

func protectSQLiteFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Chmod(candidate, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("protect snapshot file: %w", err)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("database is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writePrivateJSON(file *os.File, value any) error {
	if file == nil {
		return errors.New("receipt file is not reserved")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("position receipt: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("clear receipt: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync receipt: %w", err)
	}
	return nil
}

func berlinLocation() (*time.Location, error) {
	location, err := time.LoadLocation(rehaTimezone)
	if err != nil {
		return nil, fmt.Errorf("load Reha timezone %s: %w", rehaTimezone, err)
	}
	return location, nil
}
