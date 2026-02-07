// Package monitor provides HTTP client operations for LiDAR monitoring endpoints.
package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"path/filepath"
	"time"
)

// Client provides HTTP operations for LiDAR monitoring endpoints.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	SensorID   string
}

// NewClient creates a new monitoring client.
func NewClient(httpClient *http.Client, baseURL, sensorID string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		HTTPClient: httpClient,
		BaseURL:    baseURL,
		SensorID:   sensorID,
	}
}

// StartPCAPReplay requests a PCAP replay for the sensor.
// It retries on 409 (conflict) responses for up to maxRetries times.
func (c *Client) StartPCAPReplay(pcapFile string, maxRetries int) error {
	url := fmt.Sprintf("%s/api/lidar/pcap/start?sensor_id=%s", c.BaseURL, c.SensorID)
	payload := map[string]string{"pcap_file": filepath.Clean(pcapFile)}
	data, _ := json.Marshal(payload)

	log.Printf("Requesting PCAP replay for sensor %s: %s", c.SensorID, payload["pcap_file"])

	if maxRetries <= 0 {
		maxRetries = 60
	}

	for retry := 0; retry < maxRetries; retry++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusConflict {
			if retry == 0 {
				log.Printf("PCAP replay in progress, waiting...")
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("timeout waiting for PCAP replay slot")
}

// DefaultBuckets returns the default bucket configuration.
func DefaultBuckets() []string {
	return []string{"1", "2", "4", "8", "10", "12", "16", "20", "50", "100", "200"}
}

// FetchBuckets retrieves the bucket configuration from the server.
// Returns default buckets on error.
func (c *Client) FetchBuckets() []string {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/lidar/acceptance?sensor_id=%s", c.BaseURL, c.SensorID))
	if err != nil {
		log.Printf("WARNING: Could not fetch buckets: %v (using defaults)", err)
		return DefaultBuckets()
	}
	defer resp.Body.Close()

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return DefaultBuckets()
	}

	bm, ok := m["BucketsMeters"].([]interface{})
	if !ok || len(bm) == 0 {
		return DefaultBuckets()
	}

	// Validate bucket count to prevent excessive memory allocation
	const maxBuckets = 100
	if len(bm) > maxBuckets {
		log.Printf("WARNING: Bucket count %d exceeds maximum %d, using defaults", len(bm), maxBuckets)
		return DefaultBuckets()
	}

	buckets := make([]string, 0, len(bm))
	for _, bi := range bm {
		switch v := bi.(type) {
		case float64:
			if v == math.Trunc(v) {
				buckets = append(buckets, fmt.Sprintf("%.0f", v))
			} else {
				buckets = append(buckets, fmt.Sprintf("%.6f", v))
			}
		default:
			buckets = append(buckets, fmt.Sprintf("%v", v))
		}
	}
	return buckets
}

// ResetGrid resets the background grid for the sensor.
func (c *Client) ResetGrid() error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/lidar/grid_reset?sensor_id=%s", c.BaseURL, c.SensorID), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// BackgroundParams holds the parameters for the background model.
type BackgroundParams struct {
	NoiseRelative              float64 `json:"noise_relative"`
	ClosenessMultiplier        float64 `json:"closeness_multiplier"`
	NeighbourConfirmationCount int     `json:"neighbor_confirmation_count"`
	SeedFromFirstFrame         bool    `json:"seed_from_first_frame"`
}

// SetParams sets the background model parameters.
func (c *Client) SetParams(params BackgroundParams) error {
	data, _ := json.Marshal(params)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/lidar/params?sensor_id=%s", c.BaseURL, c.SensorID), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Applied: noise=%.4f, closeness=%.2f, neighbour=%d, seed=%v",
		params.NoiseRelative, params.ClosenessMultiplier,
		params.NeighbourConfirmationCount, params.SeedFromFirstFrame)
	return nil
}

// SetTuningParams sends a partial tuning config update to /api/lidar/params.
// The params map can contain any TuningConfig field names with their values.
func (c *Client) SetTuningParams(params map[string]interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal tuning params: %w", err)
	}

	url := fmt.Sprintf("%s/api/lidar/params?sensor_id=%s", c.BaseURL, c.SensorID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Applied tuning params: %s", string(data))
	return nil
}

// PCAPReplayConfig holds configuration for starting a PCAP replay.
type PCAPReplayConfig struct {
	PCAPFile        string
	StartSeconds    float64
	DurationSeconds float64
	MaxRetries      int
	AnalysisMode    bool // When true, preserve grid after PCAP completion
}

// StartPCAPReplayWithConfig requests a PCAP replay with extended configuration.
func (c *Client) StartPCAPReplayWithConfig(cfg PCAPReplayConfig) error {
	url := fmt.Sprintf("%s/api/lidar/pcap/start?sensor_id=%s", c.BaseURL, c.SensorID)
	payload := map[string]interface{}{
		"pcap_file": filepath.Clean(cfg.PCAPFile),
	}
	if cfg.StartSeconds > 0 {
		payload["start_seconds"] = cfg.StartSeconds
	}
	if cfg.DurationSeconds != 0 {
		payload["duration_seconds"] = cfg.DurationSeconds
	}
	if cfg.AnalysisMode {
		payload["analysis_mode"] = true
	}
	data, _ := json.Marshal(payload)

	log.Printf("Requesting PCAP replay for sensor %s: file=%s start=%.1fs duration=%.1fs",
		c.SensorID, cfg.PCAPFile, cfg.StartSeconds, cfg.DurationSeconds)

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 60
	}

	for retry := 0; retry < maxRetries; retry++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusConflict {
			if retry == 0 {
				log.Printf("PCAP replay in progress, waiting...")
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("timeout waiting for PCAP replay slot")
}

// StopPCAPReplay stops any running PCAP replay for this sensor.
func (c *Client) StopPCAPReplay() error {
	url := fmt.Sprintf("%s/api/lidar/pcap/stop?sensor_id=%s", c.BaseURL, c.SensorID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Stopped PCAP replay for sensor %s", c.SensorID)
	return nil
}

// WaitForPCAPComplete polls the data source status until the PCAP replay finishes.
// Returns nil when the PCAP is no longer in progress, or an error on timeout.
func (c *Client) WaitForPCAPComplete(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/lidar/data_source?sensor_id=%s", c.BaseURL, c.SensorID))
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var ds map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&ds) == nil {
			if inProgress, ok := ds["pcap_in_progress"].(bool); ok && !inProgress {
				resp.Body.Close()
				return nil
			}
		}
		resp.Body.Close()
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for PCAP to complete")
}

// ResetAcceptance resets the acceptance counters.
func (c *Client) ResetAcceptance() error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/lidar/acceptance/reset?sensor_id=%s", c.BaseURL, c.SensorID), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// WaitForGridSettle waits for the grid to have at least one non-zero cell.
func (c *Client) WaitForGridSettle(timeout time.Duration) {
	if timeout <= 0 {
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/lidar/grid_status?sensor_id=%s", c.BaseURL, c.SensorID))
		if err == nil {
			var gs map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&gs) == nil {
				if bc, ok := gs["background_count"]; ok {
					if n, ok := bc.(float64); ok && n > 0 {
						resp.Body.Close()
						return
					}
				}
			}
			resp.Body.Close()
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// FetchAcceptanceMetrics fetches acceptance metrics from the server.
func (c *Client) FetchAcceptanceMetrics() (map[string]interface{}, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/lidar/acceptance?sensor_id=%s", c.BaseURL, c.SensorID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// FetchGridStatus fetches the grid status from the server.
func (c *Client) FetchGridStatus() (map[string]interface{}, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/lidar/grid_status?sensor_id=%s", c.BaseURL, c.SensorID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var gs map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&gs); err != nil {
		return nil, err
	}
	return gs, nil
}

// FetchTrackingMetrics fetches velocity-trail alignment metrics from the server.
// Used by the sweep tool to evaluate tracking parameter quality.
func (c *Client) FetchTrackingMetrics() (map[string]interface{}, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/api/lidar/tracks/metrics?include_per_track=false", c.BaseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tracking metrics returned %d: %s", resp.StatusCode, string(body))
	}

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// TrackingParams holds tracker configuration values for sweep operations.
// Only non-nil fields will be updated on the server.
type TrackingParams struct {
	GatingDistanceSquared *float64 `json:"gating_distance_squared,omitempty"`
	ProcessNoisePos       *float64 `json:"process_noise_pos,omitempty"`
	ProcessNoiseVel       *float64 `json:"process_noise_vel,omitempty"`
	MeasurementNoise      *float64 `json:"measurement_noise,omitempty"`
}

// SetTrackerConfig updates tracker configuration on the server via the consolidated /api/lidar/params endpoint.
func (c *Client) SetTrackerConfig(params TrackingParams) error {
	url := fmt.Sprintf("%s/api/lidar/params?sensor_id=%s", c.BaseURL, c.SensorID)
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal tracker config: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set tracker config returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
