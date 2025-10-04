package main

import (
	"compress/gzip"
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
	"path/filepath"
	"regexp"
	"strconv"
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
	Nsec               string   `json:"nsec"`
	PostToRelays       bool     `json:"post_to_relays"`
	Relays             []string `json:"relays"`
	HWInterval         int      `json:"hw_interval"`
	HitchInterval      int      `json:"hitch_interval"`
	Debug              bool     `json:"debug"`
	DryRun             bool     `json:"dry_run"`
	SecretHitchwikiURL string   `json:"secret_hitchwiki_url,omitempty"` // Alternative Hitchwiki domain
}

// HTTPCacheEntry represents a cached HTTP response
type HTTPCacheEntry struct {
	Data      []byte
	Timestamp time.Time
}

// Daemon manages both tasks
type Daemon struct {
	config        *Config
	nostr         *NostrClient
	posted        map[string]bool
	existingNotes map[string]bool           // Track existing notes from relays
	httpCache     map[string]HTTPCacheEntry // Cache for HTTP requests
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
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
	var disableDuplicateCheck = flag.Bool("disable-duplicate-check", false, "Disable duplicate checking")
	var forcePost = flag.Bool("force-post", false, "Force post 5 Hitchwiki and 5 Hitchmap notes even if already posted")
	flag.Parse()

	// Setup logging to file
	logFile, err := os.OpenFile("logs/daemon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// Create logs directory if it doesn't exist
		os.MkdirAll("logs", 0755)
		logFile, err = os.OpenFile("logs/daemon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
	}
	defer logFile.Close()

	// Set up logging to both file and stdout
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	config := loadConfig(*configFile)
	config.Debug = *debug
	config.DryRun = *dryRun

	daemon := NewDaemon(config)

	// Skip duplicate checking if disabled
	if *disableDuplicateCheck {
		log.Printf("Duplicate checking disabled")
		daemon.existingNotes = make(map[string]bool) // Clear existing notes
	}

	if *runOnce {
		daemon.runOnce()
		return
	}

	if *forcePost {
		daemon.runForcePost()
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
		config:        config,
		posted:        make(map[string]bool),
		existingNotes: make(map[string]bool),
		httpCache:     make(map[string]HTTPCacheEntry),
		ctx:           ctx,
		cancel:        cancel,
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

			// Fetch existing notes for duplicate checking
			daemon.fetchExistingNotes()

			// Check and update profile with NIP-05 verification
			daemon.ensureProfileExists()
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
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		d.cancel()

		// If we get another signal, force exit
		sig = <-sigChan
		log.Printf("Received second signal %v, force exiting...", sig)
		os.Exit(1)
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

	// Wait for either context cancellation or all tasks to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-d.ctx.Done():
		log.Println("Context cancelled, stopping daemon...")
	case <-done:
		log.Println("All tasks completed")
	}

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

func (d *Daemon) runForcePost() {
	log.Println("Force posting 5 Hitchwiki and 5 Hitchmap notes...")

	// Clear posted tracking to force posting
	d.mu.Lock()
	d.posted = make(map[string]bool)
	d.existingNotes = make(map[string]bool) // Also clear existing notes
	d.mu.Unlock()

	// Process Hitchwiki with limit
	d.processHitchwikiForce(5)

	// Process Hitchmap with limit
	d.processHitchmapForce(5)

	log.Println("Force post completed")
}

func (d *Daemon) runHitchwikiTask() {
	// Process immediately on startup
	d.processHitchwiki()

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
	// Process immediately on startup
	d.processHitchmap()

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

	if d.config.SecretHitchwikiURL != "" {
		log.Printf("Using alternative Hitchwiki URL: %s", d.config.SecretHitchwikiURL)
	}

	// Process multiple languages - comprehensive list of Hitchwiki languages
	languages := []string{
		"en", "de", "es", "fi", "fr", "he", "hr", "it", "lt", "nl", "pl", "pt", "ro", "ru", "tr", "zh",
	}

	for i, lang := range languages {
		log.Printf("Fetching changes for language: %s", lang)
		entries := d.fetchHitchwiki(lang)
		log.Printf("Fetched %d Hitchwiki entries for %s", len(entries), lang)

		// Add delay between language requests to respect rate limits
		if i < len(languages)-1 {
			log.Printf("Waiting 10 seconds before next language...")
			select {
			case <-d.ctx.Done():
				log.Println("Context cancelled, stopping language processing")
				return
			case <-time.After(10 * time.Second):
				// Continue with next language
			}
		}

		// Process all entries (no artificial limit)
		skipped := 0
		for i, entry := range entries {
			if d.isPosted(entry.ID) {
				skipped++
				continue
			}

			log.Printf("Processing entry %d/%d: %s (%s)", i+1, len(entries), entry.Title, lang)
			event := d.createHitchwikiEvent(entry, lang)
			if event != nil {
				d.postEvent(event, entry.ID)
			}
		}

		if skipped > 0 {
			log.Printf("Skipped %d already posted Hitchwiki entries for %s", skipped, lang)
		}
	}
}

func (d *Daemon) processHitchwikiForce(limit int) {
	log.Printf("Force processing Hitchwiki recent changes (limit: %d)...", limit)

	// Process multiple languages
	languages := []string{
		"en", "de", "es", "fi", "fr", "he", "hr", "it", "lt", "nl", "pl", "pt", "ro", "ru", "tr", "zh",
	}

	postedCount := 0
	for _, lang := range languages {
		if postedCount >= limit {
			break
		}

		log.Printf("Fetching changes for language: %s", lang)
		entries := d.fetchHitchwiki(lang)
		log.Printf("Fetched %d Hitchwiki entries for %s", len(entries), lang)

		// Process entries up to limit
		for _, entry := range entries {
			if postedCount >= limit {
				break
			}

			log.Printf("Force posting entry %d: %s (%s)", postedCount+1, entry.Title, lang)
			event := d.createHitchwikiEvent(entry, lang)
			if event != nil {
				d.postEvent(event, entry.ID)
				postedCount++
			}
		}
	}

	log.Printf("Force posted %d Hitchwiki entries", postedCount)
}

func (d *Daemon) processHitchmapForce(limit int) {
	log.Printf("Force processing Hitchmap data (limit: %d)...", limit)

	entries := d.fetchHitchmap()
	log.Printf("Fetched %d Hitchmap entries", len(entries))

	postedCount := 0
	for _, entry := range entries {
		if postedCount >= limit {
			break
		}

		log.Printf("Force posting hitchmap entry %d: %d", postedCount+1, entry.ID)
		event := d.createHitchmapEvent(entry)
		if event != nil {
			d.postEvent(event, fmt.Sprintf("hitchmap_%d", entry.ID))
			postedCount++
		}
	}

	log.Printf("Force posted %d Hitchmap entries", postedCount)
}

func (d *Daemon) processHitchmap() {
	log.Println("Processing Hitchmap data...")

	entries := d.fetchHitchmap()
	log.Printf("Fetched %d Hitchmap entries", len(entries))

	skipped := 0
	for _, entry := range entries {
		if d.isPosted(fmt.Sprintf("hitchmap_%d", entry.ID)) {
			skipped++
			continue
		}

		log.Printf("Processing hitchmap entry: %d", entry.ID)
		event := d.createHitchmapEvent(entry)
		if event != nil {
			d.postEvent(event, fmt.Sprintf("hitchmap_%d", entry.ID))
		}
	}

	if skipped > 0 {
		log.Printf("Skipped %d already posted Hitchmap entries", skipped)
	}
}

// cachedHTTPGet performs HTTP GET with 1-minute caching
func (d *Daemon) cachedHTTPGet(url string) ([]byte, error) {
	d.mu.RLock()
	if entry, exists := d.httpCache[url]; exists {
		if time.Since(entry.Timestamp) < time.Minute {
			d.mu.RUnlock()
			return entry.Data, nil
		}
	}
	d.mu.RUnlock()

	// Add delay before each request to respect rate limits
	select {
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	case <-time.After(5 * time.Second):
		// Continue with request
	}

	// Create HTTP client with comprehensive headers to bypass bot detection
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableCompression: false, // Allow gzip compression
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set comprehensive headers to mimic a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Cache-Control", "max-age=0")

	// Retry logic for failed requests
	var resp *http.Response
	var body []byte
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = client.Do(req)
		if err != nil {
			log.Printf("HTTP request attempt %d failed for URL %s: %v", attempt+1, url, err)
			if attempt < maxRetries-1 {
				select {
				case <-d.ctx.Done():
					return nil, d.ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 5 * time.Second):
					// Continue with retry
				}
				continue
			}
			return nil, err
		}

		// Handle gzipped responses
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				log.Printf("Error creating gzip reader: %v", err)
				resp.Body.Close()
				if attempt < maxRetries-1 {
					select {
					case <-d.ctx.Done():
						return nil, d.ctx.Err()
					case <-time.After(time.Duration(attempt+1) * 5 * time.Second):
						// Continue with retry
					}
					continue
				}
				return nil, err
			}
			defer gzReader.Close()
			reader = gzReader
		}

		body, err = io.ReadAll(reader)
		resp.Body.Close()

		if err != nil {
			log.Printf("Reading response attempt %d failed for URL %s: %v", attempt+1, url, err)
			if attempt < maxRetries-1 {
				select {
				case <-d.ctx.Done():
					return nil, d.ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 5 * time.Second):
					// Continue with retry
				}
				continue
			}
			return nil, err
		}

		// Check if we got a valid response (not Cloudflare challenge)
		if resp.StatusCode == 200 && !strings.Contains(string(body), "Just a moment") {
			break
		}

		log.Printf("Got %d status or Cloudflare challenge on attempt %d for URL %s, retrying...", resp.StatusCode, attempt+1, url)
		if attempt < maxRetries-1 {
			select {
			case <-d.ctx.Done():
				return nil, d.ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 10 * time.Second):
				// Continue with retry
			}
		}
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP request failed with status %d after %d attempts", resp.StatusCode, maxRetries)
	}

	// Cache the response
	d.mu.Lock()
	d.httpCache[url] = HTTPCacheEntry{
		Data:      body,
		Timestamp: time.Now(),
	}
	d.mu.Unlock()

	return body, nil
}

func (d *Daemon) fetchHitchwiki(lang string) []HitchwikiEntry {
	// Use alternative Hitchwiki URL if configured, otherwise use default
	var baseURL string
	if d.config.SecretHitchwikiURL != "" {
		baseURL = d.config.SecretHitchwikiURL
		log.Printf("Using alternative Hitchwiki URL: %s", baseURL)
	} else {
		baseURL = "https://hitchwiki.org"
	}
	url := fmt.Sprintf("%s/%s/api.php?hidebots=1&urlversion=1&days=7&limit=50&action=feedrecentchanges&feedformat=atom", baseURL, lang)

	log.Printf("Fetching from: %s", url)

	data, err := d.cachedHTTPGet(url)
	if err != nil {
		log.Printf("Error fetching Hitchwiki: %v", err)
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
		if link := extractXML(match, "link href"); link != "" {
			entry.Link = link
		}
		if id := extractXML(match, "id"); id != "" {
			entry.ID = id
		}
		// Extract author from nested structure <author><name>AuthorName</name></author>
		authorMatch := regexp.MustCompile(`<author>.*?<name>(.*?)</name>.*?</author>`)
		if authorMatches := authorMatch.FindStringSubmatch(match); len(authorMatches) > 1 {
			entry.Author = authorMatches[1]
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

	// Clean up old hitchmap dumps (keep only the latest)
	d.cleanupOldHitchmapDumps(filename)

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

func (d *Daemon) cleanupOldHitchmapDumps(currentFile string) {
	// Get the directory of the current file
	dir := filepath.Dir(currentFile)

	// Read all files in the hitchmap-dumps directory
	files, err := filepath.Glob(filepath.Join(dir, "hitchmap_*.sqlite"))
	if err != nil {
		log.Printf("Error reading hitchmap dumps directory: %v", err)
		return
	}

	// If we have more than 1 file, keep only the current one
	if len(files) > 1 {
		for _, file := range files {
			if file != currentFile {
				log.Printf("Removing old hitchmap dump: %s", file)
				if err := os.Remove(file); err != nil {
					log.Printf("Error removing old dump %s: %v", file, err)
				}
			}
		}
		log.Printf("Cleaned up old hitchmap dumps, kept: %s", currentFile)
	}
}

func (d *Daemon) downloadFile(url, filename string) error {
	data, err := d.cachedHTTPGet(url)
	if err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

func (d *Daemon) createHitchwikiEvent(entry HitchwikiEntry, lang string) *nostr.Event {
	// Extract article URL from diff link
	articleURL := d.extractArticleURL(entry.Link)
	log.Printf("Original link: %s", entry.Link)
	log.Printf("Extracted article URL: %s", articleURL)

	// Build content in the exact format specified
	// Format: "📝 Author edited URL 📄 #hitchhiking"

	authorClean := ""
	if entry.Author != "" {
		authorClean = d.cleanAuthor(entry.Author)
	}

	var content string
	if articleURL != "" && authorClean != "" {
		content = fmt.Sprintf("📝 %s edited %s 📄 #hitchhiking", authorClean, articleURL)
	} else if articleURL != "" {
		content = fmt.Sprintf("📝 edited %s 📄 #hitchhiking", articleURL)
	} else {
		content = fmt.Sprintf("📝 %s 📄 #hitchhiking", entry.Title)
	}

	// Get geo info (pass original link)
	geoInfo := d.fetchGeoInfo(entry.Link)
	if geoInfo != nil {
		log.Printf("Found geo info: lat=%.6f, lng=%.6f, pluscode=%s, geohash=%s",
			geoInfo.Lat, geoInfo.Lng, geoInfo.PlusCode, geoInfo.Geohash)
	} else {
		log.Printf("No geo info found for %s", articleURL)
	}

	// Ensure there's always a summary
	summary := entry.Summary
	if summary == "" {
		// Create a summary from available data
		if entry.Author != "" {
			summary = fmt.Sprintf("Hitchwiki article '%s' was edited by %s", entry.Title, entry.Author)
		} else {
			summary = fmt.Sprintf("Hitchwiki article '%s' was edited", entry.Title)
		}
	}

	// Truncate summary to 160 characters if too long
	if len(summary) > 160 {
		summary = summary[:160] + "..."
	}

	// Create tags
	tags := nostr.Tags{
		{"r", entry.Link}, // Use full diff URL instead of RSS ID
		{"summary", summary},
	}

	// Always add hitchhiking and hitchwiki tags
	tags = append(tags,
		nostr.Tag{"t", "hitchhiking"},
		nostr.Tag{"t", "hitchwiki"},
	)

	if geoInfo != nil {
		tags = append(tags,
			nostr.Tag{"g", fmt.Sprintf("%.6f,%.6f", geoInfo.Lat, geoInfo.Lng)},
			nostr.Tag{"L", "open-location-code"},
			nostr.Tag{"l", geoInfo.PlusCode, "open-location-code"},
			nostr.Tag{"g", geoInfo.Geohash},
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
	content := fmt.Sprintf("hitchmap.com %s: %s #hitchhiking", hitchhikerName, description)

	plusCode := olc.Encode(entry.StartLat, entry.StartLng, 10)
	geohash := geohash.Encode(entry.StartLat, entry.StartLng)

	tags := nostr.Tags{
		{"d", fmt.Sprintf("%d", entry.ID)},
		{"g", fmt.Sprintf("%.6f,%.6f", entry.StartLat, entry.StartLng)}, // lat,lng coordinates
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
		Kind:      nostr.KindTextNote, // Use kind 1 for text notes
		Content:   content,
		Tags:      tags,
		CreatedAt: nostr.Now(),
	}

	return event
}

func (d *Daemon) postEvent(event *nostr.Event, id string) {
	if d.config.DryRun {
		log.Printf("[DRY RUN] Would post: %s", event.Content)
		log.Printf("[DRY RUN] Tags: %v", event.Tags)
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
	// Check both session-posted and existing notes from relays
	return d.posted[id] || d.existingNotes[id]
}

func (d *Daemon) markPosted(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.posted[id] = true
}

func (d *Daemon) fetchExistingNotes() {
	if d.nostr == nil {
		log.Printf("No Nostr client available for fetching existing notes")
		return
	}

	log.Printf("Fetching existing notes from relays for duplicate checking...")

	// Create filter for our pubkey's text notes
	filters := []nostr.Filter{{
		Authors: []string{d.nostr.pk},
		Kinds:   []int{int(nostr.KindTextNote), 30399}, // Text notes and hitchmap notes
		Limit:   1000,                                  // Get more notes to check against
	}}

	// Query all relays
	for _, relay := range d.nostr.relays {
		events, err := relay.QuerySync(context.Background(), filters[0])
		if err != nil {
			log.Printf("Error querying relay %s: %v", relay.URL, err)
			continue
		}

		log.Printf("Found %d existing events from relay %s", len(events), relay.URL)

		// Process events to extract IDs
		for _, event := range events {
			// Check for Hitchwiki entries (r tag with hitchwiki URL)
			for _, tag := range event.Tags {
				if len(tag) >= 2 && tag[0] == "r" {
					if strings.Contains(tag[1], "hitchwiki.org") {
						d.mu.Lock()
						d.existingNotes[tag[1]] = true
						d.mu.Unlock()
						break
					}
				}
			}

			// Check for Hitchmap entries (look for hitchmap tag and d tag)
			hasHitchmapTag := false
			hitchmapID := ""
			for _, tag := range event.Tags {
				if len(tag) >= 2 && tag[0] == "t" && tag[1] == "hitchmap" {
					hasHitchmapTag = true
				}
				if len(tag) >= 2 && tag[0] == "d" {
					hitchmapID = tag[1]
				}
			}
			if hasHitchmapTag && hitchmapID != "" {
				d.mu.Lock()
				d.existingNotes[fmt.Sprintf("hitchmap_%s", hitchmapID)] = true
				d.mu.Unlock()
			}
		}
	}

	log.Printf("Found %d existing notes for duplicate checking", len(d.existingNotes))
}

func (d *Daemon) ensureProfileExists() {
	if d.nostr == nil {
		log.Printf("No Nostr client available for profile check")
		return
	}

	log.Printf("Checking for existing profile...")

	// Query for existing profile events (kind 0)
	filters := []nostr.Filter{{
		Authors: []string{d.nostr.pk},
		Kinds:   []int{int(nostr.KindProfileMetadata)},
		Limit:   1,
	}}

	// Check all relays for existing profile
	var existingProfile *nostr.Event
	for _, relay := range d.nostr.relays {
		events, err := relay.QuerySync(context.Background(), filters[0])
		if err != nil {
			log.Printf("Error querying relay %s for profile: %v", relay.URL, err)
			continue
		}
		if len(events) > 0 {
			existingProfile = events[0]
			log.Printf("Found existing profile on relay %s", relay.URL)
			break
		}
	}

	// Check if existing profile has correct NIP-05, name, website, and picture
	if existingProfile != nil {
		// Parse the profile content
		var profile map[string]interface{}
		if err := json.Unmarshal([]byte(existingProfile.Content), &profile); err == nil {
			nip05, _ := profile["nip05"].(string)
			name, _ := profile["name"].(string)
			website, _ := profile["website"].(string)
			picture, _ := profile["picture"].(string)

			if nip05 == "nostrhitch@hitchwiki.org" && name == "nostrhitchbot" && website == "https://hitchwiki.org/en/Hitchwiki:Nostrhitch" && picture == "https://hitchwiki.org/en/images/en/c/c1/Nostrhitch.jpg" {
				log.Printf("Profile already exists with correct NIP-05, name, website, and picture: %s", name)
				return
			}

			log.Printf("Profile needs update - NIP-05: %s, Name: %s, Website: %s, Picture: %s", nip05, name, website, picture)
		}
		log.Printf("Existing profile found but needs updating...")
	}

	// Create or update profile
	d.createProfile()
}

func (d *Daemon) createProfile() {
	log.Printf("Creating/updating profile with NIP-05 verification...")
	// Note: Profile updates always happen regardless of dry-run mode

	// Create profile data
	profile := map[string]interface{}{
		"name":            "nostrhitchbot",
		"about":           "Bot that posts Hitchwiki and Hitchmap updates to Nostr. Follows recent changes from hitchwiki.org and hitchmap.com data.",
		"website":         "https://hitchwiki.org/en/Hitchwiki:Nostrhitch",
		"picture":         "https://hitchwiki.org/en/images/en/c/c1/Nostrhitch.jpg",
		"nip05":           "nostrhitch@hitchwiki.org",
		"lud16":           "nostrhitch@hitchwiki.org", // Lightning address (same as nip05)
		"bot":             true,
		"bot_description": "Posts Hitchwiki recent changes and Hitchmap data to Nostr relays",
	}

	profileJSON, err := json.Marshal(profile)
	if err != nil {
		log.Printf("Error marshaling profile: %v", err)
		return
	}

	// Create the profile event
	event := &nostr.Event{
		Kind:      nostr.KindProfileMetadata,
		Content:   string(profileJSON),
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"nip05", "nostrhitch@hitchwiki.org"}},
	}

	// Sign the event
	event.Sign(d.nostr.sk)

	// Publish to all relays
	successCount := 0
	for _, relay := range d.nostr.relays {
		err := relay.Publish(context.Background(), *event)
		if err != nil {
			log.Printf("Error publishing profile to relay %s: %v", relay.URL, err)
		} else {
			successCount++
			log.Printf("Profile published to relay %s", relay.URL)
		}
	}

	if successCount > 0 {
		log.Printf("Profile created/updated successfully! Published to %d/%d relays", successCount, len(d.nostr.relays))
		log.Printf("Profile content: %s", string(profileJSON))
		log.Printf("NIP-05 verification: nostrhitch@hitchwiki.org")
	} else {
		log.Printf("Failed to publish profile to any relay")
	}
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
	// Handle both content and attributes
	if strings.Contains(tag, " ") {
		// Handle attributes like "link href"
		parts := strings.Split(tag, " ")
		tagName := parts[0]
		attrName := parts[1]

		// Look for <tagName ... attrName="value" ...>
		re := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*%s="([^"]*)"`, tagName, attrName))
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1]
		}
	} else {
		// Handle content like <tag>content</tag>
		re := regexp.MustCompile(fmt.Sprintf(`<%s>(.*?)</%s>`, tag, tag))
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

func (d *Daemon) extractArticleURL(diffURL string) string {
	// Extract title from diff URL and preserve the language
	// Example: https://hitchwiki.org/ru/index.php?title=Куба&diff=114734&oldid=109364
	// Should become: https://hitchwiki.org/ru/Куба

	// Parse URL to extract language and title
	if diffURL == "" {
		return ""
	}

	// Extract language from URL (e.g., /ru/ from https://hitchwiki.org/ru/index.php)
	langStart := strings.Index(diffURL, "hitchwiki.org/")
	if langStart == -1 {
		return ""
	}
	langStart += 14 // len("hitchwiki.org/")

	langEnd := strings.Index(diffURL[langStart:], "/")
	if langEnd == -1 {
		return ""
	}
	langEnd += langStart

	language := diffURL[langStart:langEnd]

	// Find title parameter
	titleStart := strings.Index(diffURL, "title=")
	if titleStart == -1 {
		return ""
	}
	titleStart += 6 // len("title=")

	// Find the end of the title (before &diff= or end of string)
	titleEnd := strings.Index(diffURL[titleStart:], "&")
	if titleEnd == -1 {
		titleEnd = len(diffURL)
	} else {
		titleEnd += titleStart
	}

	title := diffURL[titleStart:titleEnd]

	// Create the article URL with the correct language
	return fmt.Sprintf("https://hitchwiki.org/%s/%s", language, title)
}

func (d *Daemon) cleanAuthor(author string) string {
	// Clean author name
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]+>`)
	clean := re.ReplaceAllString(author, "")

	// Remove special characters except word chars, spaces, and hyphens
	re = regexp.MustCompile(`[^\w\s-]`)
	clean = re.ReplaceAllString(clean, "")

	return strings.TrimSpace(clean)
}

func (d *Daemon) cleanSummary(summary string) string {
	// Clean up HTML and extract meaningful content
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]+>`)
	clean := re.ReplaceAllString(summary, "")

	// Normalize whitespace
	re = regexp.MustCompile(`\s+`)
	clean = re.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)

	// Try to extract just the meaningful part before the diff table
	if strings.Contains(clean, "Revision as of") {
		parts := strings.Split(clean, "Revision as of")
		clean = strings.TrimSpace(parts[0])
	}

	// Truncate if still too long
	if len(clean) > 300 {
		clean = clean[:300] + "..."
	}

	return clean
}

func (d *Daemon) normalizeWhitespace(text string) string {
	// Normalize whitespace but preserve double newlines
	// First, normalize spaces within each line
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		re := regexp.MustCompile(`\s+`)
		lines[i] = strings.TrimSpace(re.ReplaceAllString(line, " "))
	}
	return strings.Join(lines, "\n")
}

type GeoInfo struct {
	Lat      float64
	Lng      float64
	PlusCode string
	Geohash  string
}

func (d *Daemon) fetchGeoInfo(originalLink string) *GeoInfo {
	// First extract article URL from the original link
	articleURL := d.extractArticleURL(originalLink)
	if articleURL == "" {
		return nil
	}

	// Convert hitchwiki.org URL to secret URL if configured
	fetchURL := articleURL
	if d.config.SecretHitchwikiURL != "" {
		// Replace hitchwiki.org with secret URL for fetching
		fetchURL = strings.Replace(articleURL, "https://hitchwiki.org", d.config.SecretHitchwikiURL, 1)
		log.Printf("Using secret URL for geo fetch: %s", fetchURL)
	}

	// Fetch the actual article page using cached HTTP get
	body, err := d.cachedHTTPGet(fetchURL)
	if err != nil {
		log.Printf("Error fetching page for geo info: %v", err)
		return nil
	}

	content := string(body)

	// Use patterns to match latitude and longitude coordinates
	mapPatterns := []string{
		`<map[^>]*lat=['"]([0-9.-]+)['"][^>]*lng=['"]([0-9.-]+)['"]`,
		`&lt;map[^>]*lat=['"]([0-9.-]+)['"][^>]*lng=['"]([0-9.-]+)['"]`, // HTML-encoded
		`<div[^>]*class="[^"]*map[^"]*"[^>]*data-lat="([^"]+)"[^>]*data-lng="([^"]+)"`,
		`\|map\s*=\s*<map\s+lat="([^"]+)"\s+lng="([^"]+)"`, // Fallback for raw format
		`&lt;map lat='([0-9.-]+)' lng='([0-9.-]+)'`,        // HTML-encoded with single quotes
		`<map lat='([0-9.-]+)' lng='([0-9.-]+)'`,           // Direct format with single quotes
	}

	for _, pattern := range mapPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) == 3 {
			if lat, err := strconv.ParseFloat(matches[1], 64); err == nil {
				if lng, err := strconv.ParseFloat(matches[2], 64); err == nil {
					plusCode := olc.Encode(lat, lng, 10)
					geohash := geohash.Encode(lat, lng)
					return &GeoInfo{
						Lat:      lat,
						Lng:      lng,
						PlusCode: plusCode,
						Geohash:  geohash,
					}
				}
			}
		}
	}

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
