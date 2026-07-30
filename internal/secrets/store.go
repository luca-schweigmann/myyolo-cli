package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/zalando/go-keyring"
)

const serviceName = "myyolo-cli"

var profilePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

type Credentials struct {
	PartnerNumber string `json:"partner_number"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Session struct {
	NextRequestToken string   `json:"next_request_token"`
	Cookies          []Cookie `json:"cookies,omitempty"`
}

type Backend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type keyringBackend struct{}

func (keyringBackend) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (keyringBackend) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (keyringBackend) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

type Store struct {
	backend Backend
}

func NewKeyringStore() *Store {
	return &Store{backend: keyringBackend{}}
}

func NewStore(backend Backend) *Store {
	return &Store{backend: backend}
}

func ValidateProfile(profile string) error {
	if !profilePattern.MatchString(profile) {
		return fmt.Errorf("invalid profile name %q", profile)
	}
	return nil
}

func (store *Store) SaveCredentials(profile string, credentials Credentials) error {
	if err := ValidateProfile(profile); err != nil {
		return err
	}
	if err := ValidateCredentials(credentials); err != nil {
		return err
	}
	return store.save(account(profile, "credentials"), credentials)
}

func (store *Store) LoadCredentials(profile string) (Credentials, error) {
	if err := ValidateProfile(profile); err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	if err := store.load(account(profile, "credentials"), &credentials); err != nil {
		return Credentials{}, err
	}
	if err := ValidateCredentials(credentials); err != nil {
		return Credentials{}, fmt.Errorf("stored credentials are invalid: %w", err)
	}
	return credentials, nil
}

func ValidateCredentials(credentials Credentials) error {
	if credentials.PartnerNumber == "" || credentials.Username == "" || credentials.Password == "" {
		return errors.New("partner number, username and password are required")
	}
	if len(credentials.PartnerNumber) > 128 {
		return errors.New("partner number exceeds 128 bytes")
	}
	if len(credentials.Username) > 256 {
		return errors.New("username exceeds 256 bytes")
	}
	if len(credentials.Password) > 4096 {
		return errors.New("password exceeds 4096 bytes")
	}
	return nil
}

func (store *Store) SaveSession(profile string, session Session) error {
	if err := ValidateProfile(profile); err != nil {
		return err
	}
	if session.NextRequestToken == "" {
		return errors.New("session token is required")
	}
	return store.save(account(profile, "session"), session)
}

func (store *Store) LoadSession(profile string) (Session, error) {
	if err := ValidateProfile(profile); err != nil {
		return Session{}, err
	}
	var session Session
	if err := store.load(account(profile, "session"), &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (store *Store) DeleteProfile(profile string) error {
	if err := ValidateProfile(profile); err != nil {
		return err
	}
	var errs []error
	for _, kind := range []string{"credentials", "session"} {
		err := store.backend.Delete(serviceName, account(profile, kind))
		if err != nil && !errors.Is(err, keyring.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func IsNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

func account(profile, kind string) string {
	return profile + "/" + kind
}

func (store *Store) save(accountName string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode secret: %w", err)
	}
	if err := store.backend.Set(serviceName, accountName, string(data)); err != nil {
		return fmt.Errorf("store secret in operating-system keyring: %w", err)
	}
	return nil
}

func (store *Store) load(accountName string, target any) error {
	value, err := store.backend.Get(serviceName, accountName)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return keyring.ErrNotFound
		}
		return fmt.Errorf("read operating-system keyring: %w", err)
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return fmt.Errorf("decode stored secret: %w", err)
	}
	return nil
}
