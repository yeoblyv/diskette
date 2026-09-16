package locales

import (
	"reflect"
	"testing"

	Graphite "github.com/yeoblyv/graphite"
)

// allKeys reflects over the package's own exported Key* constants, so
// this test can't go stale by hand-listing them separately — a new key
// added to keys.go is automatically checked here too.
func allKeys(t *testing.T) []string {
	t.Helper()
	// There's no reflection over package-level const declarations in Go,
	// so this list is built from the four catalogs themselves instead:
	// every key that appears in at least one of them is a key this
	// package is expected to serve.
	seen := map[string]bool{}
	for _, cat := range []Graphite.Catalog{English, Ukrainian, Russian, Dutch} {
		for k := range cat {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}

func TestEnglishDefinesEveryKey(t *testing.T) {
	for _, k := range allKeys(t) {
		if _, ok := English[k]; !ok {
			t.Errorf("key %q is used in a translation but missing from English (the fallback catalog every other locale relies on)", k)
		}
	}
}

func TestNoLocaleHasAnEmptyValue(t *testing.T) {
	for name, cat := range map[string]Graphite.Catalog{"English": English, "Ukrainian": Ukrainian, "Russian": Russian, "Dutch": Dutch} {
		for k, v := range cat {
			if v == "" {
				t.Errorf("%s[%q] is empty", name, k)
			}
		}
	}
}

// TestFormatVerbsMatchAcrossLocales catches the most common translation
// bug: a %s/%d/%.0f dropped or reordered in a way that would panic (or
// silently misformat) fmt.Sprintf at runtime, by comparing each
// non-English catalog's verb sequence for a key against English's own.
func TestFormatVerbsMatchAcrossLocales(t *testing.T) {
	verbs := func(s string) []byte {
		var out []byte
		for i := 0; i < len(s); i++ {
			if s[i] != '%' {
				continue
			}
			if i+1 < len(s) && s[i+1] == '%' {
				i++ // "%%" is a literal percent sign, not a verb — skip both bytes
				continue
			}
			j := i + 1
			for j < len(s) && (s[j] == '.' || s[j] == '0' || (s[j] >= '1' && s[j] <= '9')) {
				j++
			}
			if j < len(s) {
				out = append(out, s[j])
			}
			i = j
		}
		return out
	}

	for name, cat := range map[string]Graphite.Catalog{"Ukrainian": Ukrainian, "Russian": Russian, "Dutch": Dutch} {
		for k, v := range cat {
			enV, ok := English[k]
			if !ok {
				continue // already reported by TestEnglishDefinesEveryKey
			}
			wantVerbs, gotVerbs := verbs(enV), verbs(v)
			if !reflect.DeepEqual(wantVerbs, gotVerbs) {
				t.Errorf("%s[%q] format verbs = %q, want %q (English's own) — got %q, English is %q", name, k, gotVerbs, wantVerbs, v, enV)
			}
		}
	}
}
