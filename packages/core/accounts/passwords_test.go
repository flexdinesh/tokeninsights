package accounts

import (
	"errors"
	"strings"
	"testing"
)

func TestUsernamePolicy(t *testing.T) {
	for _, input := range []string{"Alice", " ALICE ", "alice"} {
		normalized, err := NormalizeUsername(input)
		if err != nil || normalized != "alice" {
			t.Fatal(input, normalized, err)
		}
	}
	for _, input := range []string{"ab", "_alice", "-alice", "alice.name", "alice name", "Kevin", "álîce", strings.Repeat("a", MaxUsernameBytes+1)} {
		if _, err := NormalizeUsername(input); !errors.Is(err, ErrInvalidUsername) {
			t.Fatal("invalid username accepted", input, err)
		}
	}
}

func TestPasswordUnicodeAndWhitespaceArePreserved(t *testing.T) {
	password := "  a long password 😀  "
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("password hashes reused salt")
	}
	if !VerifyPassword(password, first) || VerifyPassword(strings.TrimSpace(password), first) {
		t.Fatal("password transformed")
	}
	if !VerifyPassword(password, second) {
		t.Fatal("salted hash rejected")
	}
	for _, password := range []string{"short", strings.Repeat("😀", MinPasswordCharacters-1), strings.Repeat("😀", MaxPasswordCharacters+1), string([]byte{0xff})} {
		if _, err := HashPassword(password); !errors.Is(err, ErrInvalidPassword) {
			t.Fatal("invalid password accepted", err)
		}
	}
	for _, password := range []string{strings.Repeat("😀", MinPasswordCharacters), strings.Repeat("😀", MaxPasswordCharacters)} {
		if err := ValidatePassword(password); err != nil {
			t.Fatal("Unicode password counted as bytes", err)
		}
	}
}

func TestMalformedPasswordHashesRejectBounded(t *testing.T) {
	for _, hash := range []string{"", "plain", strings.Replace(DummyPasswordHash, "m=19456", "m=999999999", 1), strings.Replace(DummyPasswordHash, "t=2", "t=999999999", 1), strings.Replace(DummyPasswordHash, "v=19", "v=16", 1), strings.Replace(DummyPasswordHash, "$AAAAAAAAAAAAAAAAAAAAAA$", "$bad$", 1)} {
		if VerifyPassword("synthetic-password-one", hash) {
			t.Fatal("malformed hash accepted")
		}
	}
}

func TestPasswordAdmissionBoundsConcurrentMemory(t *testing.T) {
	service := New(nil)
	var releases []func()
	for _, address := range []string{"192.0.2.1:1", "192.0.2.2:1", "192.0.2.3:1", "192.0.2.4:1"} {
		release, err := service.AdmitPassword(address)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	if _, err := service.AdmitPassword("192.0.2.5:1"); err == nil {
		t.Fatal("unbounded concurrent hashes")
	}
	releases[0]()
	releases[0]()
	release, err := service.AdmitPassword("192.0.2.5:2")
	if err != nil {
		t.Fatal("slot not released", err)
	}
	release()
	for _, release := range releases {
		release()
	}
	for range LoginRequestsPerMinute {
		release, err := service.AdmitPassword("192.0.2.6:1")
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := service.AdmitPassword("192.0.2.6:2"); err == nil {
		t.Fatal("per-address admission bypass")
	}
}
