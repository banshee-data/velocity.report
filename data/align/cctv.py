import asyncio
import json
import sqlite3
import logging
from datetime import datetime
from pathlib import Path
from typing import Dict, Any
import aiofiles
from uiprotect import ProtectApiClient
from uiprotect.data import Event

# Configure logging
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
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


async def init_database(db_file: str = "ui.db") -> sqlite3.Connection:
    """Initialize SQLite database with notifications table"""
    conn = sqlite3.connect(db_file)
    cursor = conn.cursor()

    cursor.execute(
        """
    CREATE TABLE IF NOT EXISTS smart_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT UNIQUE,
    timestamp DATETIME,
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

            cursor.execute(
                """
    INSERT OR REPLACE INTO smart_notifications
    (event_id, timestamp, camera_name, event_type, score, smart_detect_types, description)
    VALUES (?, ?, ?, ?, ?, ?, ?)
    """,
                (
                    event.id,
                    (
                        event.start.isoformat()
                        if event.start
                        else datetime.now().isoformat()
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
                f"Logged notification: {event.type.value} on {event.camera.name if event.camera else 'Unknown'}"
            )

        except Exception as e:
            logger.error(f"Failed to log notification: {e}")

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

        except Exception as e:
            logger.error(f"Failed to connect to controller: {e}")
            raise

    async def monitor_events(self):
        """Monitor and log smart notifications"""
        logger.info("Starting smart notification monitoring...")

        def event_callback(event: Event):
            # Filter for smart detection events
            if (
                hasattr(event, "smart_detect_types")
                and event.smart_detect_types
                and len(event.smart_detect_types) > 0
            ):
                self.log_notification(event)

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

    async def run(self):
        """Main application entry point"""
        try:
            # Connect to controller
            await self.connect_to_controller()

            # Start monitoring
            await self.monitor_events()

        except Exception as e:
            logger.error(f"Application error: {e}")
        finally:
            if self.client:
                await self.client.close()


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
