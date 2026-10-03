package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testConfigJSON is a config with one disabled stdio server. Disabled keeps
// reload() from spawning a subprocess, so the tests exercise persistence
// without depending on anything being installed.
const testConfigJSON = `{
  "mcpProxy": {
    "baseURL": "http://localhost:9090",
    "addr": ":9090",
    "name": "test-proxy",
    "version": "1.0.0"
  },
  "mcpServers": {
    "notes": {
      "command": "notes-server",
      "args": ["--root", "/tmp"],
      "env": {
        "API_KEY": "${TEST_MANAGER_SECRET}",
        "LOG_LEVEL": "debug"
      },
      "options": {
        "disabled": true
      }
    }
  }
}
`

// newTestManager builds a Manager over a real config file in a temp dir, with
// a runtime whose mux is never served, so handlers can be driven directly.
func newTestManager(t *testing.T, configJSON string) (*Manager, string) {
	t.Helper()
	t.Setenv("TEST_MANAGER_SECRET", "s3cret-value")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	opts := loadOptions{expandEnv: true, httpTimeout: 10}
	config, err := load(configPath, opts.insecure, opts.expandEnv, opts.httpHeaders, opts.httpTimeout)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	baseURL, err := url.Parse(config.McpProxy.BaseURL)
	if err != nil {
		t.Fatalf("parse baseURL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runtime := newProxyRuntime(config, baseURL, http.NewServeMux())
	manager := newManager(ctx, configPath, opts, config, runtime)
	// newManager publishes a global log store; leaving it set would have one
	// test's servers logging into another's ring.
	t.Cleanup(func() { activeLogStore.Store(nil) })
	return manager, configPath
}

// managerMux routes through the real patterns, so {id} path values and the
// method matching are covered too.
func managerMux(t *testing.T, m *Manager) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	m.registerRoutes(mux, nil)
	return mux
}

func do(t *testing.T, mux *http.ServeMux, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(data)
}

func serverEntry(t *testing.T, path, name string) map[string]any {
	t.Helper()
	var doc struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readConfigFile(t, path)), &doc); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return doc.McpServers[name]
}

// This is the reason the manager never serialises its in-memory *Config: with
// -expand-env, that struct holds the expanded secret. Writing it back would
// bake the secret into the file the user keeps in version control.
func TestPersistPreservesEnvPlaceholders(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)

	notes := manager.config.McpServers["notes"]
	if notes == nil {
		t.Fatal("notes server missing from the loaded config")
	}
	if got := notes.Env["API_KEY"]; got != "s3cret-value" {
		t.Fatalf("in-memory config should hold the expanded value, got %q", got)
	}

	mux := managerMux(t, manager)
	rec := do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{
		"args": []string{"--root", "/srv"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body %s", rec.Code, rec.Body.String())
	}

	written := readConfigFile(t, configPath)
	if !strings.Contains(written, "${TEST_MANAGER_SECRET}") {
		t.Errorf("placeholder was lost:\n%s", written)
	}
	if strings.Contains(written, "s3cret-value") {
		t.Errorf("expanded secret was written to disk:\n%s", written)
	}
	if entry := serverEntry(t, configPath, "notes"); !reflect.DeepEqual(entry["args"], []any{"--root", "/srv"}) {
		t.Errorf("args = %v, want the patched value", entry["args"])
	}
}

func TestInstalledMasksLiteralSecrets(t *testing.T) {
	const config = `{
  "mcpProxy": {"baseURL": "http://localhost:9090", "addr": ":9090", "name": "p", "version": "1"},
  "mcpServers": {
    "notes": {
      "command": "notes-server",
      "env": {"API_KEY": "literal-token", "OTHER_URL": "https://example.com", "TOKEN_REF": "${TEST_MANAGER_SECRET}"},
      "options": {"disabled": true}
    }
  }
}`
	manager, _ := newTestManager(t, config)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodGet, "/api/mcp/installed", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Servers []installedServer `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(resp.Servers))
	}
	server := resp.Servers[0]

	if server.Env["API_KEY"] != maskSentinel {
		t.Errorf("API_KEY = %q, want the mask sentinel", server.Env["API_KEY"])
	}
	if strings.Contains(rec.Body.String(), "literal-token") {
		t.Error("the literal secret reached the response body")
	}
	// A ${VAR} names a variable rather than revealing one, and hiding it would
	// stop the user seeing which variable a server reads.
	if server.Env["TOKEN_REF"] != "${TEST_MANAGER_SECRET}" {
		t.Errorf("TOKEN_REF = %q, want the placeholder as written", server.Env["TOKEN_REF"])
	}
	if server.Env["OTHER_URL"] != "https://example.com" {
		t.Errorf("OTHER_URL = %q, want it shown; it is not secret-shaped", server.Env["OTHER_URL"])
	}
	if !reflect.DeepEqual(server.MaskedEnvKeys, []string{"API_KEY"}) {
		t.Errorf("MaskedEnvKeys = %v, want [API_KEY]", server.MaskedEnvKeys)
	}
	if server.Enabled {
		t.Error("server is disabled in the config but reported as enabled")
	}
}

// The UI never receives the real value, so it sends the sentinel back for any
// field the user did not retype. Writing that literally would replace a token
// with bullet characters.
func TestUpdateRestoresMaskedEnvValues(t *testing.T) {
	const config = `{
  "mcpProxy": {"baseURL": "http://localhost:9090", "addr": ":9090", "name": "p", "version": "1"},
  "mcpServers": {
    "notes": {
      "command": "notes-server",
      "env": {"API_KEY": "literal-token", "STALE": "drop-me"},
      "options": {"disabled": true}
    }
  }
}`
	manager, configPath := newTestManager(t, config)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{
		"env": map[string]string{
			"API_KEY": maskSentinel,
			"ADDED":   "new-value",
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	entry := serverEntry(t, configPath, "notes")
	env, _ := entry["env"].(map[string]any)
	if env["API_KEY"] != "literal-token" {
		t.Errorf("API_KEY = %v, want the stored value restored", env["API_KEY"])
	}
	if env["ADDED"] != "new-value" {
		t.Errorf("ADDED = %v, want the new value", env["ADDED"])
	}
	// Keys the request omits are how the UI deletes a variable.
	if _, ok := env["STALE"]; ok {
		t.Error("STALE should have been removed; the request omitted it")
	}
}

func TestMergeEnv(t *testing.T) {
	stored := json.RawMessage(`{"API_KEY":"secret","KEEP":"a"}`)

	merged, err := mergeEnv(stored, map[string]string{
		"API_KEY": maskSentinel,
		"KEEP":    "b",
		"NEW":     "c",
		"  ":      "ignored",
	})
	if err != nil {
		t.Fatalf("mergeEnv: %v", err)
	}
	want := map[string]string{"API_KEY": "secret", "KEEP": "b", "NEW": "c"}
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged = %v, want %v", merged, want)
	}

	// The sentinel for a key that is not stored cannot be restored, so it is
	// dropped rather than written as bullet characters.
	merged, err = mergeEnv(nil, map[string]string{"UNKNOWN": maskSentinel})
	if err != nil {
		t.Fatalf("mergeEnv with no stored env: %v", err)
	}
	if len(merged) != 0 {
		t.Errorf("merged = %v, want empty", merged)
	}

	if _, err := mergeEnv(json.RawMessage(`"not-an-object"`), map[string]string{}); err == nil {
		t.Error("mergeEnv accepted a non-object stored env")
	}
}

func TestSecretEnvKeyAndPlaceholder(t *testing.T) {
	secret := []string{"API_KEY", "token", "GITHUB_TOKEN", "db_password", "MY_SECRET", "AUTH_HEADER", "credentials"}
	for _, key := range secret {
		if !secretEnvKey(key) {
			t.Errorf("secretEnvKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"LOG_LEVEL", "PATH", "ROOT_DIR"} {
		if secretEnvKey(key) {
			t.Errorf("secretEnvKey(%q) = true, want false", key)
		}
	}

	for _, v := range []string{"${VAR}", "  ${VAR}  "} {
		if !isEnvPlaceholder(v) {
			t.Errorf("isEnvPlaceholder(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"prefix-${VAR}", "${A}${B}", "plain", ""} {
		if isEnvPlaceholder(v) {
			t.Errorf("isEnvPlaceholder(%q) = true, want false", v)
		}
	}
}

// A config that fails to load is worse than a stale one: the proxy will not
// restart from it. The temp file is validated before it replaces the original.
func TestSaveRejectsConfigThatWouldNotLoad(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	before := readConfigFile(t, configPath)

	rc, err := manager.readRawConfig()
	if err != nil {
		t.Fatalf("readRawConfig: %v", err)
	}
	// command and url are mutually exclusive.
	rc.servers["broken"] = json.RawMessage(`{"command":"x","url":"http://example.com"}`)

	if err := manager.save(rc); err == nil {
		t.Fatal("save accepted a config that load() rejects")
	}
	if got := readConfigFile(t, configPath); got != before {
		t.Errorf("config was modified by a failed save:\n%s", got)
	}

	entries, err := os.ReadDir(filepath.Dir(configPath))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("failed save left a temp file behind: %s", entry.Name())
		}
	}
}

// Fields this build does not know about, and ones the user hand-wrote, have to
// survive an edit that does not mention them.
func TestSavePreservesUnknownFields(t *testing.T) {
	const config = `{
  "mcpProxy": {"baseURL": "http://localhost:9090", "addr": ":9090", "name": "p", "version": "1"},
  "someFutureTopLevelKey": {"keep": true},
  "mcpServers": {
    "notes": {
      "command": "notes-server",
      "toolFilter": {"mode": "allow", "list": ["read"]},
      "options": {"disabled": true, "logEnabled": true}
    },
    "untouched": {"command": "other", "env": {"K": "${TEST_MANAGER_SECRET}"}, "options": {"disabled": true}}
  }
}`
	manager, configPath := newTestManager(t, config)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{"command": "renamed-binary"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readConfigFile(t, configPath)), &doc); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if _, ok := doc["someFutureTopLevelKey"]; !ok {
		t.Error("an unrecognised top-level key was dropped")
	}

	entry := serverEntry(t, configPath, "notes")
	if entry["command"] != "renamed-binary" {
		t.Errorf("command = %v, want the patched value", entry["command"])
	}
	if _, ok := entry["toolFilter"]; !ok {
		t.Error("toolFilter was dropped by an unrelated edit")
	}
	options, _ := entry["options"].(map[string]any)
	if options["logEnabled"] != true {
		t.Errorf("options.logEnabled = %v, want it preserved", options["logEnabled"])
	}

	// The untouched entry must come back byte-identical in substance, still
	// holding its placeholder.
	other := serverEntry(t, configPath, "untouched")
	env, _ := other["env"].(map[string]any)
	if env["K"] != "${TEST_MANAGER_SECRET}" {
		t.Errorf("untouched server env = %v, want the placeholder", env["K"])
	}
}

func TestSetDisabledOmitsFalse(t *testing.T) {
	entry := map[string]json.RawMessage{
		"options": json.RawMessage(`{"disabled":true,"logEnabled":true}`),
	}
	if err := setDisabled(entry, false); err != nil {
		t.Fatalf("setDisabled: %v", err)
	}
	if got := string(entry["options"]); got != `{"logEnabled":true}` {
		t.Errorf("options = %s, want disabled dropped and logEnabled kept", got)
	}

	// options that exist only to carry disabled:false are removed entirely,
	// rather than leaving noise in a hand-maintained file.
	entry = map[string]json.RawMessage{"options": json.RawMessage(`{"disabled":true}`)}
	if err := setDisabled(entry, false); err != nil {
		t.Fatalf("setDisabled: %v", err)
	}
	if _, ok := entry["options"]; ok {
		t.Errorf("empty options object was kept: %s", entry["options"])
	}

	entry = map[string]json.RawMessage{}
	if err := setDisabled(entry, true); err != nil {
		t.Fatalf("setDisabled: %v", err)
	}
	if got := string(entry["options"]); got != `{"disabled":true}` {
		t.Errorf("options = %s, want disabled added", got)
	}
}

func TestSaveBacksUpAndPrunes(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	backupDir := filepath.Join(filepath.Dir(configPath), ".backups")

	for i := 0; i < backupRetention+5; i++ {
		rc, err := manager.readRawConfig()
		if err != nil {
			t.Fatalf("readRawConfig: %v", err)
		}
		if err := manager.save(rc); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "config-") {
			count++
		}
	}
	if count != backupRetention {
		t.Errorf("kept %d backups, want %d", count, backupRetention)
	}
}

// CreateTemp makes 0600, which would silently tighten a config a user made
// group-readable on purpose.
func TestSavePreservesFileMode(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	if err := os.Chmod(configPath, 0o640); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	rc, err := manager.readRawConfig()
	if err != nil {
		t.Fatalf("readRawConfig: %v", err)
	}
	if err := manager.save(rc); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("mode = %o, want 640", got)
	}
}

func TestUpdateRenamesServer(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	newName := "notebook"
	rec := do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{"name": newName})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	if serverEntry(t, configPath, "notes") != nil {
		t.Error("the old name is still configured")
	}
	entry := serverEntry(t, configPath, newName)
	if entry == nil {
		t.Fatal("the new name is not configured")
	}
	if entry["command"] != "notes-server" {
		t.Errorf("command = %v, want it carried over by the rename", entry["command"])
	}
	if _, ok := manager.config.McpServers[newName]; !ok {
		t.Error("the reload did not pick up the renamed server")
	}
}

func TestUpdateRejectsInvalidAndConflictingNames(t *testing.T) {
	const config = `{
  "mcpProxy": {"baseURL": "http://localhost:9090", "addr": ":9090", "name": "p", "version": "1"},
  "mcpServers": {
    "notes": {"command": "a", "options": {"disabled": true}},
    "other": {"command": "b", "options": {"disabled": true}}
  }
}`
	manager, _ := newTestManager(t, config)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{"name": "other"})
	if rec.Code != http.StatusConflict {
		t.Errorf("renaming onto an existing name: status = %d, want 409", rec.Code)
	}

	// The name becomes a URL route, so whitespace cannot be allowed.
	rec = do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{"name": "a b"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("renaming to a name with whitespace: status = %d, want 400", rec.Code)
	}

	rec = do(t, mux, http.MethodPatch, "/api/mcp/servers/missing", map[string]any{"command": "x"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("patching an unknown server: status = %d, want 404", rec.Code)
	}
}

func TestUpdateTogglesEnabled(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodPatch, "/api/mcp/servers/notes", map[string]any{"enabled": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	options, _ := serverEntry(t, configPath, "notes")["options"].(map[string]any)
	if options["disabled"] != true {
		t.Errorf("options.disabled = %v, want true", options["disabled"])
	}
}

func TestDeleteRemovesServer(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodDelete, "/api/mcp/servers/notes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if serverEntry(t, configPath, "notes") != nil {
		t.Error("the server is still in the config file")
	}
	if _, ok := manager.config.McpServers["notes"]; ok {
		t.Error("the reload did not drop the server from the running config")
	}

	rec = do(t, mux, http.MethodDelete, "/api/mcp/servers/notes", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("deleting twice: status = %d, want 404", rec.Code)
	}
}

// An install is always enabled, so the reload it triggers really does try to
// start the new server. The command is overridden with a path that cannot
// exist, which fails immediately instead of downloading and running the
// catalog entry's package; what is under test here is the file that is
// written, not the downstream process. withDefaults covers the command
// default on its own, without any I/O.
func TestInstallWritesCatalogDefaults(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	command := filepath.Join(t.TempDir(), "no-such-binary")
	rec := do(t, mux, http.MethodPost, "/api/mcp/install", map[string]any{
		"catalogId": "filesystem",
		"name":      "files",
		"command":   command,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	entry := serverEntry(t, configPath, "files")
	if entry == nil {
		t.Fatal("the server was not written to the config")
	}
	// The catalog supplies defaults; anything the request states wins.
	if entry["command"] != command {
		t.Errorf("command = %v, want the caller's override", entry["command"])
	}
	args, _ := entry["args"].([]any)
	if len(args) == 0 || args[0] != "-y" {
		t.Errorf("args = %v, want the catalog default", entry["args"])
	}
	if entry["transportType"] != string(MCPClientTypeStdio) {
		t.Errorf("transportType = %v, want stdio", entry["transportType"])
	}
}

func TestInstallEntryWithDefaults(t *testing.T) {
	item := catalogItem{
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "pkg"},
			Env:     map[string]string{"TOKEN": "", "REGION": "eu"},
		},
	}

	filled := installEntry{}.withDefaults(item)
	if filled.Command != "npx" || filled.TransportType != MCPClientTypeStdio {
		t.Errorf("empty entry = %+v, want the catalog defaults", filled)
	}
	if len(filled.Args) != 2 || filled.Args[0] != "-y" {
		t.Errorf("args = %v, want the catalog defaults", filled.Args)
	}
	// A blank default is a placeholder for a value the user must supply, so it
	// must not be written into the config as an empty string.
	if _, ok := filled.Env["TOKEN"]; ok {
		t.Errorf("env = %v, want the blank default dropped", filled.Env)
	}
	if filled.Env["REGION"] != "eu" {
		t.Errorf("env[REGION] = %q, want the catalog default", filled.Env["REGION"])
	}

	stated := installEntry{
		Command: "/usr/local/bin/mine",
		Args:    []string{"--flag"},
		Env:     map[string]string{"REGION": "us", "TOKEN": "abc"},
	}.withDefaults(item)
	if stated.Command != "/usr/local/bin/mine" || len(stated.Args) != 1 {
		t.Errorf("stated entry = %+v, want the request to win", stated)
	}
	if stated.Env["REGION"] != "us" || stated.Env["TOKEN"] != "abc" {
		t.Errorf("env = %v, want the request to win", stated.Env)
	}
}

func TestInstallValidatesRequest(t *testing.T) {
	manager, _ := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"no name", map[string]any{"command": "x"}, http.StatusBadRequest},
		{"bad name", map[string]any{"name": "../escape", "command": "x"}, http.StatusBadRequest},
		{"duplicate", map[string]any{"name": "notes", "command": "x"}, http.StatusConflict},
		{"unknown catalog id", map[string]any{"name": "ok", "catalogId": "nope"}, http.StatusBadRequest},
		{"neither command nor url", map[string]any{"name": "ok"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, mux, http.MethodPost, "/api/mcp/install", tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// An install that load() would reject must leave the config as it was: the
// entry is written to a temp file, validated, and only then renamed in.
func TestInstallRollsBackInvalidEntry(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	before := readConfigFile(t, configPath)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodPost, "/api/mcp/install", map[string]any{
		"name":    "broken",
		"command": "x",
		"url":     "http://example.com",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if got := readConfigFile(t, configPath); got != before {
		t.Errorf("config was modified by a rejected install:\n%s", got)
	}
}

func TestInstallRejectsUnknownFields(t *testing.T) {
	manager, _ := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp/install",
		strings.NewReader(`{"name":"x","command":"y","notAField":true}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown field", rec.Code)
	}
}

func TestDiffServers(t *testing.T) {
	previous := &Config{McpServers: map[string]*MCPClientConfigV2{
		"keep":    {Command: "a"},
		"change":  {Command: "b"},
		"removed": {Command: "c"},
	}}
	next := &Config{McpServers: map[string]*MCPClientConfigV2{
		"keep":   {Command: "a"},
		"change": {Command: "b2"},
		"added":  {Command: "d"},
	}}

	added, removed, changed := diffServers(previous, next)
	if !reflect.DeepEqual(added, []string{"added"}) {
		t.Errorf("added = %v", added)
	}
	if !reflect.DeepEqual(removed, []string{"removed"}) {
		t.Errorf("removed = %v", removed)
	}
	// "keep" must not appear anywhere: leaving it alone is what keeps its
	// connection alive across a reload.
	if !reflect.DeepEqual(changed, []string{"change"}) {
		t.Errorf("changed = %v", changed)
	}

	added, removed, changed = diffServers(nil, next)
	if len(added) != 3 || len(removed) != 0 || len(changed) != 0 {
		t.Errorf("from nil: added %v removed %v changed %v", added, removed, changed)
	}
}

func TestMarshalSortedObjectIsStable(t *testing.T) {
	fields := map[string]json.RawMessage{
		"zebra": json.RawMessage(`1`),
		"alpha": json.RawMessage(`2`),
		"mid":   json.RawMessage(`3`),
	}
	got, err := marshalSortedObject(fields)
	if err != nil {
		t.Fatalf("marshalSortedObject: %v", err)
	}
	if string(got) != `{"alpha":2,"mid":3,"zebra":1}` {
		t.Errorf("got %s, want keys in sorted order", got)
	}
}

func TestReadRawConfigHandlesMissingServers(t *testing.T) {
	const config = `{"mcpProxy": {"baseURL": "http://localhost:9090", "addr": ":9090", "name": "p", "version": "1"}}`
	manager, _ := newTestManager(t, config)

	rc, err := manager.readRawConfig()
	if err != nil {
		t.Fatalf("readRawConfig: %v", err)
	}
	if rc.servers == nil {
		t.Fatal("servers map is nil; installing the first server would panic")
	}
	if len(rc.servers) != 0 {
		t.Errorf("servers = %v, want empty", rc.servers)
	}
}

func TestLogsEndpointReturnsRecordedLines(t *testing.T) {
	manager, _ := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	recordServerLog("notes", logStreamStderr, "error", "boom\n")
	recordServerLog("notes", logStreamSystem, "info", "Connected")
	recordServerLog("other", logStreamStderr, "error", "not mine")

	rec := do(t, mux, http.MethodGet, "/api/mcp/servers/notes/logs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp struct {
		Logs []logEntry `json:"logs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(resp.Logs), resp.Logs)
	}
	if resp.Logs[0].Message != "boom" {
		t.Errorf("message = %q, want the trailing newline trimmed", resp.Logs[0].Message)
	}
	if resp.Logs[0].Stream != logStreamStderr {
		t.Errorf("stream = %q", resp.Logs[0].Stream)
	}

	rec = do(t, mux, http.MethodGet, "/api/mcp/servers/notes/logs?limit=1", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Message != "Connected" {
		t.Errorf("limit=1 returned %+v, want only the newest line", resp.Logs)
	}
}

func TestHealthEndpointReportsConfigState(t *testing.T) {
	manager, configPath := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodGet, "/api/mcp/proxy/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp struct {
		Status         string `json:"status"`
		Name           string `json:"name"`
		TotalServers   int    `json:"totalServers"`
		ActiveServers  int    `json:"activeServers"`
		ConfigPath     string `json:"configPath"`
		ConfigReadable bool   `json:"configReadable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Name != "test-proxy" || resp.TotalServers != 1 {
		t.Errorf("name = %q, totalServers = %d", resp.Name, resp.TotalServers)
	}
	if resp.ActiveServers != 0 {
		t.Errorf("activeServers = %d, want 0; the only server is disabled", resp.ActiveServers)
	}
	if resp.ConfigPath != configPath || !resp.ConfigReadable {
		t.Errorf("configPath = %q, writable = %v", resp.ConfigPath, resp.ConfigReadable)
	}
}

// The management API can start processes, so it must never be less protected
// than the MCP endpoints it manages.
// The dashboard's SPA fallback answers unmatched paths with the HTML shell.
// An API client that calls a renamed endpoint must not get a 200 and a page of
// HTML where it expected JSON.
func TestUnknownAPIPathIsJSONNotFound(t *testing.T) {
	manager, _ := newTestManager(t, testConfigJSON)
	mux := http.NewServeMux()
	manager.registerRoutes(mux, nil)
	// Registered the way startHTTPServerWithOptions does it, so the precedence
	// between "/api/" and the dashboard's "/" is what is under test.
	webUI, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}
	mux.Handle("/", webUI)

	cases := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/mcp/nope"},
		{http.MethodGet, "/api/"},
		{http.MethodGet, "/api/mcp/servers"},
		{http.MethodPost, "/api/mcp/reload"},
		{http.MethodGet, "/api/mcp/servers/notes/logss"},
	}
	for _, tc := range cases {
		rec := do(t, mux, tc.method, tc.target, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", tc.method, tc.target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s %s: Content-Type = %q, want JSON - the SPA shell leaked into /api/", tc.method, tc.target, ct)
		}
	}

	// The dashboard itself is untouched by the /api/ subtree.
	rec := do(t, mux, http.MethodGet, "/marketplace", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("/marketplace: status = %d, want the SPA shell", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("/marketplace: Content-Type = %q, want text/html", ct)
	}
}

func TestManagerRoutesInheritAuthTokens(t *testing.T) {
	manager, _ := newTestManager(t, testConfigJSON)
	mux := http.NewServeMux()
	manager.registerRoutes(mux, []string{"s3cret"})

	rec := do(t, mux, http.MethodGet, "/api/mcp/installed", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mcp/installed", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("authenticated status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}
