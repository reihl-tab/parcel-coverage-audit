package clispec

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestProgramIdentity pins the identity strings the man pages and --help
// are generated from.
func TestProgramIdentity(t *testing.T) {
	for _, c := range []struct{ name, got, re string }{
		{"Program", Program, `^[a-z][a-z0-9-]*$`},
		{"Version", Version, `^[0-9]+\.[0-9]+\.[0-9]+$`},
		{"ManDate", ManDate, `^(January|February|March|April|May|June|July|August|September|October|November|December) [0-9]{4}$`},
		{"Manual", Manual, `\S`},
	} {
		if !regexp.MustCompile(c.re).MatchString(c.got) {
			t.Errorf("%s = %q, want match for %s", c.name, c.got, c.re)
		}
	}
}

// TestAuditOptionsWellFormed checks each auditor flag: a unique lowercase
// dashed name, a known kind, a default the flag package can parse for that
// kind, and non-empty usage text.
func TestAuditOptionsWellFormed(t *testing.T) {
	if len(AuditOptions) == 0 {
		t.Fatal("no audit options")
	}
	name := regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, o := range AuditOptions {
		if !name.MatchString(o.Name) {
			t.Errorf("flag name %q is not lowercase-dashed", o.Name)
		}
		if seen[o.Name] {
			t.Errorf("duplicate flag -%s", o.Name)
		}
		seen[o.Name] = true
		if strings.TrimSpace(o.Usage) == "" {
			t.Errorf("-%s has no usage text", o.Name)
		}
		switch o.Kind {
		case String:
		case Int:
			if _, err := strconv.Atoi(o.Def); err != nil {
				t.Errorf("-%s: int default %q does not parse: %v", o.Name, o.Def, err)
			}
		case Bool:
			if o.Def != "false" {
				t.Errorf("-%s: bool default %q, want \"false\" (flags opt in)", o.Name, o.Def)
			}
		default:
			t.Errorf("-%s: unknown kind %d", o.Name, o.Kind)
		}
	}
}

// TestAuditOptionsKinds pins the kind and default of every shipped flag, so a
// silent type or default change is a deliberate test edit.
func TestAuditOptionsKinds(t *testing.T) {
	want := []struct {
		name string
		kind Kind
		def  string
	}{
		{"parcels", String, "data/ginot_hair_parcels.geojson"},
		{"published", String, "data/permit_requests.json"},
		{"checkpoint", String, ""},
		{"batch-size", Int, "280"},
		{"plans", String, ""},
		{"as-of", String, ""},
		{"clean-out", String, ""},
		{"previous", String, ""},
		{"json", Bool, "false"},
		{"mcp", Bool, "false"},
	}
	if len(AuditOptions) != len(want) {
		t.Fatalf("got %d audit options, want %d", len(AuditOptions), len(want))
	}
	for i, w := range want {
		o := AuditOptions[i]
		if o.Name != w.name || o.Kind != w.kind || o.Def != w.def {
			t.Errorf("option %d = {%s %d %q}, want {%s %d %q}", i, o.Name, o.Kind, o.Def, w.name, w.kind, w.def)
		}
	}
}

// TestCommandsSurface checks the shipped command list: the auditor first
// with the audit flags, then the support binary, unique names and binaries,
// and examples that invoke their own binary.
func TestCommandsSurface(t *testing.T) {
	if len(Commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(Commands))
	}
	audit, support := Commands[0], Commands[1]
	if audit.Name != "" || audit.Binary != Program {
		t.Errorf("auditor = {Name %q, Binary %q}, want {\"\", %q}", audit.Name, audit.Binary, Program)
	}
	if len(audit.Options) != len(AuditOptions) || (len(audit.Options) > 0 && &audit.Options[0] != &AuditOptions[0]) {
		t.Error("auditor options are not AuditOptions")
	}
	if support.Name != "support" || support.Binary != Program+"-support" {
		t.Errorf("support = {Name %q, Binary %q}, want {support, %s-support}", support.Name, support.Binary, Program)
	}
	if len(support.Options) != 0 {
		t.Errorf("support has %d flags; it takes exactly one subcommand", len(support.Options))
	}
	names, bins := map[string]bool{}, map[string]bool{}
	for _, c := range Commands {
		if names[c.Name] || bins[c.Binary] {
			t.Errorf("duplicate command %q / binary %q", c.Name, c.Binary)
		}
		names[c.Name], bins[c.Binary] = true, true
		if strings.TrimSpace(c.Summary) == "" || len(c.Details) == 0 {
			t.Errorf("%s: missing summary or details", c.Binary)
		}
		if len(c.Examples) == 0 {
			t.Errorf("%s: no examples", c.Binary)
		}
		for _, ex := range c.Examples {
			if ex[0] != c.Binary && !strings.HasPrefix(ex[0], c.Binary+" ") {
				t.Errorf("%s: example %q does not invoke %s", c.Binary, ex[0], c.Binary)
			}
			if strings.TrimSpace(ex[1]) == "" {
				t.Errorf("%s: example %q has no explanation", c.Binary, ex[0])
			}
		}
	}
}

// TestSupportSubcommands pins the support subcommands, in help order, and
// requires the support page's description to name each of them.
func TestSupportSubcommands(t *testing.T) {
	want := []string{"fetch", "callback", "privacy", "bundle", "badges", "summary"}
	if strings.Join(SupportSubcommands, ",") != strings.Join(want, ",") {
		t.Fatalf("SupportSubcommands = %v, want %v", SupportSubcommands, want)
	}
	details := strings.Join(Commands[1].Details, " ")
	for _, sub := range SupportSubcommands {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(sub) + `\b`).MatchString(details) {
			t.Errorf("support description does not mention %q", sub)
		}
	}
	for _, ex := range Commands[1].Examples {
		fields := strings.Fields(ex[0])
		if len(fields) != 2 {
			t.Errorf("support example %q: want binary plus one subcommand", ex[0])
			continue
		}
		found := false
		for _, sub := range SupportSubcommands {
			found = found || fields[1] == sub
		}
		if !found {
			t.Errorf("support example %q uses unknown subcommand %q", ex[0], fields[1])
		}
	}
}

// TestNoRealPaths enforces the package rule "invented examples only: no
// personal data or real file paths" on every default and example.
func TestNoRealPaths(t *testing.T) {
	bad := regexp.MustCompile(`(^|[\s"'=])(/|~/|[A-Za-z]:\\)|/home/|/Users/|@`)
	var texts []string
	for _, o := range AuditOptions {
		texts = append(texts, o.Def)
	}
	for _, c := range Commands {
		for _, ex := range c.Examples {
			texts = append(texts, ex[0], ex[1])
		}
	}
	for _, s := range texts {
		if bad.MatchString(s) {
			t.Errorf("absolute path or personal data in %q", s)
		}
	}
}
