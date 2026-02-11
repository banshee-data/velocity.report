/* sweep_dashboard.js — extracted from sweep_dashboard.html for testability. */
/* global echarts */

/* Shared utilities: browser receives them via prior <script> tag;
   Node/Jest pulls them in via require(). */
/* c8 ignore start -- browser-only fallback paths; unreachable under Jest/Node */
var _common =
  typeof module !== "undefined" && typeof require === "function"
    ? require("./dashboard_common.js")
    : null;
var escapeHTML = _common
  ? _common.escapeHTML
  : typeof window !== "undefined"
    ? window.escapeHTML
    : undefined;
var parseDuration = _common
  ? _common.parseDuration
  : typeof window !== "undefined"
    ? window.parseDuration
    : undefined;
var formatDuration = _common
  ? _common.formatDuration
  : typeof window !== "undefined"
    ? window.formatDuration
    : undefined;
/* c8 ignore stop */

var pollTimer = null;
var stopRequested = false;
var sweepMode = "manual"; // 'manual' or 'auto'
var sensorId = null;
var latestResults = null;
var chartConfigs = [];
var chartInstances = {};
var chartConfigCounter = 0;
var currentSweepId = null;
var viewingHistorical = false;

// Detect dark mode for ECharts (guarded for test environments)
var isDark =
  typeof window !== "undefined" &&
  window.matchMedia &&
  window.matchMedia("(prefers-color-scheme: dark)").matches;
var chartTheme = isDark ? "dark" : null;
var chartBg = "transparent";

// Parameter schema derived from TuningConfig (with descriptions)
var PARAM_SCHEMA = {
  noise_relative: {
    type: "float64",
    label: "Noise Relative",
    step: 0.001,
    defaultStart: 0.01,
    defaultEnd: 0.2,
    desc: "Fraction of measured range treated as noise threshold (0–1). Higher = more tolerant of range variation.",
  },
  closeness_multiplier: {
    type: "float64",
    label: "Closeness Multiplier",
    step: 0.5,
    defaultStart: 1.0,
    defaultEnd: 20.0,
    desc: "Multiplier for closeness threshold. Higher = wider band for background acceptance.",
  },
  neighbor_confirmation_count: {
    type: "int",
    label: "Neighbour Confirmation",
    step: 1,
    defaultStart: 0,
    defaultEnd: 8,
    desc: "Number of neighbouring cells (0–8) that must agree before marking foreground.",
  },
  seed_from_first: {
    type: "bool",
    label: "Seed From First",
    desc: "If true, initialise background model from the very first observation.",
  },
  warmup_duration_nanos: {
    type: "int64",
    label: "Warmup Duration (ns)",
    step: 1000000000,
    defaultStart: 5000000000,
    defaultEnd: 120000000000,
    desc: "Duration in nanoseconds for the warmup phase before classification begins.",
  },
  warmup_min_frames: {
    type: "int",
    label: "Warmup Min Frames",
    step: 10,
    defaultStart: 10,
    defaultEnd: 500,
    desc: "Minimum frames required before the warmup phase can complete.",
  },
  post_settle_update_fraction: {
    type: "float64",
    label: "Post-Settle Update Fraction",
    step: 0.01,
    defaultStart: 0,
    defaultEnd: 0.5,
    desc: "Background update alpha after settling (0 = freeze background).",
  },
  background_update_fraction: {
    type: "float64",
    label: "Background Update Fraction",
    step: 0.005,
    defaultStart: 0.005,
    defaultEnd: 0.1,
    desc: "EMA learning rate for background grid updates during settling (0–1).",
  },
  safety_margin_meters: {
    type: "float64",
    label: "Safety Margin (m)",
    step: 0.1,
    defaultStart: 0,
    defaultEnd: 2.0,
    desc: "Additional additive margin (metres) on the closeness threshold for background classification.",
  },
  enable_diagnostics: {
    type: "bool",
    label: "Enable Diagnostics",
    desc: "If true, enables verbose per-frame diagnostic logging.",
  },
  foreground_min_cluster_points: {
    type: "int",
    label: "FG Min Cluster Points",
    step: 1,
    defaultStart: 0,
    defaultEnd: 20,
    desc: "Minimum points for a foreground cluster to be reported. 0 = disabled.",
  },
  foreground_dbscan_eps: {
    type: "float64",
    label: "FG DBSCAN Eps",
    step: 0.1,
    defaultStart: 0,
    defaultEnd: 2.0,
    desc: "DBSCAN epsilon for foreground point clustering. 0 = disabled.",
  },
  buffer_timeout: {
    type: "string",
    label: "Buffer Timeout",
    desc: 'Max wait time for a complete frame (Go duration, e.g. "500ms").',
  },
  min_frame_points: {
    type: "int",
    label: "Min Frame Points",
    step: 100,
    defaultStart: 100,
    defaultEnd: 5000,
    desc: "Minimum points in a frame before processing. Fewer points = frame dropped.",
  },
  flush_interval: {
    type: "string",
    label: "Flush Interval",
    desc: 'How often the background grid is flushed to disk (e.g. "60s").',
  },
  background_flush: {
    type: "bool",
    label: "Background Flush",
    desc: "If true, enables periodic background grid flush to disk. Default: false (disabled).",
  },
  gating_distance_squared: {
    type: "float64",
    label: "Gating Distance²",
    step: 1.0,
    defaultStart: 4.0,
    defaultEnd: 100.0,
    desc: "Squared Mahalanobis distance threshold for track-to-cluster association.",
  },
  process_noise_pos: {
    type: "float64",
    label: "Process Noise Pos",
    step: 0.01,
    defaultStart: 0.01,
    defaultEnd: 1.0,
    desc: "Kalman process noise for position. Higher = more position uncertainty expected.",
  },
  process_noise_vel: {
    type: "float64",
    label: "Process Noise Vel",
    step: 0.01,
    defaultStart: 0.05,
    defaultEnd: 2.0,
    desc: "Kalman process noise for velocity. Higher = more velocity changes expected.",
  },
  measurement_noise: {
    type: "float64",
    label: "Measurement Noise",
    step: 0.01,
    defaultStart: 0.01,
    defaultEnd: 2.0,
    desc: "Kalman measurement noise. Higher = less trust in observations.",
  },
  occlusion_cov_inflation: {
    type: "float64",
    label: "Occlusion Cov Inflation",
    step: 0.1,
    defaultStart: 0.1,
    defaultEnd: 5.0,
    desc: "Covariance inflation factor during occlusion (missed observations).",
  },
  hits_to_confirm: {
    type: "int",
    label: "Hits to Confirm",
    step: 1,
    defaultStart: 1,
    defaultEnd: 10,
    desc: "Consecutive successful associations needed to confirm a tentative track.",
  },
  max_misses: {
    type: "int",
    label: "Max Misses",
    step: 1,
    defaultStart: 1,
    defaultEnd: 10,
    desc: "Consecutive misses before a tentative track is deleted.",
  },
  max_misses_confirmed: {
    type: "int",
    label: "Max Misses Confirmed",
    step: 1,
    defaultStart: 3,
    defaultEnd: 30,
    desc: "Consecutive misses before a confirmed track is deleted.",
  },
  max_tracks: {
    type: "int",
    label: "Max Tracks",
    step: 10,
    defaultStart: 10,
    defaultEnd: 500,
    desc: "Maximum number of simultaneous tracks the tracker maintains.",
  },
};

var paramNames = Object.keys(PARAM_SCHEMA);
var paramCounter = 0;

var CHART_COLORS = [
  "#5470c6",
  "#91cc75",
  "#fac858",
  "#ee6666",
  "#73c0de",
  "#3ba272",
  "#fc8452",
  "#9a60b4",
];

var METRIC_KEYS = [
  "overall_accept_mean",
  "overall_accept_stddev",
  "nonzero_cells_mean",
  "nonzero_cells_stddev",
  "active_tracks_mean",
  "active_tracks_stddev",
  "alignment_deg_mean",
  "alignment_deg_stddev",
  "misalignment_ratio_mean",
  "misalignment_ratio_stddev",
  "heading_jitter_deg_mean",
  "heading_jitter_deg_stddev",
  "speed_jitter_mps_mean",
  "speed_jitter_mps_stddev",
  "fragmentation_ratio_mean",
  "fragmentation_ratio_stddev",
  "foreground_capture_mean",
  "foreground_capture_stddev",
  "unbounded_point_mean",
  "unbounded_point_stddev",
  "empty_box_ratio_mean",
  "empty_box_ratio_stddev",
  "ground_truth_score",
  "detection_rate",
  "false_positive_rate",
];

function metricLabel(key) {
  if (key === "_combo") return "Combination";
  var schema = PARAM_SCHEMA[key];
  if (schema) return schema.label;
  return key.replace(/_/g, " ").replace(/\b\w/g, function (c) {
    return c.toUpperCase();
  });
}

function extractValue(result, key) {
  if (result.param_values && result.param_values[key] !== undefined) {
    return result.param_values[key];
  }
  if (result[key] !== undefined) {
    return result[key];
  }
  return null;
}

function getAvailableMetrics(results) {
  // When no results are available yet, fall back to all known metric keys so
  // the chart modal can still present sensible defaults.
  var defaultResult = { params: [], metrics: METRIC_KEYS.slice() };

  if (!results || results.length === 0) return defaultResult;

  var r0 = results[0];
  var params = Object.keys(r0.param_values || {});
  var metrics = METRIC_KEYS.filter(function (k) {
    return r0[k] !== undefined;
  });

  // If none of the known metric keys are present on the first result, fall
  // back to the default list to avoid empty metric selections in the UI.
  if (metrics.length === 0) {
    metrics = defaultResult.metrics;
  }

  return { params: params, metrics: metrics };
}

function val(id) {
  return document.getElementById(id).value;
}
function numVal(id) {
  return parseFloat(val(id));
}
function intVal(id) {
  return parseInt(val(id), 10);
}

function togglePCAP() {
  var ds = document.getElementById("data_source").value;
  document.getElementById("pcap-fields").style.display =
    ds === "pcap" ? "" : "none";
  document.getElementById("scene-fields").style.display =
    ds === "scene" ? "" : "none";
}

var sweepScenesData = [];
var currentSceneHasReference = false;

function loadSweepScenes() {
  var select = document.getElementById("scene_select");
  if (!select) return;
  fetch("/api/lidar/scenes?sensor_id=" + encodeURIComponent(sensorId))
    .then(function (r) {
      return r.json();
    })
    .then(function (data) {
      sweepScenesData = data.scenes || [];
      select.innerHTML = '<option value="">(select a scene)</option>';
      sweepScenesData.forEach(function (s) {
        var opt = document.createElement("option");
        opt.value = s.scene_id;
        var label = s.pcap_file;
        if (s.description) label = s.description + " (" + s.pcap_file + ")";
        opt.textContent = label;
        select.appendChild(opt);
      });
    })
    .catch(function () {
      select.innerHTML = '<option value="">(failed to load scenes)</option>';
    });
}

function onSweepSceneSelected() {
  var sceneId = document.getElementById("scene_select").value;
  var infoEl = document.getElementById("scene-info");
  var actionsEl = document.getElementById("scene-actions");
  var gtOption = document.getElementById("ground_truth_option");

  if (!sceneId) {
    infoEl.style.display = "none";
    actionsEl.style.display = "none";
    currentSceneHasReference = false;
    if (gtOption) gtOption.style.display = "none";
    return;
  }

  var scene = sweepScenesData.find(function (s) {
    return s.scene_id === sceneId;
  });
  if (!scene) {
    infoEl.style.display = "none";
    actionsEl.style.display = "none";
    currentSceneHasReference = false;
    if (gtOption) gtOption.style.display = "none";
    return;
  }

  // Populate the PCAP fields so buildSceneJSON / handleStartAutoTune can read them
  document.getElementById("pcap_file").value = scene.pcap_file;
  if (scene.pcap_start_secs != null) {
    document.getElementById("pcap_start_secs").value = scene.pcap_start_secs;
  }
  if (scene.pcap_duration_secs != null) {
    document.getElementById("pcap_duration_secs").value =
      scene.pcap_duration_secs;
  }

  // Show info
  var info = "File: " + scene.pcap_file;
  if (scene.pcap_start_secs != null)
    info += " | Start: " + scene.pcap_start_secs + "s";
  if (scene.pcap_duration_secs != null)
    info += " | Duration: " + scene.pcap_duration_secs + "s";

  // Check if scene has reference run (enables ground truth objective)
  currentSceneHasReference = scene.reference_run_id ? true : false;
  if (currentSceneHasReference) {
    info += " | Reference: " + scene.reference_run_id;
    if (gtOption) gtOption.style.display = "";
  } else {
    if (gtOption) gtOption.style.display = "none";
  }

  infoEl.textContent = info;
  infoEl.style.display = "";

  // Show action buttons if scene has optimal params
  if (scene.optimal_params_json) {
    actionsEl.style.display = "";
  } else {
    actionsEl.style.display = "none";
  }
}

function setMode(mode) {
  sweepMode = mode;
  document.getElementById("mode-manual").className =
    mode === "manual" ? "active" : "";
  document.getElementById("mode-auto").className =
    mode === "auto" ? "active" : "";
  var rlhfBtn = document.getElementById("mode-rlhf");
  if (rlhfBtn) rlhfBtn.className = mode === "rlhf" ? "active" : "";

  // Body classes for CSS visibility
  document.body.classList.remove("auto-mode", "rlhf-mode");
  if (mode === "auto") {
    document.body.classList.add("auto-mode");
  } else if (mode === "rlhf") {
    document.body.classList.add("rlhf-mode");
    requestNotificationPermission();
    populateRLHFScenes();
  }

  // Update button text
  if (mode === "rlhf") {
    document.getElementById("btn-start").textContent = "Start RLHF Sweep";
    document.getElementById("btn-stop").textContent = "Stop RLHF Sweep";
  } else if (mode === "auto") {
    document.getElementById("btn-start").textContent = "Start Auto-Tune";
    document.getElementById("btn-stop").textContent = "Stop Auto-Tune";
  } else {
    document.getElementById("btn-start").textContent = "Start Sweep";
    document.getElementById("btn-stop").textContent = "Stop Sweep";
  }
  updateSweepSummary();
}

function toggleWeights() {
  var obj = document.getElementById("objective").value;
  var show = obj === "weighted";
  document.getElementById("weight-fields").style.display = show ? "" : "none";
  document.getElementById("acceptance-criteria-fields").style.display = show
    ? ""
    : "none";
}

function addParamRow(name) {
  var id = paramCounter++;
  var container = document.getElementById("param-rows");
  var row = document.createElement("div");
  row.className = "param-row";
  row.id = "param-row-" + id;

  // Name dropdown
  var selHtml =
    '<label class="param-name"><span>Parameter</span><select id="pname-' +
    escapeHTML(id) +
    '" onchange="updateParamFields(' +
    escapeHTML(id) +
    ')">';
  selHtml += '<option value="">-- select --</option>';
  for (var i = 0; i < paramNames.length; i++) {
    var pn = paramNames[i];
    var schema = PARAM_SCHEMA[pn];
    var sel = name === pn ? " selected" : "";
    selHtml +=
      '<option value="' +
      escapeHTML(pn) +
      '"' +
      sel +
      ">" +
      escapeHTML(schema.label) +
      "</option>";
  }
  selHtml += "</select></label>";

  row.innerHTML =
    selHtml +
    '<div id="pfields-' +
    escapeHTML(id) +
    '" class="param-fields"></div>' +
    '<button class="btn-sm btn-remove param-remove" onclick="removeParamRow(' +
    escapeHTML(id) +
    ')">×</button>' +
    '<div id="pdesc-' +
    escapeHTML(id) +
    '" class="param-desc"></div>';

  container.appendChild(row);
  if (name) updateParamFields(id);
  return id;
}

function removeParamRow(id) {
  var row = document.getElementById("param-row-" + id);
  if (row) row.remove();
  updateSweepSummary();
  if (window.currentParamsCache)
    displayCurrentParams(window.currentParamsCache);
}

function updateParamFields(id) {
  var nameEl = document.getElementById("pname-" + id);
  var fieldsEl = document.getElementById("pfields-" + id);
  var descEl = document.getElementById("pdesc-" + id);
  var name = nameEl.value;
  if (!name) {
    fieldsEl.innerHTML = "";
    if (descEl) descEl.textContent = "";
    return;
  }

  var schema = PARAM_SCHEMA[name];
  var typ = schema.type;
  var step = schema.step || 1;

  // Show description
  if (descEl) descEl.textContent = schema.desc || "";

  if (typ === "float64" || typ === "int" || typ === "int64") {
    var startVal = schema.defaultStart !== undefined ? schema.defaultStart : 0;
    var endVal = schema.defaultEnd !== undefined ? schema.defaultEnd : 1;
    fieldsEl.innerHTML =
      '<label class="param-field"><span>Start</span><input id="pstart-' +
      escapeHTML(id) +
      '" type="number" step="' +
      escapeHTML(step) +
      '" value="' +
      escapeHTML(startVal) +
      '"></label>' +
      '<label class="param-field"><span>End</span><input id="pend-' +
      escapeHTML(id) +
      '" type="number" step="' +
      escapeHTML(step) +
      '" value="' +
      escapeHTML(endVal) +
      '"></label>' +
      '<label class="param-field param-field-step"><span>Step</span><input id="pstep-' +
      escapeHTML(id) +
      '" type="number" step="' +
      escapeHTML(step) +
      '" value="' +
      escapeHTML(step) +
      '"></label>' +
      '<label class="param-field param-field-values" style="flex:2"><span>Values (overrides)</span><input id="pvals-' +
      escapeHTML(id) +
      '" type="text" placeholder="e.g. 0.01, 0.02, 0.05"></label>';
  } else if (typ === "bool") {
    fieldsEl.innerHTML =
      '<label class="param-field"><span>Values</span><input id="pvals-' +
      escapeHTML(id) +
      '" type="text" value="true, false"></label>';
  } else if (typ === "string") {
    fieldsEl.innerHTML =
      '<label class="param-field" style="flex:2"><span>Values</span><input id="pvals-' +
      escapeHTML(id) +
      '" type="text" placeholder="e.g. 500ms, 1s, 2s"></label>';
  }
  updateSweepSummary();
  if (window.currentParamsCache)
    displayCurrentParams(window.currentParamsCache);
}

function showError(msg) {
  var el = document.getElementById("error-box");
  el.textContent = msg;
  el.style.display = msg ? "" : "none";
}

function getParamValueCount(rowId) {
  var nameEl = document.getElementById("pname-" + rowId);
  if (!nameEl || !nameEl.value) return 0;
  var schema = PARAM_SCHEMA[nameEl.value];
  if (!schema) return 0;
  var typ = schema.type;

  var valsEl = document.getElementById("pvals-" + rowId);
  var valsStr = valsEl ? valsEl.value.trim() : "";

  if (valsStr) {
    return valsStr.split(",").filter(function (s) {
      return s.trim() !== "";
    }).length;
  }

  if (typ === "bool") return 2;

  var startEl = document.getElementById("pstart-" + rowId);
  var endEl = document.getElementById("pend-" + rowId);
  var stepEl = document.getElementById("pstep-" + rowId);
  if (startEl && endEl && stepEl) {
    var start = parseFloat(startEl.value);
    var end = parseFloat(endEl.value);
    var step = parseFloat(stepEl.value);
    if (step > 0 && end >= start) {
      return Math.floor((end - start) / step) + 1;
    }
  }
  return 0;
}

function updateSweepSummary() {
  var el = document.getElementById("sweep-summary");
  var rows = document.getElementById("param-rows").children;
  if (rows.length === 0) {
    el.innerHTML = "";
    return;
  }

  var isAuto = sweepMode === "auto";
  var parts = [];
  var total = 1;
  var anyParam = false;
  var valuesPerParam = isAuto ? intVal("values_per_param") || 5 : 0;

  for (var i = 0; i < rows.length; i++) {
    var rowId = rows[i].id.replace("param-row-", "");
    var nameEl = document.getElementById("pname-" + rowId);
    if (!nameEl || !nameEl.value) continue;
    var schema = PARAM_SCHEMA[nameEl.value];
    var label = schema ? schema.label : nameEl.value;

    if (isAuto) {
      // In auto mode, show start -> end range
      var startEl = document.getElementById("pstart-" + rowId);
      var endEl = document.getElementById("pend-" + rowId);
      if (startEl && endEl) {
        parts.push(
          "<strong>" +
            escapeHTML(label) +
            "</strong>: " +
            escapeHTML(startEl.value) +
            " → " +
            escapeHTML(endEl.value) +
            " (" +
            escapeHTML(valuesPerParam) +
            " values)",
        );
      } else {
        parts.push(
          "<strong>" +
            escapeHTML(label) +
            "</strong>: " +
            escapeHTML(valuesPerParam) +
            " values",
        );
      }
      anyParam = true;
      total *= valuesPerParam;
    } else {
      var count = getParamValueCount(rowId);
      if (count === 0) continue;
      anyParam = true;
      parts.push(
        "<strong>" +
          escapeHTML(label) +
          "</strong>: " +
          escapeHTML(count) +
          " values",
      );
      total *= count;
    }
  }

  if (!anyParam) {
    el.innerHTML = "";
    return;
  }

  var seedVal = val("seed");
  var seedMultiplier = seedVal === "toggle" ? 2 : 1;
  var totalWithSeed = total * seedMultiplier;

  var iterations = intVal("iterations") || 1;
  var settleSecs = parseDuration(val("settle_time"));
  var intervalSecs = parseDuration(val("interval"));
  var settleMode = val("settle_mode");
  var activeMeasurementSecs = iterations * intervalSecs;

  var html = parts.join(" &middot; ");

  if (isAuto) {
    var maxRounds = intVal("max_rounds") || 3;
    var perRound = totalWithSeed;
    var totalAcrossRounds = perRound * maxRounds;
    var runtimePerRound;
    if (settleMode === "once" && perRound > 1) {
      var regionRestoreSecs = 2;
      runtimePerRound =
        settleSecs +
        activeMeasurementSecs +
        (perRound - 1) * (regionRestoreSecs + activeMeasurementSecs);
    } else {
      runtimePerRound = perRound * (settleSecs + activeMeasurementSecs);
    }
    var totalRuntime = runtimePerRound * maxRounds;

    html +=
      "<br/><strong>" + escapeHTML(total) + "</strong> permutations/round";
    if (seedMultiplier > 1) {
      html +=
        " &times; seed toggle &times;2 = <strong>" +
        escapeHTML(perRound) +
        "</strong>";
    }
    html +=
      " &times; <strong>" +
      escapeHTML(maxRounds) +
      "</strong> rounds = <strong>" +
      escapeHTML(totalAcrossRounds) +
      "</strong> total";
    html +=
      "<br/>active measurement per permutation: <strong>" +
      escapeHTML(formatDuration(activeMeasurementSecs)) +
      "</strong>";
    html +=
      " (" +
      escapeHTML(iterations) +
      " &times; " +
      escapeHTML(val("interval")) +
      ")";
    html +=
      " &middot; estimated total runtime: <strong>~" +
      escapeHTML(formatDuration(totalRuntime)) +
      "</strong>";
  } else {
    var runtimeSecs;
    if (settleMode === "once" && totalWithSeed > 1) {
      var regionRestoreSecs = 2;
      runtimeSecs =
        settleSecs +
        activeMeasurementSecs +
        (totalWithSeed - 1) * (regionRestoreSecs + activeMeasurementSecs);
    } else {
      runtimeSecs = totalWithSeed * (settleSecs + activeMeasurementSecs);
    }

    html += "<br/><strong>" + escapeHTML(total) + "</strong> permutations";
    if (seedMultiplier > 1) {
      html +=
        " &times; seed toggle &times;2 = <strong>" +
        escapeHTML(totalWithSeed) +
        "</strong> total";
    }
    html +=
      " &middot; <strong>" +
      escapeHTML(iterations) +
      "</strong> iterations each";
    html +=
      "<br/>active measurement per permutation: <strong>" +
      escapeHTML(formatDuration(activeMeasurementSecs)) +
      "</strong>";
    html +=
      " (" +
      escapeHTML(iterations) +
      " &times; " +
      escapeHTML(val("interval")) +
      ")";
    html +=
      " &middot; estimated total runtime: <strong>~" +
      escapeHTML(formatDuration(runtimeSecs)) +
      "</strong>";
  }
  el.innerHTML = html;
}

// ---- Scene management ----

function buildSceneJSON() {
  var ds = val("data_source");
  var req = {
    seed: val("seed"),
    iterations: intVal("iterations"),
    interval: val("interval"),
    settle_time: val("settle_time"),
    settle_mode: val("settle_mode"),
    data_source: ds === "scene" ? "pcap" : ds,
  };

  if (ds === "scene") {
    req.scene_id = val("scene_select");
  }

  if (ds === "pcap" || ds === "scene") {
    req.pcap_file = val("pcap_file");
    req.pcap_start_secs = numVal("pcap_start_secs");
    req.pcap_duration_secs = numVal("pcap_duration_secs");
  }

  var params = [];
  var rows = document.getElementById("param-rows").children;
  for (var i = 0; i < rows.length; i++) {
    var rowId = rows[i].id.replace("param-row-", "");
    var nameEl = document.getElementById("pname-" + rowId);
    if (!nameEl) continue;
    var name = nameEl.value;
    if (!name) continue;
    var schema = PARAM_SCHEMA[name];
    var typ = schema.type;
    var p = { name: name, type: typ };

    var valsEl = document.getElementById("pvals-" + rowId);
    var valsStr = valsEl ? valsEl.value.trim() : "";

    if (valsStr) {
      var parts = valsStr
        .split(",")
        .map(function (s) {
          return s.trim();
        })
        .filter(function (s) {
          return s !== "";
        });
      if (typ === "float64") {
        p.values = parts.map(function (s) {
          return parseFloat(s);
        });
      } else if (typ === "int" || typ === "int64") {
        p.values = parts.map(function (s) {
          return parseInt(s, 10);
        });
      } else if (typ === "bool") {
        p.values = parts.map(function (s) {
          return s.toLowerCase() === "true";
        });
      } else {
        p.values = parts;
      }
    } else if (typ === "float64" || typ === "int" || typ === "int64") {
      var startEl = document.getElementById("pstart-" + rowId);
      var endEl = document.getElementById("pend-" + rowId);
      var stepEl = document.getElementById("pstep-" + rowId);
      if (startEl && endEl && stepEl) {
        p.start = parseFloat(startEl.value);
        p.end = parseFloat(endEl.value);
        p.step = parseFloat(stepEl.value);
      }
    }

    params.push(p);
  }
  req.params = params;
  return req;
}

function downloadScene() {
  var obj = buildSceneJSON();
  var json = JSON.stringify(obj, null, 2);
  var blob = new Blob([json], { type: "application/json" });
  var a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = "sweep-scene.json";
  a.click();
  URL.revokeObjectURL(a.href);
}

function uploadScene(input) {
  if (!input.files || !input.files[0]) return;
  var reader = new FileReader();
  reader.onload = function (e) {
    try {
      var obj = JSON.parse(e.target.result);
      loadScene(obj);
    } catch (err) {
      showError("Invalid JSON: " + err.message);
    }
  };
  reader.readAsText(input.files[0]);
  input.value = "";
}

function loadScene(obj) {
  if (obj.seed) document.getElementById("seed").value = obj.seed;
  if (obj.iterations)
    document.getElementById("iterations").value = obj.iterations;
  if (obj.interval) document.getElementById("interval").value = obj.interval;
  if (obj.settle_time)
    document.getElementById("settle_time").value = obj.settle_time;
  if (obj.settle_mode)
    document.getElementById("settle_mode").value = obj.settle_mode;
  if (obj.scene_id) {
    document.getElementById("data_source").value = "scene";
    togglePCAP();
    var sceneSelect = document.getElementById("scene_select");
    if (sceneSelect) sceneSelect.value = obj.scene_id;
  } else if (obj.data_source) {
    document.getElementById("data_source").value = obj.data_source;
    togglePCAP();
  }
  if (obj.pcap_file) document.getElementById("pcap_file").value = obj.pcap_file;
  if (obj.pcap_start_secs !== undefined)
    document.getElementById("pcap_start_secs").value = obj.pcap_start_secs;
  if (obj.pcap_duration_secs !== undefined)
    document.getElementById("pcap_duration_secs").value =
      obj.pcap_duration_secs;

  // Clear existing param rows
  var container = document.getElementById("param-rows");
  container.innerHTML = "";
  paramCounter = 0;

  // Add rows from scenario
  if (obj.params && obj.params.length > 0) {
    obj.params.forEach(function (p) {
      var id = addParamRow(p.name);
      // Populate values
      if (p.values && p.values.length > 0) {
        var valsEl = document.getElementById("pvals-" + id);
        if (valsEl) valsEl.value = p.values.join(", ");
      } else {
        if (p.start !== undefined) {
          var startEl = document.getElementById("pstart-" + id);
          if (startEl) startEl.value = p.start;
        }
        if (p.end !== undefined) {
          var endEl = document.getElementById("pend-" + id);
          if (endEl) endEl.value = p.end;
        }
        if (p.step !== undefined) {
          var stepEl = document.getElementById("pstep-" + id);
          if (stepEl) stepEl.value = p.step;
        }
      }
    });
  }
  updateSweepSummary();
}

function toggleJSONEditor() {
  var wrap = document.getElementById("json-editor-wrap");
  var applyBtn = document.getElementById("btn-apply-json");
  if (wrap.style.display === "none") {
    wrap.style.display = "";
    applyBtn.style.display = "";
    document.getElementById("scenario-json").value = JSON.stringify(
      buildSceneJSON(),
      null,
      2,
    );
  } else {
    wrap.style.display = "none";
    applyBtn.style.display = "none";
  }
}

function applyJSONEditor() {
  try {
    var obj = JSON.parse(document.getElementById("scenario-json").value);
    loadScene(obj);
    showError("");
    // Hide the editor after successful apply
    document.getElementById("json-editor-wrap").style.display = "none";
    document.getElementById("btn-apply-json").style.display = "none";
  } catch (err) {
    showError("Invalid JSON: " + err.message);
  }
}

// ---- Sweep control ----

function handleStart() {
  showError("");
  var rows = document.getElementById("param-rows").children;
  if (sweepMode !== "rlhf" && rows.length === 0) {
    showError("Add at least one parameter.");
    return;
  }

  if (sweepMode === "rlhf") {
    handleStartRLHF();
  } else if (sweepMode === "auto") {
    handleStartAutoTune();
  } else {
    handleStartManualSweep();
  }
}

function handleStartManualSweep() {
  var req = buildSceneJSON();
  req.mode = "params";

  if (!req.params || req.params.length === 0) {
    showError("Add at least one parameter to sweep.");
    return;
  }

  fetch("/api/lidar/sweep/start", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  })
    .then(function (r) {
      if (!r.ok)
        return r.text().then(function (t) {
          throw new Error(t);
        });
      startPolling();
    })
    .catch(function (e) {
      showError(e.message);
    });
}

function handleStartAutoTune() {
  // Build auto-tune request
  var params = [];
  var rows = document.getElementById("param-rows").children;
  for (var i = 0; i < rows.length; i++) {
    var rowId = rows[i].id.replace("param-row-", "");
    var nameEl = document.getElementById("pname-" + rowId);
    if (!nameEl || !nameEl.value) continue;
    var schema = PARAM_SCHEMA[nameEl.value];
    var startEl = document.getElementById("pstart-" + rowId);
    var endEl = document.getElementById("pend-" + rowId);
    if (!startEl || !endEl) continue;
    params.push({
      name: nameEl.value,
      type: schema.type,
      start: parseFloat(startEl.value),
      end: parseFloat(endEl.value),
    });
  }

  if (params.length === 0) {
    showError("Add at least one parameter to auto-tune.");
    return;
  }

  var ds = val("data_source");
  var req = {
    params: params,
    max_rounds: intVal("max_rounds") || 3,
    values_per_param: intVal("values_per_param") || 5,
    top_k: intVal("top_k") || 5,
    objective: val("objective"),
    iterations: intVal("iterations"),
    interval: val("interval"),
    settle_time: val("settle_time"),
    seed: val("seed"),
    data_source: ds === "scene" ? "pcap" : ds,
    settle_mode: val("settle_mode"),
  };

  if (ds === "pcap" || ds === "scene") {
    req.pcap_file = val("pcap_file");
    req.pcap_start_secs = numVal("pcap_start_secs");
    req.pcap_duration_secs = numVal("pcap_duration_secs");
  }

  // Phase 5.4: Include scene_id for ground truth evaluation
  if (ds === "scene") {
    var sceneId = val("scene_select");
    if (sceneId) {
      req.scene_id = sceneId;
    }
  }

  if (req.objective === "weighted") {
    req.weights = {
      acceptance: numVal("w_acceptance") || 1.0,
      misalignment: numVal("w_misalignment") || -0.5,
      alignment: numVal("w_alignment") || -0.01,
      nonzero_cells: numVal("w_nonzero") || 0.1,
      active_tracks: numVal("w_active_tracks") || 0.3,
      foreground_capture: numVal("w_foreground_capture") || 0,
      empty_boxes: numVal("w_empty_boxes") || 0,
      fragmentation: numVal("w_fragmentation") || 0,
      heading_jitter: numVal("w_heading_jitter") || 0,
    };

    // Build acceptance criteria (only include non-empty fields)
    var ac = {};
    var acFrag = document.getElementById("ac_max_fragmentation").value;
    var acUnb = document.getElementById("ac_max_unbounded").value;
    var acEmpty = document.getElementById("ac_max_empty_boxes").value;
    if (acFrag !== "") ac.max_fragmentation_ratio = parseFloat(acFrag);
    if (acUnb !== "") ac.max_unbounded_point_ratio = parseFloat(acUnb);
    if (acEmpty !== "") ac.max_empty_box_ratio = parseFloat(acEmpty);
    if (Object.keys(ac).length > 0) {
      req.acceptance_criteria = ac;
    }
  }

  // Hide previous recommendation
  document.getElementById("recommendation-card").style.display = "none";

  fetch("/api/lidar/sweep/auto", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  })
    .then(function (r) {
      if (!r.ok)
        return r.text().then(function (t) {
          throw new Error(t);
        });
      startPolling();
    })
    .catch(function (e) {
      showError(e.message);
    });
}

function handleStop() {
  // Show stopping indicator
  stopRequested = true;
  document.getElementById("btn-stop").style.display = "none";
  document.getElementById("stopping-indicator").style.display = "block";

  var stopUrl;
  if (sweepMode === "rlhf") {
    stopUrl = "/api/lidar/sweep/rlhf/stop";
  } else if (sweepMode === "auto") {
    stopUrl = "/api/lidar/sweep/auto/stop";
  } else {
    stopUrl = "/api/lidar/sweep/stop";
  }
  fetch(stopUrl, { method: "POST" }).catch(function (e) {
    showError(e.message);
  });
}

function startPolling() {
  stopPolling();
  pollTimer = setInterval(pollStatus, 3000);
  pollStatus();
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

function comboLabel(r) {
  if (r.param_values) {
    return Object.entries(r.param_values)
      .map(function (e) {
        var key = e[0];
        var v = e[1];
        // Use short key (last segment after underscore split)
        var parts = key.split("_");
        var short = parts.length > 1 ? parts.slice(-1)[0] : key;
        if (typeof v === "number" && v !== Math.floor(v)) {
          return short + "=" + v.toFixed(3);
        }
        return short + "=" + v;
      })
      .join(" ");
  }
  // Fallback for legacy results
  return (
    "n=" +
    r.noise.toFixed(3) +
    " c=" +
    r.closeness.toFixed(1) +
    " nb=" +
    r.neighbour
  );
}

function pollStatus() {
  if (sweepMode === "rlhf") {
    pollRLHFStatus();
    return;
  }
  if (sweepMode === "auto") {
    pollAutoTuneStatus();
    return;
  }
  fetch("/api/lidar/sweep/status")
    .then(function (r) {
      return r.json();
    })
    .then(function (st) {
      var prog = document.getElementById("progress-section");
      prog.style.display = "";

      var badge = document.getElementById("status-badge");
      badge.textContent = st.status;
      badge.className = "status-badge status-" + st.status;

      document.getElementById("combo-count").textContent =
        st.completed_combos + " / " + st.total_combos + " combinations";

      if (st.status === "running" && stopRequested) {
        // Stop has been requested but sweep is still finishing current combination
        document.getElementById("btn-start").style.display = "none";
        document.getElementById("btn-stop").style.display = "none";
        document.getElementById("stopping-indicator").style.display = "block";
      } else {
        stopRequested = false;
        document.getElementById("btn-start").style.display =
          st.status === "running" ? "none" : "";
        document.getElementById("btn-stop").style.display =
          st.status === "running" ? "" : "none";
        document.getElementById("stopping-indicator").style.display = "none";
      }

      var cc = document.getElementById("current-combo");
      if (st.status === "running" && st.current_combo) {
        var c = st.current_combo;
        var lbl = comboLabel(c);
        cc.textContent =
          "Current: " +
          lbl +
          " → acceptance=" +
          ((c.overall_accept_mean || 0) * 100).toFixed(1) +
          "%";
      } else {
        cc.textContent = "";
      }

      var errEl = document.getElementById("sweep-error");
      if (st.error) {
        errEl.textContent = st.error;
        errEl.style.display = "";
      } else {
        errEl.style.display = "none";
      }

      var warnEl = document.getElementById("sweep-warnings");
      if (st.warnings && st.warnings.length > 0) {
        warnEl.innerHTML =
          '<div class="section-title" style="margin-top:8px">Warnings (' +
          escapeHTML(st.warnings.length) +
          ")</div>" +
          '<ul style="margin:4px 0 0 16px;padding:0;font-size:12px;color:var(--fg-faint)">' +
          st.warnings
            .map(function (w) {
              return "<li>" + escapeHTML(w) + "</li>";
            })
            .join("") +
          "</ul>";
        warnEl.style.display = "";
      } else {
        warnEl.style.display = "none";
      }

      if (st.results && st.results.length > 0) {
        latestResults = st.results;
        renderCharts(st.results);
        renderTable(st.results);
      }

      if (st.status === "complete" || st.status === "error") {
        stopPolling();
      }
    })
    .catch(function () {});
}

function pollAutoTuneStatus() {
  fetch("/api/lidar/sweep/auto")
    .then(function (r) {
      return r.json();
    })
    .then(function (st) {
      var prog = document.getElementById("progress-section");
      prog.style.display = "";

      var badge = document.getElementById("status-badge");
      badge.textContent = st.status;
      badge.className = "status-badge status-" + st.status;

      // Show round progress
      var roundInfo = "";
      if (st.total_rounds > 0) {
        roundInfo = "Round " + (st.round || 1) + "/" + st.total_rounds + " — ";
      }
      document.getElementById("combo-count").textContent =
        roundInfo +
        (st.completed_combos || 0) +
        " / " +
        (st.total_combos || 0) +
        " combinations";

      if (st.status === "running" && stopRequested) {
        document.getElementById("btn-start").style.display = "none";
        document.getElementById("btn-stop").style.display = "none";
        document.getElementById("stopping-indicator").style.display = "block";
      } else {
        stopRequested = false;
        document.getElementById("btn-start").style.display =
          st.status === "running" ? "none" : "";
        document.getElementById("btn-stop").style.display =
          st.status === "running" ? "" : "none";
        document.getElementById("stopping-indicator").style.display = "none";
      }

      // Show current round summary
      var cc = document.getElementById("current-combo");
      if (
        st.status === "running" &&
        st.round_results &&
        st.round_results.length > 0
      ) {
        var lastRound = st.round_results[st.round_results.length - 1];
        cc.innerHTML =
          '<div class="auto-progress">Last round best score: <strong>' +
          escapeHTML((lastRound.best_score || 0).toFixed(4)) +
          "</strong>" +
          " — " +
          escapeHTML(formatParamValues(lastRound.best_params)) +
          "</div>";
      } else if (st.status === "running") {
        cc.textContent = "Running initial round...";
      } else {
        cc.textContent = "";
      }

      var errEl = document.getElementById("sweep-error");
      if (st.error) {
        errEl.textContent = st.error;
        errEl.style.display = "";
      } else {
        errEl.style.display = "none";
      }

      // Render charts from accumulated results
      if (st.results && st.results.length > 0) {
        latestResults = st.results;
        renderCharts(st.results);
        renderTable(st.results);
      }

      if (st.status === "complete") {
        stopPolling();
        if (st.recommendation) {
          renderRecommendation(st.recommendation, st.round_results);
        }
      } else if (st.status === "error") {
        stopPolling();
      }
    })
    .catch(function () {});
}

function formatParamValues(params) {
  if (!params) return "";
  return Object.keys(params)
    .filter(function (k) {
      return (
        k !== "score" &&
        k !== "acceptance_rate" &&
        k !== "misalignment_ratio" &&
        k !== "alignment_deg" &&
        k !== "nonzero_cells"
      );
    })
    .map(function (k) {
      var v = params[k];
      var schema = PARAM_SCHEMA[k];
      var label = schema ? schema.label : k;
      if (typeof v === "number" && v !== Math.floor(v)) {
        return label + "=" + v.toFixed(4);
      }
      return label + "=" + v;
    })
    .join(", ");
}

function renderRecommendation(rec, roundResults) {
  var card = document.getElementById("recommendation-card");
  var content = document.getElementById("recommendation-content");

  // Build param cards
  var paramHtml = '<div class="recommendation-params">';
  var paramKeys = Object.keys(rec).filter(function (k) {
    return (
      k !== "score" &&
      k !== "acceptance_rate" &&
      k !== "misalignment_ratio" &&
      k !== "alignment_deg" &&
      k !== "nonzero_cells" &&
      k !== "foreground_capture" &&
      k !== "unbounded_point_ratio" &&
      k !== "empty_box_ratio" &&
      k !== "fragmentation_ratio" &&
      k !== "heading_jitter_deg"
    );
  });
  paramKeys.forEach(function (k) {
    var schema = PARAM_SCHEMA[k];
    var label = schema ? schema.label : k;
    var v = rec[k];
    var displayVal =
      typeof v === "number" && v !== Math.floor(v) ? v.toFixed(6) : v;
    paramHtml +=
      '<div class="recommendation-param">' +
      '<div class="param-name">' +
      escapeHTML(label) +
      "</div>" +
      '<div class="param-value">' +
      escapeHTML(displayVal) +
      "</div>" +
      "</div>";
  });
  paramHtml += "</div>";

  // Metrics row
  var metricsHtml = '<div class="recommendation-metrics">';
  metricsHtml +=
    '<div class="metric">Score: <span class="metric-value">' +
    escapeHTML((rec.score || 0).toFixed(4)) +
    "</span></div>";
  metricsHtml +=
    '<div class="metric">Accept: <span class="metric-value">' +
    escapeHTML(((rec.acceptance_rate || 0) * 100).toFixed(2)) +
    "%</span></div>";
  metricsHtml +=
    '<div class="metric">Misalignment: <span class="metric-value">' +
    escapeHTML(((rec.misalignment_ratio || 0) * 100).toFixed(1)) +
    "%</span></div>";
  metricsHtml +=
    '<div class="metric">Alignment: <span class="metric-value">' +
    escapeHTML((rec.alignment_deg || 0).toFixed(1)) +
    "°</span></div>";
  metricsHtml +=
    '<div class="metric">Nonzero Cells: <span class="metric-value">' +
    escapeHTML((rec.nonzero_cells || 0).toFixed(0)) +
    "</span></div>";
  metricsHtml +=
    '<div class="metric">Fg Capture: <span class="metric-value">' +
    escapeHTML(((rec.foreground_capture || 0) * 100).toFixed(1)) +
    "%</span></div>";
  metricsHtml +=
    '<div class="metric">Unbounded: <span class="metric-value">' +
    escapeHTML(((rec.unbounded_point_ratio || 0) * 100).toFixed(1)) +
    "%</span></div>";
  metricsHtml +=
    '<div class="metric">Empty Box: <span class="metric-value">' +
    escapeHTML(((rec.empty_box_ratio || 0) * 100).toFixed(1)) +
    "%</span></div>";
  metricsHtml +=
    '<div class="metric">Fragmentation: <span class="metric-value">' +
    escapeHTML(((rec.fragmentation_ratio || 0) * 100).toFixed(1)) +
    "%</span></div>";
  metricsHtml +=
    '<div class="metric">Jitter: <span class="metric-value">' +
    escapeHTML((rec.heading_jitter_deg || 0).toFixed(1)) +
    "°</span></div>";
  metricsHtml += "</div>";

  // Round history
  var historyHtml = "";
  if (roundResults && roundResults.length > 0) {
    historyHtml =
      '<details class="round-history"><summary>Round History (' +
      escapeHTML(roundResults.length) +
      " rounds)</summary>";
    roundResults.forEach(function (rs) {
      var boundsStr = Object.keys(rs.bounds || {})
        .map(function (k) {
          var b = rs.bounds[k];
          var schema = PARAM_SCHEMA[k];
          var label = schema ? schema.label : k;
          return (
            escapeHTML(label) +
            ": [" +
            escapeHTML(b[0].toFixed(4)) +
            ", " +
            escapeHTML(b[1].toFixed(4)) +
            "]"
          );
        })
        .join(" &middot; ");
      historyHtml +=
        '<div class="round-item">' +
        "Round " +
        escapeHTML(rs.round) +
        ": " +
        escapeHTML(rs.num_combos) +
        " combos, best score=" +
        escapeHTML((rs.best_score || 0).toFixed(4)) +
        "<br/>" +
        boundsStr +
        "</div>";
    });
    historyHtml += "</details>";
  }

  content.innerHTML = paramHtml + metricsHtml + historyHtml;
  card.style.display = "";
}

function applyRecommendation() {
  var card = document.getElementById("recommendation-card");
  var content = document.getElementById("recommendation-content");
  // Extract param values from the rendered recommendation
  // Re-fetch from auto-tune state
  fetch("/api/lidar/sweep/auto")
    .then(function (r) {
      return r.json();
    })
    .then(function (st) {
      if (!st.recommendation) {
        showError("No recommendation available.");
        return;
      }
      // Build tuning params (exclude score/metrics keys)
      var tuningParams = {};
      Object.keys(st.recommendation).forEach(function (k) {
        if (
          k !== "score" &&
          k !== "acceptance_rate" &&
          k !== "misalignment_ratio" &&
          k !== "alignment_deg" &&
          k !== "nonzero_cells" &&
          k !== "foreground_capture" &&
          k !== "unbounded_point_ratio" &&
          k !== "empty_box_ratio" &&
          k !== "fragmentation_ratio" &&
          k !== "heading_jitter_deg" &&
          k !== "speed_jitter_mps"
        ) {
          tuningParams[k] = st.recommendation[k];
        }
      });

      fetch("/api/lidar/params?sensor_id=" + encodeURIComponent(sensorId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(tuningParams),
      })
        .then(function (r) {
          if (!r.ok)
            return r.text().then(function (t) {
              throw new Error(t);
            });
          document.getElementById("btn-apply-recommendation").textContent =
            "Applied";
          document.getElementById("btn-apply-recommendation").disabled = true;
          fetchCurrentParams();
        })
        .catch(function (e) {
          showError("Apply failed: " + e.message);
        });
    })
    .catch(function (e) {
      showError("Failed to fetch recommendation: " + e.message);
    });
}

function applySceneParams() {
  var sceneId = document.getElementById("scene_select").value;
  if (!sceneId) {
    showError("No scene selected.");
    return;
  }

  var scene = sweepScenesData.find(function (s) {
    return s.scene_id === sceneId;
  });
  if (!scene || !scene.optimal_params_json) {
    showError("Selected scene has no optimal parameters.");
    return;
  }

  var tuningParams;
  try {
    tuningParams = JSON.parse(scene.optimal_params_json);
  } catch (e) {
    showError("Failed to parse scene parameters: " + e.message);
    return;
  }

  fetch("/api/lidar/params?sensor_id=" + encodeURIComponent(sensorId), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(tuningParams),
  })
    .then(function (r) {
      if (!r.ok)
        return r.text().then(function (t) {
          throw new Error(t);
        });
      document.getElementById("btn-apply-scene-params").textContent =
        "Applied ✓";
      setTimeout(function () {
        document.getElementById("btn-apply-scene-params").textContent =
          "Apply Scene Params";
      }, 2000);
      fetchCurrentParams();
    })
    .catch(function (e) {
      showError("Apply failed: " + e.message);
    });
}

// ---- Paste & Apply Params ----

// Keys that are metrics (not tuning parameters) — filtered out before applying.
var METRIC_FILTER_KEYS = [
  "score",
  "acceptance_rate",
  "misalignment_ratio",
  "alignment_deg",
  "nonzero_cells",
  "foreground_capture",
  "unbounded_point_ratio",
  "empty_box_ratio",
  "fragmentation_ratio",
  "heading_jitter_deg",
  "speed_jitter_mps",
];

function applyPastedParams() {
  var textarea = document.getElementById("paste-params-json");
  var statusEl = document.getElementById("paste-apply-status");
  var btn = document.getElementById("btn-paste-apply");

  var raw = textarea.value.trim();
  if (!raw) {
    showError("Paste a JSON object first.");
    return;
  }

  var parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    showError("Invalid JSON: " + e.message);
    return;
  }

  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    showError("Expected a JSON object (not an array or primitive).");
    return;
  }

  // Filter out metric keys
  var tuningParams = {};
  var filtered = [];
  Object.keys(parsed).forEach(function (k) {
    if (METRIC_FILTER_KEYS.indexOf(k) !== -1) {
      filtered.push(k);
    } else {
      tuningParams[k] = parsed[k];
    }
  });

  var paramCount = Object.keys(tuningParams).length;
  if (paramCount === 0) {
    showError(
      "No tuning parameters found after filtering metrics. Keys filtered: " +
        filtered.join(", "),
    );
    return;
  }

  statusEl.textContent = "Applying " + paramCount + " params...";
  btn.disabled = true;

  fetch("/api/lidar/params?sensor_id=" + encodeURIComponent(sensorId), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(tuningParams),
  })
    .then(function (r) {
      if (!r.ok)
        return r.text().then(function (t) {
          throw new Error(t);
        });
      statusEl.textContent =
        "Applied " +
        paramCount +
        " params" +
        (filtered.length
          ? " (" + filtered.length + " metric keys filtered)"
          : "") +
        " ✓";
      btn.disabled = false;
      fetchCurrentParams();
    })
    .catch(function (e) {
      showError("Apply failed: " + e.message);
      statusEl.textContent = "";
      btn.disabled = false;
    });
}

function loadCurrentIntoEditor() {
  fetch(
    "/api/lidar/params?sensor_id=" +
      encodeURIComponent(sensorId) +
      "&format=pretty",
  )
    .then(function (r) {
      return r.json();
    })
    .then(function (params) {
      document.getElementById("paste-params-json").value = JSON.stringify(
        params,
        null,
        2,
      );
    })
    .catch(function (e) {
      showError("Failed to load current params: " + e.message);
    });
}

function downloadCSV() {
  if (!latestResults || latestResults.length === 0) {
    showError("No results to download.");
    return;
  }

  // Determine param columns from first result
  var paramKeys = [];
  if (latestResults[0] && latestResults[0].param_values) {
    paramKeys = Object.keys(latestResults[0].param_values);
  } else {
    paramKeys = ["noise", "closeness", "neighbour"];
  }

  var metricCols = [
    "accept_rate_mean",
    "accept_rate_stddev",
    "nonzero_cells_mean",
    "nonzero_cells_stddev",
    "active_tracks_mean",
    "active_tracks_stddev",
    "alignment_deg_mean",
    "alignment_deg_stddev",
    "misalignment_ratio_mean",
    "misalignment_ratio_stddev",
    "foreground_capture_mean",
    "foreground_capture_stddev",
    "unbounded_point_mean",
    "unbounded_point_stddev",
    "empty_box_ratio_mean",
    "empty_box_ratio_stddev",
    "fragmentation_ratio_mean",
    "heading_jitter_deg_mean",
    "heading_jitter_deg_stddev",
    "speed_jitter_mps_mean",
    "speed_jitter_mps_stddev",
  ];

  var header = paramKeys.concat(metricCols);
  var rows = [header.join(",")];

  latestResults.forEach(function (r) {
    var row = [];
    paramKeys.forEach(function (k) {
      var v;
      if (r.param_values && r.param_values[k] !== undefined) {
        v = r.param_values[k];
      } else {
        v = r[k];
      }
      row.push(v);
    });
    row.push(r.overall_accept_mean);
    row.push(r.overall_accept_stddev);
    row.push(r.nonzero_cells_mean);
    row.push(r.nonzero_cells_stddev);
    row.push(r.active_tracks_mean || 0);
    row.push(r.active_tracks_stddev || 0);
    row.push(r.alignment_deg_mean || 0);
    row.push(r.alignment_deg_stddev || 0);
    row.push(r.misalignment_ratio_mean || 0);
    row.push(r.misalignment_ratio_stddev || 0);
    row.push(r.foreground_capture_mean || 0);
    row.push(r.foreground_capture_stddev || 0);
    row.push(r.unbounded_point_mean || 0);
    row.push(r.unbounded_point_stddev || 0);
    row.push(r.empty_box_ratio_mean || 0);
    row.push(r.empty_box_ratio_stddev || 0);
    row.push(r.fragmentation_ratio_mean || 0);
    row.push(r.heading_jitter_deg_mean || 0);
    row.push(r.heading_jitter_deg_stddev || 0);
    row.push(r.speed_jitter_mps_mean || 0);
    row.push(r.speed_jitter_mps_stddev || 0);
    rows.push(row.join(","));
  });

  var csv = rows.join("\n");
  var blob = new Blob([csv], { type: "text/csv" });
  var a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = "sweep-results.csv";
  a.click();
  URL.revokeObjectURL(a.href);
}

function initCharts() {
  // Generate default chart configs if none exist
  if (chartConfigs.length === 0) {
    chartConfigs = generateDefaultCharts(null);
  }
  renderDynamicCharts(null);
}

function renderCharts(results) {
  if (chartConfigs.length === 0 && results && results.length > 0) {
    chartConfigs = generateDefaultCharts(results);
  }
  renderDynamicCharts(results);
}

function generateDefaultCharts(results) {
  var charts = [];
  var order = 0;
  charts.push({
    id: "default-accept",
    title: "Acceptance Rate",
    type: "bar",
    x_metric: "_combo",
    y_metric: "overall_accept_mean",
    group_by: "",
    order: order++,
  });
  charts.push({
    id: "default-nzcells",
    title: "Nonzero Background Cells",
    type: "bar",
    x_metric: "_combo",
    y_metric: "nonzero_cells_mean",
    group_by: "",
    order: order++,
  });
  charts.push({
    id: "default-tracks",
    title: "Active Tracks",
    type: "bar",
    x_metric: "_combo",
    y_metric: "active_tracks_mean",
    group_by: "",
    order: order++,
  });

  var numParams = [];
  if (results && results.length > 0 && results[0].param_values) {
    numParams = Object.keys(results[0].param_values).filter(function (k) {
      return typeof results[0].param_values[k] === "number";
    });
  }

  if (numParams.length >= 2) {
    charts.push({
      id: "default-accept-hm",
      title: "Acceptance Heatmap",
      type: "heatmap",
      x_metric: numParams[0],
      y_metric: numParams[1],
      z_metric: "overall_accept_mean",
      group_by: "",
      order: order++,
    });
  }

  charts.push({
    id: "default-align",
    title: "Alignment & Misalignment",
    type: "bar",
    x_metric: "_combo",
    y_metric: "alignment_deg_mean",
    group_by: "",
    order: order++,
  });

  if (numParams.length >= 2) {
    charts.push({
      id: "default-tracks-hm",
      title: "Tracks Heatmap",
      type: "heatmap",
      x_metric: numParams[0],
      y_metric: numParams[1],
      z_metric: "active_tracks_mean",
      group_by: "",
      order: order++,
    });
    charts.push({
      id: "default-align-hm",
      title: "Alignment Heatmap",
      type: "heatmap",
      x_metric: numParams[0],
      y_metric: numParams[1],
      z_metric: "alignment_deg_mean",
      group_by: "",
      order: order++,
    });
  }

  return charts;
}

function renderDynamicCharts(results) {
  var grid = document.getElementById("chart-grid");
  if (!grid) return;

  // Sort configs by order
  var sorted = chartConfigs.slice().sort(function (a, b) {
    return (a.order || 0) - (b.order || 0);
  });

  // Track which chart IDs are still active
  var activeIds = {};
  sorted.forEach(function (cfg) {
    activeIds[cfg.id] = true;
  });

  // Remove stale chart containers and instances
  Object.keys(chartInstances).forEach(function (id) {
    if (!activeIds[id]) {
      if (chartInstances[id]) {
        chartInstances[id].dispose();
        delete chartInstances[id];
      }
      var el = document.getElementById("chart-card-" + id);
      if (el) el.remove();
    }
  });

  sorted.forEach(function (cfg) {
    var cardId = "chart-card-" + cfg.id;
    var chartId = "chart-el-" + cfg.id;
    var card = document.getElementById(cardId);

    // Create container if needed
    if (!card) {
      card = document.createElement("div");
      card.className = "card";
      card.id = cardId;

      var actions = document.createElement("div");
      actions.className = "chart-card-actions";

      var editButton = document.createElement("button");
      editButton.type = "button";
      editButton.title = "Edit";
      editButton.textContent = "Edit";
      editButton.addEventListener("click", function () {
        editChart(cfg.id);
      });

      var removeButton = document.createElement("button");
      removeButton.type = "button";
      removeButton.title = "Remove";
      removeButton.textContent = "×";
      removeButton.addEventListener("click", function () {
        removeChart(cfg.id);
      });

      actions.appendChild(editButton);
      actions.appendChild(removeButton);

      var chartContainer = document.createElement("div");
      chartContainer.id = chartId;
      chartContainer.className = "chart-container";

      card.appendChild(actions);
      card.appendChild(chartContainer);
      grid.appendChild(card);
    }

    // Create or get ECharts instance
    var chartEl = document.getElementById(chartId);
    if (!chartInstances[cfg.id]) {
      chartInstances[cfg.id] = echarts.init(chartEl, chartTheme);
    }
    var chart = chartInstances[cfg.id];

    // Build and set option
    if (!results || results.length === 0) {
      chart.setOption(
        {
          title: {
            text: cfg.title || "Waiting for data...",
            left: "center",
            top: "center",
            textStyle: {
              color: "#94a3b8",
              fontSize: 14,
              fontWeight: "normal",
            },
          },
          backgroundColor: chartBg,
        },
        true,
      );
      return;
    }

    var opt = null;
    if (cfg.type === "heatmap") {
      opt = buildHeatmapOption(results, cfg);
    } else if (cfg.type === "scatter") {
      opt = buildScatterOption(results, cfg);
    } else if (cfg.type === "line") {
      opt = buildSeriesOption(results, cfg, "line");
    } else {
      opt = buildSeriesOption(results, cfg, "bar");
    }
    if (opt) {
      chart.setOption(opt, true);
    }
  });

  // Resize all after DOM settles
  setTimeout(function () {
    Object.keys(chartInstances).forEach(function (id) {
      if (chartInstances[id]) chartInstances[id].resize();
    });
  }, 50);
}

function buildSeriesOption(results, cfg, chartType) {
  var labels = results.map(comboLabel);
  var title = {
    text: cfg.title,
    left: "center",
    top: 0,
    textStyle: { fontSize: 14 },
  };

  if (cfg.x_metric === "_combo" && !cfg.group_by) {
    return {
      title: title,
      tooltip: { trigger: "axis" },
      xAxis: {
        type: "category",
        data: labels,
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: { type: "value", name: metricLabel(cfg.y_metric) },
      series: [
        {
          name: metricLabel(cfg.y_metric),
          type: chartType,
          data: results.map(function (r) {
            return extractValue(r, cfg.y_metric);
          }),
          itemStyle: { color: CHART_COLORS[0] },
        },
      ],
      grid: { bottom: 100 },
      backgroundColor: chartBg,
    };
  }

  // Grouped or specific x_metric
  var xKey = cfg.x_metric === "_combo" ? null : cfg.x_metric;
  var groupKey = cfg.group_by || null;

  if (groupKey) {
    var groups = {};
    results.forEach(function (r) {
      var gval = String(extractValue(r, groupKey));
      if (!groups[gval]) groups[gval] = [];
      groups[gval].push(r);
    });

    var xSet = {};
    results.forEach(function (r) {
      var xv = xKey ? extractValue(r, xKey) : comboLabel(r);
      xSet[xv] = true;
    });
    var xVals = Object.keys(xSet);
    if (xKey) {
      xVals = xVals.map(Number).sort(function (a, b) {
        return a - b;
      });
    }

    var series = [];
    var ci = 0;
    Object.keys(groups).forEach(function (gkey) {
      var gdata = groups[gkey];
      var dataMap = {};
      gdata.forEach(function (r) {
        var xv = xKey ? extractValue(r, xKey) : comboLabel(r);
        dataMap[String(xv)] = extractValue(r, cfg.y_metric);
      });
      series.push({
        name: metricLabel(groupKey) + "=" + gkey,
        type: chartType,
        data: xVals.map(function (xv) {
          return dataMap[String(xv)] != null ? dataMap[String(xv)] : null;
        }),
        itemStyle: { color: CHART_COLORS[ci % CHART_COLORS.length] },
      });
      ci++;
    });

    return {
      title: title,
      tooltip: { trigger: "axis" },
      legend: { bottom: 0, type: "scroll" },
      xAxis: {
        type: "category",
        data: xVals.map(String),
        name: xKey ? metricLabel(xKey) : "",
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: { type: "value", name: metricLabel(cfg.y_metric) },
      series: series,
      grid: { bottom: 80, top: 40 },
      backgroundColor: chartBg,
    };
  }

  // No group, specific x_metric
  if (xKey) {
    var sorted = results.slice().sort(function (a, b) {
      return (extractValue(a, xKey) || 0) - (extractValue(b, xKey) || 0);
    });
    return {
      title: title,
      tooltip: { trigger: "axis" },
      xAxis: {
        type: "category",
        data: sorted.map(function (r) {
          return String(extractValue(r, xKey));
        }),
        name: metricLabel(xKey),
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: { type: "value", name: metricLabel(cfg.y_metric) },
      series: [
        {
          name: metricLabel(cfg.y_metric),
          type: chartType,
          data: sorted.map(function (r) {
            return extractValue(r, cfg.y_metric);
          }),
          itemStyle: { color: CHART_COLORS[0] },
        },
      ],
      grid: { bottom: 100 },
      backgroundColor: chartBg,
    };
  }

  // Fallback: combo labels on x
  return {
    title: title,
    tooltip: { trigger: "axis" },
    xAxis: {
      type: "category",
      data: labels,
      axisLabel: { rotate: 45, fontSize: 10 },
    },
    yAxis: { type: "value", name: metricLabel(cfg.y_metric) },
    series: [
      {
        name: metricLabel(cfg.y_metric),
        type: chartType,
        data: results.map(function (r) {
          return extractValue(r, cfg.y_metric);
        }),
        itemStyle: { color: CHART_COLORS[0] },
      },
    ],
    grid: { bottom: 100 },
    backgroundColor: chartBg,
  };
}

function buildHeatmapOption(results, cfg) {
  var xKey = cfg.x_metric;
  var yKey = cfg.y_metric;
  var zKey = cfg.z_metric || "overall_accept_mean";

  var xSet = {};
  var ySet = {};
  results.forEach(function (r) {
    var xVal = extractValue(r, xKey);
    var yVal = extractValue(r, yKey);
    if (xVal != null) xSet[String(xVal)] = true;
    if (yVal != null) ySet[String(yVal)] = true;
  });

  // Check if values are numeric to decide on sorting strategy
  var xKeys = Object.keys(xSet);
  var yKeys = Object.keys(ySet);
  var xIsNumeric = xKeys.every(function (k) {
    return !isNaN(Number(k));
  });
  var yIsNumeric = yKeys.every(function (k) {
    return !isNaN(Number(k));
  });

  var xVals = xIsNumeric
    ? xKeys
        .map(Number)
        .sort(function (a, b) {
          return a - b;
        })
        .map(String)
    : xKeys.sort();
  var yVals = yIsNumeric
    ? yKeys
        .map(Number)
        .sort(function (a, b) {
          return a - b;
        })
        .map(String)
    : yKeys.sort();

  var data = [];
  var hmMax = 0;
  var hmMin = Infinity;
  results.forEach(function (r) {
    var xVal = String(extractValue(r, xKey));
    var yVal = String(extractValue(r, yKey));
    var xi = xVals.indexOf(xVal);
    var yi = yVals.indexOf(yVal);
    var v = extractValue(r, zKey) || 0;
    if (xi >= 0 && yi >= 0) {
      data.push([xi, yi, v]);
      if (v > hmMax) hmMax = v;
      if (v > 0 && v < hmMin) hmMin = v;
    }
  });
  if (hmMin === Infinity || hmMin >= hmMax) hmMin = 0;

  return {
    title: {
      text: cfg.title,
      left: "center",
      top: 0,
      textStyle: { fontSize: 14 },
    },
    tooltip: {
      formatter: function (p) {
        return (
          metricLabel(xKey) +
          ": " +
          xVals[p.value[0]] +
          "<br/>" +
          metricLabel(yKey) +
          ": " +
          yVals[p.value[1]] +
          "<br/>" +
          metricLabel(zKey) +
          ": " +
          p.value[2].toFixed(4)
        );
      },
    },
    xAxis: {
      type: "category",
      data: xVals.map(String),
      name: metricLabel(xKey),
      axisLabel: { fontSize: 10 },
    },
    yAxis: {
      type: "category",
      data: yVals.map(String),
      name: metricLabel(yKey),
      axisLabel: { fontSize: 10 },
    },
    visualMap: {
      min: hmMin,
      max: hmMax || 1,
      calculable: true,
      orient: "horizontal",
      left: "center",
      bottom: 0,
      inRange: {
        color: [
          "#313695",
          "#4575b4",
          "#74add1",
          "#abd9e9",
          "#fee090",
          "#fdae61",
          "#f46d43",
          "#d73027",
        ],
      },
    },
    series: [
      {
        type: "heatmap",
        data: data,
        emphasis: {
          itemStyle: { shadowBlur: 10, shadowColor: "rgba(0,0,0,0.5)" },
        },
      },
    ],
    grid: { bottom: 80, top: 60 },
    backgroundColor: chartBg,
  };
}

function buildScatterOption(results, cfg) {
  var xKey = cfg.x_metric;
  var yKey = cfg.y_metric;

  return {
    title: {
      text: cfg.title,
      left: "center",
      top: 0,
      textStyle: { fontSize: 14 },
    },
    tooltip: {
      formatter: function (p) {
        return (
          metricLabel(xKey) +
          ": " +
          p.value[0] +
          "<br/>" +
          metricLabel(yKey) +
          ": " +
          p.value[1]
        );
      },
    },
    xAxis: {
      type: "value",
      name: metricLabel(xKey),
    },
    yAxis: {
      type: "value",
      name: metricLabel(yKey),
    },
    series: [
      {
        type: "scatter",
        data: results.map(function (r) {
          return [extractValue(r, xKey), extractValue(r, yKey)];
        }),
        itemStyle: { color: CHART_COLORS[0] },
      },
    ],
    grid: { bottom: 60, top: 40 },
    backgroundColor: chartBg,
  };
}

function renderTable(results) {
  // Determine param columns from first result
  var paramKeys = [];
  if (results[0] && results[0].param_values) {
    paramKeys = Object.keys(results[0].param_values);
  } else {
    paramKeys = ["noise", "closeness", "neighbour"];
  }

  // Check if we have ground truth scores (Phase 5.4)
  var hasGroundTruth =
    results[0] &&
    (results[0].detection_rate !== undefined ||
      results[0].ground_truth_score !== undefined);

  // Rebuild header
  var thead = document.getElementById("results-head");
  var headerHtml = "<tr>";
  paramKeys.forEach(function (k) {
    var schema = PARAM_SCHEMA[k];
    headerHtml += "<th>" + escapeHTML(schema ? schema.label : k) + "</th>";
  });

  if (hasGroundTruth) {
    // Phase 5.4: Ground truth columns
    headerHtml += "<th>GT Score</th>";
    headerHtml += "<th>Detection %</th>";
    headerHtml += "<th>Frag.</th>";
    headerHtml += "<th>FP Rate</th>";
    headerHtml += "<th>Qual. Prem.</th>";
    headerHtml += "<th>Trunc.</th>";
    headerHtml += "<th>Vel. Noise</th>";
    headerHtml += "<th>Stopped OK</th>";
  } else {
    // Standard metrics
    headerHtml +=
      "<th>Accept Rate</th><th>± StdDev</th><th>Nonzero Cells</th><th>± StdDev</th>";
    headerHtml +=
      "<th>Active Tracks</th><th>Alignment (°)</th><th>Misalignment</th>";
    headerHtml +=
      "<th>Fg Capture</th><th>Unbounded</th><th>Empty Box</th><th>Fragmentation</th><th>Jitter (°)</th>";
  }

  headerHtml += "</tr>";
  thead.innerHTML = headerHtml;

  // Rebuild body
  var tbody = document.getElementById("results-body");
  tbody.innerHTML = "";
  results.forEach(function (r) {
    var tr = document.createElement("tr");
    var html = "";
    paramKeys.forEach(function (k) {
      var v;
      if (r.param_values && r.param_values[k] !== undefined) {
        v = r.param_values[k];
      } else {
        v = r[k];
      }
      if (typeof v === "number" && v !== Math.floor(v)) {
        html += '<td class="mono">' + escapeHTML(v.toFixed(4)) + "</td>";
      } else {
        html += '<td class="mono">' + escapeHTML(v) + "</td>";
      }
    });

    if (hasGroundTruth) {
      // Phase 5.4: Ground truth score columns
      html +=
        '<td class="mono">' +
        escapeHTML(
          ((r.ground_truth_score || r.composite_score || 0) * 100).toFixed(2),
        ) +
        "</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.detection_rate || 0) * 100).toFixed(1)) +
        "%</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.fragmentation || 0) * 100).toFixed(1)) +
        "%</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.false_positive_rate || 0) * 100).toFixed(1)) +
        "%</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.quality_premium || 0) * 100).toFixed(1)) +
        "%</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.truncation_rate || 0) * 100).toFixed(1)) +
        "%</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.velocity_noise_rate || 0) * 100).toFixed(1)) +
        "%</td>";
      html +=
        '<td class="mono">' +
        escapeHTML(((r.stopped_recovery_rate || 0) * 100).toFixed(1)) +
        "%</td>";
    } else {
      // Standard metrics
      html +=
        '<td class="mono">' +
        escapeHTML((r.overall_accept_mean * 100).toFixed(2)) +
        "%</td>" +
        '<td class="mono" style="color:var(--fg-faint)">±' +
        escapeHTML((r.overall_accept_stddev * 100).toFixed(2)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML(r.nonzero_cells_mean.toFixed(0)) +
        "</td>" +
        '<td class="mono" style="color:var(--fg-faint)">±' +
        escapeHTML(r.nonzero_cells_stddev.toFixed(0)) +
        "</td>" +
        '<td class="mono">' +
        escapeHTML((r.active_tracks_mean || 0).toFixed(1)) +
        "</td>" +
        '<td class="mono">' +
        escapeHTML((r.alignment_deg_mean || 0).toFixed(1)) +
        "°</td>" +
        '<td class="mono">' +
        escapeHTML(((r.misalignment_ratio_mean || 0) * 100).toFixed(1)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML(((r.foreground_capture_mean || 0) * 100).toFixed(1)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML(((r.unbounded_point_mean || 0) * 100).toFixed(1)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML(((r.empty_box_ratio_mean || 0) * 100).toFixed(1)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML(((r.fragmentation_ratio_mean || 0) * 100).toFixed(1)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML((r.heading_jitter_deg_mean || 0).toFixed(1)) +
        "°</td>";
    }

    tr.innerHTML = html;
    tbody.appendChild(tr);
  });
}

window.addEventListener("resize", function () {
  Object.keys(chartInstances).forEach(function (id) {
    if (chartInstances[id]) chartInstances[id].resize();
  });
});

// ---- Current params display ----

function fetchCurrentParams() {
  fetch("/api/lidar/params?sensor_id=" + encodeURIComponent(sensorId))
    .then(function (r) {
      if (!r.ok) throw new Error("Failed to fetch params");
      return r.json();
    })
    .then(function (params) {
      window.currentParamsCache = params;
      displayCurrentParams(params);
    })
    .catch(function (err) {
      document.getElementById("current-params-display").textContent =
        "Error loading parameters: " + err.message;
    });
}

function displayCurrentParams(params) {
  // Get list of currently swept parameters in order
  var sweptParams = {};
  var sweptParamOrder = [];
  var rows = document.getElementById("param-rows").children;
  for (var i = 0; i < rows.length; i++) {
    var rowId = rows[i].id.replace("param-row-", "");
    var nameEl = document.getElementById("pname-" + rowId);
    if (nameEl && nameEl.value) {
      sweptParams[nameEl.value] = true;
      sweptParamOrder.push(nameEl.value);
    }
  }

  // Sort keys: swept params first (in order), then remaining alphabetically
  var keys = Object.keys(params);
  var sweptKeys = sweptParamOrder.filter(function (k) {
    return keys.indexOf(k) !== -1;
  });
  var otherKeys = keys
    .filter(function (k) {
      return sweptParamOrder.indexOf(k) === -1;
    })
    .sort();
  var sortedKeys = sweptKeys.concat(otherKeys);

  var lines = [];
  sortedKeys.forEach(function (key) {
    var value = params[key];
    var displayValue = value;

    // Format value for display
    if (typeof value === "boolean") {
      displayValue = value ? "true" : "false";
    } else if (typeof value === "number") {
      if (Number.isInteger(value)) {
        displayValue = value.toString();
      } else {
        displayValue = value.toFixed(6).replace(/\.?0+$/, "");
      }
    } else if (value === null) {
      displayValue = "null";
    }

    var line = escapeHTML(key) + ": " + escapeHTML(displayValue);
    var isSwept = sweptParams[key] === true;

    if (isSwept) {
      lines.push('<span class="param-line swept">' + line + "</span>");
    } else {
      lines.push('<span class="param-line">' + line + "</span>");
    }
  });

  document.getElementById("current-params-display").innerHTML = lines.join("");
}

// ---- Chart builder ----

function openChartModal(editId) {
  var modal = document.getElementById("chart-modal");
  var titleEl = document.getElementById("chart-modal-title");
  var idEl = document.getElementById("chart-cfg-id");
  populateChartModalSelects();

  if (editId) {
    titleEl.textContent = "Edit Chart";
    idEl.value = editId;
    var cfg = chartConfigs.find(function (c) {
      return c.id === editId;
    });
    if (cfg) {
      document.getElementById("chart-cfg-title").value = cfg.title || "";
      document.getElementById("chart-cfg-type").value = cfg.type || "bar";
      document.getElementById("chart-cfg-x").value = cfg.x_metric || "_combo";
      document.getElementById("chart-cfg-y").value =
        cfg.y_metric || "overall_accept_mean";
      document.getElementById("chart-cfg-z").value =
        cfg.z_metric || "overall_accept_mean";
      document.getElementById("chart-cfg-group").value = cfg.group_by || "";
    }
  } else {
    titleEl.textContent = "Add Chart";
    idEl.value = "";
    document.getElementById("chart-cfg-title").value = "";
    document.getElementById("chart-cfg-type").value = "bar";
    document.getElementById("chart-cfg-x").value = "_combo";
    document.getElementById("chart-cfg-y").value = "overall_accept_mean";
    document.getElementById("chart-cfg-z").value = "overall_accept_mean";
    document.getElementById("chart-cfg-group").value = "";
  }
  onChartTypeChange();
  modal.style.display = "";
}

function closeChartModal() {
  document.getElementById("chart-modal").style.display = "none";
}

function onChartTypeChange() {
  var type = document.getElementById("chart-cfg-type").value;
  document.getElementById("chart-cfg-z-row").style.display =
    type === "heatmap" ? "" : "none";
  document.getElementById("chart-cfg-group-row").style.display =
    type === "heatmap" ? "none" : "";
}

function populateChartModalSelects() {
  var avail = getAvailableMetrics(latestResults);
  var xSel = document.getElementById("chart-cfg-x");
  var ySel = document.getElementById("chart-cfg-y");
  var zSel = document.getElementById("chart-cfg-z");
  var gSel = document.getElementById("chart-cfg-group");

  // X axis: _combo + params + metrics
  var xOpts = '<option value="_combo">Combination (label)</option>';
  avail.params.forEach(function (k) {
    xOpts +=
      '<option value="' +
      escapeHTML(k) +
      '">' +
      escapeHTML(metricLabel(k)) +
      "</option>";
  });
  avail.metrics.forEach(function (k) {
    xOpts +=
      '<option value="' +
      escapeHTML(k) +
      '">' +
      escapeHTML(metricLabel(k)) +
      "</option>";
  });
  xSel.innerHTML = xOpts;

  // Y axis: metrics
  var yOpts = "";
  avail.metrics.forEach(function (k) {
    yOpts +=
      '<option value="' +
      escapeHTML(k) +
      '">' +
      escapeHTML(metricLabel(k)) +
      "</option>";
  });
  // also add params as potential Y values
  avail.params.forEach(function (k) {
    yOpts +=
      '<option value="' +
      escapeHTML(k) +
      '">' +
      escapeHTML(metricLabel(k)) +
      "</option>";
  });
  ySel.innerHTML = yOpts;

  // Z axis (heatmap color): metrics only
  zSel.innerHTML = yOpts;

  // Group by: params
  var gOpts = '<option value="">(none)</option>';
  avail.params.forEach(function (k) {
    gOpts +=
      '<option value="' +
      escapeHTML(k) +
      '">' +
      escapeHTML(metricLabel(k)) +
      "</option>";
  });
  gSel.innerHTML = gOpts;
}

function applyChartModal() {
  var idEl = document.getElementById("chart-cfg-id");
  var editId = idEl.value;
  var type = document.getElementById("chart-cfg-type").value;

  var cfg = {
    id: editId || "chart-" + ++chartConfigCounter,
    title: document.getElementById("chart-cfg-title").value || "Chart",
    type: type,
    x_metric: document.getElementById("chart-cfg-x").value,
    y_metric: document.getElementById("chart-cfg-y").value,
    z_metric:
      type === "heatmap" ? document.getElementById("chart-cfg-z").value : "",
    group_by:
      type !== "heatmap"
        ? document.getElementById("chart-cfg-group").value
        : "",
    order: 0,
  };

  if (editId) {
    var idx = chartConfigs.findIndex(function (c) {
      return c.id === editId;
    });
    if (idx >= 0) {
      cfg.order = chartConfigs[idx].order;
      chartConfigs[idx] = cfg;
    }
  } else {
    cfg.order = chartConfigs.length;
    chartConfigs.push(cfg);
  }

  closeChartModal();
  renderDynamicCharts(latestResults);
  showSaveChartsButton();
}

function editChart(id) {
  openChartModal(id);
}

function removeChart(id) {
  chartConfigs = chartConfigs.filter(function (c) {
    return c.id !== id;
  });
  if (chartInstances[id]) {
    chartInstances[id].dispose();
    delete chartInstances[id];
  }
  var el = document.getElementById("chart-card-" + id);
  if (el) el.remove();
  showSaveChartsButton();
}

function showSaveChartsButton() {
  var btn = document.getElementById("btn-save-charts");
  if (btn && currentSweepId) {
    btn.style.display = "";
  }
}

function saveChartConfigs() {
  if (!currentSweepId) return;
  fetch("/api/lidar/sweeps/charts", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      sweep_id: currentSweepId,
      charts: chartConfigs,
    }),
  })
    .then(function (r) {
      if (!r.ok) throw new Error("Save failed");
      var btn = document.getElementById("btn-save-charts");
      if (btn) btn.textContent = "Saved";
      setTimeout(function () {
        if (btn) btn.textContent = "Save Charts";
      }, 1500);
    })
    .catch(function (e) {
      showError("Failed to save chart config: " + e.message);
    });
}

// ---- Sweep history ----

function loadSweepHistory() {
  fetch("/api/lidar/sweeps?sensor_id=" + encodeURIComponent(sensorId))
    .then(function (r) {
      return r.json();
    })
    .then(function (sweeps) {
      var sel = document.getElementById("sweep-history-select");
      // Preserve current value
      var curVal = sel.value;
      sel.innerHTML = '<option value="">Current (live)</option>';
      if (!sweeps || sweeps.length === 0) return;
      sweeps.forEach(function (s) {
        var opt = document.createElement("option");
        opt.value = s.sweep_id;
        var d = new Date(s.started_at);
        var label = s.mode + " | " + s.status + " | " + d.toLocaleString();
        opt.textContent = label;
        sel.appendChild(opt);
      });
      sel.value = curVal;
    })
    .catch(function () {});
}

function onSweepHistorySelected() {
  var sel = document.getElementById("sweep-history-select");
  var sweepId = sel.value;
  if (!sweepId) {
    // Switch back to live mode
    viewingHistorical = false;
    currentSweepId = null;
    document.getElementById("btn-save-charts").style.display = "none";
    // Clear and re-init to live state
    chartConfigs = [];
    disposeAllCharts();
    initCharts();
    startPolling();
    return;
  }
  loadHistoricalSweep(sweepId);
}

function loadHistoricalSweep(sweepId) {
  fetch("/api/lidar/sweeps/" + encodeURIComponent(sweepId))
    .then(function (r) {
      if (!r.ok) throw new Error("Failed to load sweep");
      return r.json();
    })
    .then(function (sweep) {
      viewingHistorical = true;
      currentSweepId = sweep.sweep_id;
      stopPolling();

      // Load chart configs if saved
      if (sweep.charts) {
        try {
          var parsed = JSON.parse(sweep.charts);
          if (Array.isArray(parsed) && parsed.length > 0) {
            chartConfigs = parsed;
          } else {
            chartConfigs = [];
          }
        } catch (e) {
          chartConfigs = [];
        }
      } else {
        chartConfigs = [];
      }

      // Parse and display results
      var results = null;
      if (sweep.results) {
        try {
          results =
            typeof sweep.results === "string"
              ? JSON.parse(sweep.results)
              : sweep.results;
        } catch (e) {
          results = null;
        }
      }

      if (results && results.length > 0) {
        latestResults = results;
        if (chartConfigs.length === 0) {
          chartConfigs = generateDefaultCharts(results);
        }
        disposeAllCharts();
        renderDynamicCharts(results);
        renderTable(results);
      }

      // Show recommendation if auto-tune
      if (sweep.recommendation) {
        try {
          var rec =
            typeof sweep.recommendation === "string"
              ? JSON.parse(sweep.recommendation)
              : sweep.recommendation;
          var roundResults = null;
          if (sweep.round_results) {
            roundResults =
              typeof sweep.round_results === "string"
                ? JSON.parse(sweep.round_results)
                : sweep.round_results;
          }
          renderRecommendation(rec, roundResults);
        } catch (e) {}
      }

      // Show save button
      showSaveChartsButton();

      // Update progress display
      var prog = document.getElementById("progress-section");
      prog.style.display = "";
      var badge = document.getElementById("status-badge");
      badge.textContent = sweep.status;
      badge.className = "status-badge status-" + sweep.status;
    })
    .catch(function (e) {
      showError("Failed to load sweep: " + e.message);
    });
}

function disposeAllCharts() {
  Object.keys(chartInstances).forEach(function (id) {
    if (chartInstances[id]) {
      chartInstances[id].dispose();
    }
  });
  chartInstances = {};
  var grid = document.getElementById("chart-grid");
  if (grid) grid.innerHTML = "";
}

// ---- CommonJS exports for testing ----
if (typeof module !== "undefined" && module.exports) {
  module.exports = {
    escapeHTML: escapeHTML,
    parseDuration: parseDuration,
    formatDuration: formatDuration,
    comboLabel: comboLabel,
    formatParamValues: formatParamValues,
    PARAM_SCHEMA: PARAM_SCHEMA,
    val: val,
    numVal: numVal,
    intVal: intVal,
    showError: showError,
    getParamValueCount: getParamValueCount,
    updateSweepSummary: updateSweepSummary,
    addParamRow: addParamRow,
    removeParamRow: removeParamRow,
    updateParamFields: updateParamFields,
    buildSceneJSON: buildSceneJSON,
    downloadScene: downloadScene,
    uploadScene: uploadScene,
    loadScene: loadScene,
    toggleJSONEditor: toggleJSONEditor,
    applyJSONEditor: applyJSONEditor,
    handleStart: handleStart,
    handleStartManualSweep: handleStartManualSweep,
    handleStartAutoTune: handleStartAutoTune,
    handleStop: handleStop,
    startPolling: startPolling,
    stopPolling: stopPolling,
    pollStatus: pollStatus,
    pollAutoTuneStatus: pollAutoTuneStatus,
    renderRecommendation: renderRecommendation,
    applyRecommendation: applyRecommendation,
    applySceneParams: applySceneParams,
    applyPastedParams: applyPastedParams,
    loadCurrentIntoEditor: loadCurrentIntoEditor,
    downloadCSV: downloadCSV,
    initCharts: initCharts,
    renderCharts: renderCharts,
    renderDynamicCharts: renderDynamicCharts,
    generateDefaultCharts: generateDefaultCharts,
    buildSeriesOption: buildSeriesOption,
    buildHeatmapOption: buildHeatmapOption,
    buildScatterOption: buildScatterOption,
    openChartModal: openChartModal,
    closeChartModal: closeChartModal,
    applyChartModal: applyChartModal,
    editChart: editChart,
    removeChart: removeChart,
    saveChartConfigs: saveChartConfigs,
    loadSweepHistory: loadSweepHistory,
    onSweepHistorySelected: onSweepHistorySelected,
    loadHistoricalSweep: loadHistoricalSweep,
    disposeAllCharts: disposeAllCharts,
    metricLabel: metricLabel,
    extractValue: extractValue,
    getAvailableMetrics: getAvailableMetrics,
    renderTable: renderTable,
    fetchCurrentParams: fetchCurrentParams,
    displayCurrentParams: displayCurrentParams,
    loadSweepScenes: loadSweepScenes,
    onSweepSceneSelected: onSweepSceneSelected,
    setMode: setMode,
    toggleWeights: toggleWeights,
    togglePCAP: togglePCAP,
    init: init,
  };
}

function init() {
  // Reset module-level state (prevents leaking between test runs)
  latestResults = null;
  stopRequested = false;
  sweepScenesData = [];
  currentSceneHasReference = false;
  chartConfigs = [];
  chartInstances = {};
  chartConfigCounter = 0;
  currentSweepId = null;
  viewingHistorical = false;

  sensorId = document.querySelector('meta[name="sensor-id"]').content;

  // Initialise charts empty on page load
  initCharts();

  // Load sweep history
  loadSweepHistory();

  // Check for existing sweep on page load
  fetch("/api/lidar/sweep/auto")
    .then(function (r) {
      return r.json();
    })
    .then(function (st) {
      if (st.status === "running") {
        setMode("auto");
        startPolling();
      } else if (st.status === "complete" && st.recommendation) {
        setMode("auto");
        if (st.results && st.results.length > 0) {
          document.getElementById("progress-section").style.display = "";
          var badge = document.getElementById("status-badge");
          badge.textContent = st.status;
          badge.className = "status-badge status-" + st.status;
          latestResults = st.results;
          renderCharts(st.results);
          renderTable(st.results);
        }
        renderRecommendation(st.recommendation, st.round_results);
      } else {
        // No auto-tune running, check manual sweep
        fetch("/api/lidar/sweep/status")
          .then(function (r) {
            return r.json();
          })
          .then(function (st) {
            if (st.status === "running") {
              startPolling();
            } else if (st.results && st.results.length > 0) {
              document.getElementById("progress-section").style.display = "";
              var badge = document.getElementById("status-badge");
              badge.textContent = st.status;
              badge.className = "status-badge status-" + st.status;
              document.getElementById("combo-count").textContent =
                st.completed_combos + " / " + st.total_combos + " combinations";
              latestResults = st.results;
              renderCharts(st.results);
              renderTable(st.results);
            }
          })
          .catch(function () {});
      }
    })
    .catch(function () {
      // Auto-tune endpoint not available, fall back to manual sweep check
      fetch("/api/lidar/sweep/status")
        .then(function (r) {
          return r.json();
        })
        .then(function (st) {
          if (st.status === "running") {
            startPolling();
          } else if (st.results && st.results.length > 0) {
            document.getElementById("progress-section").style.display = "";
            var badge = document.getElementById("status-badge");
            badge.textContent = st.status;
            badge.className = "status-badge status-" + st.status;
            document.getElementById("combo-count").textContent =
              st.completed_combos + " / " + st.total_combos + " combinations";
            latestResults = st.results;
            renderCharts(st.results);
            renderTable(st.results);
          }
        })
        .catch(function () {});
    });

  // Add a default parameter row
  addParamRow("noise_relative");
  updateSweepSummary();

  // Fetch and display current tuning parameters
  fetchCurrentParams();

  // Load scenes for scene selector
  loadSweepScenes();

  // Wire up live summary updates for top-level fields
  document
    .getElementById("iterations")
    .addEventListener("input", updateSweepSummary);
  document
    .getElementById("interval")
    .addEventListener("input", updateSweepSummary);
  document
    .getElementById("settle_time")
    .addEventListener("input", updateSweepSummary);
  document
    .getElementById("seed")
    .addEventListener("change", updateSweepSummary);

  // Wire up auto-tune settings for summary updates
  document
    .getElementById("max_rounds")
    .addEventListener("input", updateSweepSummary);
  document
    .getElementById("values_per_param")
    .addEventListener("input", updateSweepSummary);

  // Event delegation for dynamic param row inputs (start/end/step/values)
  document
    .getElementById("param-rows")
    .addEventListener("input", updateSweepSummary);

  // Update highlighted params when param rows change
  document.getElementById("param-rows").addEventListener("change", function () {
    displayCurrentParams(window.currentParamsCache || {});
  });
}

// ---- RLHF Functions ----

var rlhfPollTimer = null;
var lastRLHFPhase = "";

function handleStartRLHF() {
  var sceneSelect = document.getElementById("rlhf_scene_select");
  var sceneId = sceneSelect ? sceneSelect.value : "";
  if (!sceneId) {
    showError("Select a scene before starting RLHF sweep.");
    return;
  }

  var rows = document.getElementById("param-rows").children;
  var params = [];
  for (var i = 0; i < rows.length; i++) {
    var row = rows[i];
    var nameInput = row.querySelector(".param-name");
    var typeInput = row.querySelector(".param-type");
    var startInput = row.querySelector(".param-start");
    var endInput = row.querySelector(".param-end");
    if (nameInput && startInput && endInput) {
      params.push({
        name: nameInput.value,
        type: typeInput ? typeInput.value : "float64",
        start: parseFloat(startInput.value),
        end: parseFloat(endInput.value),
      });
    }
  }
  if (params.length === 0) {
    showError("Add at least one parameter.");
    return;
  }

  var durationsStr = (document.getElementById("rlhf_durations").value || "60").trim();
  var durations = durationsStr.split(",").map(function (s) {
    return parseInt(s.trim(), 10) || 60;
  });

  var req = {
    scene_id: sceneId,
    num_rounds: parseInt(document.getElementById("rlhf_rounds").value, 10) || 3,
    round_durations: durations,
    params: params,
    values_per_param: parseInt(document.getElementById("values_per_param").value, 10) || 5,
    top_k: parseInt(document.getElementById("top_k").value, 10) || 3,
    min_label_threshold:
      (parseInt(document.getElementById("rlhf_threshold").value, 10) || 90) / 100,
    carry_over_labels: document.getElementById("rlhf_carryover").checked,
  };

  document.getElementById("btn-start").style.display = "none";
  document.getElementById("btn-stop").style.display = "block";
  showError("");

  fetch("/api/lidar/sweep/rlhf", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  })
    .then(function (r) {
      return r.json().then(function (j) {
        if (!r.ok) throw new Error(j.error || "Start failed");
        return j;
      });
    })
    .then(function () {
      startRLHFPolling();
    })
    .catch(function (e) {
      showError(e.message);
      document.getElementById("btn-start").style.display = "block";
      document.getElementById("btn-stop").style.display = "none";
    });
}

function startRLHFPolling() {
  stopRLHFPolling();
  rlhfPollTimer = setInterval(pollRLHFStatus, 5000);
  pollRLHFStatus();
}

function stopRLHFPolling() {
  if (rlhfPollTimer) {
    clearInterval(rlhfPollTimer);
    rlhfPollTimer = null;
  }
}

function pollRLHFStatus() {
  fetch("/api/lidar/sweep/rlhf")
    .then(function (r) {
      return r.json();
    })
    .then(function (st) {
      renderRLHFState(st);

      // Phase transition notifications
      if (st.status !== lastRLHFPhase) {
        if (st.status === "awaiting_labels") {
          fireNotification("Labels needed — Round " + st.current_round,
            "RLHF sweep is waiting for track labels.");
        } else if (st.status === "completed") {
          fireNotification("RLHF Sweep Complete",
            "Parameter optimisation finished.");
        }
        lastRLHFPhase = st.status;
      }

      // Stop polling when complete or failed
      if (st.status === "completed" || st.status === "failed" || st.status === "idle") {
        stopRLHFPolling();
        document.getElementById("btn-start").style.display = "block";
        document.getElementById("btn-stop").style.display = "none";
        document.getElementById("stopping-indicator").style.display = "none";
      }
    })
    .catch(function () {});
}

function renderRLHFState(st) {
  var progressCard = document.getElementById("rlhf-progress-card");
  var historyCard = document.getElementById("rlhf-round-history");
  progressCard.style.display = "block";

  var statusText = document.getElementById("rlhf-status-text");
  var labelSection = document.getElementById("rlhf-label-progress");
  var sweepSection = document.getElementById("rlhf-sweep-progress");

  statusText.innerHTML =
    "<strong>Round " + st.current_round + " / " + st.total_rounds + "</strong> — " +
    "<span class=\"status-badge status-" + st.status + "\">" + st.status.replace(/_/g, " ") + "</span>";

  if (st.status === "awaiting_labels") {
    labelSection.style.display = "block";
    sweepSection.style.display = "none";

    if (st.label_progress) {
      var lp = st.label_progress;
      document.getElementById("rlhf-label-count").textContent =
        lp.labelled + "/" + lp.total + " labelled";
      document.getElementById("rlhf-label-pct").textContent =
        lp.progress_pct.toFixed(1) + "%";
      document.getElementById("rlhf-label-bar").style.width = lp.progress_pct + "%";

      var continueBtn = document.getElementById("rlhf-continue-btn");
      continueBtn.disabled = lp.progress_pct < (st.min_label_threshold * 100);
    }

    // Set threshold marker
    var marker = document.getElementById("rlhf-threshold-marker");
    marker.style.left = (st.min_label_threshold * 100) + "%";
    marker.title = (st.min_label_threshold * 100).toFixed(0) + "% threshold";

    // Countdown
    if (st.label_deadline) {
      var deadline = new Date(st.label_deadline);
      var remaining = Math.max(0, Math.floor((deadline - Date.now()) / 1000));
      var mins = Math.floor(remaining / 60);
      var secs = remaining % 60;
      document.getElementById("rlhf-countdown").textContent =
        "Deadline: " + mins + "m " + secs + "s remaining";
    }

    // Carried-over labels
    if (st.labels_carried_over > 0) {
      document.getElementById("rlhf-carried-count").textContent =
        "↻ " + st.labels_carried_over + " labels carried over";
    }

    // Tracks link
    if (st.reference_run_id) {
      document.getElementById("rlhf-tracks-link").href =
        "/lidar/tracks?run_id=" + encodeURIComponent(st.reference_run_id);
    }

  } else if (st.status === "running_sweep") {
    labelSection.style.display = "none";
    sweepSection.style.display = "block";

    if (st.auto_tune_state) {
      var ats = st.auto_tune_state;
      document.getElementById("rlhf-sweep-info").textContent =
        "Sweep: " + ats.completed_combos + "/" + ats.total_combos + " combos " +
        "(round " + ats.round + "/" + ats.total_rounds + ")";
    }

  } else if (st.status === "running_reference") {
    labelSection.style.display = "none";
    sweepSection.style.display = "block";
    document.getElementById("rlhf-sweep-info").textContent = "Creating reference run…";

  } else if (st.status === "completed") {
    labelSection.style.display = "none";
    sweepSection.style.display = "none";
    statusText.innerHTML += " — <strong>Optimisation complete</strong>";

    if (st.recommendation) {
      var recCard = document.getElementById("recommendation-card");
      recCard.style.display = "block";
      var html = "<table class=\"results-table\"><thead><tr><th>Param</th><th>Value</th></tr></thead><tbody>";
      for (var key in st.recommendation) {
        html += "<tr><td>" + key + "</td><td>" + st.recommendation[key] + "</td></tr>";
      }
      html += "</tbody></table>";
      document.getElementById("recommendation-content").innerHTML = html;
    }

  } else if (st.status === "failed") {
    labelSection.style.display = "none";
    sweepSection.style.display = "none";
    statusText.innerHTML += " — <span style=\"color:#cc0000\">" + (st.error || "Unknown error") + "</span>";
  }

  // Round history
  if (st.round_history && st.round_history.length > 0) {
    historyCard.style.display = "block";
    var list = document.getElementById("rlhf-rounds-list");
    var historyHtml = "";
    for (var i = 0; i < st.round_history.length; i++) {
      var rnd = st.round_history[i];
      historyHtml += "<div style=\"padding: 6px 0; border-bottom: 1px solid var(--card-border)\">";
      historyHtml += "<strong>Round " + rnd.round + "</strong>";
      if (rnd.best_score) historyHtml += " — Score: " + rnd.best_score.toFixed(4);
      if (rnd.labels_carried_over > 0) historyHtml += " (↻ " + rnd.labels_carried_over + " labels)";
      if (rnd.reference_run_id) {
        historyHtml += " <a href=\"/lidar/tracks?run_id=" + encodeURIComponent(rnd.reference_run_id) +
          "\" target=\"_blank\" style=\"font-size:12px\">tracks →</a>";
      }
      historyHtml += "</div>";
    }
    list.innerHTML = historyHtml;
  }
}

function handleRLHFContinue() {
  var nextDuration = parseInt(document.getElementById("rlhf-next-duration").value, 10) || 0;
  var addRound = document.getElementById("rlhf-add-round").checked;

  fetch("/api/lidar/sweep/rlhf/continue", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      next_sweep_duration_mins: nextDuration,
      add_round: addRound,
    }),
  })
    .then(function (r) {
      return r.json().then(function (j) {
        if (!r.ok) throw new Error(j.error || "Continue failed");
        return j;
      });
    })
    .then(function () {
      document.getElementById("rlhf-continue-btn").disabled = true;
    })
    .catch(function (e) {
      showError(e.message);
    });
}

function populateRLHFScenes() {
  var select = document.getElementById("rlhf_scene_select");
  if (!select) return;

  // Copy options from scene_select if it exists
  var mainSelect = document.getElementById("scene_select");
  if (mainSelect) {
    select.innerHTML = mainSelect.innerHTML;
  } else {
    fetch("/api/lidar/scenes")
      .then(function (r) { return r.json(); })
      .then(function (scenes) {
        select.innerHTML = "<option value=\"\">-- Select Scene --</option>";
        if (scenes && scenes.length) {
          for (var i = 0; i < scenes.length; i++) {
            var opt = document.createElement("option");
            opt.value = scenes[i].scene_id;
            opt.textContent = scenes[i].scene_id + (scenes[i].description ? " - " + scenes[i].description : "");
            select.appendChild(opt);
          }
        }
      })
      .catch(function () {});
  }
}

function onRLHFSceneSelected() {
  // Currently no additional action needed on scene selection
}

// ---- Browser Notifications ----

function requestNotificationPermission() {
  if (typeof Notification !== "undefined" && Notification.permission === "default") {
    Notification.requestPermission();
  }
}

function fireNotification(title, body) {
  if (typeof Notification !== "undefined" && Notification.permission === "granted") {
    var n = new Notification(title, { body: body, icon: "/favicon.ico" });
    n.onclick = function () {
      window.focus();
      n.close();
    };
  }
}

// ---- Page initialization (runs only in browser, not when required by Jest) ----
/* c8 ignore next 3 -- browser-only auto-init */
if (typeof document !== "undefined" && typeof module === "undefined") {
  init();
}
