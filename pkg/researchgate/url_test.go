package researchgate

import (
	"net/url"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	normalized, err := NormalizeURL("HTTPS://Example.COM:443/docs?q=1#frag")
	if err != nil {
		t.Fatalf("normalize url: %v", err)
	}
	if normalized != "https://example.com:443/docs?q=1" {
		t.Fatalf("unexpected normalized url: %s", normalized)
	}
}

func TestNormalizeURLInvalid(t *testing.T) {
	if _, err := NormalizeURL("example.com/no-scheme"); err == nil {
		t.Fatal("expected error for schemeless url")
	}
}

func TestIsSupportedScheme(t *testing.T) {
	u, _ := url.Parse("https://example.com")
	if !IsSupportedScheme(u, []string{"http", "https"}) {
		t.Fatal("https should be supported")
	}

	u2, _ := url.Parse("ftp://example.com")
	if IsSupportedScheme(u2, []string{"http", "https"}) {
		t.Fatal("ftp should be blocked")
	}
}

func TestIsPrivateOrLocalHostCases(t *testing.T) {
	blocked := []string{
		"localhost",
		"localhost.",
		"127.0.0.1",
		"127.1",
		"0.0.0.0",
		"[::1]",
		"[::]",
		"10.1.2.3",
		"172.16.1.10",
		"172.31.255.255",
		"192.168.1.10",
		"169.254.169.254",
		"169.254.8.8",
		"fc00::1",
		"fe80::1",
	}
	for _, host := range blocked {
		if !IsPrivateOrLocalHost(host) {
			t.Fatalf("expected blocked host: %s", host)
		}
	}

	if IsPrivateOrLocalHost("example.com") {
		t.Fatal("example.com should be public")
	}
}

func TestHostLooksLikeCloudMetadata(t *testing.T) {
	blocked := []string{
		"metadata.google.internal",
		"metadata",
		"imds.amazonaws.com",
		"instance-data",
		"METADATA.GOOGLE.INTERNAL.",
	}
	for _, host := range blocked {
		if !HostLooksLikeCloudMetadata(host) {
			t.Fatalf("expected metadata host to be blocked: %s", host)
		}
	}

	if HostLooksLikeCloudMetadata("example.com") {
		t.Fatal("example.com should not match metadata")
	}
}

func TestIsCredentialedURL(t *testing.T) {
	u, _ := url.Parse("https://user:pass@example.com/path")
	if !IsCredentialedURL(u) {
		t.Fatal("credentialed url should be detected")
	}
}
