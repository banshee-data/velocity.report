#!/usr/bin/env python3

import asyncio
import json
import sqlite3
import logging
from datetime import datetime, timezone, timedelta
from pathlib import Path
from typing import Dict, Any
import aiofiles
from uiprotect import ProtectApiClient
from uiprotect.data import Event

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(filename)s:%(lineno)d - %(message)s",
)
logger = logging.getLogger(__name__)


async def load_config(
    config_file: str = "config.json",
) -> Dict[str, Any]:
    """Load configuration from JSON file"""
    # Define default/sample config in one place
    default_config = {
        "ip_address": "192.168.1.1",
        "port": 443,
        "username": "your_username",
        "password": "your_password",
        "verify_ssl": False,
    }

    try:
        async with aiofiles.open(config_file, "r") as f:
            content = await f.read()
            config = json.loads(content)

        # Check if config matches default values (user hasn't updated it)
        if all(config.get(key) == value for key, value in default_config.items()):
            logger.error(
                "Configuration file contains default values. Please update config.json with your actual UIProtect credentials."
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
    """Initialize SQLite database with notifications table"""
    conn = sqlite3.connect(db_file)
    cursor = conn.cursor()

    cursor.execute(
        """
    CREATE TABLE IF NOT EXISTS unifi (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT UNIQUE,
    timestamp DATETIME,
    ts_unix REAL,
    camera_name TEXT,
    event_type TEXT,
    score REAL,
    smart_detect_types TEXT,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )
  """
    )

    conn.commit()
    logger.info("Database initialized successfully")
    return conn


class UIProtectLogger:
    def __init__(self, config: Dict[str, Any], db_connection: sqlite3.Connection):
        self.config = config
        self.db_connection = db_connection
        self.client = None

    def log_notification(self, event: Event):
        """Log smart notification to database"""
        try:
            cursor = self.db_connection.cursor()

            # Extract smart detection types
            smart_types = []
            if hasattr(event, "smart_detect_types") and event.smart_detect_types:
                smart_types = [str(t) for t in event.smart_detect_types]

            logger.debug(f"Logging event to database: {event.id}")

            cursor.execute(
                """
    INSERT OR REPLACE INTO unifi
    (event_id, timestamp, ts_unix, camera_name, event_type, score, smart_detect_types, description)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    """,
                (
                    event.id,
                    (
                        event.start.isoformat()
                        if event.start
                        else datetime.now(timezone.utc).isoformat()
                    ),
                    (
                        event.start.timestamp()
                        if event.start
                        else datetime.now(timezone.utc).timestamp()
                    ),
                    event.camera.name if event.camera else "Unknown",
                    event.type.value if event.type else "Unknown",
                    getattr(event, "score", 0.0),
                    ",".join(smart_types),
                    f"{event.type.value} detected on {event.camera.name if event.camera else 'Unknown'}",
                ),
            )

            self.db_connection.commit()

            logger.info(
                f"Successfully logged notification: {event.type.value} on {event.camera.name if event.camera else 'Unknown'}"
            )

        except Exception as e:
            logger.error(f"Failed to log notification: {e}")
            logger.debug(f"Event details: {vars(event)}")

    async def connect_to_controller(self):
        """Connect to UIProtect controller"""
        try:
            self.client = ProtectApiClient(
                host=self.config["ip_address"],
                port=self.config.get("port", 443),
                username=self.config["username"],
                password=self.config["password"],
                verify_ssl=self.config.get("verify_ssl", False),
            )

            await self.client.update()
            logger.info(
                f"Connected to UIProtect controller at {self.config['ip_address']}"
            )

            # Log some basic info about the system
            if hasattr(self.client, "bootstrap") and self.client.bootstrap:
                cameras = getattr(self.client.bootstrap, "cameras", {})
                logger.info(f"Found {len(cameras)} cameras")
                for camera in cameras.values():
                    logger.debug(f"Camera: {camera.name} (ID: {camera.id})")
            else:
                logger.warning("No bootstrap data available")

        except Exception as e:
            logger.error(f"Failed to connect to controller: {e}")
            raise

    async def monitor_events(self):
        """Monitor and log smart notifications"""
        logger.info("Starting smart notification monitoring...")

        def event_callback(msg):
            # Log all websocket messages for debugging
            logger.debug(f"Received websocket message: {type(msg)} - {msg}")

            # The message should contain event data - extract it
            if hasattr(msg, "new_obj") and msg.new_obj is not None:
                event = msg.new_obj
                logger.info(f"Processing event: {type(event)} - {event}")

                # Check if this is an Event object and has the attributes we need
                if hasattr(event, "type") and hasattr(event, "camera"):
                    logger.info(
                        f"Found event: {event.type.value if event.type else 'Unknown'} on {event.camera.name if event.camera else 'Unknown'}"
                    )

                    # More permissive filtering - log all events for now
                    if event.type and event.camera:
                        logger.info(
                            f"Logging event: {event.type.value} on {event.camera.name}"
                        )
                        self.log_notification(event)
                    else:
                        logger.warning(f"Skipping event - missing type or camera info")
                else:
                    logger.debug(
                        f"Not an event object or missing attributes: {type(event)}"
                    )
            else:
                logger.debug(f"No new_obj in message or new_obj is None")

        # Subscribe to events
        unsub = self.client.subscribe_websocket(event_callback)

        try:
            # Keep monitoring
            while True:
                await asyncio.sleep(1)

        except KeyboardInterrupt:
            logger.info("Stopping monitoring...")
        finally:
            if unsub:
                unsub()

    async def get_events(self, start_date: datetime, end_date: datetime):
        """Get events from UIProtect controller for a specific date range"""
        try:
            if not self.client:
                logger.error(
                    "Client not connected. Call connect_to_controller() first."
                )
                return

            logger.info(f"Fetching events from {start_date} to {end_date}")

            # Get events using the uiprotect library's get_events method
            events = await self.client.get_events(start=start_date, end=end_date)

            logger.info(f"Retrieved {len(events)} events from controller")

            # Log each event to database
            for event in events:
                if (
                    hasattr(event, "type")
                    and hasattr(event, "camera")
                    and event.type
                    and event.camera
                ):
                    self.log_notification(event)

        except Exception as e:
            logger.error(f"Failed to get events: {e}")

    def get_date_range(self) -> tuple[datetime, datetime]:
        """Get date range for fetching events. Start from 2025-04-01 or latest DB entry, end at now."""
        end_date = datetime.now(timezone.utc)
        default_start = datetime(2025, 4, 1, tzinfo=timezone.utc)

        try:
            cursor = self.db_connection.cursor()
            cursor.execute("SELECT MAX(timestamp) FROM unifi")
            result = cursor.fetchone()

            if result[0] is not None:
                # Parse the timestamp and add 1 second
                latest_timestamp = datetime.fromisoformat(result[0])
                # Ensure it's timezone-aware
                if latest_timestamp.tzinfo is None:
                    latest_timestamp = latest_timestamp.replace(tzinfo=timezone.utc)
                start_date = latest_timestamp + timedelta(seconds=1)
            else:
                start_date = default_start

        except Exception as e:
            logger.warning(f"Failed to get latest timestamp from database: {e}")
            start_date = default_start

        # Ensure start_date is not after end_date
        if start_date > end_date:
            start_date = end_date

        return start_date, end_date

    async def run(self):
        """Main application entry point"""
        try:
            # Connect to controller
            await self.connect_to_controller()

            # Start monitoring
            # await self.monitor_events()

            start_date, end_date = self.get_date_range()

            current_start = start_date
            while current_start < end_date:
                current_end = min(current_start + timedelta(weeks=1), end_date)
                logger.info(
                    f"Fetching events for week: {current_start.strftime('%Y-%m-%d')} to {current_end.strftime('%Y-%m-%d')}"
                )
                await self.get_events(current_start, current_end)
                current_start = current_end

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
            app = UIProtectLogger(config, db_connection)
            await app.run()
        finally:
            db_connection.close()

    except Exception as e:
        logger.error(f"Failed to start application: {e}")


if __name__ == "__main__":
    asyncio.run(main())
