package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil/bech32"
	olc "github.com/google/open-location-code/go"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
)

// Config holds all configuration
type Config struct {
	Nsec          string   `json:"nsec"`
	PostToRelays  bool     `json:"post_to_relays"`
	Relays        []string `json:"relays"`
	HWInterval    int      `json:"hw_interval"`
	HitchInterval int      `json:"hitch_interval"`
	Debug         bool     `json:"debug"`
	DryRun        bool     `json:"dry_run"`
}

// Daemon manages both tasks
type Daemon struct {
	config *Config
	nostr  *NostrClient
	posted map[string]bool
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NostrClient handles all Nostr operations
type NostrClient struct {
	sk     string
	pk     string
	relays []*nostr.Relay
}

// HitchwikiEntry represents a recent change
type HitchwikiEntry struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	Updated string `xml:"updated"`
	Summary string `xml:"summary"`
	ID      string `xml:"id"`
	Author  string `xml:"author>name"`
}

// HitchmapEntry represents hitchhiking data
type HitchmapEntry struct {
	ID           int      `json:"id"`
	StartLat     float64  `json:"start_lat"`
	StartLng     float64  `json:"start_lng"`
	Rating       float64  `json:"rating"`
	Country      string   `json:"country"`
	Wait         *int     `json:"wait"`
	Hitchhiker   *string  `json:"nickname"`
	Description  *string  `json:"comment"`
	DateTime     string   `json:"datetime"`
	EndLat       *float64 `json:"dest_lat"`
	EndLng       *float64 `json:"dest_lon"`
	Signal       *string  `json:"signal"`
	RideDateTime *string  `json:"ride_datetime"`
	UserID       *int     `json:"user_id"`
	FromWiki     *bool    `json:"from_hitchwiki"`
}

func main() {
	var configFile = flag.String("config", "config.json", "Configuration file")
	var debug = flag.Bool("debug", false, "Enable debug logging")
	var dryRun = flag.Bool("dry-run", false, "Don't post to relays")
	var runOnce = flag.Bool("once", false, "Run once and exit")
	flag.Parse()

	config := loadConfig(*configFile)
	config.Debug = *debug
	config.DryRun = *dryRun

	daemon := NewDaemon(config)

	if *runOnce {
		daemon.runOnce()
		return
	}

	daemon.run()
}

func loadConfig(filename string) *Config {
	// Check if config file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		log.Fatalf("Configuration file '%s' not found. Please create it with your settings.", filename)
	}

	// Read config file
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading config file '%s': %v", filename, err)
	}

	config := &Config{}
	if err := json.Unmarshal(data, config); err != nil {
		log.Fatalf("Error parsing config file '%s': %v", filename, err)
	}

	// Validate required fields
	if config.Nsec == "" {
		log.Fatal("Configuration error: 'nsec' field is required")
	}
	if len(config.Relays) == 0 {
		log.Fatal("Configuration error: 'relays' field is required and cannot be empty")
	}

	return config
}

func NewDaemon(config *Config) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())

	daemon := &Daemon{
		config: config,
		posted: make(map[string]bool),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize Nostr client
	if sk, err := decodeNsec(config.Nsec); err == nil {
		if pk, err := nostr.GetPublicKey(sk); err == nil {
			daemon.nostr = &NostrClient{
				sk: sk,
				pk: pk,
			}
			daemon.nostr.connectRelays(config.Relays)
			log.Printf("Nostr client initialized with public key: %s", pk)
		} else {
			log.Printf("Failed to get public key: %v", err)
		}
	} else {
		log.Printf("Failed to decode nsec key: %v", err)
	}

	return daemon
}

func (d *Daemon) run() {
	log.Println("Starting Nostr Hitchhiking Bot Daemon")

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		d.cancel()
	}()

	// Start both tasks concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		d.runHitchwikiTask()
	}()

	go func() {
		defer wg.Done()
		d.runHitchmapTask()
	}()

	wg.Wait()
	log.Println("Daemon stopped")
}

func (d *Daemon) runOnce() {
	log.Println("Running tasks once...")
	// Clear posted tracking for one-time run
	d.mu.Lock()
	d.posted = make(map[string]bool)
	d.mu.Unlock()
	d.processHitchwiki()
	d.processHitchmap()
	log.Println("One-time run completed")
}

func (d *Daemon) runHitchwikiTask() {
	ticker := time.NewTicker(time.Duration(d.config.HWInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.processHitchwiki()
		}
	}
}

func (d *Daemon) runHitchmapTask() {
	ticker := time.NewTicker(time.Duration(d.config.HitchInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.processHitchmap()
		}
	}
}

func (d *Daemon) processHitchwiki() {
	log.Println("Processing Hitchwiki recent changes...")

	// Language codes for different Hitchwiki versions
	languages := []string{"en", "bg", "de", "es", "fi", "fr", "he", "hr", "it", "lt", "nl", "pl", "pt", "ro", "ru", "tr", "zh"}

	for _, lang := range languages {
		log.Printf("Fetching changes for language: %s", lang)
		entries := d.fetchHitchwiki(lang)
		log.Printf("Fetched %d Hitchwiki entries for %s", len(entries), lang)

		for _, entry := range entries {
			if d.isPosted(entry.ID) {
				log.Printf("Skipping already posted: %s", entry.ID)
				continue
			}

			log.Printf("Processing entry: %s (%s)", entry.Title, lang)
			event := d.createHitchwikiEvent(entry, lang)
			if event != nil {
				d.postEvent(event, entry.ID)
			}
		}
	}
}

func (d *Daemon) processHitchmap() {
	log.Println("Processing Hitchmap data...")

	entries := d.fetchHitchmap()
	log.Printf("Fetched %d Hitchmap entries", len(entries))

	for _, entry := range entries {
		if d.isPosted(fmt.Sprintf("hitchmap_%d", entry.ID)) {
			log.Printf("Skipping already posted: hitchmap_%d", entry.ID)
			continue
		}

		log.Printf("Processing hitchmap entry: %d", entry.ID)
		event := d.createHitchmapEvent(entry)
		if event != nil {
			d.postEvent(event, fmt.Sprintf("hitchmap_%d", entry.ID))
		}
	}
}

func (d *Daemon) fetchHitchwiki(lang string) []HitchwikiEntry {
	url := fmt.Sprintf("https://hitchwiki.org/%s/api.php?hidebots=1&urlversion=1&days=90&limit=500&action=feedrecentchanges&feedformat=atom", lang)

	log.Printf("Fetching from: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error fetching Hitchwiki: %v", err)
		return nil
	}
	defer resp.Body.Close()

	log.Printf("Response status: %s", resp.Status)

	// Simple XML parsing - in production, use proper XML parser
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		return nil
	}
	content := string(data)
	log.Printf("Response length: %d bytes", len(content))

	// Debug: show first 500 characters
	if len(content) > 500 {
		log.Printf("First 500 chars: %s", content[:500])
	} else {
		log.Printf("Full content: %s", content)
	}

	var entries []HitchwikiEntry
	// Extract entries using regex (simplified for brevity)
	entryRegex := regexp.MustCompile(`(?s)<entry>.*?</entry>`)
	matches := entryRegex.FindAllString(content, -1)
	log.Printf("Found %d entry matches", len(matches))

	for i, match := range matches {
		entry := HitchwikiEntry{}
		if title := extractXML(match, "title"); title != "" {
			entry.Title = title
		}
		if link := extractXML(match, "link"); link != "" {
			entry.Link = link
		}
		if id := extractXML(match, "id"); id != "" {
			entry.ID = id
		}
		if author := extractXML(match, "author>name"); author != "" {
			entry.Author = author
		}
		if summary := extractXML(match, "summary"); summary != "" {
			entry.Summary = summary
		}
		entries = append(entries, entry)

		if i < 3 { // Log first few entries for debugging
			log.Printf("Entry %d: %s", i+1, entry.Title)
		}
	}

	return entries
}

func (d *Daemon) fetchHitchmap() []HitchmapEntry {
	log.Println("Fetching Hitchmap data...")

	// Create hitchmap-dumps directory if it doesn't exist
	os.MkdirAll("hitchmap-dumps", 0755)

	// Calculate date range
	today := time.Now().Format("2006-01-02")
	earlier := time.Now().AddDate(0, 0, -12).Format("2006-01-02")

	filename := fmt.Sprintf("hitchmap-dumps/hitchmap_%s.sqlite", today)
	url := "https://hitchmap.com/dump.sqlite"

	// Download data if needed
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		log.Printf("Downloading Hitchmap data from %s", url)
		if err := d.downloadFile(url, filename); err != nil {
			log.Printf("Error downloading Hitchmap data: %v", err)
			return []HitchmapEntry{}
		}
	} else {
		log.Printf("Hitchmap file %s already exists", filename)
	}

	// Query database
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		log.Printf("Error opening database: %v", err)
		return []HitchmapEntry{}
	}
	defer db.Close()

	query := fmt.Sprintf("SELECT * FROM points WHERE datetime > '%s'", earlier)
	log.Printf("Executing query: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Error querying database: %v", err)
		return []HitchmapEntry{}
	}
	defer rows.Close()

	var entries []HitchmapEntry
	for rows.Next() {
		var entry HitchmapEntry
		var reviewed, banned int
		var revisedBy *int
		var ip string

		err := rows.Scan(
			&entry.ID, &entry.StartLat, &entry.StartLng, &entry.Rating,
			&entry.Country, &entry.Wait, &entry.Hitchhiker, &entry.Description,
			&entry.DateTime, &reviewed, &banned, &ip, &entry.EndLat, &entry.EndLng,
			&entry.Signal, &entry.RideDateTime, &entry.UserID, &entry.FromWiki, &revisedBy,
		)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		entries = append(entries, entry)
	}

	log.Printf("Found %d Hitchmap entries", len(entries))
	return entries
}

func (d *Daemon) downloadFile(url, filename string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func (d *Daemon) createHitchwikiEvent(entry HitchwikiEntry, lang string) *nostr.Event {
	// Extract article URL from diff link
	articleURL := d.extractArticleURL(entry.Link)

	// Build content with language indicator
	content := fmt.Sprintf("📝 %s edited %s (%s)\n\n📄 %s\n\n#hitchhiking #hitchwiki",
		entry.Author, articleURL, strings.ToUpper(lang), d.cleanSummary(entry.Summary))

	// Get geo info
	geoInfo := d.fetchGeoInfo(articleURL)

	// Create tags
	tags := nostr.Tags{
		{"r", entry.ID},
		{"summary", entry.Summary},
	}

	if geoInfo != nil {
		tags = append(tags,
			nostr.Tag{"g", fmt.Sprintf("%.6f,%.6f", geoInfo.Lat, geoInfo.Lng)},
			nostr.Tag{"L", "open-location-code"},
			nostr.Tag{"l", geoInfo.PlusCode, "open-location-code"},
			nostr.Tag{"g", geoInfo.Geohash},
			nostr.Tag{"t", "hitchwiki"},
		)
	}

	event := &nostr.Event{
		Kind:      nostr.KindTextNote,
		Content:   content,
		Tags:      tags,
		CreatedAt: nostr.Now(),
	}

	return event
}

func (d *Daemon) createHitchmapEvent(entry HitchmapEntry) *nostr.Event {
	hitchhikerName := ""
	if entry.Hitchhiker != nil {
		hitchhikerName = *entry.Hitchhiker
	}
	description := ""
	if entry.Description != nil {
		description = *entry.Description
	}
	content := fmt.Sprintf("hitchmap.com %s: %s", hitchhikerName, description)

	plusCode := olc.Encode(entry.StartLat, entry.StartLng, 10)
	geohash := geohash.Encode(entry.StartLat, entry.StartLng)

	tags := nostr.Tags{
		{"d", fmt.Sprintf("%d", entry.ID)},
		{"L", "open-location-code"},
		{"l", plusCode, "open-location-code"},
		{"L", "open-location-code-prefix"},
		{"l", plusCode[:6] + "00+", plusCode[:4] + "0000+", plusCode[:2] + "000000+", "open-location-code-prefix"},
		{"L", "trustroots-circle"},
		{"l", "hitchhikers", "trustroots-circle"},
		{"g", geohash},
		{"t", "hitchmap"},
		{"t", "map-notes"},
	}

	event := &nostr.Event{
		Kind:      30399, // Custom kind for hitchhiking
		Content:   content,
		Tags:      tags,
		CreatedAt: nostr.Now(),
	}

	return event
}

func (d *Daemon) postEvent(event *nostr.Event, id string) {
	if d.config.DryRun {
		log.Printf("[DRY RUN] Would post: %s", event.Content)
		d.markPosted(id)
		return
	}

	if d.nostr != nil {
		log.Printf("Posting event for ID: %s", id)
		event.Sign(d.nostr.sk)
		d.nostr.publish(event)
		d.markPosted(id)
		log.Printf("Posted: %s", id)
	} else {
		log.Printf("Nostr client is nil, cannot post event for ID: %s", id)
	}
}

func (d *Daemon) isPosted(id string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.posted[id]
}

func (d *Daemon) markPosted(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.posted[id] = true
}

// Helper functions
func decodeNsec(nsec string) (string, error) {
	// Decode bech32 nsec to get the private key bytes
	_, data, err := bech32.Decode(nsec)
	if err != nil {
		return "", fmt.Errorf("failed to decode bech32: %v", err)
	}

	// Convert to hex string
	return hex.EncodeToString(data), nil
}

func extractXML(content, tag string) string {
	re := regexp.MustCompile(fmt.Sprintf(`<%s>(.*?)</%s>`, tag, tag))
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func (d *Daemon) extractArticleURL(diffURL string) string {
	// Extract title from diff URL and create article URL
	// Simplified implementation
	return strings.Replace(diffURL, "&diff=", "", 1)
}

func (d *Daemon) cleanSummary(summary string) string {
	// Remove HTML tags and clean up
	re := regexp.MustCompile(`<[^>]+>`)
	clean := re.ReplaceAllString(summary, "")
	clean = strings.TrimSpace(clean)
	if len(clean) > 300 {
		clean = clean[:300] + "..."
	}
	return clean
}

type GeoInfo struct {
	Lat      float64
	Lng      float64
	PlusCode string
	Geohash  string
}

func (d *Daemon) fetchGeoInfo(url string) *GeoInfo {
	// Simplified geo fetching - in production, implement proper parsing
	return nil
}

// NostrClient methods
func (nc *NostrClient) connectRelays(relayURLs []string) {
	for _, url := range relayURLs {
		if relay, err := nostr.RelayConnect(context.Background(), url); err == nil {
			nc.relays = append(nc.relays, relay)
		}
	}
}

func (nc *NostrClient) publish(event *nostr.Event) {
	for _, relay := range nc.relays {
		relay.Publish(context.Background(), *event)
	}
}
