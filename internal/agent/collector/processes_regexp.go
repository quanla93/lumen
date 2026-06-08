// processes_regexp.go — small wrapper around regexp so the heavy
// dep isn't pulled in by every binary that imports the collector
// package. Compiles once on first use; per-call cost is the
// FindStringIndex sweep.

package collector

import "regexp"

var (
	processesRedactCache *regexp.Regexp
	processesRedactRaw   string
)

// regexCompileImpl returns (true, nil) if s matches pattern.
// Compiles the pattern (cached) and runs a single FindIndex. The
// first call to RedactCmd with a new pattern re-compiles; a bad
// pattern returns an error so the caller can log + leave the
// cmdline unredacted.
func regexCompileImpl(pattern, s string) (bool, error) {
	if pattern == "" {
		return false, nil
	}
	if processesRedactRaw != pattern {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		processesRedactCache = re
		processesRedactRaw = pattern
	}
	return processesRedactCache.MatchString(s), nil
}
