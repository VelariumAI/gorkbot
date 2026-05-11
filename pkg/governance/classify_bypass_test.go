package governance

import "testing"

func TestIsPrivateTargetBypasses(t *testing.T) {
	cases := []struct {
		url  string
		want bool
		desc string
	}{
		{"http://127.1/path", true, "127.1 abbreviated loopback"},
		{"http://LOCALHOST/path", true, "uppercase localhost"},
		{"http://localhost./path", true, "trailing dot localhost"},
		{"http://[::1]:8080/path", true, "IPv6 bracket loopback"},
		{"file:///etc/passwd", false, "file scheme - no host"},
		{"gopher://127.0.0.1/", true, "gopher to loopback"},
		{"http://169.254.169.254/latest", true, "metadata service"},
		{"http://10.0.0.1/", true, "private 10.x"},
		{"http://192.168.1.1/", true, "private 192.168.x"},
		{"http://172.16.0.1/", true, "private 172.16.x"},
		{"https://example.com/", false, "public host"},
	}
	for _, tc := range cases {
		got := isPrivateTarget(tc.url)
		if got != tc.want {
			t.Errorf("[%s] isPrivateTarget(%q) = %v, want %v", tc.desc, tc.url, got, tc.want)
		}
	}
}

func TestIsSecretKeyAPIKeyVariants(t *testing.T) {
	for _, k := range []string{"x-api-key", "X-Api-Key", "api-key", "x_api_key"} {
		if !isSecretKey(k) {
			t.Errorf("isSecretKey(%q) should be true", k)
		}
	}
}
