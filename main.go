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
const SCHEMA_VERSION = "0.0.1"

// Global Variables
var commandID int

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
}

func createDatabaseSchema(db *sql.DB) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS meta (version TEXT);
		CREATE TABLE IF NOT EXISTS data (
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
		backupFile = fmt.Sprintf("backup/sensor_data_backup_%s_%d.db", timestamp, iteration)
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

	buf := make([]byte, 256) // Increased buffer size ✅

	for {
		n, err := port.Read(buf)
		if err != nil {
			if strings.Contains(err.Error(), "interrupted system call") {
				log.Println("⚠️ Serial read interrupted, retrying...")
				continue
			}
			log.Println("❌ Serial read error:", err)
			time.Sleep(time.Millisecond * 100) // Brief delay before retrying ✅
			continue
		}

		data := strings.TrimSpace(string(buf[:n]))
		log.Printf("🔍 Raw Serial Data: [%s]", data)

		processData(data)
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

	// Ensure the data is in the expected "magnitude, speed" format
	if !isValidSensorData(line) {
		log.Println("Invalid sensor data format:", line)
		return
	}

	// Parse the valid sensor data
	magnitude, speed := parseSensorData(line)
	storeSensorData(magnitude, speed)
}

func isValidSensorData(line string) bool {
	// Ensure exactly two float values separated by a comma
	parts := strings.Split(line, ",")
	if len(parts) != 2 {
		log.Println("Invalid sensor data format (wrong number of parts):", line)
		return false
	}

	// Check if both parts are valid floating-point numbers
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if _, err := strconv.ParseFloat(part, 64); err != nil {
			log.Println("Invalid sensor data format (not a valid float):", line)
			return false
		}
	}

	return true
}

// Parse sensor data into magnitude and speed
func parseSensorData(line string) (float64, float64) {
	var magnitude, speed float64
	fmt.Sscanf(line, "%f, %f", &magnitude, &speed)
	return magnitude, speed
}

// Store sensor data in DuckDB
func storeSensorData(magnitude, speed float64) {
	db, err := sql.Open("duckdb", DB_FILE)
	if err != nil {
		log.Println("Failed to connect to database:", err)
		return
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO data (magnitude, speed) VALUES (?, ?)", magnitude, speed)
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

	_, err := db.Exec("COPY data TO 'backup/sensor_data_backup.parquet' (FORMAT 'parquet')")
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

	router.Run(":8000")
}

// Main
func main() {
	initializeDatabase()
	go serialReader("/dev/ttySC1", 115200)
	go scheduleJobs()
	setupAPI()
}
