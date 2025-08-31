import requests

url = "https://raw.githubusercontent.com/Hitchwiki/hitchhiking-data-standard/refs/heads/main/python/python.py"
response = requests.get(url)

with open("data_standard_pydantic_model.py", "w") as f:
    f.write(response.text)

from data_standard_pydantic_model import Hitchhiker, HitchhikingRecord, Location, Signal, Stop
import sqlite3
import pandas as pd
from tqdm import tqdm
import os
import wget
from dotenv import load_dotenv
from post_data_standard import NostrHitchhikingPostDataStandard

load_dotenv()

pd.set_option('display.max_rows', None)
pd.set_option('display.max_columns', None)
pd.set_option('display.max_colwidth', None) 

url = 'https://hitchmap.com/dump.sqlite'
filename = 'dump.sqlite'
if os.path.exists(filename):
        os.remove(filename)
filename = wget.download(url)
fn = 'dump.sqlite'
points = pd.read_sql('select * from points where not banned', sqlite3.connect(fn))
points["datetime"] = points["datetime"].astype("datetime64[ns]")

points.loc[points["datetime"] < "2000-01-01", "datetime"] = None

# cleaning invalid timestamps

points["ride_datetime"] = points["ride_datetime"].replace("0224-10-31T21:30", None)
points["ride_datetime"] = points["ride_datetime"].replace("0025-03-07T08:00", None)
points["ride_datetime"] = points["ride_datetime"].replace("1014-11-04T14:30", None)
points["ride_datetime"] = points["ride_datetime"].replace("0202-04-03T18:50", None)

points["ride_datetime"] = points["ride_datetime"].astype("datetime64[ns]")
len(points)
points.head()
# assume that during the lifershalte time the timestamps where not always set
# thus attribute this part of the dataset to the lifershalte
no_date = points[points["datetime"].isna()]
with_date = points[~points["datetime"].isna()]

lift = pd.concat([no_date, with_date[with_date["datetime"] < "2010-08-11"]])

wiki = with_date[(with_date["datetime"] >= "2010-08-11") & (with_date["datetime"] < "2022-10-13")]

map = with_date[with_date["datetime"] >= "2022-10-13"]
len(lift), len(wiki), len(map), len(lift) + len(wiki) + len(map)
def map_signal(signal: str) -> Signal:
    if not signal:
        return None

    if signal == "sign":
        return Signal(
            methods=["sign"],
        )
    elif signal == "thumb":
        return Signal(
            methods=["thumb"],
        )
    elif signal == "ask":
        return Signal(
            methods=["asking"],
        )
    elif signal == "ask-sign":
        return Signal(
            methods=["asking", "sign"],
        )
    else:
        return None


def create_record_from_row(row: pd.Series, source: str, license: str, rating_formula= lambda x: x) -> HitchhikingRecord:
    stops = [
        Stop(
            location=Location(latitude=row["lat"], longitude=row["lon"], is_exact=True),
            arrival_time=row["ride_datetime"].strftime("%Y-%m-%dT%H:%M:%S") if pd.notna(row["ride_datetime"]) else None,
            departure_time=(row["ride_datetime"] + pd.to_timedelta(row["wait"], unit="m")).strftime(
                "%Y-%m-%dT%H:%M:%S"
            )
            if pd.notna(row["ride_datetime"]) and pd.notna(row["wait"])
            else None,
            waiting_duration=f"{int(row['wait'])}M" if pd.notna(row["wait"]) else None,
        ),
    ]
    if pd.notna(row["dest_lat"]) and pd.notna(row["dest_lon"]):
        stops.append(Stop(location=Location(latitude=row["dest_lat"], longitude=row["dest_lon"], is_exact=False)))

    record = HitchhikingRecord(
        version="0.0.0",
        stops=stops,
        rating=rating_formula(row["rating"]),
        hitchhikers=[
            Hitchhiker(
                nickname=row["nickname"] if pd.notna(row["nickname"]) else "Anonymous"
            )
        ],
        comment=row["comment"],
        signals=[map_signal(row["signal"])] if row["signal"] else None,
        occupants=None,
        mode_of_transportation=None,
        ride=None,
        declined_rides=None,
        source=source,
        license=license,
        submission_time=row["datetime"].strftime("%Y-%m-%dT%H:%M:%S") if pd.notna(row["datetime"]) else None,
    )

    return record
records = []

for _, row in tqdm(lift.iterrows(), total=len(lift)):
    records.append(
        create_record_from_row(
            row,
            source="liftershalte.info",
            license="cc-by-sa-4.0",
        )
    )

for _, row in tqdm(wiki.iterrows(), total=len(wiki)):
    records.append(
        create_record_from_row(
            row,
            source="hitchwiki.org",
            license="cc-by-sa-4.0",
        )
    )

for _, row in tqdm(map.iterrows(), total=len(map)):
    records.append(
        create_record_from_row(
            row,
            source="hitchmap.com",
            license="odbl",
        )
    )

print(records[0].model_dump_json(indent=2, exclude_none=True))


poster = NostrHitchhikingPostDataStandard()
for record in records[-10:]:
    poster.post(ride_record=record)

poster.close()