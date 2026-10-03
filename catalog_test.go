package main

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// These recipes are intentionally pinned and contain no credentials in args.
// The commands are documentation-checked, not runtime-verified.
func TestCuratedCatalogRecipes(t *testing.T) {
	tests := []struct {
		id, vendor, category, source, command, url string
		args                                       []string
		required, secrets                          []string
	}{
		{"grafana", "Grafana Labs", "monitoring", "https://github.com/grafana/mcp-grafana", "uvx", "", []string{"mcp-grafana==1.6.3", "--disable-write"}, []string{"GRAFANA_URL", "GRAFANA_SERVICE_ACCOUNT_TOKEN"}, []string{"GRAFANA_SERVICE_ACCOUNT_TOKEN"}},
		{"clickhouse", "ClickHouse", "database", "https://github.com/ClickHouse/mcp-clickhouse", "uv", "", []string{"run", "--with", "mcp-clickhouse==0.7.0", "--python", "3.12", "mcp-clickhouse"}, []string{"CLICKHOUSE_HOST", "CLICKHOUSE_USER", "CLICKHOUSE_PASSWORD"}, []string{"CLICKHOUSE_PASSWORD"}},
		{"mysql", "Ben Borla", "database", "https://github.com/benborla/mcp-server-mysql", "npx", "", []string{"-y", "@benborla29/mcp-server-mysql@2.0.9"}, []string{"MYSQL_HOST", "MYSQL_USER", "MYSQL_PASS", "MYSQL_DB"}, []string{"MYSQL_PASS"}},
		{"aws-documentation", "AWS", "development", "https://github.com/awslabs/mcp/tree/main/src/aws-documentation-mcp-server", "uvx", "", []string{"awslabs.aws-documentation-mcp-server==1.2.2"}, nil, nil},
		{"microsoft-learn", "Microsoft", "development", "https://github.com/MicrosoftDocs/mcp", "", "https://learn.microsoft.com/api/mcp", nil, nil, nil},
		{"cloudflare-documentation", "Cloudflare", "development", "https://github.com/cloudflare/mcp-server-cloudflare/tree/main/apps/docs-ai-search", "", "https://docs.mcp.cloudflare.com/mcp", nil, nil, nil},
		{"portainer", "Portainer", "monitoring", "https://github.com/portainer/portainer-mcp", "uvx", "", []string{"--from", "mcp-portainer==2.45.1", "mcp-portainer"}, []string{"PORTAINER_URL", "PORTAINER_API_KEY"}, []string{"PORTAINER_API_KEY"}},
		{"redis", "Redis", "database", "https://github.com/redis/mcp-redis", "uvx", "", []string{"--from", "redis-mcp-server==0.5.1", "redis-mcp-server"}, []string{"REDIS_HOST"}, []string{"REDIS_PWD"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			item, ok := catalogByID(tt.id)
			if !ok {
				t.Fatalf("catalog entry %s missing", tt.id)
			}
			if item.Vendor != tt.vendor || item.Category != tt.category || item.SourceURL != tt.source {
				t.Errorf("metadata: vendor=%q category=%q source=%q", item.Vendor, item.Category, item.SourceURL)
			}
			if item.Description == "" || item.DescriptionFr == "" || item.Verified {
				t.Errorf("bilingual description missing or untested entry marked verified: %+v", item)
			}
			if item.DefaultConfig.Command != tt.command || item.DefaultConfig.URL != tt.url || !reflect.DeepEqual(item.DefaultConfig.Args, tt.args) {
				t.Errorf("recipe: %+v", item.DefaultConfig)
			}
			for _, name := range tt.required {
				found := false
				for _, env := range item.Env {
					if env.Name == name && env.Required {
						found = true
					}
				}
				if !found {
					t.Errorf("required env %s missing", name)
				}
			}
			for _, name := range tt.secrets {
				found := false
				for _, env := range item.Env {
					if env.Name == name && env.Secret {
						found = true
					}
				}
				if !found {
					t.Errorf("secret env %s missing", name)
				}
			}
			for _, env := range item.Env {
				if _, ok := item.DefaultConfig.Env[env.Name]; !ok {
					t.Errorf("env %s absent from default config", env.Name)
				}
				if env.Secret && item.DefaultConfig.Env[env.Name] != "" {
					t.Errorf("secret %s has nonempty default", env.Name)
				}
			}
			for _, arg := range item.DefaultConfig.Args {
				if strings.Contains(strings.ToLower(arg), "password") || strings.Contains(strings.ToLower(arg), "token") || strings.Contains(arg, "${") || strings.Contains(arg, "@latest") {
					t.Errorf("unsafe or mutable arg %q", arg)
				}
			}
		})
	}
}

// The catalog is hand-maintained data, and a malformed entry is a broken
// install button rather than a compile error - so the invariants the install
// path relies on are asserted over the whole table.
func TestBuiltinCatalogIsWellFormed(t *testing.T) {
	if len(builtinCatalog) == 0 {
		t.Fatal("the catalog is empty")
	}

	seen := make(map[string]struct{}, len(builtinCatalog))
	for _, item := range builtinCatalog {
		if item.ID == "" {
			t.Errorf("%q: missing id", item.Name)
			continue
		}
		if _, dup := seen[item.ID]; dup {
			t.Errorf("%q: duplicate id - catalogByID would never reach the second entry", item.ID)
		}
		seen[item.ID] = struct{}{}

		if item.Name == "" {
			t.Errorf("%s: missing name", item.ID)
		}
		if item.Category == "" {
			t.Errorf("%s: missing category, so the filter bar cannot reach it", item.ID)
		}
		if item.TransportType == "" {
			t.Errorf("%s: missing transportType", item.ID)
		}
		// Every entry but the blank templates must be launchable as it stands:
		// load() rejects a server with neither, so installing it unedited
		// could not be saved. The "custom" entries exist precisely to be
		// filled in, so they are exempt.
		if item.Category != "custom" && item.DefaultConfig.Command == "" && item.DefaultConfig.URL == "" {
			t.Errorf("%s: has neither a command nor a url", item.ID)
		}
		if item.DefaultConfig.Command != "" && item.DefaultConfig.URL != "" {
			t.Errorf("%s: command and url are mutually exclusive", item.ID)
		}
		// Both languages ship compiled in so the UI can switch locale without
		// a round trip; a missing French description silently falls back.
		if item.Description == "" {
			t.Errorf("%s: missing description", item.ID)
		}
		if item.DescriptionFr == "" {
			t.Errorf("%s: missing descriptionFr", item.ID)
		}
	}
}

func TestCatalogByID(t *testing.T) {
	want := builtinCatalog[0]
	got, ok := catalogByID(want.ID)
	if !ok {
		t.Fatalf("catalogByID(%q) reported a miss", want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("name = %q, want %q", got.Name, want.Name)
	}

	if _, ok := catalogByID("definitely-not-a-catalog-entry"); ok {
		t.Error("catalogByID reported a hit for an unknown id")
	}
	if _, ok := catalogByID(""); ok {
		t.Error("an empty id must not match")
	}
}

func TestFilterCatalogByCategory(t *testing.T) {
	category := builtinCatalog[0].Category

	items := filterCatalog("", category)
	if len(items) == 0 {
		t.Fatalf("no entries in category %q", category)
	}
	for _, item := range items {
		if !strings.EqualFold(item.Category, category) {
			t.Errorf("%s: category %q leaked into a filter on %q", item.ID, item.Category, category)
		}
	}

	// The UI sends "all" for the unfiltered state, and casing comes from
	// whatever the dropdown was populated with.
	if got := len(filterCatalog("", "all")); got != len(builtinCatalog) {
		t.Errorf(`filterCatalog("", "all") returned %d entries, want all %d`, got, len(builtinCatalog))
	}
	if got := len(filterCatalog("", "  ")); got != len(builtinCatalog) {
		t.Errorf("a blank category returned %d entries, want all %d", got, len(builtinCatalog))
	}
	if got := len(filterCatalog("", strings.ToUpper(category))); got != len(items) {
		t.Errorf("category matching is case sensitive: %d != %d", got, len(items))
	}
	if got := filterCatalog("", "no-such-category"); len(got) != 0 {
		t.Errorf("an unknown category returned %d entries, want 0", len(got))
	}
}

func TestFilterCatalogByQuery(t *testing.T) {
	item := builtinCatalog[0]

	for _, query := range []string{item.ID, item.Name, strings.ToUpper(item.Name)} {
		found := false
		for _, got := range filterCatalog(query, "") {
			if got.ID == item.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("searching %q did not return %s", query, item.ID)
		}
	}

	// The French description is searchable too, so a French-speaking user's
	// search terms hit the same entries.
	if item.DescriptionFr != "" {
		word := strings.Fields(item.DescriptionFr)[0]
		if len(filterCatalog(word, "")) == 0 {
			t.Errorf("searching the French description %q returned nothing", word)
		}
	}

	if got := filterCatalog("zzzz-no-entry-says-this", ""); len(got) != 0 {
		t.Errorf("a query that matches nothing returned %d entries", len(got))
	}
	// A non-nil empty slice keeps the JSON body as [] rather than null, which
	// the UI would have to special-case.
	if filterCatalog("zzzz-no-entry-says-this", "") == nil {
		t.Error("an empty result must marshal as [], not null")
	}
}

func TestCatalogCategoriesAreSortedAndUnique(t *testing.T) {
	categories := catalogCategories()
	if len(categories) == 0 {
		t.Fatal("no categories")
	}
	if !sort.StringsAreSorted(categories) {
		t.Errorf("categories are not sorted: %v", categories)
	}

	seen := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		if _, dup := seen[category]; dup {
			t.Errorf("duplicate category %q", category)
		}
		seen[category] = struct{}{}
	}
	for _, item := range builtinCatalog {
		if _, ok := seen[item.Category]; !ok {
			t.Errorf("%s: category %q is missing from the filter bar", item.ID, item.Category)
		}
	}
}

func TestCatalogEndpointFilters(t *testing.T) {
	manager, _ := newTestManager(t, testConfigJSON)
	mux := managerMux(t, manager)

	rec := do(t, mux, http.MethodGet, "/api/mcp/catalog", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var all struct {
		Items      []catalogItem `json:"items"`
		Categories []string      `json:"categories"`
	}
	decodeBody(t, rec, &all)
	if len(all.Items) != len(builtinCatalog) {
		t.Errorf("items = %d, want the whole catalog (%d)", len(all.Items), len(builtinCatalog))
	}
	if len(all.Categories) == 0 {
		t.Error("the response carries no categories")
	}

	category := builtinCatalog[0].Category
	rec = do(t, mux, http.MethodGet, "/api/mcp/catalog?category="+category, nil)
	var filtered struct {
		Items      []catalogItem `json:"items"`
		Categories []string      `json:"categories"`
	}
	decodeBody(t, rec, &filtered)
	if len(filtered.Items) == 0 || len(filtered.Items) > len(all.Items) {
		t.Errorf("filtered items = %d, want a non-empty subset of %d", len(filtered.Items), len(all.Items))
	}
	// The dropdown is populated from the same response, so filtering must not
	// narrow the list of categories the user can switch to.
	if len(filtered.Categories) != len(all.Categories) {
		t.Errorf("categories = %d under a filter, want all %d", len(filtered.Categories), len(all.Categories))
	}
}
