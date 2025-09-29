#!/usr/bin/env python3
"""
Test script for the Nostr Hitchhiking Bot Daemon

This script tests the daemon functionality without actually posting to relays.
"""

import sys
import os
import time
import logging

# Add current directory to path so we can import our modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from daemon import NostrHitchDaemon

def test_daemon():
    """Test the daemon functionality."""
    print("🧪 Testing Nostr Hitchhiking Bot Daemon")
    print("=" * 50)
    
    # Setup logging for test
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    try:
        # Create daemon instance with short intervals for testing
        daemon = NostrHitchDaemon(
            hw_interval=10,  # 10 seconds for testing
            hitchmap_interval=30,  # 30 seconds for testing
            debug=True,
            dry_run=True  # Don't actually post
        )
        
        print("✅ Daemon instance created successfully")
        
        # Test initialization
        print("\n🔧 Testing initialization...")
        daemon.initialize_nostr_posts()
        print("✅ Initialization successful")
        
        # Test Hitchwiki task
        print("\n📰 Testing Hitchwiki task...")
        daemon.run_hitchwiki_task()
        print("✅ Hitchwiki task completed")
        
        # Test Hitchmap task
        print("\n🗺️  Testing Hitchmap task...")
        daemon.run_hitchmap_task()
        print("✅ Hitchmap task completed")
        
        # Test one-time run
        print("\n🔄 Testing one-time run...")
        daemon.running = True
        daemon.run_hitchwiki_task()
        daemon.run_hitchmap_task()
        print("✅ One-time run completed")
        
        # Cleanup
        print("\n🧹 Cleaning up...")
        daemon.cleanup()
        print("✅ Cleanup completed")
        
        print("\n🎉 All tests passed!")
        return True
        
    except Exception as e:
        print(f"\n❌ Test failed: {e}")
        import traceback
        print(f"Traceback: {traceback.format_exc()}")
        return False

def test_imports():
    """Test that all required modules can be imported."""
    print("📦 Testing imports...")
    
    try:
        from hwrecentchanges import NostrPost as HWNostrPost, fetch_atom, entry_to_nostr
        print("✅ hwrecentchanges imported successfully")
        
        from nostrhitch import NostrPost as HitchmapNostrPost, download_hitchmap_data, fetch_data_from_hitchmapdb
        print("✅ nostrhitch imported successfully")
        
        from settings import nsec, post_to_relays, relays
        print("✅ settings imported successfully")
        
        return True
        
    except Exception as e:
        print(f"❌ Import failed: {e}")
        return False

def test_settings():
    """Test that settings are properly configured."""
    print("⚙️  Testing settings...")
    
    try:
        from settings import nsec, post_to_relays, relays
        
        if not nsec:
            print("❌ nsec not configured")
            return False
        print("✅ nsec configured")
        
        if not relays:
            print("❌ relays not configured")
            return False
        print("✅ relays configured")
        
        print(f"✅ Settings look good (post_to_relays: {post_to_relays})")
        return True
        
    except Exception as e:
        print(f"❌ Settings test failed: {e}")
        return False

def main():
    """Run all tests."""
    print("🚀 Starting Nostr Hitchhiking Bot Daemon Tests")
    print("=" * 60)
    
    tests = [
        ("Import Test", test_imports),
        ("Settings Test", test_settings),
        ("Daemon Test", test_daemon),
    ]
    
    passed = 0
    total = len(tests)
    
    for test_name, test_func in tests:
        print(f"\n🧪 Running {test_name}...")
        if test_func():
            passed += 1
            print(f"✅ {test_name} PASSED")
        else:
            print(f"❌ {test_name} FAILED")
    
    print("\n" + "=" * 60)
    print(f"📊 Test Results: {passed}/{total} tests passed")
    
    if passed == total:
        print("🎉 All tests passed! The daemon is ready to use.")
        return 0
    else:
        print("⚠️  Some tests failed. Please check the errors above.")
        return 1

if __name__ == "__main__":
    sys.exit(main())
