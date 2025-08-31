import time
import uuid


from pprint import pprint

from pynostr.key import PrivateKey
from pynostr.relay_manager import RelayManager
from pynostr.event import Event, EventKind

import geohash2

import settings
from data_standard_pydantic_model import HitchhikingRecord


class NostrHitchhikingPostDataStandard:
    def __init__(self):
        private_key_obj = PrivateKey.from_nsec(settings.nsec)
        self.private_key_hex = private_key_obj.hex()
        self.npub = private_key_obj.public_key.bech32()
        print(f"Posting as npub {self.npub}")

        # Initialize the relay manager
        self.relay_manager = RelayManager(timeout=5)
        for relay in settings.relays:
            self.relay_manager.add_relay(relay)

        self.event_kind = 30399  # Event kind for hitchhiking notes

    def post(self, ride_record: HitchhikingRecord):
        tags = []
        for tag, value in ride_record.model_dump(exclude_none=False, by_alias=True).items():
            tags.append([tag, value])

        start_location = ride_record.stops[0].location

        geohash = geohash2.encode(start_location.latitude, start_location.longitude, precision=9)

        event = Event(
            kind=self.event_kind,
            created_at=int(time.time()),
            content=ride_record.comment,
            pubkey=self.npub,
            id=f"{ride_record.source}-{uuid.uuid4()}",
            sig=None,  # Signature will be added later
            tags=[
                ["expiration", 0],
                ["d", f"{ride_record.source}-{uuid.uuid4()}"],
                ["g", geohash],
            ] + tags,
        )

        event.sign(self.private_key_hex)

        print("vars(event)")
        pprint(vars(event))

        if settings.post_to_relays:
            print("posting to relays")
            self.relay_manager.publish_event(event)
            self.relay_manager.run_sync()  # Sync with the relay to send the event
            print("posted, waiting a bit")
            time.sleep(3)

    def close(self):
        self.relay_manager.close_all_relay_connections()