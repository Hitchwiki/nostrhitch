import requests
import xml.etree.ElementTree as ET
import json
import time
import argparse
import urllib.parse
import re
import ssl
import uuid
from openlocationcode import openlocationcode
import geohash2

from pynostr.key import PrivateKey
from pynostr.relay_manager import RelayManager
from pynostr.event import Event, EventKind
from pynostr.filters import Filters, FiltersList

# Import configuration from settings
from settings import nsec, post_to_relays, relays

# Atom feed URL for recent changes
ATOM_URL = ("https://hitchwiki.org/en/api.php?"
            "hidebots=1&urlversion=1&days=90&limit=500"
            "&action=feedrecentchanges&feedformat=atom")

# -------------------------------------------------
# Fetch geo information from Hitchwiki articles

def fetch_geo_info(article_url, debug=False):
    """Fetch geo information from a Hitchwiki article."""
    try:
        # Convert article URL to raw format
        if "index.php" in article_url:
            # Extract title from URL parameters
            parsed_url = urllib.parse.urlparse(article_url)
            query_params = urllib.parse.parse_qs(parsed_url.query)
            if "title" in query_params:
                title = query_params["title"][0]
                raw_url = f"https://hitchwiki.org/en/index.php?title={title}&action=raw"
            else:
                return None
        else:
            return None
        
        if not raw_url:
            return None
            
        # Fetch raw content
        response = requests.get(raw_url, timeout=10)
        if response.status_code != 200:
            return None
            
        content = response.text
        
        # Parse map tag for geo coordinates
        # Look for pattern: |map = <map lat="48.864716" lng="2.349014" zoom="9" view="3"/>
        map_pattern = r'\|map\s*=\s*<map\s+lat="([^"]+)"\s+lng="([^"]+)"'
        match = re.search(map_pattern, content)
        
        if match:
            lat = float(match.group(1))
            lng = float(match.group(2))
            
            # Generate geo codes like in nostrhitch.py
            pluscode = openlocationcode.encode(lat, lng)
            geohash = geohash2.encode(lat, lng)
            
            return {
                "lat": lat,
                "lng": lng,
                "pluscode": pluscode,
                "geohash": geohash
            }
        
        return None
        
    except Exception as e:
        if debug:
            print(f"DEBUG: Error fetching geo info for {article_url}: {e}")
        return None

# -------------------------------------------------
# Fetch recent changes from Hitchwiki Atom feed

def fetch_atom(url: str) -> list[dict]:
    resp = requests.get(url, timeout=10)
    resp.raise_for_status()
    root = ET.fromstring(resp.content)

    ns = {"atom": "http://www.w3.org/2005/Atom"}
    entries = []
    for entry in root.findall("atom:entry", ns):
        title = entry.findtext("atom:title", default="", namespaces=ns)
        link = entry.find("atom:link", ns).attrib.get("href", "")
        updated = entry.findtext("atom:updated", default="", namespaces=ns)
        summary = entry.findtext("atom:summary", default="", namespaces=ns)
        rss_id = entry.findtext("atom:id", default="", namespaces=ns)
        
        # Extract author name
        author = ""
        author_elem = entry.find("atom:author", ns)
        if author_elem is not None:
            author = author_elem.findtext("atom:name", default="", namespaces=ns)

        entries.append({
            "title": title,
            "link": link,
            "updated": updated,
            "summary": summary,
            "rss_id": rss_id,
            "author": author,
        })
    return entries

# -------------------------------------------------
# Build a Nostr note (kind 1) from an entry

def entry_to_nostr(entry: dict, debug=False) -> tuple[Event, dict]:
    # Extract article title from the diff link to create the article URL
    # The link format is: https://hitchwiki.org/en/index.php?title=Edinburgh&diff=113427&oldid=112470
    # We want: https://hitchwiki.org/en/Edinburgh
    article_url = ""
    if entry["link"]:
        # Extract title parameter from the link
        parsed = urllib.parse.urlparse(entry["link"])
        query_params = urllib.parse.parse_qs(parsed.query)
        if "title" in query_params:
            title = query_params["title"][0]
            article_url = f"https://hitchwiki.org/en/{title}"
    
    # Build content with article link, author, and summary
    content_parts = []
    
    # Add article link with author info
    if article_url and entry["author"]:
        # Clean author name
        author_clean = re.sub(r'<[^>]+>', '', entry["author"])  # Remove HTML tags
        author_clean = re.sub(r'[^\w\s-]', '', author_clean)  # Remove special characters except word chars, spaces, and hyphens
        author_clean = author_clean.strip()
        if author_clean:  # Only add if we have a clean author name
            content_parts.append(f"📝 {author_clean} edited {article_url}")
        else:
            content_parts.append(f"📝 {article_url}")
    elif article_url:
        content_parts.append(f"📝 {article_url}")
    else:
        content_parts.append(f"📝 {entry['title']}")
    
    # Add summary (truncated if too long)
    if entry["summary"]:
        # Clean up HTML and extract meaningful content
        summary_clean = re.sub(r'<[^>]+>', '', entry["summary"])
        summary_clean = re.sub(r'\s+', ' ', summary_clean)  # Normalize whitespace
        summary_clean = summary_clean.strip()
        
        # Try to extract just the meaningful part before the diff table
        if "Revision as of" in summary_clean:
            summary_clean = summary_clean.split("Revision as of")[0].strip()
        
        # Truncate if still too long
        if len(summary_clean) > 300:
            summary_clean = summary_clean[:300] + "..."
            
        if summary_clean:
            content_parts.append(f"📄 {summary_clean}")
    
    # Add hashtag
    content_parts.append("#hitchhiking")
    
    content = "\n\n".join(content_parts)
    
    # Ensure URLs are properly spaced and clean
    # Add space after URLs to prevent them from running into other text
    # Clean up any extra spaces
    content = re.sub(r"\s+", " ", content)
    content = content.strip()
    
    # Fetch geo information if we have an article URL
    geo_info = None
    if article_url:
        geo_info = fetch_geo_info(entry["link"], debug=debug)  # Use original link for raw content
        if geo_info and debug:
            print(f"DEBUG: Found geo info: lat={geo_info['lat']}, lng={geo_info['lng']}, geohash={geo_info['geohash']}")

    # Include RSS ID, summary, and geo information as tags
    tags = []
    if entry["rss_id"]:
        tags.append(["r", entry["rss_id"]])  # 'r' tag for RSS ID (NIP-01: basic protocol)
    if entry["summary"]:
        # Custom 'summary' tag - not defined in official NIPs but commonly used
        # Stores the full HTML summary from the RSS feed for reference
        tags.append(["summary", entry["summary"]])
    
    # Add geo tags if available (following nostrhitch.py pattern)
    if geo_info:
        # Geohash tag (NIP-52: Geohash)
        tags.append(["g", geo_info["geohash"]])
        
        # Open Location Code tags (NIP-52: Geohash)
        tags.append(["L", "open-location-code"])
        tags.append(["l", geo_info["pluscode"], "open-location-code"])
        
        # Open Location Code prefixes
        tags.append(["L", "open-location-code-prefix"])
        tags.append(["l", geo_info["pluscode"][:6] + "00+", 
                    geo_info["pluscode"][:4] + "0000+", 
                    geo_info["pluscode"][:2] + "000000+", "open-location-code-prefix"])
        
        # Trustroots circle tags (like in nostrhitch.py)
        tags.append(["L", "trustroots-circle"])
        tags.append(["l", "hitchhikers", "trustroots-circle"])
        
        # Hitchwiki tag
        tags.append(["t", "hitchwiki"])

    event = Event(
        kind=EventKind.TEXT_NOTE,
        content=content,
        tags=tags
    )
    return event, geo_info

# -------------------------------------------------
# NostrPost class for handling posting to relays

class NostrPost:
    def __init__(self, debug=False, enable_duplicate_check=True):
        private_key_obj = PrivateKey.from_nsec(nsec)
        self.private_key_hex = private_key_obj.hex()
        self.pubkey = private_key_obj.public_key.hex()
        npub = private_key_obj.public_key.bech32()
        print(f"Posting as npub {npub}")
        
        self.debug = debug
        self.enable_duplicate_check = enable_duplicate_check
        self.posted_rss_ids = set()  # Track posted RSS IDs in this session
        self.existing_notes = {}  # Store existing notes for duplicate checking

        # Initialize the relay manager (following nostrhitch.py pattern)
        self.relay_manager = RelayManager(timeout=5)
        for relay in relays:
            self.relay_manager.add_relay(relay)
            if self.debug:
                print(f"DEBUG: Added relay: {relay}")
        
        if self.debug:
            print(f"DEBUG: Total relays configured: {len(self.relay_manager.relays)}")
            print(f"DEBUG: Relay URLs: {[str(relay) for relay in self.relay_manager.relays]}")
        
        # Fetch existing notes for duplicate checking
        if self.enable_duplicate_check:
            self.fetch_existing_notes()

    def fetch_existing_notes(self):
        """Fetch all existing notes from the relay for duplicate checking."""
        print("🔍 Fetching existing notes from relay for duplicate checking...")
        
        try:
            # Create filter for our pubkey's text notes
            filters = FiltersList([Filters(
                authors=[self.pubkey],
                kinds=[EventKind.TEXT_NOTE],
                limit=100
            )])
            
            if self.debug:
                print(f"DEBUG: Filter created: authors={[self.pubkey]}, kinds={[EventKind.TEXT_NOTE]}, limit=100")
            
            # Generate unique subscription ID
            subscription_id = uuid.uuid1().hex
            
            # Add subscription to all relays
            if self.debug:
                print(f"DEBUG: Adding subscription with ID: {subscription_id}")
            self.relay_manager.add_subscription_on_all_relays(subscription_id, filters)
            
            # Run relay manager to establish connections and fetch events
            if self.debug:
                print("DEBUG: Running relay manager...")
            self.relay_manager.run_sync()
            
            # Allow time for messages to be received
            if self.debug:
                print("DEBUG: Waiting for events...")
            time.sleep(2)
            
            # Process received events
            rss_notes = {}
            total_events = 0
            
            while self.relay_manager.message_pool.has_events():
                event_msg = self.relay_manager.message_pool.get_event()
                total_events += 1
                
                if hasattr(event_msg, 'event') and event_msg.event:
                    event = event_msg.event
                    
                    # Check if this event has RSS tags
                    for tag in event.tags:
                        if len(tag) >= 2 and tag[0] == 'r':
                            rss_id = tag[1]
                            rss_notes[rss_id] = event
                            if self.debug:
                                print(f"DEBUG: Found existing note with RSS ID: {rss_id}")
                            break
            
            self.existing_notes = rss_notes
            print(f"✅ Fetched {total_events} total events from relay")
            print(f"📊 Found {len(self.existing_notes)} notes with RSS IDs for duplicate checking")
            
            if self.debug and self.existing_notes:
                print("DEBUG: Sample RSS IDs found:")
                for i, rss_id in enumerate(list(self.existing_notes.keys())[:3]):
                    print(f"  {i+1}. {rss_id}")
                
        except Exception as e:
            print(f"⚠️  Could not fetch existing notes: {e}")
            if self.debug:
                import traceback
                print(f"DEBUG: Error details: {traceback.format_exc()}")
            print("🔄 Falling back to session-based duplicate checking")
            self.existing_notes = {}

    def is_already_posted(self, rss_id: str) -> bool:
        """Check if an RSS item has already been posted by checking fetched notes."""
        if not self.enable_duplicate_check:
            if self.debug:
                print("DEBUG: Duplicate checking disabled")
            return False
            
        if self.debug:
            print(f"DEBUG: Checking if RSS ID {rss_id} already posted...")
        
        # Check if we've already posted this RSS ID in this session
        if rss_id in self.posted_rss_ids:
            if self.debug:
                print(f"DEBUG: RSS ID {rss_id} already posted in this session")
            return True
        
        # Check if this RSS ID exists in our fetched notes
        if rss_id in self.existing_notes:
            if self.debug:
                print(f"DEBUG: RSS ID {rss_id} found in existing notes from relay")
            return True
        
        if self.debug:
            print(f"DEBUG: RSS ID {rss_id} not found in existing notes")
        return False

    def post(self, event: Event, rss_id: str):
        # Check if already posted
        if self.is_already_posted(rss_id):
            if self.debug:
                print(f"DEBUG: RSS item {rss_id} already posted, skipping")
            print(f"RSS item {rss_id} already posted, skipping")
            return False
            
        if self.debug:
            print(f"DEBUG: Signing event for RSS ID: {rss_id}")
        event.sign(self.private_key_hex)
        
        if post_to_relays:
            print("Posting to relays")
            if self.debug:
                print(f"DEBUG: Publishing event to {len(self.relay_manager.relays)} relays")
                print(f"DEBUG: Relay URLs: {[str(relay) for relay in self.relay_manager.relays]}")
                print(f"DEBUG: Event ID: {event.id}")
                print(f"DEBUG: Event signature: {event.sig}")
            
            try:
                self.relay_manager.publish_event(event)
                self.relay_manager.run_sync()  # Sync with the relay to send the event
                print("✅ Event published successfully")
                if self.debug:
                    print("DEBUG: Sleeping for 3 seconds...")
                time.sleep(3)
                # Track this RSS ID as posted
                self.posted_rss_ids.add(rss_id)
                return True
            except Exception as e:
                print(f"❌ Failed to publish event: {e}")
                if self.debug:
                    import traceback
                    print(f"DEBUG: Publish error details: {traceback.format_exc()}")
                return False
        else:
            print("Dry run - would post:", event.content)
            # Even in dry run, track the RSS ID to prevent duplicates in the same session
            self.posted_rss_ids.add(rss_id)
            return False

    def sync_and_close(self):
        """Close connections after posting events."""
        if self.debug:
            print("DEBUG: Closing relay connections...")
        self.close()
        if self.debug:
            print("DEBUG: All connections closed")

    def close(self):
        self.relay_manager.close_all_relay_connections()

# -------------------------------------------------
# Main workflow

def main():
    parser = argparse.ArgumentParser(description="Publish Hitchwiki recent changes to Nostr")
    parser.add_argument("--dry-run", action="store_true", 
                       help="Show what notes will look like without posting them")
    parser.add_argument("--limit", type=int, default=None,
                       help="Limit number of entries to process")
    parser.add_argument("--debug", action="store_true",
                       help="Enable debug logging for troubleshooting")
    parser.add_argument("--disable-duplicate-check", action="store_true",
                        help="Disable duplicate checking (enabled by default)")
    parser.add_argument("--geo-only", action="store_true",
                        help="Only post notes that have geo information (coordinates)")
    args = parser.parse_args()
    
    # Initialize NostrPost
    if args.debug:
        print("DEBUG: Initializing NostrPost...")
    enable_duplicate_check = not args.disable_duplicate_check
    if args.debug:
        print(f"DEBUG: Duplicate checking enabled: {enable_duplicate_check}")
    np = NostrPost(debug=args.debug, enable_duplicate_check=enable_duplicate_check)
    
    # Override post_to_relays if dry-run is specified
    if args.dry_run:
        global post_to_relays
        post_to_relays = False
        if args.debug:
            print("DEBUG: Dry run mode enabled, post_to_relays set to False")
    
    if args.debug:
        print(f"DEBUG: Fetching RSS feed from {ATOM_URL}")
    entries = fetch_atom(ATOM_URL)
    if args.debug:
        print(f"DEBUG: Fetched {len(entries)} entries from RSS feed")
    
    if args.limit:
        entries = entries[:args.limit]
        if args.debug:
            print(f"DEBUG: Limited to {len(entries)} entries")

    if args.debug:
        print(f"DEBUG: Processing {len(entries)} entries...")
    
    for i, entry in enumerate(entries, 1):
        if args.debug:
            print(f"DEBUG: Processing entry {i}/{len(entries)}: {entry['title']}")
        
        nostr_event, geo_info = entry_to_nostr(entry, debug=args.debug)
        rss_id = entry.get('rss_id', '')
        
        if args.debug:
            print(f"DEBUG: RSS ID: {rss_id}")
            print(f"DEBUG: Author: {entry.get('author', 'N/A')}")
        
        # Check if geo-only filtering is enabled
        if args.geo_only and not geo_info:
            if args.debug:
                print(f"DEBUG: Skipping {entry['title']} - no geo information found")
            print(f"⏭️  Skipped (no geo data): {entry['title']}")
            continue
        
        # Always show note preview
        print(f"\n{'='*60}")
        print(f"📝 NOTE PREVIEW - {entry['title']}")
        print(f"{'='*60}")
        print(f"🔗 RSS ID: {rss_id}")
        print(f"👤 Author: {entry.get('author', 'N/A')}")
        print(f"📅 Created: {nostr_event.created_at}")
        print(f"🏷️  Kind: {nostr_event.kind}")
        print(f"🏷️  Tags: {nostr_event.tags}")
        if geo_info:
            print(f"🌍 Geo: lat={geo_info['lat']}, lng={geo_info['lng']}, geohash={geo_info['geohash']}")
        print(f"\n📄 CONTENT:")
        print(f"{'-'*40}")
        print(nostr_event.content)
        print(f"{'-'*40}")
        
        # Show the complete event data that will be sent
        print(f"\n🔧 EVENT DATA (what gets sent to relay):")
        print(f"Event ID: {nostr_event.id}")
        print(f"Pubkey: {nostr_event.pubkey}")
        print(f"Signature: {nostr_event.sig}")
        print(f"Created at: {nostr_event.created_at}")
        print(f"Kind: {nostr_event.kind}")
        print(f"Content length: {len(nostr_event.content)} characters")
        print(f"Tags count: {len(nostr_event.tags)}")
        print(f"{'='*60}")
        
        # Always check for duplicates, even in dry-run mode
        if np.is_already_posted(rss_id):
            print(f"RSS item {rss_id} already posted, skipping")
            print("(DRY RUN - Not posting)" if args.dry_run else "")
        else:
            if args.dry_run:
                print("(DRY RUN - Not posting)")
                # Track as posted even in dry-run to prevent duplicates in same session
                np.posted_rss_ids.add(rss_id)
            else:
                try:
                    if args.debug:
                        print(f"DEBUG: Calling np.post() for {entry['title']}")
                    posted = np.post(nostr_event, rss_id)
                    if posted:
                        print(f"Posted: {entry['title']}")
                    else:
                        print(f"Skipped (already posted): {entry['title']}")
                except Exception as e:
                    print(f"Failed to post {entry['title']}: {e}")
                    if args.debug:
                        import traceback
                        print(f"DEBUG: Exception details: {traceback.format_exc()}")
    
    if args.debug:
        print("DEBUG: Starting final sync and cleanup...")
    np.sync_and_close()

if __name__ == "__main__":
    main()