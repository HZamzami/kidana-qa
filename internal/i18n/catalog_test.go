package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	arabic  = regexp.MustCompile(`[\x{0600}-\x{06FF}]`)
	literal = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`)
)

// Files whose Arabic literals are not UI text: input aliases for parsing,
// the Arabic noun forms Count picks from, and Hijri month names.
var skip = map[string]bool{
	"excel/parse.go":     true,
	"domain/count.go":    true,
	"dates/dates.go":     true,
	"i18n/catalog_en.go": true,
}

// TestCatalogComplete fails when an Arabic string in the app code has no
// English entry, so the English UI never shows untranslated text.
func TestCatalogComplete(t *testing.T) {
	root := ".."
	var missing []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if skip[rel] || strings.HasPrefix(rel, "db/gen/") || strings.HasSuffix(rel, "_templ.go") ||
			strings.HasSuffix(rel, "_test.go") || (!strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".templ")) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, lit := range literal.FindAllString(line, -1) {
				if !arabic.MatchString(lit) {
					continue
				}
				s, err := strconv.Unquote(lit)
				if err != nil {
					continue
				}
				if !Has(s) {
					missing = append(missing, rel+":"+strconv.Itoa(n+1)+": "+s)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("%d strings have no English entry in catalog_en.go:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func TestT(t *testing.T) {
	if got := T(EN, "حفظ"); got != "Save" {
		t.Fatalf("T(en, حفظ) = %q", got)
	}
	if got := T(AR, "حفظ"); got != "حفظ" {
		t.Fatalf("Arabic must pass through, got %q", got)
	}
	if got := T(EN, "منى ١٢"); got != "منى ١٢" {
		t.Fatalf("unknown strings (user data) must pass through, got %q", got)
	}
	if got := Tf(EN, "الصف %d: %s", 3, "x"); got != "Row 3: x" {
		t.Fatalf("Tf = %q", got)
	}
	if Parse("EN") != EN || Parse("xx") != AR || EN.Dir() != "ltr" || AR.Dir() != "rtl" {
		t.Fatal("Parse/Dir wrong")
	}
	if Pick(EN, "منى", "Mina") != "Mina" || Pick(EN, "منى", " ") != "منى" || Pick(AR, "منى", "Mina") != "منى" {
		t.Fatal("Pick wrong")
	}
}
