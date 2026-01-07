#!/bin/bash
# ensure-web-stub.sh - Generate stub web/build/index.html if it doesn't exist
# This allows Go embed to work on fresh clones without tracking generated files

set -e

WEB_BUILD_DIR="web/build"
STUB_FILE="$WEB_BUILD_DIR/index.html"

# If index.html already exists (from actual build or previous stub), do nothing
if [ -f "$STUB_FILE" ]; then
    exit 0
fi

# Create directory if it doesn't exist
mkdir -p "$WEB_BUILD_DIR"

# Generate stub file
cat > "$STUB_FILE" << 'HTMLEOF'
<!DOCTYPE html>
<html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>🚨 Web Assets Not Built : velocity.report</title>

        <style>
        html {
            height: 100%;
            margin: 0;
            padding: 0;
            background-color: #01241a;
            overscroll-behavior: none;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            padding: 20px;
            box-sizing: border-box;
            background: linear-gradient(135deg, #047857 0%, #01241a 100%) no-repeat center center fixed;
            background-size: cover;
            overscroll-behavior: none;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            max-width: 600px;
            padding: 40px;
            text-align: center;
        }
        h1 {
            color: #333;
            margin-top: 0;
            font-size: 28px;
        }
        .warning {
            background: #fff3cd;
            border: 2px solid #ffc107;
            border-radius: 8px;
            padding: 20px;
            margin: 20px 0;
        }
        .warning-icon {
            font-size: 48px;
            margin-bottom: 10px;
        }
        code {
            background: #f4f4f4;
            border: 1px solid #ddd;
            border-radius: 4px;
            padding: 2px 6px;
            font-family: "Courier New", monospace;
            font-size: 14px;
            color: #047857;
        }
        .command-box {
            background: #2d2d2d;
            color: #f8f8f2;
            border-radius: 6px;
            padding: 15px;
            margin: 20px 0;
            text-align: left;
            font-family: "Courier New", monospace;
            font-size: 14px;
            overflow-x: auto;
        }
        .command-box .prompt {
            color: #047857;
        }
        .info {
            color: #666;
            font-size: 14px;
            margin-top: 20px;
        }
        a {
            color: #0ea5e9;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        .container p.left-align {
            text-align: left;
            margin: 0 0 12px 0;
        }
        .local-services {
            margin-top: 30px;
            border-top: 1px solid #eee;
            padding-top: 20px;
        }
        .local-services h3 {
            margin-top: 0;
            color: #333;
            font-size: 1.2em;
            margin-bottom: 15px;
        }
        .local-services p {
            text-align: left;
            margin-bottom: 15px;
            color: #555;
        }
        .local-services ul {
            text-align: left;
            padding-left: 20px;
            margin: 0;
        }
        .local-services li {
            margin-bottom: 8px;
            color: #555;
        }
    </style>
    </head>
    <body>
        <div class="container">
            <h1>🚧 Under Construction 🚧</h1>
            <h2>Web Frontend Not Built</h2>
            <div class="warning">
                <div class="warning-icon">⚠️</div>
                <p><strong>The web frontend has not been built yet</strong></p>
                <p>This is a stub file to allow Go compilation to succeed</p>
            </div>

            <h2>How to Build the Web Frontend</h2>
            <p class="left-align">From the repo root run:</p>
            <div class="command-box">
                <span class="prompt">$</span> make build-web
            </div>

            <p class="left-align">Or manually from the web directory:</p>
            <div class="command-box">
                <span class="prompt">$</span> cd web && pnpm run build
            </div>

            <div class="info">
                <p>For more info, see the <a
                        href="https://github.com/banshee-data/velocity.report">velocity.report
                        repo</a></p>
            </div>

            <div class="local-services">
                <h3>Local Service Catalog</h3>
                <p>APIs are also available locally:</p>
                <ul>
                    <li><a href="http://localhost:8080/debug/">Radar Debug
                            tools</a></li>
                    <li><a href="http://localhost:8080/debug/tail">Debug: Serial
                            Log Tail</a></li>
                    <li><a href="http://localhost:8080/debug/tailsql/">Debug: SQL
                            Interface</a></li>
                    <li><a href="http://localhost:8080/api/config">API:
                            Configuration</a></li>
                    <li><a href="http://localhost:8081">Lidar Dashboard</a>
                        (Port 8081)</li>
                </ul>
            </div>
        </div>
    </body>
</html>
HTMLEOF

echo "Generated stub file: $STUB_FILE"
