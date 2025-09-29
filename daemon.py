#!/usr/bin/env python3
"""
Nostr Hitchhiking Bot Daemon

This daemon combines the functionality of:
- hwrecentchanges.py: Posts Hitchwiki recent changes to Nostr
- nostrhitch.py: Posts Hitchmap data to Nostr

The daemon runs both tasks on configurable intervals and provides
proper logging, error handling, and graceful shutdown.
"""

import os
import sys
import time
import signal
import logging
import argparse
import threading
from datetime import datetime, timedelta
from typing import Optional

# Import the existing modules
from hwrecentchanges import NostrPost as HWNostrPost, fetch_atom, entry_to_nostr, ATOM_URL
from nostrhitch import NostrPost as HitchmapNostrPost, download_hitchmap_data, fetch_data_from_hitchmapdb

# Import settings
from settings import nsec, post_to_relays, relays


class NostrHitchDaemon:
    """Main daemon class that manages both Hitchwiki and Hitchmap posting tasks."""
    
    def __init__(self, 
                 hw_interval: int = 300,  # 5 minutes
                 hitchmap_interval: int = 86400,  # 24 hours
                 debug: bool = False,
                 dry_run: bool = False):
        """
        Initialize the daemon.
        
        Args:
            hw_interval: Interval in seconds between Hitchwiki checks (default: 5 minutes)
            hitchmap_interval: Interval in seconds between Hitchmap checks (default: 24 hours)
            debug: Enable debug logging
            dry_run: Don't actually post to relays
        """
        self.hw_interval = hw_interval
        self.hitchmap_interval = hitchmap_interval
        self.debug = debug
        self.dry_run = dry_run
        self.running = False
        self.hw_thread: Optional[threading.Thread] = None
        self.hitchmap_thread: Optional[threading.Thread] = None
        
        # Setup logging
        self.setup_logging()
        
        # Initialize shared components
        self.hw_nostr_post = None
        self.hitchmap_nostr_post = None
        
        # Setup signal handlers for graceful shutdown
        signal.signal(signal.SIGINT, self.signal_handler)
        signal.signal(signal.SIGTERM, self.signal_handler)
        
        self.logger.info("Nostr Hitchhiking Bot Daemon initialized")
        self.logger.info(f"Hitchwiki interval: {hw_interval}s ({hw_interval/60:.1f} minutes)")
        self.logger.info(f"Hitchmap interval: {hitchmap_interval}s ({hitchmap_interval/3600:.1f} hours)")
        self.logger.info(f"Debug mode: {debug}")
        self.logger.info(f"Dry run mode: {dry_run}")

    def setup_logging(self):
        """Setup logging configuration."""
        log_level = logging.DEBUG if self.debug else logging.INFO
        
        # Create logs directory if it doesn't exist
        os.makedirs("logs", exist_ok=True)
        
        # Setup file handler
        log_filename = f"logs/daemon_{datetime.now().strftime('%Y%m%d')}.log"
        file_handler = logging.FileHandler(log_filename)
        file_handler.setLevel(log_level)
        
        # Setup console handler
        console_handler = logging.StreamHandler()
        console_handler.setLevel(log_level)
        
        # Setup formatter
        formatter = logging.Formatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
        )
        file_handler.setFormatter(formatter)
        console_handler.setFormatter(formatter)
        
        # Setup logger
        self.logger = logging.getLogger('nostr_hitch_daemon')
        self.logger.setLevel(log_level)
        self.logger.addHandler(file_handler)
        self.logger.addHandler(console_handler)

    def signal_handler(self, signum, frame):
        """Handle shutdown signals gracefully."""
        self.logger.info(f"Received signal {signum}, initiating graceful shutdown...")
        self.stop()

    def initialize_nostr_posts(self):
        """Initialize the Nostr posting components."""
        try:
            self.logger.info("Initializing Nostr posting components...")
            
            # Initialize Hitchwiki NostrPost
            self.hw_nostr_post = HWNostrPost(debug=self.debug, enable_duplicate_check=True)
            
            # Initialize Hitchmap NostrPost
            self.hitchmap_nostr_post = HitchmapNostrPost()
            
            self.logger.info("Nostr posting components initialized successfully")
            
        except Exception as e:
            self.logger.error(f"Failed to initialize Nostr posting components: {e}")
            if self.debug:
                import traceback
                self.logger.error(f"Debug traceback: {traceback.format_exc()}")
            raise

    def run_hitchwiki_task(self):
        """Run the Hitchwiki recent changes task."""
        self.logger.info("Starting Hitchwiki recent changes task...")
        
        try:
            # Fetch recent changes
            entries = fetch_atom(ATOM_URL)
            self.logger.info(f"Fetched {len(entries)} entries from Hitchwiki RSS feed")
            
            if not entries:
                self.logger.warning("No entries found in Hitchwiki RSS feed")
                return
            
            # Process entries
            posted_count = 0
            skipped_count = 0
            
            for i, entry in enumerate(entries, 1):
                try:
                    self.logger.debug(f"Processing entry {i}/{len(entries)}: {entry['title']}")
                    
                    # Convert to Nostr event
                    nostr_event, geo_info = entry_to_nostr(entry, debug=self.debug)
                    rss_id = entry.get('rss_id', '')
                    
                    # Check if already posted
                    if self.hw_nostr_post.is_already_posted(rss_id):
                        self.logger.debug(f"RSS item {rss_id} already posted, skipping")
                        skipped_count += 1
                        continue
                    
                    # Post the event
                    if self.dry_run:
                        self.logger.info(f"[DRY RUN] Would post: {entry['title']}")
                        self.hw_nostr_post.posted_rss_ids.add(rss_id)
                        posted_count += 1
                    else:
                        posted = self.hw_nostr_post.post(nostr_event, rss_id)
                        if posted:
                            self.logger.info(f"Posted: {entry['title']}")
                            posted_count += 1
                        else:
                            skipped_count += 1
                            
                except Exception as e:
                    self.logger.error(f"Error processing entry {entry.get('title', 'Unknown')}: {e}")
                    if self.debug:
                        import traceback
                        self.logger.error(f"Debug traceback: {traceback.format_exc()}")
                    continue
            
            self.logger.info(f"Hitchwiki task completed: {posted_count} posted, {skipped_count} skipped")
            
        except Exception as e:
            self.logger.error(f"Hitchwiki task failed: {e}")
            if self.debug:
                import traceback
                self.logger.error(f"Debug traceback: {traceback.format_exc()}")

    def run_hitchmap_task(self):
        """Run the Hitchmap data task."""
        self.logger.info("Starting Hitchmap data task...")
        
        try:
            # Calculate date range
            today = datetime.today().strftime('%Y-%m-%d')
            earlier = (datetime.today() - timedelta(days=12)).strftime('%Y-%m-%d')
            
            filename = f'hitchmap-dumps/hitchmap_{today}.sqlite'
            url = 'https://hitchmap.com/dump.sqlite'
            
            # Create hitchmap-dumps directory if it doesn't exist
            os.makedirs("hitchmap-dumps", exist_ok=True)
            
            # Download data if needed
            if not os.path.exists(filename):
                self.logger.info("Downloading latest Hitchmap data...")
                download_hitchmap_data(url, filename)
            else:
                self.logger.info(f"Hitchmap file '{filename}' already exists")
            
            # Query and post data
            query = f"SELECT * FROM points WHERE datetime > '{earlier}'"
            self.logger.info(f"Querying Hitchmap data: {query}")
            
            if self.dry_run:
                self.logger.info("[DRY RUN] Would process Hitchmap data")
            else:
                fetch_data_from_hitchmapdb(filename, query)
            
            self.logger.info("Hitchmap task completed")
            
        except Exception as e:
            self.logger.error(f"Hitchmap task failed: {e}")
            if self.debug:
                import traceback
                self.logger.error(f"Debug traceback: {traceback.format_exc()}")

    def hitchwiki_worker(self):
        """Worker thread for Hitchwiki task."""
        while self.running:
            try:
                self.run_hitchwiki_task()
            except Exception as e:
                self.logger.error(f"Hitchwiki worker error: {e}")
                if self.debug:
                    import traceback
                    self.logger.error(f"Debug traceback: {traceback.format_exc()}")
            
            # Wait for next interval
            for _ in range(self.hw_interval):
                if not self.running:
                    break
                time.sleep(1)

    def hitchmap_worker(self):
        """Worker thread for Hitchmap task."""
        while self.running:
            try:
                self.run_hitchmap_task()
            except Exception as e:
                self.logger.error(f"Hitchmap worker error: {e}")
                if self.debug:
                    import traceback
                    self.logger.error(f"Debug traceback: {traceback.format_exc()}")
            
            # Wait for next interval
            for _ in range(self.hitchmap_interval):
                if not self.running:
                    break
                time.sleep(1)

    def start(self):
        """Start the daemon."""
        self.logger.info("Starting Nostr Hitchhiking Bot Daemon...")
        
        try:
            # Initialize components
            self.initialize_nostr_posts()
            
            # Set running flag
            self.running = True
            
            # Start worker threads
            self.hw_thread = threading.Thread(target=self.hitchwiki_worker, name="HitchwikiWorker")
            self.hitchmap_thread = threading.Thread(target=self.hitchmap_worker, name="HitchmapWorker")
            
            self.hw_thread.start()
            self.hitchmap_thread.start()
            
            self.logger.info("Daemon started successfully")
            self.logger.info("Press Ctrl+C to stop the daemon")
            
            # Wait for threads to complete
            self.hw_thread.join()
            self.hitchmap_thread.join()
            
        except Exception as e:
            self.logger.error(f"Failed to start daemon: {e}")
            if self.debug:
                import traceback
                self.logger.error(f"Debug traceback: {traceback.format_exc()}")
            raise
        finally:
            self.cleanup()

    def stop(self):
        """Stop the daemon."""
        self.logger.info("Stopping daemon...")
        self.running = False

    def cleanup(self):
        """Cleanup resources."""
        self.logger.info("Cleaning up resources...")
        
        try:
            if self.hw_nostr_post:
                self.hw_nostr_post.close()
            if self.hitchmap_nostr_post:
                self.hitchmap_nostr_post.close()
        except Exception as e:
            self.logger.error(f"Error during cleanup: {e}")
        
        self.logger.info("Cleanup completed")


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Nostr Hitchhiking Bot Daemon")
    parser.add_argument("--hw-interval", type=int, default=300,
                       help="Interval in seconds between Hitchwiki checks (default: 300)")
    parser.add_argument("--hitchmap-interval", type=int, default=86400,
                       help="Interval in seconds between Hitchmap checks (default: 86400)")
    parser.add_argument("--debug", action="store_true",
                       help="Enable debug logging")
    parser.add_argument("--dry-run", action="store_true",
                       help="Don't actually post to relays")
    parser.add_argument("--run-once", action="store_true",
                       help="Run each task once and exit (useful for testing)")
    
    args = parser.parse_args()
    
    # Override post_to_relays if dry-run is specified
    if args.dry_run:
        import settings
        settings.post_to_relays = False
    
    try:
        daemon = NostrHitchDaemon(
            hw_interval=args.hw_interval,
            hitchmap_interval=args.hitchmap_interval,
            debug=args.debug,
            dry_run=args.dry_run
        )
        
        if args.run_once:
            # Run each task once and exit
            daemon.logger.info("Running tasks once and exiting...")
            daemon.initialize_nostr_posts()
            daemon.run_hitchwiki_task()
            daemon.run_hitchmap_task()
            daemon.cleanup()
            daemon.logger.info("One-time run completed")
        else:
            # Run as daemon
            daemon.start()
            
    except KeyboardInterrupt:
        print("\nReceived interrupt signal, shutting down...")
    except Exception as e:
        print(f"Fatal error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
