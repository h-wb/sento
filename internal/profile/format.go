package profile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Condition types.
const (
	CondTitle      = "title"      // regex against the raw release title
	CondGroup      = "group"      // regex against release group / scanlator
	CondLanguage   = "language"   // base-language equality ("fr" matches fr-ca)
	CondFlag       = "flag"       // one boolean parse fact (see Flags)
	CondProtocol   = "protocol"   // torrent | source
	CondSite       = "site"       // regex against the indexer entry name
	CondResolution = "resolution" // minimum parsed pixel height
)

// Flags are the boolean parse facts a flag condition can reference.
var Flags = []string{
	"digital", "scan", "web", "premium", "colored",
	"omnibus", "deluxe", "boxset", "official", "raw", "complete",
}

// Condition is one testable predicate of a custom format.
type Condition struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Negate   bool   `json:"negate,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// Format is a named condition list. Matching follows Sonarr's semantics:
// every required condition must be satisfied, and for each condition type
// that has non-required conditions, at least one of them must be satisfied
// (OR within a type, AND across types). Negate inverts a condition.
type Format struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Conditions []Condition `json:"conditions"`
}

// Input is the candidate view a format matches against.
type Input struct {
	Title      string
	Group      string // release group or scanlator
	Language   string
	Protocol   string
	Site       string
	Resolution int
	Flags      map[string]bool
}

// Matches reports whether the format applies to the candidate. A format
// without conditions matches nothing (it can't mean "everything").
func (f Format) Matches(in Input) bool {
	if len(f.Conditions) == 0 {
		return false
	}
	optionalHit := map[string]bool{}
	optionalSeen := map[string]bool{}
	for _, c := range f.Conditions {
		sat := c.satisfied(in)
		if c.Required {
			if !sat {
				return false
			}
			continue
		}
		optionalSeen[c.Type] = true
		if sat {
			optionalHit[c.Type] = true
		}
	}
	for t := range optionalSeen {
		if !optionalHit[t] {
			return false
		}
	}
	return true
}

func (c Condition) satisfied(in Input) bool {
	return c.test(in) != c.Negate
}

func (c Condition) test(in Input) bool {
	switch c.Type {
	case CondTitle:
		return matchRegexp(c.Value, in.Title)
	case CondGroup:
		return in.Group != "" && matchRegexp(c.Value, in.Group)
	case CondLanguage:
		return in.Language != "" && baseLang(in.Language) == baseLang(c.Value)
	case CondFlag:
		return in.Flags[strings.ToLower(c.Value)]
	case CondProtocol:
		return strings.EqualFold(in.Protocol, c.Value)
	case CondSite:
		return matchRegexp(c.Value, in.Site)
	case CondResolution:
		minPx, err := strconv.Atoi(c.Value)
		return err == nil && in.Resolution >= minPx
	}
	return false
}

func matchRegexp(pattern, s string) bool {
	re, err := regexp.Compile("(?i)" + pattern)
	return err == nil && re.MatchString(s)
}

func baseLang(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.ToLower(base)
}

// Validate rejects malformed conditions before they're persisted.
func (c Condition) Validate() error {
	switch c.Type {
	case CondTitle, CondGroup, CondSite:
		if _, err := regexp.Compile("(?i)" + c.Value); err != nil {
			return fmt.Errorf("invalid regex %q: %w", c.Value, err)
		}
	case CondLanguage:
		if strings.TrimSpace(c.Value) == "" {
			return fmt.Errorf("language condition needs a value")
		}
	case CondFlag:
		v := strings.ToLower(c.Value)
		for _, f := range Flags {
			if f == v {
				return nil
			}
		}
		return fmt.Errorf("unknown flag %q (one of %s)", c.Value, strings.Join(Flags, ", "))
	case CondProtocol:
		if v := strings.ToLower(c.Value); v != "torrent" && v != "source" {
			return fmt.Errorf("protocol must be torrent or source, got %q", c.Value)
		}
	case CondResolution:
		if n, err := strconv.Atoi(c.Value); err != nil || n <= 0 {
			return fmt.Errorf("resolution needs a positive pixel height, got %q", c.Value)
		}
	default:
		return fmt.Errorf("unknown condition type %q", c.Type)
	}
	return nil
}
