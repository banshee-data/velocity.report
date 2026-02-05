#!/usr/bin/env python3

import logging
import sqlite3
import json
import asyncio
from typing import Dict, Any, List
from datetime import datetime, timedelta
import aiofiles
import aiohttp

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(filename)s:%(lineno)d - %(message)s",
)
logger = logging.getLogger(__name__)


async def load_config(
    config_file: str = "config_telraam.json",
) -> Dict[str, Any]:
    """Load configuration from JSON file"""
    # Define default/sample config in one place
    default_config = {
        "api_key": "your_api_key",
        "api_url": "https://api.telraam.net/v1",
        "device_id": "your_device_id",
        "start_date": "2020-01-01",
    }

    try:
        async with aiofiles.open(config_file, "r") as f:
            content = await f.read()
            config = json.loads(content)

        # Check if config matches default values (user hasn't updated it)
        if all(config.get(key) == value for key, value in default_config.items()):
            logger.error(
                "Configuration file contains default values. Please update config_telraam.json with your actual Telraam API credentials."
            )
            raise SystemExit("Exiting: Configuration not updated")

        return config

    except FileNotFoundError:
        # Create sample config file using the same default_config
        async with aiofiles.open(config_file, "w") as f:
            await f.write(json.dumps(default_config, indent=2))
        logger.error(
            f"Created sample config file: {config_file}. Please update with your credentials."
        )
        raise
    except json.JSONDecodeError as e:
        logger.error(f"Failed to decode JSON from config file: {e}")
        raise


async def init_database(db_file: str = "align.db") -> sqlite3.Connection:
    """Initialise SQLite database with Telraam table"""
    conn = sqlite3.connect(db_file)
    cursor = conn.cursor()

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS telraam (
        device_id TEXT,
        instance_id TEXT,
        segment_id TEXT,
        date TEXT,
        interval INTEGER,
        uptime REAL,
        heavy INTEGER,
        car INTEGER,
        bike INTEGER,
        pedestrian INTEGER,
        night INTEGER,
        heavy_lft INTEGER,
        heavy_rgt INTEGER,
        car_lft INTEGER,
        car_rgt INTEGER,
        bike_lft INTEGER,
        bike_rgt INTEGER,
        pedestrian_lft INTEGER,
        pedestrian_rgt INTEGER,
        direction INTEGER,
        car_speed_hist_0to70plus JSON,
        car_speed_hist_0to120plus JSON,
        mode_bicycle_lft INTEGER,
        mode_bicycle_rgt INTEGER,
        mode_bus_lft INTEGER,
        mode_bus_rgt INTEGER,
        mode_car_lft INTEGER,
        mode_car_rgt INTEGER,
        mode_lighttruck_lft INTEGER,
        mode_lighttruck_rgt INTEGER,
        mode_motorcycle_lft INTEGER,
        mode_motorcycle_rgt INTEGER,
        mode_pedestrian_lft INTEGER,
        mode_pedestrian_rgt INTEGER,
        mode_stroller_lft INTEGER,
        mode_stroller_rgt INTEGER,
        mode_tractor_lft INTEGER,
        mode_tractor_rgt INTEGER,
        mode_trailer_lft INTEGER,
        mode_trailer_rgt INTEGER,
        mode_truck_lft INTEGER,
        mode_truck_rgt INTEGER,
        mode_night_lft INTEGER,
        mode_night_rgt INTEGER,
        speed_hist_car_lft JSON,
        speed_hist_car_rgt JSON,
        brightness REAL,
        sharpness REAL,
        period_start TEXT,
        period_duration INTEGER,
        v85 REAL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        PRIMARY KEY (device_id, instance_id, segment_id, date, interval)
    )
  """)

    conn.commit()
    logger.info("Database initialized successfully")
    return conn


class TelraamLogger:
    def __init__(self, config: Dict[str, Any], db_connection: sqlite3.Connection):
        self.config = config
        self.db_connection = db_connection
        self.client = None

    def get_latest_date_from_db(self) -> str:
        """Get the latest date from the telraam table, or return config start_date if no data exists"""
        cursor = self.db_connection.cursor()

        try:
            cursor.execute(
                """
                SELECT MAX(date) as latest_date
                FROM telraam
                WHERE device_id = ?
            """,
                (self.config["device_id"],),
            )

            result = cursor.fetchone()
            latest_date = result[0] if result and result[0] else None

            if latest_date:
                # Parse the date and add one day to continue from the next day
                latest_dt = datetime.fromisoformat(latest_date.replace("Z", "+00:00"))
                next_day = (latest_dt + timedelta(days=1)).strftime("%Y-%m-%d")
                logger.info(f"Found existing data. Starting from: {next_day}")
                return next_day
            else:
                logger.info(
                    f"No existing data found. Starting from config date: {self.config['start_date']}"
                )
                return self.config["start_date"]

        except sqlite3.Error as e:
            logger.error(f"Database error getting latest date: {e}")
            return self.config["start_date"]

    async def insert_traffic_data(self, traffic_data: List[Dict]) -> int:
        """Insert traffic data into the database"""
        if not traffic_data:
            logger.info("No data to insert")
            return 0

        cursor = self.db_connection.cursor()
        inserted_count = 0

        # Define the insert query with all columns
        insert_query = """
        INSERT OR REPLACE INTO telraam (
            device_id, instance_id, segment_id, date, interval, uptime, heavy, car, bike,
            pedestrian, night, heavy_lft, heavy_rgt, car_lft, car_rgt, bike_lft, bike_rgt,
            pedestrian_lft, pedestrian_rgt, direction, car_speed_hist_0to70plus,
            car_speed_hist_0to120plus, mode_bicycle_lft, mode_bicycle_rgt, mode_bus_lft,
            mode_bus_rgt, mode_car_lft, mode_car_rgt, mode_lighttruck_lft, mode_lighttruck_rgt,
            mode_motorcycle_lft, mode_motorcycle_rgt, mode_pedestrian_lft, mode_pedestrian_rgt,
            mode_stroller_lft, mode_stroller_rgt, mode_tractor_lft, mode_tractor_rgt,
            mode_trailer_lft, mode_trailer_rgt, mode_truck_lft, mode_truck_rgt,
            mode_night_lft, mode_night_rgt, speed_hist_car_lft, speed_hist_car_rgt,
            brightness, sharpness, period_start, period_duration, v85
        ) VALUES (
            ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
            ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
        )
        """

        try:
            for record in traffic_data:
                # Convert array fields to JSON strings
                car_speed_hist_0to70plus = (
                    json.dumps(record.get("car_speed_hist_0to70plus"))
                    if record.get("car_speed_hist_0to70plus")
                    else None
                )
                car_speed_hist_0to120plus = (
                    json.dumps(record.get("car_speed_hist_0to120plus"))
                    if record.get("car_speed_hist_0to120plus")
                    else None
                )
                speed_hist_car_lft = (
                    json.dumps(record.get("speed_hist_car_lft"))
                    if record.get("speed_hist_car_lft")
                    else None
                )
                speed_hist_car_rgt = (
                    json.dumps(record.get("speed_hist_car_rgt"))
                    if record.get("speed_hist_car_rgt")
                    else None
                )

                values = (
                    record.get("device_id"),
                    record.get("instance_id"),
                    record.get("segment_id"),
                    record.get("date"),
                    record.get("interval"),
                    record.get("uptime"),
                    record.get("heavy"),
                    record.get("car"),
                    record.get("bike"),
                    record.get("pedestrian"),
                    record.get("night"),
                    record.get("heavy_lft"),
                    record.get("heavy_rgt"),
                    record.get("car_lft"),
                    record.get("car_rgt"),
                    record.get("bike_lft"),
                    record.get("bike_rgt"),
                    record.get("pedestrian_lft"),
                    record.get("pedestrian_rgt"),
                    record.get("direction"),
                    car_speed_hist_0to70plus,
                    car_speed_hist_0to120plus,
                    record.get("mode_bicycle_lft"),
                    record.get("mode_bicycle_rgt"),
                    record.get("mode_bus_lft"),
                    record.get("mode_bus_rgt"),
                    record.get("mode_car_lft"),
                    record.get("mode_car_rgt"),
                    record.get("mode_lighttruck_lft"),
                    record.get("mode_lighttruck_rgt"),
                    record.get("mode_motorcycle_lft"),
                    record.get("mode_motorcycle_rgt"),
                    record.get("mode_pedestrian_lft"),
                    record.get("mode_pedestrian_rgt"),
                    record.get("mode_stroller_lft"),
                    record.get("mode_stroller_rgt"),
                    record.get("mode_tractor_lft"),
                    record.get("mode_tractor_rgt"),
                    record.get("mode_trailer_lft"),
                    record.get("mode_trailer_rgt"),
                    record.get("mode_truck_lft"),
                    record.get("mode_truck_rgt"),
                    record.get("mode_night_lft"),
                    record.get("mode_night_rgt"),
                    speed_hist_car_lft,
                    speed_hist_car_rgt,
                    record.get("brightness"),
                    record.get("sharpness"),
                    record.get("period_start"),
                    record.get("period_duration"),
                    record.get("v85"),
                )

                cursor.execute(insert_query, values)
                inserted_count += 1

            self.db_connection.commit()
            logger.info(f"Successfully inserted {inserted_count} traffic records")
            return inserted_count

        except sqlite3.Error as e:
            logger.error(f"Database error inserting traffic data: {e}")
            self.db_connection.rollback()
            return 0

    async def check_api_authorization(self) -> bool:
        """Check if the API key is authorized by calling the /v1 endpoint"""
        try:
            api_url = self.config["api_url"]
            api_key = self.config["api_key"]

            headers = {"X-Api-Key": api_key, "Content-Type": "application/json"}

            async with aiohttp.ClientSession() as session:
                async with session.get(f"{api_url}/v1", headers=headers) as response:
                    if response.status == 200:
                        data = await response.json()
                        logger.info(
                            f"API authorization successful: {data.get('message', 'Connected to Telraam API')}"
                        )
                        return True
                    elif response.status == 401:
                        logger.error("API authorization failed: Invalid API key")
                        return False
                    else:
                        logger.error(
                            f"API authorization failed: HTTP {response.status}"
                        )
                        return False

        except aiohttp.ClientError as e:
            logger.error(f"Network error during API check: {e}")
            return False
        except Exception as e:
            logger.error(f"Unexpected error during API check: {e}")
            return False

    async def fetch_traffic_data(
        self, start_date: str, end_date: str = None
    ) -> List[Dict]:
        """
        Fetch traffic data from Telraam API with 24-hour pagination

        Args:
            start_date: Start date in YYYY-MM-DD format
            end_date: End date in YYYY-MM-DD format (optional, defaults to today)

        Returns:
            List of traffic data records
        """
        all_data = []

        # Parse start date
        start_dt = datetime.strptime(start_date, "%Y-%m-%d")

        # Default end date to today if not provided
        if end_date is None:
            end_dt = datetime.now().replace(hour=0, minute=0, second=0, microsecond=0)
        else:
            end_dt = datetime.strptime(end_date, "%Y-%m-%d")

        # API configuration
        api_url = self.config["api_url"]
        api_key = self.config["api_key"]
        device_id = self.config["device_id"]

        # Define all columns to request
        columns = ",".join(
            [
                "device_id",
                "instance_id",
                "segment_id",
                "date",
                "interval",
                "uptime",
                "heavy",
                "car",
                "bike",
                "pedestrian",
                "night",
                "heavy_lft",
                "heavy_rgt",
                "car_lft",
                "car_rgt",
                "bike_lft",
                "bike_rgt",
                "pedestrian_lft",
                "pedestrian_rgt",
                "direction",
                "car_speed_hist_0to70plus",
                "car_speed_hist_0to120plus",
                "mode_bicycle_lft",
                "mode_bicycle_rgt",
                "mode_bus_lft",
                "mode_bus_rgt",
                "mode_car_lft",
                "mode_car_rgt",
                "mode_lighttruck_lft",
                "mode_lighttruck_rgt",
                "mode_motorcycle_lft",
                "mode_motorcycle_rgt",
                "mode_pedestrian_lft",
                "mode_pedestrian_rgt",
                "mode_stroller_lft",
                "mode_stroller_rgt",
                "mode_tractor_lft",
                "mode_tractor_rgt",
                "mode_trailer_lft",
                "mode_trailer_rgt",
                "mode_truck_lft",
                "mode_truck_rgt",
                "mode_night_lft",
                "mode_night_rgt",
                "speed_hist_car_lft",
                "speed_hist_car_rgt",
                "brightness",
                "sharpness",
                "period_start",
                "period_duration",
                "v85",
            ]
        )

        headers = {
            "X-Api-Key": api_key,
            "Content-Type": "application/json",
            "accept": "application/json",
        }

        current_date = start_dt

        logger.info(
            f"Fetching traffic data from {start_date} to {end_dt.strftime('%Y-%m-%d')}"
        )

        async with aiohttp.ClientSession() as session:
            while current_date < end_dt:
                # Calculate end time for this batch (24 hours later or end_dt, whichever is earlier)
                batch_end = min(current_date + timedelta(days=1), end_dt)

                # Format timestamps for API
                time_start = current_date.strftime("%Y-%m-%d %H:%M:%S") + "Z"
                time_end = (
                    batch_end.strftime("%Y-%m-%d %H:%M:%S") + "Z"
                )  # 00:00 of the next day

                payload = {
                    "level": "devices",
                    "id": device_id,
                    "format": "per-quarter",
                    "time_start": time_start,
                    "time_end": time_end,
                    "columns": columns,
                }

                try:
                    logger.info(
                        f"Fetching data for {current_date.strftime('%Y-%m-%d')}"
                    )

                    async with session.post(
                        f"{api_url}/advanced/reports/traffic",
                        headers=headers,
                        json=payload,
                    ) as response:

                        if response.status == 200:
                            data = await response.json()

                            if data.get("status_code") == 200 and "report" in data:
                                batch_records = data["report"]
                                all_data.extend(batch_records)
                                logger.info(
                                    f"Retrieved {len(batch_records)} records for {current_date.strftime('%Y-%m-%d')}"
                                )
                            else:
                                logger.warning(
                                    f"No data returned for {current_date.strftime('%Y-%m-%d')}: {data.get('message', 'Unknown error')}"
                                )

                        elif response.status == 401:
                            logger.error("API authentication failed")
                            break
                        elif response.status == 429:
                            logger.warning("Rate limit hit, waiting 10 seconds...")
                            await asyncio.sleep(10)
                            continue  # Retry the same date
                        else:
                            logger.error(
                                f"API request failed with status {response.status}"
                            )
                            response_text = await response.text()
                            logger.error(f"Response: {response_text}")

                except aiohttp.ClientError as e:
                    logger.error(
                        f"Network error for {current_date.strftime('%Y-%m-%d')}: {e}"
                    )
                except Exception as e:
                    logger.error(
                        f"Unexpected error for {current_date.strftime('%Y-%m-%d')}: {e}"
                    )

                # Move to next day
                current_date += timedelta(days=1)

                # Add small delay to be respectful to the API
                await asyncio.sleep(2)

        logger.info(f"Total records fetched: {len(all_data)}")
        return all_data

    async def run(self):
        """Main application entry point"""
        try:
            # Check API authorization first
            if not await self.check_api_authorization():
                logger.error("API authorization check failed. Exiting.")
                return

            logger.info(
                "API authorization successful, proceeding with data collection..."
            )

            # Get the start date from the latest data in database, or config if no data exists
            start_date = self.get_latest_date_from_db()

            # Fetch and insert traffic data day by day to avoid losing data on interruption
            total_inserted = 0
            current_date = datetime.strptime(start_date, "%Y-%m-%d")
            end_date = datetime.now().replace(hour=0, minute=0, second=0, microsecond=0)

            logger.info(
                f"Fetching traffic data from {start_date} to {end_date.strftime('%Y-%m-%d')}"
            )

            while current_date < end_date:
                day_str = current_date.strftime("%Y-%m-%d")
                next_day_str = (current_date + timedelta(days=1)).strftime("%Y-%m-%d")

                # Fetch data for just this day
                daily_data = await self.fetch_traffic_data(day_str, next_day_str)

                if daily_data:
                    # Insert this day's data immediately
                    inserted_count = await self.insert_traffic_data(daily_data)
                    total_inserted += inserted_count
                    logger.info(f"Inserted {inserted_count} records for {day_str}")
                else:
                    logger.warning(f"No data retrieved for {day_str}")

                # Move to next day
                current_date += timedelta(days=1)

            if total_inserted > 0:
                logger.info(
                    f"Data collection complete. Total inserted: {total_inserted} records."
                )
            else:
                logger.warning("No records were inserted into the database")

        except Exception as e:
            logger.error(f"Application error: {e}")
        finally:
            if self.client:
                # Proper cleanup - the client doesn't have a close method
                if hasattr(self.client, "close_session"):
                    await self.client.close_session()
                elif hasattr(self.client, "_session") and self.client._session:
                    await self.client._session.close()


async def main():
    try:
        config = await load_config()

        # Initialise database and get connection
        db_connection = await init_database()

        try:
            app = TelraamLogger(config, db_connection)
            await app.run()
        finally:
            db_connection.close()

    except Exception as e:
        logger.error(f"Failed to start application: {e}")


if __name__ == "__main__":
    asyncio.run(main())
