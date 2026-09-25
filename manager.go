package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maskSentinel is what the API returns instead of a secret-looking value, and
// what the UI sends back unchanged when the user did not retype it. Round
// tripping the sentinel restores the stored value rather than writing the
// sentinel itself, so editing a server's command never destroys its token.
const maskSentinel = "••••••••"

// backupRetention is how many timestamped copies of config.json are kept under
// .backups/. Enough to undo a bad afternoon by hand; not so many that a busy
// instance fills the volume.
const backupRetention = 10

// loadOptions carries the flags load() needs, so the manager can re-read the
// config exactly the way startup did.
type loadOptions struct {
	insecure    bool
	expandEnv   bool
	httpHeaders string
	httpTimeout int
}

// Manager implements the optional management API behind the web UI: it reads
// and rewrites the config file, and drives proxyRuntime to apply changes
// without restarting the process.
//
// Every mutating handler funnels through mu, so two concurrent edits cannot
// interleave a read-modify-write on the config file.
type Manager struct {
	configPath string
	opts       loadOptions
	runtime    *proxyRuntime
	logs       *logStore
	startedAt  time.Time

	// ctx is the server's lifetime. Downstream servers started after boot are
	// children of it, so they stop when the proxy does.
	ctx context.Context

	mu         sync.Mutex
	config     *Config
	lastReload time.Time
}

func newManager(ctx context.Context, configPath string, opts loadOptions, config *Config, runtime *proxyRuntime) *Manager {
	store := newLogStore()
	activeLogStore.Store(store)
	return &Manager{
		configPath: configPath,
		opts:       opts,
		runtime:    runtime,
		logs:       store,
		startedAt:  time.Now(),
		ctx:        ctx,
		config:     config,
		lastReload: time.Now(),
	}
}

// ---- HTTP plumbing ----

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Debug("Failed to write JSON response", "err", err)
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message, "error": message})
}

// decodeJSONBody reads a request body with a hard size limit: these endpoints
// are reachable by anyone who can reach the UI, and an unbounded decode is an
// easy way to make the proxy allocate.
func decodeJSONBody(r *http.Request, dst any) error {
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20)) }()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// snapshotConfig returns the config the proxy is currently running, which a
// reload may have replaced since boot. The pointer is only ever swapped, never
// mutated in place, so the caller can read it without holding the lock.
func (m *Manager) snapshotConfig() *Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// registerRoutes mounts the management API. Handlers are registered on the
// shared mux directly (not through dynamicRouter) because these routes never
// change for the lifetime of the process.
//
// The whole subtree inherits mcpProxy.options.authTokens when set: the API can
// start processes and read config, so it must never be less protected than the
// MCP endpoints it manages.
func (m *Manager) registerRoutes(mux *http.ServeMux, authTokens []string) {
	middlewares := []MiddlewareFunc{recoverMiddleware("manager")}
	if len(authTokens) > 0 {
		middlewares = append(middlewares, newAuthMiddleware(authTokens))
	}
	handle := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, chainMiddleware(h, middlewares...))
	}

	handle("GET /api/mcp/catalog", m.handleCatalog)
	handle("GET /api/mcp/installed", m.handleInstalled)
	handle("GET /api/mcp/proxy/health", m.handleHealth)
	handle("POST /api/mcp/proxy/reload", m.handleReload)
	handle("POST /api/mcp/install", m.handleInstall)
	handle("PATCH /api/mcp/servers/{id}", m.handleUpdate)
	handle("DELETE /api/mcp/servers/{id}", m.handleDelete)
	handle("GET /api/mcp/servers/{id}/logs", m.handleLogs)

	// The dashboard is mounted at "/", whose SPA fallback answers every
	// unmatched path with the HTML shell. That must not extend to /api/: a
	// client calling a renamed or misspelled endpoint would get a 200 and a
	// page of HTML where it expected JSON. Registering the subtree claims
	// those paths first, so an unknown endpoint is an honest JSON 404.
	handle("/api/", notFoundAPIHandler)
}

func notFoundAPIHandler(w http.ResponseWriter, r *http.Request) {
	writeAPIError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no API endpoint %s %s", r.Method, r.URL.Path))
}

// ---- raw config access ----

// rawConfig is the config file parsed only as far as it needs to be.
//
// The manager deliberately never serialises the in-memory *Config back to
// disk. With -expand-env (the default), load() has already replaced every
// ${VAR} with its value, so writing that struct out would bake secrets into
// config.json in plaintext - a one-way change the user never asked for. Every
// write instead re-reads the file as raw JSON and edits only the entries the
// request touched, leaving placeholders, comments of key order, and unknown
// fields in other entries exactly as the user wrote them.
type rawConfig struct {
	doc     map[string]json.RawMessage
	servers map[string]json.RawMessage
}

func (m *Manager) readRawConfig() (*rawConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	doc := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	servers := make(map[string]json.RawMessage)
	if raw, ok := doc["mcpServers"]; ok && jsonPresent(raw) {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, fmt.Errorf("parse mcpServers: %w", err)
		}
	}
	return &rawConfig{doc: doc, servers: servers}, nil
}

// marshal renders the document back to bytes, with mcpServers re-attached and
// keys sorted so that repeated edits produce a stable diff.
func (rc *rawConfig) marshal() ([]byte, error) {
	serversJSON, err := marshalSortedObject(rc.servers)
	if err != nil {
		return nil, err
	}
	rc.doc["mcpServers"] = serversJSON
	data, err := marshalSortedObject(rc.doc)
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	if err := indentJSON(&buf, data); err != nil {
		return nil, err
	}
	return []byte(buf.String() + "\n"), nil
}

// marshalSortedObject encodes a raw object with its keys in sorted order.
// encoding/json does that for map[string]T already, but only for the map it is
// given - doing it explicitly keeps the behaviour obvious.
func marshalSortedObject(fields map[string]json.RawMessage) (json.RawMessage, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(fields[k])
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.String()), nil
}

// indentJSON re-indents data with two spaces, matching the style config.json
// ships in. It is applied to the whole document at the end rather than while
// building it, so the untouched raw entries are reformatted consistently with
// the edited ones.
func indentJSON(dst *strings.Builder, data []byte) error {
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", "  "); err != nil {
		return err
	}
	dst.Write(out.Bytes())
	return nil
}

// ---- atomic persistence ----

// save writes the config durably and atomically, and refuses to install a
// document that would not load.
//
// The sequence matters: a config file that is truncated or half-written is
// worse than a stale one, because the proxy will not start from it. So the new
// contents go to a temp file in the same directory, are fsynced, are validated
// by the real loader, and only then replace the original with a rename. The
// directory is fsynced afterwards so the rename itself survives a power loss.
func (m *Manager) save(rc *rawConfig) error {
	data, err := rc.marshal()
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	dir := filepath.Dir(m.configPath)
	if err := m.backupCurrent(); err != nil {
		// A failed backup is not a reason to refuse the write, but the user
		// should know the safety net is missing.
		slog.Warn("Failed to back up config before write", "err", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", err)
	}

	// Validate with the loader the process actually uses, so a request can
	// never leave behind a file that the next restart rejects.
	if _, err := load(tmpName, m.opts.insecure, m.opts.expandEnv, m.opts.httpHeaders, m.opts.httpTimeout); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("resulting config is invalid: %w", err)
	}

	// Preserve the mode of the file being replaced; CreateTemp makes 0600,
	// which would silently tighten a deliberately group-readable config.
	if info, statErr := os.Stat(m.configPath); statErr == nil {
		if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
			slog.Warn("Failed to preserve config file mode", "err", err)
		}
	}

	if err := os.Rename(tmpName, m.configPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace config: %w", err)
	}
	return fsyncDir(dir)
}

// fsyncDir flushes a directory entry, which is what makes a rename durable.
// Not every platform allows opening a directory for this; a failure there is
// not fatal.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		slog.Debug("Failed to open config directory for sync", "err", err)
		return nil
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		slog.Debug("Failed to sync config directory", "err", err)
	}
	return nil
}

// backupCurrent copies the live config aside before it is replaced, and prunes
// the oldest copies beyond backupRetention.
func (m *Manager) backupCurrent() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	backupDir := filepath.Join(filepath.Dir(m.configPath), ".backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("config-%s.json", time.Now().UTC().Format("20060102T150405.000Z"))
	if err := os.WriteFile(filepath.Join(backupDir, name), data, 0o600); err != nil {
		return err
	}
	return pruneBackups(backupDir, backupRetention)
}

func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "config-") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) <= keep {
		return nil
	}
	// The timestamp format is lexicographically ordered, so sorting by name
	// sorts by age.
	sort.Strings(names)
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			slog.Debug("Failed to prune config backup", "file", name, "err", err)
		}
	}
	return nil
}

// ---- reload ----

// reloadResult describes what applying a config change did, for the UI banner.
type reloadResult struct {
	Success       bool     `json:"success"`
	Strategy      string   `json:"strategy"`
	ReloadedCount int      `json:"reloadedCount"`
	Started       []string `json:"started,omitempty"`
	Stopped       []string `json:"stopped,omitempty"`
	Timestamp     string   `json:"timestamp"`
	Message       string   `json:"message"`
}

// reload re-reads the config and applies the difference.
//
// This is the whole point of the manager: servers whose definition did not
// change are never touched, so adding a tenth server does not drop the
// sessions of the nine that were already running. Only added, removed, and
// modified entries are stopped and started.
func (m *Manager) reload() (*reloadResult, error) {
	config, err := load(m.configPath, m.opts.insecure, m.opts.expandEnv, m.opts.httpHeaders, m.opts.httpTimeout)
	if err != nil {
		return nil, err
	}

	previous := m.config
	added, removed, changed := diffServers(previous, config)

	// Stop first: a changed entry may hold a port or a lock the new definition
	// needs, and a removed one should stop serving before its replacement (if
	// any) appears.
	stopped := make([]string, 0, len(removed)+len(changed))
	for _, name := range append(append([]string{}, removed...), changed...) {
		if err := m.runtime.stop(name); err != nil {
			slog.Warn("Failed to stop server during reload", "client", name, "err", err)
		}
		stopped = append(stopped, name)
	}
	for _, name := range removed {
		m.logs.forget(name)
	}

	started := make([]string, 0, len(added)+len(changed))
	for _, name := range append(append([]string{}, added...), changed...) {
		clientConfig := config.McpServers[name]
		if clientConfig == nil || clientConfig.Options.Disabled {
			continue
		}
		m.runtime.start(m.ctx, name, clientConfig)
		started = append(started, name)
	}

	m.config = config
	m.lastReload = time.Now()

	sort.Strings(started)
	sort.Strings(stopped)
	count := len(started) + len(removed)
	return &reloadResult{
		Success:       true,
		Strategy:      "in-memory",
		ReloadedCount: count,
		Started:       started,
		Stopped:       stopped,
		Timestamp:     m.lastReload.UTC().Format(time.RFC3339),
		Message:       fmt.Sprintf("Applied %d change(s) without restarting the proxy", count),
	}, nil
}

// diffServers compares two configs by the marshaled form of each entry, so any
// field that affects how a server is launched counts as a change - including
// ones added to MCPClientConfigV2 after this code was written.
func diffServers(previous, next *Config) (added, removed, changed []string) {
	prev := map[string][]byte{}
	if previous != nil {
		for name, conf := range previous.McpServers {
			prev[name] = mustMarshal(conf)
		}
	}
	for name, conf := range next.McpServers {
		encoded := mustMarshal(conf)
		old, ok := prev[name]
		switch {
		case !ok:
			added = append(added, name)
		case string(old) != string(encoded):
			changed = append(changed, name)
		}
	}
	for name := range prev {
		if _, ok := next.McpServers[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)
	return added, removed, changed
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		// Only reachable if MCPClientConfigV2 gains an unmarshalable field,
		// which the config could not have been loaded with in the first place.
		return []byte(fmt.Sprintf("%v", v))
	}
	return data
}

// ---- server views ----

// installedServer is the UI's view of one configured server.
type installedServer struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	TransportType MCPClientType     `json:"transportType"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	URL           string            `json:"url,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	MaskedEnvKeys []string          `json:"maskedEnvKeys,omitempty"`
	Enabled       bool              `json:"enabled"`
	Status        string            `json:"status"`
	StatusMessage string            `json:"statusMessage,omitempty"`
}

// secretEnvKey guesses whether a value should be masked before it leaves the
// process. It errs towards masking: showing a token in a browser cannot be
// undone, whereas an over-masked value is merely retyped.
func secretEnvKey(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"key", "token", "secret", "password", "passwd", "credential", "auth"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

// maskEnv prepares an env map for display. A ${VAR} placeholder is shown as
// written - it names an environment variable rather than revealing one - while
// a literal secret value is replaced by the sentinel.
func maskEnv(env map[string]string) (map[string]string, []string) {
	if len(env) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(env))
	var masked []string
	for k, v := range env {
		if v != "" && secretEnvKey(k) && !isEnvPlaceholder(v) {
			out[k] = maskSentinel
			masked = append(masked, k)
			continue
		}
		out[k] = v
	}
	sort.Strings(masked)
	return out, masked
}

// isEnvPlaceholder reports a value that is entirely a ${VAR} reference.
func isEnvPlaceholder(v string) bool {
	trimmed := strings.TrimSpace(v)
	return strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") && !strings.Contains(trimmed[2:len(trimmed)-1], "${")
}

// rawServerEnv reads one server's env straight from the file, so the UI is
// shown ${VAR} placeholders rather than the values load() expanded them into.
func rawServerEnv(rc *rawConfig, name string) map[string]string {
	raw, ok := rc.servers[name]
	if !ok {
		return nil
	}
	var entry struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil
	}
	return entry.Env
}

func (m *Manager) serverView(name string, conf *MCPClientConfigV2, env map[string]string) installedServer {
	maskedEnv, maskedKeys := maskEnv(env)
	status, message := m.serverStatus(name, conf)
	return installedServer{
		ID:            name,
		Name:          name,
		TransportType: conf.TransportType,
		Command:       conf.Command,
		Args:          conf.Args,
		URL:           conf.URL,
		Env:           maskedEnv,
		MaskedEnvKeys: maskedKeys,
		Enabled:       !conf.Options.Disabled,
		Status:        status,
		StatusMessage: message,
	}
}

func (m *Manager) serverStatus(name string, conf *MCPClientConfigV2) (string, string) {
	if conf.Options.Disabled {
		return "disabled", "Disabled in configuration"
	}
	client := m.runtime.lookup(name)
	if client == nil {
		return "stopped", "Not connected"
	}
	switch client.Health() {
	case healthOK:
		return "running", "Connected"
	case healthFailed:
		return "error", "Connection failed; see logs"
	default:
		return "degraded", "Connecting"
	}
}

// ---- handlers ----

func (m *Manager) handleInstalled(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc, err := m.readRawConfig()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "config_unreadable", err.Error())
		return
	}

	names := make([]string, 0, len(m.config.McpServers))
	for name := range m.config.McpServers {
		names = append(names, name)
	}
	sort.Strings(names)

	servers := make([]installedServer, 0, len(names))
	for _, name := range names {
		servers = append(servers, m.serverView(name, m.config.McpServers[name], rawServerEnv(rc, name)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (m *Manager) handleHealth(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	config := m.config
	lastReload := m.lastReload
	m.mu.Unlock()

	total := len(config.McpServers)
	active := 0
	unhealthy := make([]string, 0)
	for name, conf := range config.McpServers {
		if conf.Options.Disabled {
			continue
		}
		client := m.runtime.lookup(name)
		if client == nil {
			continue
		}
		switch client.Health() {
		case healthOK:
			active++
		case healthFailed:
			unhealthy = append(unhealthy, name)
		}
	}
	sort.Strings(unhealthy)

	status := "ok"
	if len(unhealthy) > 0 {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         status,
		"name":           config.McpProxy.Name,
		"version":        config.McpProxy.Version,
		"uptime":         time.Since(m.startedAt).Truncate(time.Second).String(),
		"totalServers":   total,
		"activeServers":  active,
		"serverCount":    total,
		"unhealthy":      unhealthy,
		"lastReload":     lastReload.UTC().Format(time.RFC3339),
		"configPath":     m.configPath,
		"configReadable": m.configWritable() == nil,
	})
}

// configWritable reports whether the manager can persist changes, so the UI
// can disable its edit controls instead of failing on save.
func (m *Manager) configWritable() error {
	file, err := os.OpenFile(m.configPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

func (m *Manager) handleReload(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.reload()
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "reload_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// installRequest is the body of POST /api/mcp/install.
type installRequest struct {
	CatalogID     string            `json:"catalogId"`
	Name          string            `json:"name"`
	TransportType MCPClientType     `json:"transportType"`
	Command       string            `json:"command"`
	Args          []string          `json:"args"`
	URL           string            `json:"url"`
	Env           map[string]string `json:"env"`
}

func (m *Manager) handleInstall(w http.ResponseWriter, r *http.Request) {
	var req installRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "name_required", "a server name is required")
		return
	}
	if err := validateServerName(name); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_name", err.Error())
		return
	}

	entry := installEntry{
		TransportType: req.TransportType,
		Command:       req.Command,
		Args:          req.Args,
		URL:           req.URL,
		Env:           req.Env,
	}
	// A catalog id only supplies defaults: anything the request states wins,
	// so the install dialog can prefill a template and still be edited.
	if req.CatalogID != "" {
		item, ok := catalogByID(req.CatalogID)
		if !ok {
			writeAPIError(w, http.StatusBadRequest, "unknown_catalog_item", fmt.Sprintf("no catalog entry %q", req.CatalogID))
			return
		}
		entry = entry.withDefaults(item)
	}
	if entry.TransportType == "" {
		if entry.URL != "" {
			entry.TransportType = MCPClientTypeStreamable
		} else {
			entry.TransportType = MCPClientTypeStdio
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rc, err := m.readRawConfig()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "config_unreadable", err.Error())
		return
	}
	if _, exists := rc.servers[name]; exists {
		writeAPIError(w, http.StatusConflict, "already_exists", fmt.Sprintf("a server named %q is already configured", name))
		return
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "encode_failed", err.Error())
		return
	}
	rc.servers[name] = encoded

	if err := m.save(rc); err != nil {
		writeAPIError(w, http.StatusBadRequest, "save_failed", err.Error())
		return
	}
	result, err := m.reload()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "reload_failed", err.Error())
		return
	}

	conf := m.config.McpServers[name]
	writeJSON(w, http.StatusCreated, map[string]any{
		"server": m.serverView(name, conf, rawServerEnv(rc, name)),
		"reload": result,
	})
}

// installEntry is the JSON written into mcpServers for a new server. Only the
// fields a user can set through the UI are emitted, so the file stays as small
// as a hand-written one.
type installEntry struct {
	TransportType MCPClientType     `json:"transportType,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	URL           string            `json:"url,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
}

func (e installEntry) withDefaults(item catalogItem) installEntry {
	if e.TransportType == "" {
		e.TransportType = item.TransportType
	}
	if e.Command == "" {
		e.Command = item.DefaultConfig.Command
	}
	if len(e.Args) == 0 {
		e.Args = item.DefaultConfig.Args
	}
	if e.URL == "" {
		e.URL = item.DefaultConfig.URL
	}
	if len(item.DefaultConfig.Env) > 0 {
		merged := make(map[string]string, len(item.DefaultConfig.Env)+len(e.Env))
		for k, v := range item.DefaultConfig.Env {
			if v != "" {
				merged[k] = v
			}
		}
		for k, v := range e.Env {
			merged[k] = v
		}
		e.Env = merged
	}
	return e
}

// updateRequest is the body of PATCH /api/mcp/servers/{id}. Every field is a
// pointer so that "not mentioned" is distinguishable from "set to empty".
type updateRequest struct {
	Name    *string            `json:"name"`
	Enabled *bool              `json:"enabled"`
	Env     *map[string]string `json:"env"`
	Args    *[]string          `json:"args"`
	Command *string            `json:"command"`
	URL     *string            `json:"url"`
}

func (m *Manager) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rc, err := m.readRawConfig()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "config_unreadable", err.Error())
		return
	}
	current, ok := rc.servers[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no server named %q", id))
		return
	}

	// Edit the entry as a raw object so fields this build does not know about
	// (or that the user hand-wrote, like toolFilter) survive the round trip.
	entry := make(map[string]json.RawMessage)
	if err := json.Unmarshal(current, &entry); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "parse_failed", err.Error())
		return
	}

	if req.Command != nil {
		setRawField(entry, "command", *req.Command, *req.Command == "")
	}
	if req.URL != nil {
		setRawField(entry, "url", *req.URL, *req.URL == "")
	}
	if req.Args != nil {
		setRawField(entry, "args", *req.Args, len(*req.Args) == 0)
	}
	if req.Env != nil {
		merged, err := mergeEnv(entry["env"], *req.Env)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_env", err.Error())
			return
		}
		setRawField(entry, "env", merged, len(merged) == 0)
	}
	if req.Enabled != nil {
		if err := setDisabled(entry, !*req.Enabled); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "encode_failed", err.Error())
			return
		}
	}

	encoded, err := marshalSortedObject(entry)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "encode_failed", err.Error())
		return
	}

	name := id
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" && *req.Name != id {
		name = strings.TrimSpace(*req.Name)
		if err := validateServerName(name); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_name", err.Error())
			return
		}
		if _, exists := rc.servers[name]; exists {
			writeAPIError(w, http.StatusConflict, "already_exists", fmt.Sprintf("a server named %q is already configured", name))
			return
		}
		// A rename is a remove plus an add: the old route has to stop serving,
		// which reload() handles once the file no longer mentions it.
		delete(rc.servers, id)
	}
	rc.servers[name] = encoded

	if err := m.save(rc); err != nil {
		writeAPIError(w, http.StatusBadRequest, "save_failed", err.Error())
		return
	}
	result, err := m.reload()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "reload_failed", err.Error())
		return
	}
	if name != id {
		m.logs.forget(id)
	}

	conf := m.config.McpServers[name]
	if conf == nil {
		writeAPIError(w, http.StatusInternalServerError, "missing_after_reload", "server disappeared after reload")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"server": m.serverView(name, conf, rawServerEnv(rc, name)),
		"reload": result,
	})
}

// setRawField assigns a field, or drops it when the value is empty, so
// clearing a command does not leave `"command": ""` behind.
func setRawField(entry map[string]json.RawMessage, key string, value any, empty bool) {
	if empty {
		delete(entry, key)
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		delete(entry, key)
		return
	}
	entry[key] = encoded
}

// mergeEnv applies the env map the UI sent on top of the stored one.
//
// A value equal to maskSentinel means "unchanged": the UI never received the
// real value, so writing what it sent back would replace a token with bullet
// characters. Keys the request omits are removed, which is how the UI deletes
// a variable.
func mergeEnv(currentRaw json.RawMessage, incoming map[string]string) (map[string]string, error) {
	current := map[string]string{}
	if jsonPresent(currentRaw) {
		if err := json.Unmarshal(currentRaw, &current); err != nil {
			return nil, fmt.Errorf("stored env is not an object: %w", err)
		}
	}
	merged := make(map[string]string, len(incoming))
	for k, v := range incoming {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if v == maskSentinel {
			if existing, ok := current[key]; ok {
				merged[key] = existing
			}
			continue
		}
		merged[key] = v
	}
	return merged, nil
}

// setDisabled writes options.disabled without disturbing the other options a
// user may have set on the server.
func setDisabled(entry map[string]json.RawMessage, disabled bool) error {
	options := make(map[string]json.RawMessage)
	if raw, ok := entry["options"]; ok && jsonPresent(raw) {
		if err := json.Unmarshal(raw, &options); err != nil {
			return fmt.Errorf("stored options is not an object: %w", err)
		}
	}
	if disabled {
		options["disabled"] = json.RawMessage("true")
	} else {
		// Absent means enabled; writing `false` would add noise to a file the
		// user maintains by hand.
		delete(options, "disabled")
	}
	if len(options) == 0 {
		delete(entry, "options")
		return nil
	}
	encoded, err := marshalSortedObject(options)
	if err != nil {
		return err
	}
	entry["options"] = encoded
	return nil
}

func (m *Manager) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	m.mu.Lock()
	defer m.mu.Unlock()

	rc, err := m.readRawConfig()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "config_unreadable", err.Error())
		return
	}
	if _, ok := rc.servers[id]; !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no server named %q", id))
		return
	}
	delete(rc.servers, id)

	if err := m.save(rc); err != nil {
		writeAPIError(w, http.StatusBadRequest, "save_failed", err.Error())
		return
	}
	result, err := m.reload()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "reload_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "reload": result})
}

func (m *Manager) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := logRingSize
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": m.logs.tail(id, limit)})
}
