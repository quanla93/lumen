package alerts

import (
	"errors"
	"fmt"
	"strings"
)

// Selector is the parsed form of an `alert_rules.host_selector` string.
// AND semantics: a host matches iff every requirement matches.
//
// Today we ship exact-equality only — Kubernetes-style `key in (a, b)`,
// `key != v`, and existence operators land in a follow-up if real usage
// asks. Equality covers the common "tier=critical,env=prod" pattern.
type Selector struct {
	Reqs []SelectorReq
}

// SelectorReq is one `key=value` requirement. Value may be empty to
// match "tag exists with any value" — wait, no: today value="" matches
// only when the stored tag value is also "". Existence semantics is a
// follow-up; we want the parser to be unambiguous now.
type SelectorReq struct {
	Key   string
	Value string
}

// IsEmpty reports whether the selector imposes no requirements. The
// engine treats an empty selector as "don't filter by tags" and falls
// back to the rule's host name/glob field.
func (s Selector) IsEmpty() bool { return len(s.Reqs) == 0 }

// ParseSelector accepts "tier=critical,env=prod". Whitespace around
// commas and `=` is tolerated. Repeated keys keep the last value
// (matches Kubernetes-style "later wins" expectations).
func ParseSelector(raw string) (Selector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Selector{}, nil
	}
	parts := strings.Split(raw, ",")
	seen := map[string]int{}
	out := Selector{Reqs: make([]SelectorReq, 0, len(parts))}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		var key, value string
		if eq < 0 {
			key = strings.TrimSpace(p)
		} else {
			key = strings.TrimSpace(p[:eq])
			value = strings.TrimSpace(p[eq+1:])
		}
		if key == "" {
			return Selector{}, errors.New("selector requirement missing key")
		}
		if err := validateSelectorKey(key); err != nil {
			return Selector{}, fmt.Errorf("key %q: %w", key, err)
		}
		if strings.ContainsAny(value, "=,") {
			return Selector{}, fmt.Errorf("value for %q contains reserved chars", key)
		}
		req := SelectorReq{Key: key, Value: value}
		if idx, ok := seen[key]; ok {
			out.Reqs[idx] = req
			continue
		}
		seen[key] = len(out.Reqs)
		out.Reqs = append(out.Reqs, req)
	}
	return out, nil
}

func validateSelectorKey(k string) error {
	if len(k) > 64 {
		return errors.New("key too long (max 64)")
	}
	for _, r := range k {
		if !(r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return errors.New("key may only contain letters, digits, '-', '_', '.'")
		}
	}
	return nil
}

// Matches reports whether a host's tag set satisfies every requirement.
// nil tags are treated as the empty set — anything with a non-empty
// selector fails to match.
func (s Selector) Matches(tags map[string]string) bool {
	if s.IsEmpty() {
		return true
	}
	for _, req := range s.Reqs {
		v, ok := tags[req.Key]
		if !ok || v != req.Value {
			return false
		}
	}
	return true
}
