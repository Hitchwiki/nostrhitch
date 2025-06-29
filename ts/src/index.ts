import { NostrFetcher } from "nostr-fetch";

const nHoursAgo = (hrs: number): number =>
  Math.floor((Date.now() - hrs * 60 * 60 * 1000) / 1000);

const fetcher = NostrFetcher.init();
const relayUrls = [
    "wss://relay.trustroots.org"
];

// fetches all text events since 24 hr ago in streaming manner
const postIter = fetcher.allEventsIterator(
    relayUrls, 
    /* filter (kinds, authors, ids, tags) */
    { kinds: [ 30399] },
    /* time range filter (since, until) */
    { since: nHoursAgo(10000) },
    /* fetch options (optional) */
    { skipFilterMatching: true }
);
for await (const ev of postIter) {
    console.log(ev.content);
}