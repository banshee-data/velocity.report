/* regions_dashboard.js — extracted from regions_dashboard.html for testability. */

var canvas = null;
var ctx = null;
var tooltip = null;
var regionData = null;
var rings = 40;
var azimuthBins = 1800;
var selectedRegionId = null;

// Color palette for regions
var regionColors = [
  "#e74c3c",
  "#3498db",
  "#2ecc71",
  "#f39c12",
  "#9b59b6",
  "#1abc9c",
  "#e67e22",
  "#34495e",
  "#16a085",
  "#c0392b",
  "#8e44ad",
  "#2980b9",
  "#27ae60",
  "#f1c40f",
  "#d35400",
  "#7f8c8d",
  "#95a5a6",
  "#bdc3c7",
  "#ecf0f1",
  "#e8daef",
];

var sensorId = null;

// Compute which grid cell (ring, azBin) corresponds to a canvas pixel position.
// Returns { ring, azBin, cellIdx } or null if out of bounds.
function cellAtPixel(canvasX, canvasY, canvasWidth, canvasHeight) {
  var azBin = Math.floor(canvasX / (canvasWidth / azimuthBins));
  var ring = Math.floor(canvasY / (canvasHeight / rings));
  if (azBin < 0 || azBin >= azimuthBins || ring < 0 || ring >= rings)
    return null;
  return { ring: ring, azBin: azBin, cellIdx: ring * azimuthBins + azBin };
}

function loadRegions() {
  fetch(
    "/debug/lidar/background/regions?sensor_id=" + encodeURIComponent(sensorId),
  )
    .then(function (response) {
      return response.json();
    })
    .then(function (data) {
      regionData = data;
      updateInfoPanel(data);
      drawRegions(data);
    })
    .catch(function (error) {
      console.error("Error loading regions:", error);
      document.getElementById("status").textContent = "Error loading";
      document.getElementById("status").className = "status status-pending";
    });
}

function updateInfoPanel(data) {
  document.getElementById("sensorId").textContent = data.sensor_id;
  document.getElementById("regionCount").textContent = data.region_count;
  document.getElementById("framesSampled").textContent = data.frames_sampled;

  var statusEl = document.getElementById("status");
  if (data.identification_complete) {
    statusEl.textContent = "Complete";
    statusEl.className = "status status-complete";
  } else {
    statusEl.textContent = "Collecting...";
    statusEl.className = "status status-pending";
  }
}

function drawRegions(data) {
  if (!data.grid_mapping || data.grid_mapping.length === 0) {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = "#666";
    ctx.font = "16px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(
      "No region data available",
      canvas.width / 2,
      canvas.height / 2,
    );
    return;
  }

  // Clear canvas
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  // Calculate cell size
  var cellWidth = canvas.width / azimuthBins;
  var cellHeight = canvas.height / rings;

  // Draw grid with regions
  for (var ring = 0; ring < rings; ring++) {
    for (var azBin = 0; azBin < azimuthBins; azBin++) {
      var cellIdx = ring * azimuthBins + azBin;
      var regionId = data.grid_mapping[cellIdx];

      if (regionId >= 0) {
        var x = azBin * cellWidth;
        var y = ring * cellHeight;

        ctx.fillStyle = regionColors[regionId % regionColors.length];
        ctx.fillRect(x, y, cellWidth, cellHeight);
      }
    }
  }

  // Draw grid lines (sparse for performance)
  ctx.strokeStyle = "rgba(0, 0, 0, 0.05)";
  ctx.lineWidth = 0.5;

  // Draw horizontal lines (every 5 rings)
  for (var ring = 0; ring <= rings; ring += 5) {
    var y = ring * cellHeight;
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(canvas.width, y);
    ctx.stroke();
  }

  // Update legend
  updateLegend(data);

  // Draw selected region outline if any
  if (selectedRegionId !== null) {
    drawRegionOutline(selectedRegionId);
  }
}

function updateLegend(data) {
  var legendItems = document.getElementById("legendItems");
  legendItems.innerHTML = "";

  if (!data.regions || data.regions.length === 0) {
    legendItems.innerHTML = "<div>No regions identified yet</div>";
    return;
  }

  data.regions.forEach(function (region, idx) {
    var item = document.createElement("div");
    item.className = "legend-item";
    item.style.cursor = "pointer";
    item.onclick = function () {
      selectRegion(idx);
    };

    var colorBox = document.createElement("div");
    colorBox.className = "legend-color";
    colorBox.style.backgroundColor = regionColors[idx % regionColors.length];

    var label = document.createElement("span");
    var variance = (region.mean_variance || 0).toFixed(3);
    var noise = (
      (region.params && region.params.noise_relative_fraction) ||
      0
    ).toFixed(3);
    var cellCount = region.cell_count || 0;
    label.textContent =
      "Region " +
      idx +
      " (var=" +
      variance +
      ", noise=" +
      noise +
      ", " +
      cellCount +
      " cells)";

    item.appendChild(colorBox);
    item.appendChild(label);
    legendItems.appendChild(item);
  });
}

function selectRegion(regionId) {
  selectedRegionId = selectedRegionId === regionId ? null : regionId;
  drawRegions(regionData);
}

function drawRegionOutline(regionId) {
  if (!regionData || !regionData.grid_mapping) return;

  ctx.strokeStyle = "#FFD700";
  ctx.lineWidth = 3;

  var cellWidth = canvas.width / azimuthBins;
  var cellHeight = canvas.height / rings;

  for (var ring = 0; ring < rings; ring++) {
    for (var azBin = 0; azBin < azimuthBins; azBin++) {
      var cellIdx = ring * azimuthBins + azBin;
      if (regionData.grid_mapping[cellIdx] === regionId) {
        var x = azBin * cellWidth;
        var y = ring * cellHeight;
        ctx.strokeRect(x, y, cellWidth, cellHeight);
      }
    }
  }
}

// Auto-refresh every 5 seconds when not complete
function autoRefresh() {
  loadRegions();
  if (!regionData || !regionData.identification_complete) {
    setTimeout(autoRefresh, 5000);
  } else {
    setTimeout(autoRefresh, 30000); // Slower refresh when complete
  }
}

// ---- CommonJS exports for testing ----
if (typeof module !== "undefined" && module.exports) {
  module.exports = {
    regionColors: regionColors,
    cellAtPixel: cellAtPixel,
  };
}

// ---- Page initialization (runs only in browser, not when required by Jest) ----
if (typeof document !== "undefined" && typeof module === "undefined") {
  canvas = document.getElementById("regionCanvas");
  ctx = canvas.getContext("2d");
  tooltip = document.getElementById("tooltip");
  sensorId = document.querySelector('meta[name="sensor-id"]').content;

  // Mouse hover to show region details
  canvas.addEventListener("mousemove", function (e) {
    if (!regionData || !regionData.grid_mapping) return;

    var rect = canvas.getBoundingClientRect();
    var x = e.clientX - rect.left;
    var y = e.clientY - rect.top;

    // Use actual rendered dimensions for calculations
    var scaleX = canvas.width / rect.width;
    var scaleY = canvas.height / rect.height;
    var canvasX = x * scaleX;
    var canvasY = y * scaleY;

    var cell = cellAtPixel(canvasX, canvasY, canvas.width, canvas.height);
    if (cell) {
      var regionId = regionData.grid_mapping[cell.cellIdx];

      if (regionId >= 0 && regionData.regions[regionId]) {
        var region = regionData.regions[regionId];
        var azDegrees = ((cell.azBin * 360.0) / azimuthBins).toFixed(1);
        var variance = (region.mean_variance || 0).toFixed(3);
        var noiseRel = (
          (region.params && region.params.noise_relative_fraction) ||
          0
        ).toFixed(3);
        var neighbors =
          (region.params && region.params.neighbor_confirmation_count) || 0;
        var alpha = (
          (region.params && region.params.settle_update_fraction) ||
          0
        ).toFixed(3);
        var color = regionColors[regionId % regionColors.length];

        tooltip.innerHTML =
          '<div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px;">' +
          '<div style="width: 16px; height: 16px; background: ' +
          color +
          '; border: 1px solid #fff; border-radius: 2px;"></div>' +
          "<strong>Region " +
          regionId +
          "</strong></div>" +
          "Ring: " +
          cell.ring +
          ", Azimuth: " +
          azDegrees +
          "\u00b0<br>" +
          "Variance: " +
          variance +
          "<br>" +
          "Noise Rel: " +
          noiseRel +
          "<br>" +
          "Neighbors: " +
          neighbors +
          "<br>" +
          "Alpha: " +
          alpha;

        tooltip.style.display = "block";
        tooltip.style.left = e.clientX + 10 + "px";
        tooltip.style.top = e.clientY + 10 + "px";
      } else {
        tooltip.style.display = "none";
      }
    }
  });

  canvas.addEventListener("mouseleave", function () {
    tooltip.style.display = "none";
  });

  // Initial load
  autoRefresh();
}
