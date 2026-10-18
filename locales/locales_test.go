package locales

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var placeholderRe = regexp.MustCompile(`\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*[,}]`)

func loadARB(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := map[string]string{}
	for k, v := range raw {
		if strings.HasPrefix(k, "@") {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			t.Fatalf("%s: key %s: %v", name, k, err)
		}
		out[k] = s
	}
	return out
}

func placeholders(s string) []string {
	set := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		set[m[1]] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestCatalogsMatchEnglish(t *testing.T) {
	en := loadARB(t, "en.json")
	files, err := filepath.Glob("*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "en.json" {
			continue
		}
		t.Run(f, func(t *testing.T) {
			cat := loadARB(t, f)
			for k, v := range en {
				tv, ok := cat[k]
				if !ok {
					t.Errorf("missing key %s", k)
					continue
				}
				if a, b := strings.Join(placeholders(v), ","), strings.Join(placeholders(tv), ","); a != b {
					t.Errorf("key %s: placeholders %q != %q", k, b, a)
				}
			}
			for k := range cat {
				if _, ok := en[k]; !ok {
					t.Errorf("extra key %s", k)
				}
			}
		})
	}
}
