package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// --- ASN Resolver ---

// ASNStore is the cache interface needed by the ASN resolver.
type ASNStore interface {
	GetCache(ctx context.Context, key string) (string, error)
	SetCache(ctx context.Context, key, value string) error
}

// ripeStatResponse is the response from the RIPEstat Announced Prefixes API.
type ripeStatResponse struct {
	Status string `json:"status"`
	Data   struct {
		Prefixes []struct {
			Prefix string `json:"prefix"`
		} `json:"prefixes"`
	} `json:"data"`
}

// ripeStatClient is a simple HTTP client for the RIPEstat API.
var ripeStatClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	},
}

// resolveASNPrefixes fetches announced IPv4 CIDR prefixes for a single ASN from RIPEstat.
// Returns a deduplicated, sorted list of CIDR strings.
func resolveASNPrefixes(ctx context.Context, asn int) ([]string, error) {
	url := fmt.Sprintf("https://stat.ripe.net/data/announced-prefixes/data.json?resource=AS%d", asn)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create ripestat request: %w", err)
	}

	resp, err := ripeStatClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ripestat request for AS%d: %w", asn, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ripestat returned status %d for AS%d", resp.StatusCode, asn)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ripestat response for AS%d: %w", asn, err)
	}

	var result ripeStatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse ripestat response for AS%d: %w", asn, err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("ripestat returned status %q for AS%d", result.Status, asn)
	}

	// Collect only IPv4 prefixes (skip IPv6 — most VPN routing is IPv4).
	var prefixes []string
	seen := map[string]bool{}
	for _, p := range result.Data.Prefixes {
		prefix := strings.TrimSpace(p.Prefix)
		if prefix == "" || seen[prefix] {
			continue
		}
		// Skip IPv6 (contains ":").
		if strings.Contains(prefix, ":") {
			continue
		}
		seen[prefix] = true
		prefixes = append(prefixes, prefix)
	}

	sort.Strings(prefixes)
	return prefixes, nil
}

// resolveMultiASNPrefixes resolves prefixes for multiple ASNs and merges them.
// Uses SQLite cache to avoid hammering the RIPEstat API on repeated calls.
// Cache key pattern: "asn_prefixes_{asn}".
func resolveMultiASNPrefixes(ctx context.Context, store ASNStore, asns []int) ([]string, error) {
	if len(asns) == 0 {
		return nil, nil
	}

	allPrefixes := map[string]bool{}

	for _, asn := range asns {
		cacheKey := fmt.Sprintf("asn_prefixes_%d", asn)

		// Check cache first.
		cached, _ := store.GetCache(ctx, cacheKey)
		if cached != "" {
			var prefixes []string
			if err := json.Unmarshal([]byte(cached), &prefixes); err == nil && len(prefixes) > 0 {
				for _, p := range prefixes {
					allPrefixes[p] = true
				}
				log.Printf("[ASN] AS%d: %d prefixes (cached)", asn, len(prefixes))
				continue
			}
		}

		// Fetch from RIPEstat.
		prefixes, err := resolveASNPrefixes(ctx, asn)
		if err != nil {
			log.Printf("[ASN] failed to resolve AS%d: %v", asn, err)
			continue // Non-fatal: skip this ASN and continue with others.
		}

		// Cache the result.
		if data, err := json.Marshal(prefixes); err == nil {
			_ = store.SetCache(ctx, cacheKey, string(data))
		}

		for _, p := range prefixes {
			allPrefixes[p] = true
		}
		log.Printf("[ASN] AS%d: %d prefixes (fetched)", asn, len(prefixes))
	}

	// Flatten to sorted list.
	result := make([]string, 0, len(allPrefixes))
	for p := range allPrefixes {
		result = append(result, p)
	}
	sort.Strings(result)

	return result, nil
}

// --- OPNsense Alias CRUD ---

// ensureAlias creates or updates an OPNsense alias containing CIDR blocks.
// Alias type is "network" (list of networks/hosts).
// The content is a newline-separated list of CIDR strings.
// Returns the alias UUID and whether it was newly created.
func ensureAlias(ctx context.Context, api *opnsenseAPIClient, aliasName string, cidrs []string) (string, bool, error) {
	// Content for a "network" alias: newline-separated CIDR entries.
	content := strings.Join(cidrs, "\n")

	payload := map[string]any{
		"alias": map[string]any{
			"enabled":     "1",
			"name":        aliasName,
			"type":        "network",
			"proto":       "",
			"content":     content,
			"description": "Gator IP-based routing alias (auto-managed)",
		},
	}

	// Search for existing alias by name.
	searchResp, err := api.Post(ctx, "/api/firewall/alias/search_item", map[string]any{})
	if err == nil {
		for _, raw := range asSlice(searchResp["rows"]) {
			row := asMap(raw)
			if asString(row["name"]) == aliasName {
				uuid := asString(row["uuid"])
				if uuid != "" {
					// Update existing alias.
					_, err := api.Post(ctx, "/api/firewall/alias/set_item/"+uuid, payload)
					if err != nil {
						return "", false, fmt.Errorf("update alias %s: %w", aliasName, err)
					}
					log.Printf("[Alias] updated %s (%s) with %d CIDRs", aliasName, uuid, len(cidrs))
					return uuid, false, nil
				}
			}
		}
	}

	// Create new alias.
	resp, err := api.Post(ctx, "/api/firewall/alias/add_item", payload)
	if err != nil {
		return "", false, fmt.Errorf("create alias %s: %w", aliasName, err)
	}

	uuid, err := extractUUID(resp)
	if err != nil {
		return "", false, fmt.Errorf("alias %s created but no UUID returned: %w", aliasName, err)
	}

	log.Printf("[Alias] created %s (%s) with %d CIDRs", aliasName, uuid, len(cidrs))
	return uuid, true, nil
}

// applyAliases calls reconfigure to apply alias changes.
func applyAliases(ctx context.Context, api *opnsenseAPIClient) error {
	_, err := api.Post(ctx, "/api/firewall/alias/reconfigure", map[string]any{})
	return err
}

// deleteAlias removes an OPNsense alias by name.
// Returns true if the alias was found and deleted.
func deleteAlias(ctx context.Context, api *opnsenseAPIClient, aliasName string) bool {
	// Find the alias UUID by name.
	searchResp, err := api.Post(ctx, "/api/firewall/alias/search_item", map[string]any{})
	if err != nil {
		return false
	}

	for _, raw := range asSlice(searchResp["rows"]) {
		row := asMap(raw)
		if asString(row["name"]) == aliasName {
			uuid := asString(row["uuid"])
			if uuid != "" {
				_, err := api.Post(ctx, "/api/firewall/alias/del_item/"+uuid, map[string]any{})
				if err != nil {
					log.Printf("[Alias] failed to delete %s (%s): %v", aliasName, uuid, err)
					return false
				}
				log.Printf("[Alias] deleted %s (%s)", aliasName, uuid)
				_ = applyAliases(ctx, api)
				return true
			}
		}
	}

	return false
}

// aliasNameForApp returns the OPNsense alias name for an app profile's IP ranges.
func aliasNameForApp(appID string) string {
	// OPNsense alias names: alphanumeric + underscore, max ~32 chars.
	// Prefix with GATOR_ for easy identification.
	name := "GATOR_" + strings.ToUpper(strings.ReplaceAll(appID, "-", "_")) + "_IPS"
	if len(name) > 31 {
		name = name[:31]
	}
	return name
}

// deleteAliasesByPrefix removes all OPNsense aliases whose name starts with the given prefix.
func deleteAliasesByPrefix(ctx context.Context, api *opnsenseAPIClient, prefix string) int {
	searchResp, err := api.Post(ctx, "/api/firewall/alias/search_item", map[string]any{})
	if err != nil {
		return 0
	}

	deleted := 0
	for _, raw := range asSlice(searchResp["rows"]) {
		row := asMap(raw)
		name := asString(row["name"])
		if strings.HasPrefix(name, prefix) {
			uuid := asString(row["uuid"])
			if uuid != "" {
				_, err := api.Post(ctx, "/api/firewall/alias/del_item/"+uuid, map[string]any{})
				if err == nil {
					log.Printf("[Alias] deleted %s (%s)", name, uuid)
					deleted++
				}
			}
		}
	}
	if deleted > 0 {
		_ = applyAliases(ctx, api)
	}
	return deleted
}
