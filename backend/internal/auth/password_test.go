package auth

import (
	"strings"
	"testing"
)

func TestPasswordVerifierRoundTrip(t *testing.T) {
	password := "correct horse battery"
	verifier, err := GeneratePasswordVerifier(password)
	if err != nil {
		t.Fatalf("GeneratePasswordVerifier() error = %v", err)
	}
	parsed, err := ParsePasswordVerifier(verifier)
	if err != nil {
		t.Fatalf("ParsePasswordVerifier() error = %v", err)
	}
	if parsed.Algorithm != "argon2id" || parsed.Version != 19 || parsed.MemoryKiB != 32768 || parsed.Iterations != 3 || parsed.Parallelism != 1 {
		t.Fatal("verifier parameters do not match the locked Argon2id configuration")
	}
	if len(parsed.Salt) != argonSaltBytes || len(parsed.Hash) != argonOutputBytes {
		t.Fatal("verifier contains invalid salt or hash length")
	}
	if !VerifyPassword(password, verifier) {
		t.Fatal("correct password was rejected")
	}
	if VerifyPassword("wrong password", verifier) {
		t.Fatal("wrong password was accepted")
	}
}

func TestPasswordVerifierUsesDifferentSalts(t *testing.T) {
	first, err := GeneratePasswordVerifier("correct horse battery")
	if err != nil {
		t.Fatalf("first verifier error = %v", err)
	}
	second, err := GeneratePasswordVerifier("correct horse battery")
	if err != nil {
		t.Fatalf("second verifier error = %v", err)
	}
	if first == second {
		t.Fatal("two verifier generations were identical")
	}
	firstParsed, _ := ParsePasswordVerifier(first)
	secondParsed, _ := ParsePasswordVerifier(second)
	if string(firstParsed.Salt) == string(secondParsed.Salt) {
		t.Fatal("two verifier generations reused a salt")
	}
}

func TestPasswordVerifierRejectsInvalidFormats(t *testing.T) {
	valid, err := GeneratePasswordVerifier("correct horse battery")
	if err != nil {
		t.Fatalf("GeneratePasswordVerifier() error = %v", err)
	}
	invalid := []string{
		valid + "$extra",
		strings.Replace(valid, "$argon2id$", "$argon2i$", 1),
		strings.Replace(valid, "$v=19$", "$v=18$", 1),
		strings.Replace(valid, "$m=32768,t=3,p=1$", "$m=65536,t=3,p=1$", 1),
		strings.Replace(valid, "$m=32768,t=3,p=1$", "$m=32768,t=4,p=1$", 1),
		strings.Replace(valid, "$m=32768,t=3,p=1$", "$m=32768,t=3,p=2$", 1),
		strings.Replace(valid, "$m=32768,t=3,p=1$", "$m=32768,p=1,t=3$", 1),
		strings.Replace(valid, "$", "", 1),
	}
	for _, candidate := range invalid {
		if _, err := ParsePasswordVerifier(candidate); err == nil {
			t.Fatal("invalid verifier was accepted")
		}
		if VerifyPassword("correct horse battery", candidate) {
			t.Fatal("invalid verifier authenticated a password")
		}
	}
}

func TestPasswordPolicyBoundaries(t *testing.T) {
	if err := ValidatePassword("abcdefghijkl"); err != nil {
		t.Fatalf("12-character password rejected: %v", err)
	}
	if err := ValidatePassword("abcdefghijk"); err == nil {
		t.Fatal("11-character password accepted")
	}
	if err := ValidatePassword(strings.Repeat("a", PasswordMaxBytes)); err != nil {
		t.Fatalf("256-byte password rejected: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("a", PasswordMaxBytes+1)); err == nil {
		t.Fatal("257-byte password accepted")
	}
	if err := ValidatePassword(strings.Repeat("界", PasswordMinCharacters)); err != nil {
		t.Fatalf("12-Unicode-character password rejected: %v", err)
	}
	if err := ValidatePassword("  abcdefghijk "); err != nil {
		t.Fatalf("password with significant spaces rejected: %v", err)
	}
}
