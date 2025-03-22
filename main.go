package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	// "regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	_ "github.com/marcboeker/go-duckdb"
	"go.bug.st/serial.v1"
)

// Constants
const DB_FILE = "sensor_data.db"
const SCHEMA_VERSION = "0.0.2"

// Global Variables
var commandID int
var lineCounter int = 0

func initializeDatabase() {
	db, err := sql.Open("duckdb", DB_FILE)
	if err != nil {
		log.Fatalf("Failed to open DuckDB database: %v", err)
	}
	defer db.Close()

	// Create meta table if it doesn't exist
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS meta (version TEXT)")
	if err != nil {
		log.Fatalf("Failed to create meta table: %v", err)
	}

	// Check version
	var existingVersion string
	err = db.QueryRow("SELECT version FROM meta LIMIT 1").Scan(&existingVersion)
	if err != nil {
		// If meta table is empty, insert schema version
		_, _ = db.Exec("INSERT INTO meta (version) VALUES (?)", SCHEMA_VERSION)
	} else if existingVersion != SCHEMA_VERSION {
		log.Println("Schema version mismatch. Archiving old database...")
		archiveExistingDatabase()
		createDatabaseSchema(db)
		return
	}

	// Ensure all tables exist
	createDatabaseSchema(db)

	// Initialize commandID, select max from commands table or set to 0 if empty
	err = db.QueryRow("SELECT COALESCE(MAX(command_id), 0) FROM commands").Scan(&commandID)
	if err != nil {
		log.Fatalf("Failed to initialize commandID: %v", err)
	}
	log.Printf("Initialized commandID: %d", commandID)
	log.Println("Database initialized successfully.")
}

func createDatabaseSchema(db *sql.DB) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS meta (version TEXT);
		CREATE TABLE IF NOT EXISTS data (
			uptime DOUBLE,
			magnitude DOUBLE, 
			speed DOUBLE, 
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS commands (
			command_id BIGINT PRIMARY KEY, 
			command TEXT, 
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS log (
			log_id BIGINT PRIMARY KEY, 
			command_id BIGINT, 
			log_data TEXT, 
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP, 
			FOREIGN KEY(command_id) REFERENCES commands(command_id)
		);
	`)

	if err != nil {
		log.Fatalf("Failed to create database schema: %v", err)
	}
}

// Archive old DuckDB database
func archiveExistingDatabase() {
	timestamp := time.Now().Format("20060102")
	iteration := 0
	var backupFile string

	for {
		backupFile = fmt.Sprintf("backup/sensor_data_archive_%s_%d.db", timestamp, iteration)
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			break
		}
		iteration++
	}

	os.Rename(DB_FILE, backupFile)
	fmt.Println("Archived old database:", backupFile)
}

func serialReader(portName string, baudRate int) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		Parity:   serial.NoParity,   // No parity bit ✅
		DataBits: 8,                 // 8-bit data ✅
		StopBits: serial.OneStopBit, // 1 stop bit ✅
	}
	port, err := serial.Open(portName, mode)
	if err != nil {
		log.Fatalf("❌ Failed to open serial port: %v", err)
	}
	defer port.Close()

	var buffer strings.Builder
	buf := make([]byte, 256)

	for {
		n, err := port.Read(buf)
		if err != nil {
			if strings.Contains(err.Error(), "interrupted system call") {
				// Retry immediately — no delay, no log spam
				continue
			}
			// Real error — log and wait briefly
			log.Printf("❌ Serial read error: %v", err)
			time.Sleep(time.Millisecond * 100) // Brief delay before retrying ✅
			continue
		}

		// Append bytes to buffer
		buffer.Write(buf[:n])
		data := buffer.String()

		// Split into complete lines
		lines := strings.Split(data, "\n")
		if !strings.HasSuffix(data, "\n") {
			// Incomplete line — keep it for next read
			buffer.Reset()
			buffer.WriteString(lines[len(lines)-1])
			lines = lines[:len(lines)-1]
		} else {
			buffer.Reset()
		}

		// Process full lines
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				log.Printf("🔍 Full Serial Line: [%s]", line)
				processData(line)
			}
		}
	}
}

func processData(line string) {
	line = strings.TrimSpace(line)

	// Skip empty lines
	if len(line) == 0 {
		return
	}

	// If it looks like JSON, parse it
	if strings.HasPrefix(line, "{") {
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &jsonData); err == nil {
			logJSONResponse(jsonData)
			return
		}
	}

	// Ensure the data is in the expected "uptime, magnitude, speed" format
	if !isValidSensorData(line) {
		log.Println("Invalid sensor data format:", line)
		return
	}

	// Parse the valid sensor data
	uptime, magnitude, speed := parseSensorData(line)
	lineCounter++
	log.Printf("✅ [%d] Stored: uptime=%.3f, magnitude=%.3f, speed=%.3f", lineCounter, uptime, magnitude, speed)
	storeSensorData(uptime, magnitude, speed)
}

func isValidSensorData(line string) bool {
	// Remove any leading/trailing brackets
	line = strings.Trim(line, "[]")

	parts := strings.Split(line, ",")
	if len(parts) != 3 {
		log.Println("❌ Invalid sensor data format (expected 3 parts):", line)
		return false
	}

	// Ensure all 3 parts are floats
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if _, err := strconv.ParseFloat(part, 64); err != nil {
			log.Println("❌ Invalid float in data:", part)
			return false
		}
	}

	return true
}

// Parse sensor data into magnitude and speed
func parseSensorData(line string) (float64, float64, float64) {
	line = strings.Trim(line, "[]")
	parts := strings.Split(line, ",")

	uptime, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	magnitude, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	speed, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)

	return uptime, magnitude, speed
}

// Store sensor data in DuckDB
func storeSensorData(uptime, magnitude, speed float64) {
	db, err := sql.Open("duckdb", DB_FILE)
	if err != nil {
		log.Println("Failed to connect to database:", err)
		return
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO data (uptime, magnitude, speed) VALUES (?, ?, ?)", uptime, magnitude, speed)
	if err != nil {
		log.Println("Failed to insert sensor data:", err)
	}
}

// Log JSON responses
func logJSONResponse(data map[string]interface{}) {
	db, err := sql.Open("duckdb", DB_FILE)
	if err != nil {
		log.Println("Failed to connect to database:", err)
		return
	}
	defer db.Close()

	jsonStr, _ := json.Marshal(data)
	_, err = db.Exec("INSERT INTO log (command_id, log_data) VALUES (?, ?)", commandID, string(jsonStr))
	if err != nil {
		log.Println("Failed to insert log data:", err)
	}
}

// Send commands to sensor
func sendCommand(command string) {
	db, err := sql.Open("duckdb", DB_FILE)
	if err != nil {
		log.Println("Failed to connect to database:", err)
		return
	}
	defer db.Close()

	commandID++
	_, err = db.Exec("INSERT INTO commands (command_id, command) VALUES (?, ?)", commandID, command)
	if err != nil {
		log.Println("Failed to insert command:", err)
	}
}

// Scheduled Jobs
func scheduleJobs() {
	s := gocron.NewScheduler(time.UTC)

	s.Every(1).Hour().Do(backupDatabase)
	s.Every(30).Minutes().Do(createReportingTable)

	s.StartAsync()
}

func backupDatabase() {
	// Ensure backup directory exists
	if _, err := os.Stat("backup"); os.IsNotExist(err) {
		os.Mkdir("backup", os.ModePerm)
	}

	db, _ := sql.Open("duckdb", DB_FILE)
	defer db.Close()

	timestamp := time.Now().Format("2006-01-02_150405")
	backupFile := fmt.Sprintf("backup/sensor_data_backup_%s.parquet", timestamp)
	_, err := db.Exec(fmt.Sprintf(`
		COPY (
			SELECT * 
			FROM data 
			WHERE timestamp >= NOW() - INTERVAL '70 MINUTE'
		) TO '%s' (FORMAT 'parquet')`, backupFile))
	if err != nil {
		log.Println("Backup failed:", err)
	}
}

func createReportingTable() {
	db, _ := sql.Open("duckdb", DB_FILE)
	defer db.Close()

	// Ensure data table exists before running the report
	_, err := db.Exec("SELECT 1 FROM data LIMIT 1")
	if err != nil {
		log.Println("Skipping report creation: 'data' table does not exist yet.")
		return
	}

	_, err = db.Exec(`
		CREATE OR REPLACE TABLE report AS 
		SELECT AVG(magnitude) AS avg_magnitude, 
		       AVG(speed) AS avg_speed, 
		       COUNT(*) AS total_records 
		FROM data;
	`)
	if err != nil {
		log.Println("Failed to create reporting table:", err)
	}
}

// API Server
func setupAPI() {
	router := gin.Default()

	router.GET("/logs", func(c *gin.Context) {
		db, _ := sql.Open("duckdb", DB_FILE)
		defer db.Close()

		rows, _ := db.Query("SELECT * FROM log")
		defer rows.Close()

		var logs []map[string]interface{}
		for rows.Next() {
			var id int
			var cmdID int
			var logData string
			var timestamp string
			rows.Scan(&id, &cmdID, &logData, &timestamp)
			logs = append(logs, map[string]interface{}{"id": id, "cmd_id": cmdID, "log_data": logData, "timestamp": timestamp})
		}
		c.JSON(200, logs)
	})

	router.POST("/query", func(c *gin.Context) {
		db, err := sql.Open("duckdb", DB_FILE)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to connect to database"})
			return
		}
		defer db.Close()

		command := c.PostForm("sql")
		if command == "" {
			c.JSON(400, gin.H{"error": "No command provided"})
			return
		}

		rows, err := db.Query(command)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to execute SQL query", "details": err.Error()})
			return
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to get columns", "details": err.Error()})
			return
		}

		results := []map[string]interface{}{}
		for rows.Next() {
			row := make([]interface{}, len(columns))
			rowPtrs := make([]interface{}, len(columns))
			for i := range row {
				rowPtrs[i] = &row[i]
			}

			if err := rows.Scan(rowPtrs...); err != nil {
				c.JSON(500, gin.H{"error": "Failed to scan row", "details": err.Error()})
				return
			}

			result := map[string]interface{}{}
			for i, col := range columns {
				result[col] = row[i]
			}
			results = append(results, result)
		}

		c.JSON(200, results)
	})

	router.Run(":8000")
}

// Main
func main() {
	initializeDatabase()
	go serialReader("/dev/ttySC1", 19200)
	go scheduleJobs()
	setupAPI()
}
