import os
import sys
import time
import uuid
import requests
import sqlite3

from datetime import datetime, timedelta
from pprint import pprint

from pynostr.key import PrivateKey
from pynostr.relay_manager import RelayManager
from pynostr.event import Event, EventKind

import geohash2

import settings


def download_hitchmap_data(url, filename):
    print ("Downloading", url)
    response = requests.get(url)
    if response.status_code == 200:
        with open(filename, 'wb') as file:
            file.write(response.content)
        print('File saved as ', filename)
    else:
        print("Failed to fetch the page, status code:", response.status_code)

def fetch_data_from_hitchmapdb(filename, query):
    conn = sqlite3.connect(filename)
    cursor = conn.cursor()
    print (query)
    cursor.execute(query)

    results = cursor.fetchall()
    np = NostrPost()

    for row in results:
        np.post(row)
    
    conn.close()
    

def main():
    today = datetime.today().strftime('%Y-%m-%d')
    earlier = (datetime.today() - timedelta(days=12)).strftime('%Y-%m-%d')

    filename = f'hitchmap-dumps/hitchmap_{today}.sqlite'
    url = 'https://hitchmap.com/dump.sqlite'
    print ("nostr hitch: putting hitchmap data into nostr, and thus on notes.trustroots.org")

    if not os.path.exists("hitchmap-dumps"):
        os.mkdir("hitchmap-dumps")
    
    if not os.path.exists(filename):
        print("Downloading latest hitchmap data")
        download_hitchmap_data(url, filename)
    else:
        print(f"File '{filename}' already exists.")

    query = f"SELECT * FROM points WHERE datetime > '{earlier}'"
    fetch_data_from_hitchmapdb(filename, query)

class NostrHitchhikingPostDataStandard:
    def __init__(self):
        private_key_obj = PrivateKey.from_nsec(settings.nsec)
        self.private_key_hex = private_key_obj.hex()
        npub = private_key_obj.public_key.bech32()
        print(f"Posting as npub {npub}")

        self.nh_conn = sqlite3.connect("nostrhitch.sqlite")
        self.nh_cursor = self.nh_conn.cursor()
        self.nh_cursor.execute("""
            CREATE TABLE IF NOT EXISTS posted_hitchmap_ids (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                hitchmap_id TEXT NOT NULL
            )
        """)

        # Initialize the relay manager
        self.relay_manager = RelayManager(timeout=5)
        for relay in settings.relays:
            self.relay_manager.add_relay(relay)

        self.event_kind = 30399  # Event kind for hitchhiking notes

    def post(self, hitchnote):
        # (3312039017314494891, 46.55134501890075, 0.401676893234253, 4.0, 'FR', 5.0, 'Anonyme', "On peut attendre 5 minutes comme une demie heure. Mais entre le rond point et la station total c'est pas mal, il y a des endroits pour s'arrêter..", '2024-10-14 09:58:11.703127', 0, 0, '', 46.55134501890075, 0.401676893234253, None, None)
        hitchmap_id, start_lat, start_lng, rating, country, col6, hitchhiker_name, desc, datetime, col10, col11, end_lat, end_lng, col13, col14, col15 = hitchnote

        if not hitchhiker_name:
            hitchhiker_name = ''
        event_content = f"hitchmap.com {hitchhiker_name}: {desc}"
        hitchmap_id = str(int(hitchmap_id))

        geohash = geohash2.encode(start_lat, start_lng)

        event = Event(kind=self.event_kind, content=event_content, tags=
                      [
                       ['d', hitchmap_id],
                       ['l', "hitchhikers", "trustroots-circle"],
                       ['g', geohash],
                        ])
        
        event.sign(self.private_key_hex)
        
        self.nh_cursor.execute("SELECT COUNT(*) FROM posted_hitchmap_ids WHERE hitchmap_id = ?", (hitchmap_id,))
        if self.nh_cursor.fetchone()[0] == 0:
            print('vars(event)')
            pprint(vars(event))

            if settings.post_to_relays:
                print("posting to relays")
                self.relay_manager.publish_event(event)
                self.relay_manager.run_sync()  # Sync with the relay to send the event
                self.nh_cursor.execute("INSERT INTO posted_hitchmap_ids (hitchmap_id) VALUES (?)", (hitchmap_id,))
                self.nh_conn.commit()  # Commit the transaction to save the changes to the database
                print("posted, waiting a bit")
                time.sleep(3)
        else:
            print("already posted")

    def close(self):
        self.relay_manager.close_all_relay_connections()

if __name__ == "__main__":
    main()

