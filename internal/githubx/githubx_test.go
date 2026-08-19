package githubx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSplitRepo(t *testing.T) {
	o, n, err := SplitRepo("oasdiff/oasdiff")
	if err != nil || o != "oasdiff" || n != "oasdiff" {
		t.Fatalf("%s %s %v", o, n, err)
	}
	if _, _, err := SplitRepo("../etc/passwd"); err == nil {
		t.Fatal("expected reject")
	}
	if _, _, err := SplitRepo(`C:\git\repo`); err == nil {
		t.Fatal("expected reject windows path")
	}
	if _, _, err := SplitRepo("https://evil.example/x"); err == nil {
		t.Fatal("expected reject host")
	}
	o, n, err = SplitRepo("https://github.com/oasdiff/oasdiff.git")
	if err != nil || o != "oasdiff" || n != "oasdiff" {
		t.Fatalf("url form %s %s %v", o, n, err)
	}
}

func TestWebhookHMAC(t *testing.T) {
	body := []byte(`{"action":"completed"}`)
	if ValidWebhookSignature("", "sha256=abc", body) {
		t.Fatal("empty secret")
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !ValidWebhookSignature("secret", sig, body) {
		t.Fatal("want valid")
	}
	if ValidWebhookSignature("secret", "sha256=00", body) {
		t.Fatal("want invalid")
	}
}

func TestParseAllowlist(t *testing.T) {
	got := ParseAllowlist(" oasdiff/oasdiff , bad, actions/checkout ")
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
}
