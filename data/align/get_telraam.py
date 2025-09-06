#!/usr/bin/env python3

import logging
import sqlite3
import json
import asyncio
from typing import Dict, Any
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
                "Configuration file contains default values. Please update config_telraam.json with your actual UIProtect credentials."
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
    """Initialize SQLite database with Telraam table"""
    conn = sqlite3.connect(db_file)
    cursor = conn.cursor()

    cursor.execute(
        """
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
  """
    )

    conn.commit()
    logger.info("Database initialized successfully")
    return conn


class TelraamLogger:
    def __init__(self, config: Dict[str, Any], db_connection: sqlite3.Connection):
        self.config = config
        self.db_connection = db_connection
        self.client = None

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

    async def run(self):
        """Main application entry point"""
        try:
            # Check API authorization first
            if not await self.check_api_authorization():
                logger.error("API authorization check failed. Exiting.")
                return

            logger.info("API authorization successful, proceeding with application...")
            # Continue with the main logic of the application

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

        # Initialize database and get connection
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
