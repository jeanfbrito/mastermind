package extract

import "testing"

func TestContainsRawTranscript(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"cursor line", "Context.\n\n{\"role\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"Yes.\"}]}}\n", true},
		{"claude code line", "{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":\"hi\"}}", true},
		{"turn marker", "{\"type\":\"turn_ended\",\"status\":\"success\"}", true},
		{"prose", "Electron has split TLS: Chromium trusts OS certs, Node does not.", false},
		{"json example without envelope keys", "Config shape:\n{\"url\":\"https://x\",\"token\":\"abc\"}\n", false},
		{"invalid json line", "{not json", false},
	}
	for _, c := range cases {
		if got := ContainsRawTranscript(c.body); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
