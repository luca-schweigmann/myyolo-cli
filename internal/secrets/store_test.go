package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

type memoryBackend struct {
	values map[string]string
}

func (backend *memoryBackend) Get(service, user string) (string, error) {
	value, ok := backend.values[service+":"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (backend *memoryBackend) Set(service, user, password string) error {
	backend.values[service+":"+user] = password
	return nil
}

func (backend *memoryBackend) Delete(service, user string) error {
	key := service + ":" + user
	if _, ok := backend.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(backend.values, key)
	return nil
}

func TestCredentialsAndSessionRoundTrip(t *testing.T) {
	store := NewStore(&memoryBackend{values: map[string]string{}})
	credentials := Credentials{
		PartnerNumber: "123",
		Username:      "reader",
		Password:      "secret",
	}
	if err := store.SaveCredentials("studio-a", credentials); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession("studio-a", Session{
		NextRequestToken: "token",
		Cookies:          []Cookie{{Name: "session", Value: "opaque"}},
	}); err != nil {
		t.Fatal(err)
	}

	gotCredentials, err := store.LoadCredentials("studio-a")
	if err != nil {
		t.Fatal(err)
	}
	if gotCredentials != credentials {
		t.Fatalf("credentials = %#v, want %#v", gotCredentials, credentials)
	}
	gotSession, err := store.LoadSession("studio-a")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.NextRequestToken != "token" || len(gotSession.Cookies) != 1 {
		t.Fatalf("session = %#v", gotSession)
	}

	if err := store.DeleteProfile("studio-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadCredentials("studio-a"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("load after delete error = %v", err)
	}
}

func TestProfileValidation(t *testing.T) {
	for _, profile := range []string{"", "../escape", "name with spaces", "-leading"} {
		if err := ValidateProfile(profile); err == nil {
			t.Fatalf("profile %q should be rejected", profile)
		}
	}
}
