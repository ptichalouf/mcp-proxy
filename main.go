package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

var BuildVersion = "dev"

func main() {
	conf := flag.String("config", "config.json", "path to config file or a http(s) url")
	insecure := flag.Bool("insecure", false, "allow insecure HTTPS connections by skipping TLS certificate verification")
	expandEnv := flag.Bool("expand-env", true, "expand environment variables in config file")
	httpHeaders := flag.String("http-headers", "", "optional HTTP headers for config URL, format: 'Key1:Value1;Key2:Value2'")
	httpTimeout := flag.Int("http-timeout", 10, "HTTP timeout in seconds when fetching config from URL")
	authorize := flag.String("authorize", "", "run a one-time interactive OAuth authorization for the named mcpServers entry, then exit. Opens a browser; run this by hand, not from the daemon/service.")
	checkConfig := flag.Bool("check-config", false, "load and validate the config, then exit without starting the server")
	authStatus := flag.Bool("auth-status", false, "list every configured MCP server with its transport and authentication state, then exit. Local-only: reads config.json and cached OAuth token expiry, makes no network calls.")
	doctor := flag.Bool("doctor", false, "like -auth-status, but also connects to each remote server to confirm its credentials are accepted right now (may refresh an expired OAuth token via its refresh token; never opens a browser)")
	web := flag.Bool("web", false, "serve the management dashboard and its API, which can add, edit, and remove servers in the config file. Off by default: it grants whoever can reach it the ability to run commands as this process. Protect it with mcpProxy.options.authTokens and do not expose it publicly.")
	var logLevel slog.Level
	flag.TextVar(&logLevel, "log-level", slog.LevelInfo, "log level (debug, info, warn, error)")

	version := flag.Bool("version", false, "print version and exit")
	help := flag.Bool("help", false, "print help and exit")
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *version {
		fmt.Println(BuildVersion)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
	if *authorize != "" {
		if err := runAuthorize(*conf, *authorize, *insecure, *expandEnv, *httpHeaders, *httpTimeout); err != nil {
			slog.Error("Failed to authorize server", "server", *authorize, "err", redactURLCredentials(err))
			os.Exit(1)
		}
		return
	}
	if *authStatus || *doctor {
		ok, err := runDoctor(*conf, *insecure, *expandEnv, *httpHeaders, *httpTimeout, *doctor)
		if err != nil {
			slog.Error("Failed to run doctor", "err", redactURLCredentials(err))
			os.Exit(1)
		}
		if !ok {
			os.Exit(1)
		}
		return
	}
	config, err := load(*conf, *insecure, *expandEnv, *httpHeaders, *httpTimeout)
	if err != nil {
		slog.Error("Failed to load config", "err", redactURLCredentials(err))
		os.Exit(1)
	}
	if *checkConfig {
		fmt.Printf("Config OK: %d MCP server(s) configured\n", len(config.McpServers))
		return
	}
	if *web && remoteConfigPath(*conf) {
		slog.Error("-web needs a local config file: a config served over http(s) cannot be written back to", "config", *conf)
		os.Exit(1)
	}
	if *web && len(config.McpProxy.Options.AuthTokens) == 0 {
		// Not fatal - a proxy bound to localhost for one developer is a
		// legitimate setup - but this endpoint can start processes, so nobody
		// should reach it by accident.
		slog.Warn("Management UI is enabled without authTokens: anyone who can reach this address can add servers and run commands as this process", "addr", config.McpProxy.Addr)
	}
	err = startHTTPServerWithOptions(config, proxyOptions{
		configPath: *conf,
		loadOpts: loadOptions{
			insecure:    *insecure,
			expandEnv:   *expandEnv,
			httpHeaders: *httpHeaders,
			httpTimeout: *httpTimeout,
		},
		webUI: *web,
	})
	if err != nil {
		slog.Error("Failed to start server", "err", redactURLCredentials(err))
		os.Exit(1)
	}
}
