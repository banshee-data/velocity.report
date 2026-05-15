package chart

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// OSMTileFetcher fetches and stitches OpenStreetMap raster tiles into a single
// PNG centred on a given (lat, lon) at a given zoom level. Tiles are cached
// on disk so repeated runs don't re-fetch.
type OSMTileFetcher struct {
	CacheDir  string
	UserAgent string
	Client    *http.Client
}

// NewOSMTileFetcher returns a fetcher with sensible defaults.
func NewOSMTileFetcher() *OSMTileFetcher {
	cache := filepath.Join(os.TempDir(), "velocity-osm-tiles")
	_ = os.MkdirAll(cache, 0o755)
	return &OSMTileFetcher{
		CacheDir:  cache,
		UserAgent: "velocity.report/1.0 (https://velocity.report)",
		Client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// latLonToTile converts (lat, lon, zoom) to fractional tile coordinates.
func latLonToTile(lat, lon float64, zoom int) (xf, yf float64) {
	n := math.Pow(2, float64(zoom))
	xf = (lon + 180.0) / 360.0 * n
	latRad := lat * math.Pi / 180.0
	yf = (1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n
	return
}

// FetchTile returns a single 256x256 OSM tile, using the on-disk cache when
// available.
func (f *OSMTileFetcher) FetchTile(z, x, y int) (image.Image, error) {
	cachePath := filepath.Join(f.CacheDir, fmt.Sprintf("%d_%d_%d.png", z, x, y))
	if data, err := os.ReadFile(cachePath); err == nil {
		return png.Decode(bytes.NewReader(data))
	}

	url := fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", z, x, y)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osm tile %d/%d/%d: HTTP %d", z, x, y, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(cachePath, body, 0o644)
	return png.Decode(bytes.NewReader(body))
}

// StitchTiles fetches a 3x3 grid of tiles centred on (lat, lon) at the given
// zoom and returns a 768x768 PNG with the centre point at the geometric
// centre of the image.
func (f *OSMTileFetcher) StitchTiles(lat, lon float64, zoom int) ([]byte, error) {
	xf, yf := latLonToTile(lat, lon, zoom)
	cx, cy := int(math.Floor(xf)), int(math.Floor(yf))

	const tileSize = 256
	const grid = 3
	half := grid / 2
	out := image.NewRGBA(image.Rect(0, 0, tileSize*grid, tileSize*grid))
	for dy := -half; dy <= half; dy++ {
		for dx := -half; dx <= half; dx++ {
			tile, err := f.FetchTile(zoom, cx+dx, cy+dy)
			if err != nil {
				return nil, err
			}
			r := image.Rect(
				(dx+half)*tileSize, (dy+half)*tileSize,
				(dx+half+1)*tileSize, (dy+half+1)*tileSize,
			)
			draw.Draw(out, r, tile, image.Point{0, 0}, draw.Src)
		}
	}

	// The centre lat/lon sits at fractional tile (xf, yf). The top-left of
	// the grid is at tile (cx-half, cy-half) = pixel (0, 0). So the centre
	// pixel is at:
	centreX := (xf - float64(cx-half)) * tileSize
	centreY := (yf - float64(cy-half)) * tileSize

	// Crop a square centred on the lat/lon. We want a final 600x600 image.
	const out2 = 600
	x0 := int(math.Round(centreX - out2/2))
	y0 := int(math.Round(centreY - out2/2))
	cropped := image.NewRGBA(image.Rect(0, 0, out2, out2))
	draw.Draw(cropped, cropped.Bounds(), out, image.Point{x0, y0}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, cropped); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
