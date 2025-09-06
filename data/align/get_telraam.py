#!/usr/bin/env python3

import logging
import sqlite3
import json
import asyncio
from typing import Dict, Any
import aiofiles

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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
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

    async def run(self):
        """Main application entry point"""
        try:
            pass

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
