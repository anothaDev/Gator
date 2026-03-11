package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/anothaDev/gator/internal/storage"
)

const (
	asnCachePrefix   = "asn_prefixes_"
	asnRefreshTicker = 24 * time.Hour
	// Stagger RIPEstat requests to be polite — 2s between ASNs.
	asnRefreshDelay = 2 * time.Second
)

// StartASNRefreshLoop runs a background goroutine that refreshes cached ASN prefix data
// every 24 hours. It scans the cache table for keys matching "asn_prefixes_*", extracts
// the ASN number, re-fetches from RIPEstat, and updates the cache.
func StartASNRefreshLoop(store *storage.SQLiteStore) {
	go func() {
		// Wait a bit on startup before the first refresh to not compete with initial requests.
		time.Sleep(5 * time.Minute)

		for {
			refreshASNCache(store)
			time.Sleep(asnRefreshTicker)
		}
	}()
}

func refreshASNCache(store *storage.SQLiteStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	entries, err := store.ListCacheKeysByPrefix(ctx, asnCachePrefix)
	if err != nil {
		log.Printf("[ASN refresh] failed to list cache keys: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	log.Printf("[ASN refresh] found %d cached ASN(s), starting refresh", len(entries))

	refreshed := 0
	for _, entry := range entries {
		// Extract ASN number from key "asn_prefixes_12345".
		asnStr := strings.TrimPrefix(entry.Key, asnCachePrefix)
		asn, err := strconv.Atoi(asnStr)
		if err != nil {
			log.Printf("[ASN refresh] invalid cache key %q, skipping", entry.Key)
			continue
		}

		// Check if the entry is less than 20 hours old (avoid re-fetching very recent data).
		if updatedAt, err := time.Parse("2006-01-02 15:04:05", entry.UpdatedAt); err == nil {
			if time.Since(updatedAt) < 20*time.Hour {
				continue
			}
		}

		// Fetch fresh data from RIPEstat.
		prefixes, err := resolveASNPrefixes(ctx, asn)
		if err != nil {
			log.Printf("[ASN refresh] AS%d: failed: %v", asn, err)
			// Keep the stale cache — don't delete it.
			time.Sleep(asnRefreshDelay)
			continue
		}

		// Update cache.
		data, err := json.Marshal(prefixes)
		if err != nil {
			continue
		}
		if err := store.SetCache(ctx, entry.Key, string(data)); err != nil {
			log.Printf("[ASN refresh] AS%d: failed to update cache: %v", asn, err)
		} else {
			log.Printf("[ASN refresh] AS%d: refreshed, %d prefixes", asn, len(prefixes))
			refreshed++
		}

		time.Sleep(asnRefreshDelay)
	}

	if refreshed > 0 {
		log.Printf("[ASN refresh] completed: %d/%d ASN(s) refreshed", refreshed, len(entries))
	}
}
