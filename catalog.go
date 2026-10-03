package main

import (
	"net/http"
	"sort"
	"strings"
)

// catalogEnvVar describes one environment variable a catalog entry needs. The
// UI renders a form from these, so the description is what a user reads when
// deciding what to paste into the field.
type catalogEnvVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"isSecret,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// catalogConfig is the server definition a catalog entry installs.
type catalogConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// catalogItem is one installable entry. Descriptions are carried in both
// languages so the UI can switch locale without a round trip; the frontend
// falls back to the English field when descriptionFr is empty.
type catalogItem struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	DescriptionFr string          `json:"descriptionFr,omitempty"`
	Category      string          `json:"category"`
	Vendor        string          `json:"vendor,omitempty"`
	TransportType MCPClientType   `json:"transportType"`
	DefaultConfig catalogConfig   `json:"defaultConfig"`
	Env           []catalogEnvVar `json:"env,omitempty"`
	DocsURL       string          `json:"docsUrl,omitempty"`
	SourceURL     string          `json:"sourceUrl,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Verified      bool            `json:"verified,omitempty"`
}

// builtinCatalog is a curated, offline list of widely used MCP servers.
//
// It is deliberately static and compiled in: the proxy must not reach out to a
// third-party index at runtime, which would add a network dependency, a
// privacy question, and a supply-chain path into a tool whose whole job is to
// broker access to other tools. Entries only describe how to launch a server -
// installation itself is whatever the command does (npx, uvx, docker), exactly
// as if the user had written the config by hand.
var builtinCatalog = []catalogItem{
	{
		ID:            "filesystem",
		Name:          "Filesystem",
		Description:   "Read and write files in directories you explicitly allow.",
		DescriptionFr: "Lire et écrire des fichiers dans les répertoires que vous autorisez explicitement.",
		Category:      "files",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/dir"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem",
		Tags:      []string{"files", "local"},
		Verified:  true,
	},
	{
		ID:            "git",
		Name:          "Git",
		Description:   "Inspect repositories: read history, search commits, and review diffs.",
		DescriptionFr: "Inspecter des dépôts : lire l'historique, chercher dans les commits et examiner les diffs.",
		Category:      "development",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "uvx",
			Args:    []string{"mcp-server-git", "--repository", "/path/to/repo"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/git",
		Tags:      []string{"git", "vcs"},
		Verified:  true,
	},
	{
		ID:            "github",
		Name:          "GitHub",
		Description:   "Work with issues, pull requests, and repository contents on GitHub.",
		DescriptionFr: "Travailler avec les issues, les pull requests et le contenu des dépôts GitHub.",
		Category:      "development",
		Vendor:        "GitHub",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-github"},
			Env:     map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": ""},
		},
		Env: []catalogEnvVar{{
			Name:        "GITHUB_PERSONAL_ACCESS_TOKEN",
			Description: "Personal access token with the scopes you want to expose.",
			Required:    true,
			Secret:      true,
			Placeholder: "ghp_...",
		}},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/github",
		Tags:      []string{"git", "issues", "api"},
		Verified:  true,
	},
	{
		ID:            "gitlab",
		Name:          "GitLab",
		Description:   "Browse GitLab projects, issues, and merge requests.",
		DescriptionFr: "Parcourir les projets, issues et merge requests GitLab.",
		Category:      "development",
		Vendor:        "GitLab",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-gitlab"},
			Env:     map[string]string{"GITLAB_PERSONAL_ACCESS_TOKEN": "", "GITLAB_API_URL": "https://gitlab.com/api/v4"},
		},
		Env: []catalogEnvVar{
			{Name: "GITLAB_PERSONAL_ACCESS_TOKEN", Description: "GitLab personal access token.", Required: true, Secret: true, Placeholder: "glpat-..."},
			{Name: "GITLAB_API_URL", Description: "API endpoint, for self-hosted instances.", Required: false, Placeholder: "https://gitlab.com/api/v4"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/gitlab",
		Tags:      []string{"git", "ci"},
	},
	{
		ID:            "postgres",
		Name:          "PostgreSQL",
		Description:   "Query a PostgreSQL database and inspect its schema, read-only.",
		DescriptionFr: "Interroger une base PostgreSQL et inspecter son schéma, en lecture seule.",
		Category:      "database",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-postgres", "postgresql://localhost/mydb"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/postgres",
		Tags:      []string{"sql", "data"},
		Verified:  true,
	},
	{
		ID:            "sqlite",
		Name:          "SQLite",
		Description:   "Query a local SQLite database file and explore its tables.",
		DescriptionFr: "Interroger un fichier SQLite local et explorer ses tables.",
		Category:      "database",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "uvx",
			Args:    []string{"mcp-server-sqlite", "--db-path", "/path/to/database.db"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/sqlite",
		Tags:      []string{"sql", "data", "local"},
	},
	{
		ID:            "fetch",
		Name:          "Fetch",
		Description:   "Retrieve a web page and convert it to markdown for reading.",
		DescriptionFr: "Récupérer une page web et la convertir en markdown pour la lire.",
		Category:      "web",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "uvx",
			Args:    []string{"mcp-server-fetch"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/fetch",
		Tags:      []string{"http", "scraping"},
		Verified:  true,
	},
	{
		ID:            "puppeteer",
		Name:          "Puppeteer",
		Description:   "Drive a headless browser: navigate, click, fill forms, and screenshot.",
		DescriptionFr: "Piloter un navigateur headless : naviguer, cliquer, remplir des formulaires et capturer l'écran.",
		Category:      "web",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-puppeteer"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/puppeteer",
		Tags:      []string{"browser", "automation"},
	},
	{
		ID:            "playwright",
		Name:          "Playwright",
		Description:   "Browser automation and testing across Chromium, Firefox, and WebKit.",
		DescriptionFr: "Automatisation et tests de navigateur sur Chromium, Firefox et WebKit.",
		Category:      "web",
		Vendor:        "Microsoft",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@playwright/mcp@latest"},
		},
		SourceURL: "https://github.com/microsoft/playwright-mcp",
		Tags:      []string{"browser", "testing"},
	},
	{
		ID:            "brave-search",
		Name:          "Brave Search",
		Description:   "Search the web through the Brave Search API.",
		DescriptionFr: "Rechercher sur le web via l'API Brave Search.",
		Category:      "search",
		Vendor:        "Brave",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-brave-search"},
			Env:     map[string]string{"BRAVE_API_KEY": ""},
		},
		Env: []catalogEnvVar{{
			Name:        "BRAVE_API_KEY",
			Description: "API key from the Brave Search developer dashboard.",
			Required:    true,
			Secret:      true,
		}},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/brave-search",
		Tags:      []string{"search", "api"},
	},
	{
		ID:            "memory",
		Name:          "Memory",
		Description:   "A knowledge graph that persists facts across conversations.",
		DescriptionFr: "Un graphe de connaissances qui conserve des faits d'une conversation à l'autre.",
		Category:      "productivity",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/memory",
		Tags:      []string{"knowledge", "state"},
		Verified:  true,
	},
	{
		ID:            "sequential-thinking",
		Name:          "Sequential Thinking",
		Description:   "Structured step-by-step reasoning for problems that need decomposition.",
		DescriptionFr: "Raisonnement structuré étape par étape pour les problèmes qui demandent une décomposition.",
		Category:      "productivity",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking",
		Tags:      []string{"reasoning"},
	},
	{
		ID:            "slack",
		Name:          "Slack",
		Description:   "Read channels and post messages in a Slack workspace.",
		DescriptionFr: "Lire les canaux et publier des messages dans un espace de travail Slack.",
		Category:      "communication",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-slack"},
			Env:     map[string]string{"SLACK_BOT_TOKEN": "", "SLACK_TEAM_ID": ""},
		},
		Env: []catalogEnvVar{
			{Name: "SLACK_BOT_TOKEN", Description: "Bot user OAuth token for your Slack app.", Required: true, Secret: true, Placeholder: "xoxb-..."},
			{Name: "SLACK_TEAM_ID", Description: "Workspace (team) identifier.", Required: true, Placeholder: "T01234567"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/slack",
		Tags:      []string{"chat", "api"},
	},
	{
		ID:            "google-drive",
		Name:          "Google Drive",
		Description:   "Search Drive and read the contents of documents.",
		DescriptionFr: "Chercher dans Drive et lire le contenu des documents.",
		Category:      "files",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-gdrive"},
			Env:     map[string]string{"GDRIVE_CREDENTIALS_PATH": ""},
		},
		Env: []catalogEnvVar{{
			Name:        "GDRIVE_CREDENTIALS_PATH",
			Description: "Path to the OAuth credentials file created during setup.",
			Required:    true,
			Placeholder: "/path/to/.gdrive-server-credentials.json",
		}},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/gdrive",
		Tags:      []string{"documents", "google"},
	},
	{
		ID:            "sentry",
		Name:          "Sentry",
		Description:   "Pull issue details and stack traces from Sentry.",
		DescriptionFr: "Récupérer le détail des incidents et les stack traces depuis Sentry.",
		Category:      "monitoring",
		Vendor:        "Sentry",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "uvx",
			Args:    []string{"mcp-server-sentry", "--auth-token", "${SENTRY_AUTH_TOKEN}"},
			Env:     map[string]string{"SENTRY_AUTH_TOKEN": ""},
		},
		Env: []catalogEnvVar{{
			Name:        "SENTRY_AUTH_TOKEN",
			Description: "Sentry auth token with issue read access.",
			Required:    true,
			Secret:      true,
		}},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/sentry",
		Tags:      []string{"errors", "observability"},
	},
	{
		ID:            "time",
		Name:          "Time",
		Description:   "Current time and timezone conversion.",
		DescriptionFr: "Heure courante et conversion de fuseaux horaires.",
		Category:      "utilities",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "uvx",
			Args:    []string{"mcp-server-time"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/time",
		Tags:      []string{"utility"},
	},
	{
		ID:            "everything",
		Name:          "Everything (reference)",
		Description:   "Reference server exercising every MCP feature. Useful for testing a proxy setup.",
		DescriptionFr: "Serveur de référence qui exerce toutes les fonctionnalités MCP. Utile pour tester une installation de proxy.",
		Category:      "utilities",
		Vendor:        "Anthropic",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
		},
		SourceURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/everything",
		Tags:      []string{"testing", "reference"},
		Verified:  true,
	},
	{
		ID: "grafana", Name: "Grafana", Vendor: "Grafana Labs", Category: "monitoring",
		Description:   "Inspect dashboards, alerts, and data sources in Grafana without write tools.",
		DescriptionFr: "Inspecter les tableaux de bord, alertes et sources de données Grafana sans outils d'écriture.",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "uvx", Args: []string{"mcp-grafana==1.6.3", "--disable-write"}, Env: map[string]string{"GRAFANA_URL": "", "GRAFANA_SERVICE_ACCOUNT_TOKEN": ""}},
		Env: []catalogEnvVar{
			{Name: "GRAFANA_URL", Description: "Grafana instance URL (Grafana 9+).", Required: true, Placeholder: "https://grafana.example.com"},
			{Name: "GRAFANA_SERVICE_ACCOUNT_TOKEN", Description: "Service account token with limited read permissions.", Required: true, Secret: true},
		},
		SourceURL: "https://github.com/grafana/mcp-grafana", Tags: []string{"observability", "dashboards"},
	},
	{
		ID: "clickhouse", Name: "ClickHouse", Vendor: "ClickHouse", Category: "database",
		Description:   "Explore ClickHouse tables and run read-only SQL queries by default.",
		DescriptionFr: "Explorer les tables ClickHouse et exécuter des requêtes SQL en lecture seule par défaut.",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "uv", Args: []string{"run", "--with", "mcp-clickhouse==0.7.0", "--python", "3.12", "mcp-clickhouse"}, Env: map[string]string{"CLICKHOUSE_HOST": "", "CLICKHOUSE_USER": "", "CLICKHOUSE_PASSWORD": "", "CLICKHOUSE_ALLOW_WRITE_ACCESS": "false"}},
		Env: []catalogEnvVar{
			{Name: "CLICKHOUSE_HOST", Description: "ClickHouse HTTP endpoint host.", Required: true, Placeholder: "clickhouse.example.com"},
			{Name: "CLICKHOUSE_USER", Description: "Database user with only the required grants.", Required: true, Placeholder: "readonly"},
			{Name: "CLICKHOUSE_PASSWORD", Description: "Database password (empty for passwordless users).", Required: true, Secret: true},
			{Name: "CLICKHOUSE_ALLOW_WRITE_ACCESS", Description: "Keep false for read-only queries.", Required: false, Placeholder: "false"},
		},
		SourceURL: "https://github.com/ClickHouse/mcp-clickhouse", Tags: []string{"sql", "analytics"},
	},
	{
		ID: "mysql", Name: "MySQL", Vendor: "Ben Borla", Category: "database",
		Description:   "Inspect MySQL schemas and run queries; writes are disabled by default.",
		DescriptionFr: "Inspecter les schémas MySQL et interroger les données ; l'écriture est désactivée par défaut.",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "npx", Args: []string{"-y", "@benborla29/mcp-server-mysql@2.0.9"}, Env: map[string]string{"MYSQL_HOST": "", "MYSQL_PORT": "3306", "MYSQL_USER": "", "MYSQL_PASS": "", "MYSQL_DB": ""}},
		Env: []catalogEnvVar{
			{Name: "MYSQL_HOST", Description: "MySQL server hostname.", Required: true, Placeholder: "127.0.0.1"},
			{Name: "MYSQL_PORT", Description: "MySQL port.", Placeholder: "3306"},
			{Name: "MYSQL_USER", Description: "Database user; prefer read-only grants.", Required: true},
			{Name: "MYSQL_PASS", Description: "Database password.", Required: true, Secret: true},
			{Name: "MYSQL_DB", Description: "Database name.", Required: true},
		},
		SourceURL: "https://github.com/benborla/mcp-server-mysql", Tags: []string{"sql", "schema"},
	},
	{
		ID: "aws-documentation", Name: "AWS Documentation", Vendor: "AWS", Category: "development",
		Description:   "Search and read official AWS documentation without AWS credentials.",
		DescriptionFr: "Rechercher et lire la documentation officielle AWS sans identifiants AWS.",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "uvx", Args: []string{"awslabs.aws-documentation-mcp-server==1.2.2"}},
		SourceURL:     "https://github.com/awslabs/mcp/tree/main/src/aws-documentation-mcp-server", Tags: []string{"aws", "docs"},
	},
	{
		ID: "microsoft-learn", Name: "Microsoft Learn", Vendor: "Microsoft", Category: "development",
		Description:   "Search Microsoft technical documentation and official code examples.",
		DescriptionFr: "Rechercher dans la documentation technique Microsoft et ses exemples de code officiels.",
		TransportType: MCPClientTypeStreamable,
		DefaultConfig: catalogConfig{URL: "https://learn.microsoft.com/api/mcp"},
		SourceURL:     "https://github.com/MicrosoftDocs/mcp", Tags: []string{"microsoft", "azure", "docs", "remote"},
	},
	{
		ID: "cloudflare-documentation", Name: "Cloudflare Documentation", Vendor: "Cloudflare", Category: "development",
		Description:   "Look up current Cloudflare product documentation.",
		DescriptionFr: "Consulter la documentation actuelle des produits Cloudflare.",
		TransportType: MCPClientTypeStreamable,
		DefaultConfig: catalogConfig{URL: "https://docs.mcp.cloudflare.com/mcp"},
		SourceURL:     "https://github.com/cloudflare/mcp-server-cloudflare/tree/main/apps/docs-ai-search", Tags: []string{"cloudflare", "docs", "remote"},
	},
	{
		ID: "portainer", Name: "Portainer", Vendor: "Portainer", Category: "monitoring",
		Description:   "Inspect Portainer environments with read-only tools and no Docker/Kubernetes proxy tools. Requires Portainer 2.45.x.",
		DescriptionFr: "Inspecter les environnements Portainer avec des outils en lecture seule, sans proxy Docker/Kubernetes. Nécessite Portainer 2.45.x.",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "uvx", Args: []string{"--from", "mcp-portainer==2.45.1", "mcp-portainer"}, Env: map[string]string{"PORTAINER_URL": "", "PORTAINER_API_KEY": "", "PORTAINER_READ_ONLY": "1", "PORTAINER_NO_PROXY": "1"}},
		Env: []catalogEnvVar{
			{Name: "PORTAINER_URL", Description: "Portainer 2.45.x instance URL.", Required: true, Placeholder: "https://portainer.example.com"},
			{Name: "PORTAINER_API_KEY", Description: "Portainer API access token; use least privilege.", Required: true, Secret: true},
			{Name: "PORTAINER_READ_ONLY", Description: "Only expose GET/HEAD API tools; keep set to 1.", Required: true, Placeholder: "1"},
			{Name: "PORTAINER_NO_PROXY", Description: "Disable Docker/Kubernetes API proxy tools; keep set to 1.", Required: true, Placeholder: "1"},
		},
		SourceURL: "https://github.com/portainer/portainer-mcp", Tags: []string{"containers", "inventory"},
	},
	{
		ID: "redis", Name: "Redis", Vendor: "Redis", Category: "database",
		Description:   "Inspect and manage Redis data; use a read-only Redis ACL user for safe browsing.",
		DescriptionFr: "Inspecter et gérer les données Redis ; utiliser un compte ACL en lecture seule pour une consultation sûre.",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "uvx", Args: []string{"--from", "redis-mcp-server==0.5.1", "redis-mcp-server"}, Env: map[string]string{"REDIS_HOST": "", "REDIS_PORT": "6379", "REDIS_USERNAME": "", "REDIS_PWD": ""}},
		Env: []catalogEnvVar{
			{Name: "REDIS_HOST", Description: "Redis hostname.", Required: true, Placeholder: "127.0.0.1"},
			{Name: "REDIS_PORT", Description: "Redis port.", Placeholder: "6379"},
			{Name: "REDIS_USERNAME", Description: "ACL user; limit grants to the intended data and operations.", Placeholder: "readonlyuser"},
			{Name: "REDIS_PWD", Description: "Redis ACL password, if required.", Secret: true},
		},
		SourceURL: "https://github.com/redis/mcp-redis", Tags: []string{"cache", "data"},
	},
	{
		ID:            "custom-stdio",
		Name:          "Custom stdio server",
		Description:   "Start from a blank stdio definition and fill in your own command.",
		DescriptionFr: "Partir d'une définition stdio vierge et renseigner votre propre commande.",
		Category:      "custom",
		TransportType: MCPClientTypeStdio,
		DefaultConfig: catalogConfig{Command: "", Args: []string{}},
		Tags:          []string{"custom"},
	},
	{
		ID:            "custom-http",
		Name:          "Custom HTTP server",
		Description:   "Connect to an MCP server that is already running over streamable HTTP.",
		DescriptionFr: "Se connecter à un serveur MCP déjà en fonctionnement en HTTP streamable.",
		Category:      "custom",
		TransportType: MCPClientTypeStreamable,
		DefaultConfig: catalogConfig{URL: ""},
		Tags:          []string{"custom", "remote"},
	},
}

// catalogByID indexes builtinCatalog for install lookups.
func catalogByID(id string) (catalogItem, bool) {
	for _, item := range builtinCatalog {
		if item.ID == id {
			return item, true
		}
	}
	return catalogItem{}, false
}

// filterCatalog applies the UI's search box and category dropdown. The query
// matches on everything a user might type: name, either description, vendor,
// or a tag.
func filterCatalog(query, category string) []catalogItem {
	q := strings.ToLower(strings.TrimSpace(query))
	cat := strings.ToLower(strings.TrimSpace(category))

	items := make([]catalogItem, 0, len(builtinCatalog))
	for _, item := range builtinCatalog {
		if cat != "" && cat != "all" && !strings.EqualFold(item.Category, cat) {
			continue
		}
		if q != "" && !catalogMatches(item, q) {
			continue
		}
		items = append(items, item)
	}
	return items
}

func catalogMatches(item catalogItem, q string) bool {
	haystack := []string{
		item.ID, item.Name, item.Description, item.DescriptionFr,
		item.Vendor, item.Category,
	}
	haystack = append(haystack, item.Tags...)
	for _, field := range haystack {
		if field != "" && strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

// catalogCategories lists the distinct categories present, sorted, for the
// filter bar.
func catalogCategories() []string {
	seen := make(map[string]struct{}, len(builtinCatalog))
	out := make([]string, 0, len(builtinCatalog))
	for _, item := range builtinCatalog {
		if _, ok := seen[item.Category]; ok {
			continue
		}
		seen[item.Category] = struct{}{}
		out = append(out, item.Category)
	}
	sort.Strings(out)
	return out
}

// handleCatalog serves GET /api/mcp/catalog.
func (m *Manager) handleCatalog(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	items := filterCatalog(query.Get("q"), query.Get("category"))
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"categories": catalogCategories(),
	})
}
