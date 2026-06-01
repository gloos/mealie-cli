package cli

import (
	"encoding/json"
	"sort"
	"testing"
)

// loadSchema runs `mealie schema --output json` through the real CLI and decodes
// the discovery tree, so the assertions below pin exactly what an agent would see.
func loadSchema(t *testing.T) cmdSchema {
	t.Helper()
	stdout, stderr, code := runCLI(t, cliRun{args: []string{"schema", "--output", "json"}})
	if code != 0 || stderr != "" {
		t.Fatalf("schema failed: code=%d stderr=%q", code, stderr)
	}
	var root cmdSchema
	if err := json.Unmarshal([]byte(stdout), &root); err != nil {
		t.Fatalf("schema is not valid JSON: %v\n%s", err, stdout)
	}
	return root
}

func findCmd(t *testing.T, root cmdSchema, path ...string) cmdSchema {
	t.Helper()
	cur := root
	for _, seg := range path {
		next, ok := childByName(cur, seg)
		if !ok {
			t.Fatalf("command %v: no child %q under %q", path, seg, cur.Path)
		}
		cur = next
	}
	return cur
}

func childByName(parent cmdSchema, name string) (cmdSchema, bool) {
	for _, sub := range parent.Commands {
		if sub.Name == name {
			return sub, true
		}
	}
	return cmdSchema{}, false
}

func flagByName(cmd cmdSchema, name string) (flagSchema, bool) {
	for _, fl := range cmd.Flags {
		if fl.Name == name {
			return fl, true
		}
	}
	return flagSchema{}, false
}

// TestSchemaGlobalFlags pins the root persistent flags as the *complete* set of
// global flags, with their types. --yes is deliberately NOT here: it is local to
// destructive commands (asserted separately), so a regression that promoted it to
// a global flag would fail here.
func TestSchemaGlobalFlags(t *testing.T) {
	root := loadSchema(t)

	want := map[string]string{
		"profile":  "string",
		"url":      "string",
		"token":    "string",
		"output":   "string",
		"quiet":    "bool",
		"no-input": "bool",
		"no-color": "bool",
		"config":   "string",
		"timeout":  "duration",
	}

	got := map[string]string{}
	for _, fl := range root.GlobalFlags {
		got[fl.Name] = fl.Type
	}
	if len(got) != len(want) {
		t.Errorf("global flag set = %v, want exactly %v", keys(got), keys(want))
	}
	for name, typ := range want {
		fl, ok := got[name]
		if !ok {
			t.Errorf("missing global flag %q", name)
			continue
		}
		if fl != typ {
			t.Errorf("global flag %q type = %q, want %q", name, fl, typ)
		}
	}
	if _, ok := got["yes"]; ok {
		t.Error("--yes must NOT be a global flag; it is local to destructive commands")
	}
}

// TestSchemaCommandSet asserts every documented top-level command is present.
func TestSchemaCommandSet(t *testing.T) {
	root := loadSchema(t)
	if root.Name != "mealie" {
		t.Errorf("root name = %q, want mealie", root.Name)
	}
	for _, name := range []string{"recipe", "mealplan", "shopping", "auth", "config", "doctor", "schema", "version"} {
		if _, ok := childByName(root, name); !ok {
			t.Errorf("schema is missing top-level command %q", name)
		}
	}
}

// TestSchemaAliases pins each command's actual aliases from source — the docs
// promise schema exposes every alias, so this guards against drift in both
// directions.
func TestSchemaAliases(t *testing.T) {
	root := loadSchema(t)
	cases := []struct {
		path []string
		want []string
	}{
		{[]string{"recipe"}, []string{"recipes"}},
		{[]string{"shopping"}, []string{"shop"}},
		{[]string{"mealplan"}, []string{"mealplans", "plan"}},
		{[]string{"recipe", "delete"}, []string{"rm"}},
		{[]string{"shopping", "delete"}, []string{"rm"}},
		{[]string{"shopping", "item", "delete"}, []string{"rm"}},
		{[]string{"mealplan", "delete"}, []string{"rm"}},
		{[]string{"config", "list"}, []string{"ls"}},
	}
	for _, c := range cases {
		cmd := findCmd(t, root, c.path...)
		if !equalStrings(cmd.Aliases, c.want) {
			t.Errorf("%v aliases = %v, want %v", c.path, cmd.Aliases, c.want)
		}
	}
}

// TestSchemaYesIsLocalToDestructive asserts --yes/-y is present on every
// destructive command and absent from read-only commands.
func TestSchemaYesIsLocalToDestructive(t *testing.T) {
	root := loadSchema(t)

	destructive := [][]string{
		{"recipe", "delete"},
		{"shopping", "delete"},
		{"shopping", "item", "delete"},
		{"mealplan", "delete"},
	}
	for _, path := range destructive {
		cmd := findCmd(t, root, path...)
		fl, ok := flagByName(cmd, "yes")
		if !ok {
			t.Errorf("%v: expected a local --yes flag", path)
			continue
		}
		if fl.Shorthand != "y" {
			t.Errorf("%v: --yes shorthand = %q, want y", path, fl.Shorthand)
		}
		if fl.Type != "bool" {
			t.Errorf("%v: --yes type = %q, want bool", path, fl.Type)
		}
	}

	readOnly := [][]string{
		{"recipe", "list"},
		{"recipe", "get"},
		{"shopping", "list"},
		{"shopping", "get"},
		{"mealplan", "list"},
		{"mealplan", "today"},
		{"auth", "status"},
		{"config", "list"},
	}
	for _, path := range readOnly {
		cmd := findCmd(t, root, path...)
		if _, ok := flagByName(cmd, "yes"); ok {
			t.Errorf("%v: read-only command must not carry --yes", path)
		}
	}
}

// TestSchemaLeafFlags pins a representative leaf (recipe list) and a spread of
// flag types/defaults across command families, so a flag whose type or default
// silently changed would fail discovery.
func TestSchemaLeafFlags(t *testing.T) {
	root := loadSchema(t)

	list := findCmd(t, root, "recipe", "list")
	leafWant := []struct{ name, typ, def, short string }{
		{"search", "string", "", "s"},
		{"page", "int", "0", ""},
		{"per-page", "int", "0", ""},
		{"limit", "int", "0", ""},
		{"all", "bool", "false", ""},
		{"category", "stringSlice", "[]", ""},
		{"tag", "stringSlice", "[]", ""},
		{"cookbook", "string", "", ""},
	}
	for _, w := range leafWant {
		fl, ok := flagByName(list, w.name)
		if !ok {
			t.Errorf("recipe list: missing flag %q", w.name)
			continue
		}
		if fl.Type != w.typ {
			t.Errorf("recipe list --%s type = %q, want %q", w.name, fl.Type, w.typ)
		}
		if fl.Default != w.def {
			t.Errorf("recipe list --%s default = %q, want %q", w.name, fl.Default, w.def)
		}
		if fl.Shorthand != w.short {
			t.Errorf("recipe list --%s shorthand = %q, want %q", w.name, fl.Shorthand, w.short)
		}
	}

	// Representative checks across other families.
	addType, _ := flagByName(findCmd(t, root, "mealplan", "add"), "type")
	if addType.Type != "string" || addType.Default != "dinner" {
		t.Errorf("mealplan add --type = {%q,%q}, want {string,dinner}", addType.Type, addType.Default)
	}
	qty, _ := flagByName(findCmd(t, root, "shopping", "item", "add"), "quantity")
	if qty.Type != "float64" {
		t.Errorf("shopping item add --quantity type = %q, want float64", qty.Type)
	}
	export := findCmd(t, root, "recipe", "export")
	outFile, _ := flagByName(export, "output-file")
	if outFile.Shorthand != "O" {
		t.Errorf("recipe export --output-file shorthand = %q, want O", outFile.Shorthand)
	}
	if force, ok := flagByName(export, "force"); !ok || force.Type != "bool" {
		t.Errorf("recipe export --force = {%v,%q}, want a bool", ok, force.Type)
	}
}

// TestSchemaGolden snapshots the whole discovery tree. This is a secondary review
// aid only — the structural invariants above are the contract — but a diff here
// makes any intentional surface change visible in review.
func TestSchemaGolden(t *testing.T) {
	stdout, _, code := runCLI(t, cliRun{args: []string{"schema", "--output", "json"}})
	if code != 0 {
		t.Fatalf("schema exit = %d", code)
	}
	assertGolden(t, "schema.json", stdout)
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
