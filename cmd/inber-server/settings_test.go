package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/kayushkin/inber/server"
	"github.com/kayushkin/llm-bridge/msg"
	"github.com/kayushkin/llm-bridge/servicesettings"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// repositoryRoot is where the source scan starts: this package is
// cmd/inber-server.
const repositoryRoot = "../.."

// commandDirectory is the one command in the repository, and the only
// directory allowed to read the environment.
const commandDirectory = "cmd/inber-server"

// libraryReadsAwaitingConversion is every environment read left in inber's
// library packages, file by file, as the scan words it. They are configuration
// of inber-server that its settings page cannot show yet. Each entry names the
// child of todo 2997128c-e6ab-4b7c-a285-1e63f56110e6 that moves it into
// settingDefinitions and hands the value down. The list may only shrink: a
// listed read that is gone fails the scan, so a conversion must delete its
// entry, and a new read in a library package fails it too.
var libraryReadsAwaitingConversion = map[string][]string{
	// Provider API keys: todo 5e-1.
	"agent/clients.go":   {"reads an environment variable whose name is computed, which no declaration can be held to"},
	"server/selftest.go": {"reads ANTHROPIC_API_KEY, which its declaration list does not declare"},
	// The egress redactor's snapshot of every value: todo 5e-1.
	"agent/redaction.go": {"reads the environment through os.Environ, which no declaration can be held to"},
	// The deploy tool, which posts to retired forge: todo 5e-3 left it for the
	// user to decide whether to delete the tool rather than configure it.
	"tools/deploy.go": {
		"reads BUS_AGENT_URL, which its declaration list does not declare",
		"reads INBER_AGENT, which its declaration list does not declare",
	},
}

// The registry gives the server what its os.Getenv reads gave it before
// 2026-09-24: a set variable wins over the config file, an empty or unset one
// leaves the file's value, and the bus falls back to nats://localhost:4222.
func TestTheSettingsGiveTheServerTheSameValuesItAlwaysRead(t *testing.T) {
	nothing, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := nothing.String(settingAuthStoreURL); got != "http://127.0.0.1:8303" {
		t.Errorf("auth-store URL with nothing set = %q", got)
	}
	if got := nothing.String(settingAuthStoreToken); got != "" {
		t.Errorf("auth-store token with nothing set = %q", got)
	}

	var bare server.Config
	applySettings(&bare, nothing)
	if bare.NatsURL != "nats://localhost:4222" || bare.BusToken != "" || bare.OpenClawURL != "" || bare.OpenClawToken != "" {
		t.Errorf("nothing set, no config file: %+v", bare)
	}

	fromFile := server.Config{NatsURL: "nats://file:4222", BusToken: "file-bus", OpenClawURL: "http://file:18789", OpenClawToken: "file-openclaw"}
	applySettings(&fromFile, nothing)
	if fromFile.NatsURL != "nats://file:4222" || fromFile.BusToken != "file-bus" || fromFile.OpenClawURL != "http://file:18789" || fromFile.OpenClawToken != "file-openclaw" {
		t.Errorf("nothing set left the config file's values as %+v", fromFile)
	}

	// A variable set to the empty string is the same as unset, as it was when
	// main compared os.Getenv to "".
	empty, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{"NATS_URL": "", "BUS_TOKEN": "", "AUTH_STORE_URL": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.String(settingAuthStoreURL); got != "http://127.0.0.1:8303" {
		t.Errorf("empty AUTH_STORE_URL = %q", got)
	}
	fromFile = server.Config{NatsURL: "nats://file:4222", BusToken: "file-bus"}
	applySettings(&fromFile, empty)
	if fromFile.NatsURL != "nats://file:4222" || fromFile.BusToken != "file-bus" {
		t.Errorf("empty variables overrode the config file: %+v", fromFile)
	}

	set, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{
		"AUTH_STORE_URL":   "http://auth:1",
		"AUTH_STORE_TOKEN": "auth-token",
		"NATS_URL":         "nats://env:4222",
		"BUS_TOKEN":        "env-bus",
		"OPENCLAW_URL":     "http://env:18789",
		"OPENCLAW_TOKEN":   "env-openclaw",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if set.String(settingAuthStoreURL) != "http://auth:1" || set.String(settingAuthStoreToken) != "auth-token" {
		t.Errorf("auth-store settings not read back: %q", set.String(settingAuthStoreURL))
	}
	fromFile = server.Config{NatsURL: "nats://file:4222", BusToken: "file-bus", OpenClawURL: "http://file:18789", OpenClawToken: "file-openclaw"}
	applySettings(&fromFile, set)
	if fromFile.NatsURL != "nats://env:4222" || fromFile.BusToken != "env-bus" || fromFile.OpenClawURL != "http://env:18789" || fromFile.OpenClawToken != "env-openclaw" {
		t.Errorf("set variables did not win over the config file: %+v", fromFile)
	}
	if fromFile.SettingsHandler == nil {
		t.Error("applySettings left the server with no settings handler, and Serve refuses to start without one")
	}

	// The three the library packages used to read themselves. Unset, they are
	// agent-store's default path, no logstack and no blueprint, as before.
	if bare.AgentStorePath != "" || bare.LogstackURL != "" || bare.Blueprint {
		t.Errorf("nothing set: agent-store %q, logstack %q, blueprint %v", bare.AgentStorePath, bare.LogstackURL, bare.Blueprint)
	}
	withLibraryReads, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{
		"AGENT_STORE_PATH": "/tmp/agents.db",
		"LOGSTACK_URL":     "http://localhost:8088",
		"INBER_BLUEPRINT":  "1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var fromLibraryReads server.Config
	applySettings(&fromLibraryReads, withLibraryReads)
	if fromLibraryReads.AgentStorePath != "/tmp/agents.db" || fromLibraryReads.LogstackURL != "http://localhost:8088" || !fromLibraryReads.Blueprint {
		t.Errorf("set: agent-store %q, logstack %q, blueprint %v", fromLibraryReads.AgentStorePath, fromLibraryReads.LogstackURL, fromLibraryReads.Blueprint)
	}

	// The tool connections the tools package used to read itself. Unset, they
	// are empty, which tool-store reads as its default URLs and no credential.
	if bare.ToolConnections != (toolstoretools.OutsideServiceConnections{}) {
		t.Errorf("nothing set: tool connections %+v", bare.ToolConnections)
	}
	withToolConnections, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{
		"PINCHTAB_URL":    "http://pinchtab:1",
		"PINCHTAB_TOKEN":  "pinchtab-token",
		"BRAVE_API_KEY":   "brave-key",
		"SCHEDULER_URL":   "http://scheduler:2",
		"SCHEDULER_TOKEN": "scheduler-token",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var fromToolConnections server.Config
	applySettings(&fromToolConnections, withToolConnections)
	wantToolConnections := toolstoretools.OutsideServiceConnections{
		Pinchtab:    toolstoretools.PinchtabConnection{BaseURL: "http://pinchtab:1", Token: "pinchtab-token"},
		BraveAPIKey: "brave-key",
		Scheduler:   toolstoretools.SchedulerConnection{BaseURL: "http://scheduler:2", Token: "scheduler-token"},
	}
	if fromToolConnections.ToolConnections != wantToolConnections {
		t.Errorf("set: tool connections %+v, want %+v", fromToolConnections.ToolConnections, wantToolConnections)
	}
}

// INBER_BLUEPRINT used to be on for exactly "1" and "true" and silently off for
// anything else. Now it takes what strconv.ParseBool takes, and any other value
// stops the server at start rather than being read as off.
func TestTheBlueprintSwitchIsOnForTrueOffForFalseAndRefusesAnythingElse(t *testing.T) {
	for value, want := range map[string]bool{"1": true, "true": true, "TRUE": true, "0": false, "false": false, "": false} {
		registry, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{"INBER_BLUEPRINT": value}))
		if err != nil {
			t.Errorf("INBER_BLUEPRINT=%q refused: %v", value, err)
			continue
		}
		var cfg server.Config
		applySettings(&cfg, registry)
		if cfg.Blueprint != want {
			t.Errorf("INBER_BLUEPRINT=%q gave blueprint %v, want %v", value, cfg.Blueprint, want)
		}
	}
	for _, value := range []string{"yes", "on", "enabled"} {
		if _, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{"INBER_BLUEPRINT": value})); err == nil {
			t.Errorf("INBER_BLUEPRINT=%q was accepted; it used to mean off without saying so", value)
		}
	}
}

// inber-server owns no prefix, so a variable meant for another program never
// stops it.
func TestVariablesMeantForOtherProgramsDoNotStopTheServer(t *testing.T) {
	others := map[string]string{
		"AUTH_STORE_URLS": "x",
		"NATS_URL_BACKUP": "x",
		"INBER_URL":       "http://127.0.0.1:8200",
		"OPENCLAW_PORT":   "18789",
		"PATH":            "/bin",
		"HOME":            "/root",
	}
	if _, err := newSettingsRegistry(servicesettings.MapEnvironment(others)); err != nil {
		t.Errorf("variables meant for others were refused: %v", err)
	}
}

func TestGetSettingsDescribesTheServerHidesSecretsAndNothingCanBeWritten(t *testing.T) {
	const secret = "a-token-that-must-not-be-served"
	registry, err := newSettingsRegistry(servicesettings.MapEnvironment(map[string]string{
		"NATS_URL":         "nats://example:4222",
		"AUTH_STORE_TOKEN": secret,
		"BUS_TOKEN":        secret,
		"OPENCLAW_TOKEN":   secret,
	}))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /settings", settingsHandler(registry))

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /settings = %d: %s", recorder.Code, recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), secret) {
		t.Fatal("GET /settings served a secret's value")
	}
	var described msg.ServiceSettings
	if err := json.Unmarshal(recorder.Body.Bytes(), &described); err != nil {
		t.Fatal(err)
	}
	if described.Service != serviceName || len(described.Settings) != len(settingDefinitions()) {
		t.Fatalf("service=%q with %d settings, want %q with %d", described.Service, len(described.Settings), serviceName, len(settingDefinitions()))
	}
	for _, setting := range described.Settings {
		if setting.Editable {
			t.Errorf("%s is editable, and :8200 has no gate to put a write behind", setting.Key)
		}
		if setting.Key == settingNatsURL && (setting.Value != "nats://example:4222" || setting.Source != msg.ServiceSettingSourceEnvironment) {
			t.Errorf("NATS URL served as %q from %q", setting.Value, setting.Source)
		}
	}

	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/settings/"+settingNatsURL, strings.NewReader(`{"value":"x"}`)))
	if recorder.Code == http.StatusOK {
		t.Errorf("PUT /settings/%s = 200: a write route is mounted", settingNatsURL)
	}
}

// Every environment variable the command reads by name is declared, and the
// library packages read nothing beyond libraryReadsAwaitingConversion. A read
// that is not declared is invisible on the settings page. The walk and
// environmentReadFaults are agent-store's (internal/config/settings_test.go).
func TestEveryEnvironmentVariableTheServiceReadsIsDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, definition := range settingDefinitions() {
		declared[definition.EnvironmentVariable] = true
	}
	faultsByFile := map[string][]string{}
	filesRead := 0
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != repositoryRoot && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || entry.Name() == "testdata") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		directory := filepath.ToSlash(filepath.Dir(relative))
		if strings.HasPrefix(directory, "cmd/") && directory != commandDirectory {
			t.Errorf("%s is a command this scan has no declaration list for", directory)
		}
		allowed := map[string]bool{}
		if directory == commandDirectory {
			allowed = declared
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		filesRead++
		if faults := environmentReadFaults(file, allowed); len(faults) > 0 {
			faultsByFile[relative] = faults
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// If this package moved, the walk would start somewhere else, read nothing
	// and pass.
	if _, err := os.Stat(filepath.Join(repositoryRoot, commandDirectory, "main.go")); err != nil {
		t.Fatalf("the scan starts somewhere that is not the repository root: %v", err)
	}
	if filesRead < 100 {
		t.Fatalf("the scan read %d files; it is not looking at inber", filesRead)
	}

	files := map[string]bool{}
	for file := range faultsByFile {
		files[file] = true
	}
	for file := range libraryReadsAwaitingConversion {
		files[file] = true
	}
	var sorted []string
	for file := range files {
		sorted = append(sorted, file)
	}
	sort.Strings(sorted)
	for _, file := range sorted {
		found := append([]string(nil), faultsByFile[file]...)
		listed := append([]string(nil), libraryReadsAwaitingConversion[file]...)
		sort.Strings(found)
		sort.Strings(listed)
		for _, fault := range difference(found, listed) {
			t.Errorf("%s %s", file, fault)
		}
		for _, fault := range difference(listed, found) {
			t.Errorf("%s is listed in libraryReadsAwaitingConversion as %q, and the scan no longer finds it: delete the entry", file, fault)
		}
	}
}

// difference returns the entries of a not in b, counting repeats.
func difference(a, b []string) []string {
	remaining := map[string]int{}
	for _, entry := range b {
		remaining[entry]++
	}
	var missing []string
	for _, entry := range a {
		if remaining[entry] > 0 {
			remaining[entry]--
			continue
		}
		missing = append(missing, entry)
	}
	return missing
}

// The scan's own controls: each shape it exists to refuse is refused.
func TestTheSourceScanRefusesEachShapeOfUndeclaredRead(t *testing.T) {
	declared := map[string]bool{"NATS_URL": true}
	for name, source := range map[string]string{
		"an undeclared name":      `package p; import "os"; var v = os.Getenv("NATS_URLS")`,
		"a computed name":         `package p; import "os"; var n = "X"; var v = os.Getenv(n)`,
		"the whole environment":   `package p; import "os"; var v = os.Environ()`,
		"os.Getenv as a value":    `package p; import "os"; var read = os.Getenv`,
		"an undeclared LookupEnv": `package p; import "os"; func f() { os.LookupEnv("OTHER") }`,
		"os.ExpandEnv":            `package p; import "os"; var v = os.ExpandEnv("$HOME/repos")`,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "control.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if faults := environmentReadFaults(file, declared); len(faults) == 0 {
			t.Errorf("%s: the scan found nothing", name)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "control.go", `package p; import "os"; var v = os.Getenv("NATS_URL")`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if faults := environmentReadFaults(file, declared); len(faults) != 0 {
		t.Errorf("a declared read was refused: %v", faults)
	}
}

func environmentReadFaults(file *ast.File, declared map[string]bool) []string {
	var faults []string
	called := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || !isOsFunction(selector, "Getenv", "LookupEnv") {
			return true
		}
		called[selector] = true
		literal, isLiteral := call.Args[0].(*ast.BasicLit)
		if !isLiteral {
			faults = append(faults, "reads an environment variable whose name is computed, which no declaration can be held to")
			return true
		}
		name, _ := strconv.Unquote(literal.Value)
		if !declared[name] {
			faults = append(faults, "reads "+name+", which its declaration list does not declare")
		}
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		if isOsFunction(selector, "Environ", "ExpandEnv") {
			faults = append(faults, "reads the environment through os."+selector.Sel.Name+", which no declaration can be held to")
		}
		if isOsFunction(selector, "Getenv", "LookupEnv") && !called[selector] {
			faults = append(faults, "hands os."+selector.Sel.Name+" on as a value, so the names it reads cannot be seen here")
		}
		return true
	})
	return faults
}

func isOsFunction(selector *ast.SelectorExpr, names ...string) bool {
	packageName, isIdentifier := selector.X.(*ast.Ident)
	if !isIdentifier || packageName.Name != "os" {
		return false
	}
	for _, name := range names {
		if selector.Sel.Name == name {
			return true
		}
	}
	return false
}
