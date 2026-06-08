package collector

import "testing"

// TestRedactCmd covers the defensive-default regex proposed in
// RFC 0003 Q2. The pattern is operator-overridable via
// processes.redact_regex in the settings table; this test exercises
// both the empty-pattern path (raw cmdline returned) and a
// custom-pattern path.
func TestRedactCmd(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		pattern string
		want    string
	}{
		{
			name:    "empty pattern returns raw",
			cmd:     "redis-cli --password=hunter2 ping",
			pattern: "",
			want:    "redis-cli --password=hunter2 ping",
		},
		{
			name:    "default catch — password=",
			cmd:     "redis-cli --password=hunter2 ping",
			pattern: "(?i)(password|secret|token|api[_-]?key)\\s*=",
			want:    "<redacted: matches processes.redact_regex>",
		},
		{
			name:    "default catch — case-insensitive TOKEN=",
			cmd:     "curl -H TOKEN=abc123",
			pattern: "(?i)(password|secret|token|api[_-]?key)\\s*=",
			want:    "<redacted: matches processes.redact_regex>",
		},
		{
			name:    "default catch — api_key= variant",
			cmd:     "mything --api_key=zzz",
			pattern: "(?i)(password|secret|token|api[_-]?key)\\s*=",
			want:    "<redacted: matches processes.redact_regex>",
		},
		{
			name:    "no match returns raw",
			cmd:     "python3 -m jupyter notebook --port=8888",
			pattern: "(?i)(password|secret|token|api[_-]?key)\\s*=",
			want:    "python3 -m jupyter notebook --port=8888",
		},
		{
			name:    "custom pattern",
			cmd:     "deploy.sh --git-token=ghp_abc",
			pattern: "ghp_[A-Za-z0-9]+",
			want:    "<redacted: matches processes.redact_regex>",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RedactCmd(c.cmd, c.pattern); got != c.want {
				t.Errorf("RedactCmd(%q, %q) = %q, want %q", c.cmd, c.pattern, got, c.want)
			}
		})
	}
}

// TestRedactCmd_BadPatternReturnsRaw — a bad regex shouldn't
// crash the broadcast path. Leave the cmdline alone + log a boot
// error so the operator sees the misconfiguration.
func TestRedactCmd_BadPatternReturnsRaw(t *testing.T) {
	cmd := "redis-cli --password=hunter2 ping"
	if got := RedactCmd(cmd, "[unclosed"); got != cmd {
		t.Errorf("RedactCmd with bad pattern should leave cmd unchanged, got %q", got)
	}
}
