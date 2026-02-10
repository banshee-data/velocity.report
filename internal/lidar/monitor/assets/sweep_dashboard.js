/* sweep_dashboard.js — extracted from sweep_dashboard.html for testability. */
/* global echarts */

/* Shared utilities: browser receives them via prior <script> tag;
   Node/Jest pulls them in via require(). */
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

var pollTimer = null;
var stopRequested = false;
var sweepMode = "manual"; // 'manual' or 'auto'
var sensorId = null;
var acceptChart = null;
var nzChart = null;
var bktChart = null;
var alignChart = null;
var tracksChart = null;
var paramHeatmapChart = null;
var tracksHeatmapChart = null;
var alignHeatmapChart = null;
var latestResults = null;

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
    desc: "Fraction of measured range treated as noise threshold (0\u20131). Higher = more tolerant of range variation.",
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
    desc: "Number of neighbouring cells (0\u20138) that must agree before marking foreground.",
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
    label: "Gating Distance\u00b2",
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
};

var paramNames = Object.keys(PARAM_SCHEMA);
var paramCounter = 0;

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

  // Populate the PCAP fields so buildScenarioJSON / handleStartAutoTune can read them
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
  if (mode === "auto") {
    document.body.classList.add("auto-mode");
  } else {
    document.body.classList.remove("auto-mode");
  }
  // Update button text
  document.getElementById("btn-start").textContent =
    mode === "auto" ? "Start Auto-Tune" : "Start Sweep";
  document.getElementById("btn-stop").textContent =
    mode === "auto" ? "Stop Auto-Tune" : "Stop Sweep";
  updateSweepSummary();
}

function toggleWeights() {
  var obj = document.getElementById("objective").value;
  document.getElementById("weight-fields").style.display =
    obj === "weighted" ? "" : "none";
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
            " \u2192 " +
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

// ---- Scenario management ----

function buildScenarioJSON() {
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

function downloadScenario() {
  var obj = buildScenarioJSON();
  var json = JSON.stringify(obj, null, 2);
  var blob = new Blob([json], { type: "application/json" });
  var a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = "sweep-scenario.json";
  a.click();
  URL.revokeObjectURL(a.href);
}

function uploadScenario(input) {
  if (!input.files || !input.files[0]) return;
  var reader = new FileReader();
  reader.onload = function (e) {
    try {
      var obj = JSON.parse(e.target.result);
      loadScenario(obj);
    } catch (err) {
      showError("Invalid JSON: " + err.message);
    }
  };
  reader.readAsText(input.files[0]);
  input.value = "";
}

function loadScenario(obj) {
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
      buildScenarioJSON(),
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
    loadScenario(obj);
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
  if (rows.length === 0) {
    showError("Add at least one parameter.");
    return;
  }

  if (sweepMode === "auto") {
    handleStartAutoTune();
  } else {
    handleStartManualSweep();
  }
}

function handleStartManualSweep() {
  var req = buildScenarioJSON();
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
    };
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

  var stopUrl =
    sweepMode === "auto"
      ? "/api/lidar/sweep/auto/stop"
      : "/api/lidar/sweep/stop";
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
          " \u2192 acceptance=" +
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
        roundInfo =
          "Round " + (st.round || 1) + "/" + st.total_rounds + " \u2014 ";
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
          " \u2014 " +
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
      k !== "nonzero_cells"
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
    "\u00b0</span></div>";
  metricsHtml +=
    '<div class="metric">Nonzero Cells: <span class="metric-value">' +
    escapeHTML((rec.nonzero_cells || 0).toFixed(0)) +
    "</span></div>";
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
          k !== "nonzero_cells"
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
        "Applied \u2713";
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
  acceptChart = echarts.init(
    document.getElementById("acceptance-chart"),
    chartTheme,
  );
  nzChart = echarts.init(document.getElementById("nonzero-chart"), chartTheme);
  bktChart = echarts.init(document.getElementById("bucket-chart"), chartTheme);
  alignChart = echarts.init(
    document.getElementById("alignment-chart"),
    chartTheme,
  );
  tracksChart = echarts.init(
    document.getElementById("tracks-chart"),
    chartTheme,
  );
  paramHeatmapChart = echarts.init(
    document.getElementById("param-heatmap"),
    chartTheme,
  );
  tracksHeatmapChart = echarts.init(
    document.getElementById("tracks-heatmap"),
    chartTheme,
  );
  alignHeatmapChart = echarts.init(
    document.getElementById("alignment-heatmap"),
    chartTheme,
  );

  var emptyOpt = {
    title: {
      text: "Waiting for data...",
      left: "center",
      top: "center",
      textStyle: { color: "#94a3b8", fontSize: 14, fontWeight: "normal" },
    },
    backgroundColor: chartBg,
  };
  acceptChart.setOption(emptyOpt);
  nzChart.setOption(emptyOpt);
  bktChart.setOption(emptyOpt);
  alignChart.setOption(emptyOpt);
  tracksChart.setOption(emptyOpt);
  paramHeatmapChart.setOption(emptyOpt);
  tracksHeatmapChart.setOption(emptyOpt);
  alignHeatmapChart.setOption(emptyOpt);
}

function renderCharts(results) {
  var labels = results.map(comboLabel);

  // Overall acceptance chart
  acceptChart.setOption(
    {
      title: {
        text: "Overall Acceptance Rate",
        left: "center",
        top: 0,
        textStyle: { fontSize: 14 },
      },
      tooltip: { trigger: "axis" },
      xAxis: {
        type: "category",
        data: labels,
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: {
        type: "value",
        name: "Acceptance Rate",
        axisLabel: {
          formatter: function (v) {
            return (v * 100).toFixed(0) + "%";
          },
        },
      },
      series: [
        {
          name: "Mean",
          type: "bar",
          data: results.map(function (r) {
            return r.overall_accept_mean;
          }),
          itemStyle: { color: "#5470c6" },
        },
      ],
      grid: { bottom: 100 },
      backgroundColor: chartBg,
    },
    true,
  );

  // Nonzero cells chart
  nzChart.setOption(
    {
      title: {
        text: "Nonzero Background Cells",
        left: "center",
        top: 0,
        textStyle: { fontSize: 14 },
      },
      tooltip: { trigger: "axis" },
      xAxis: {
        type: "category",
        data: labels,
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: { type: "value", name: "Cell Count" },
      series: [
        {
          name: "Mean",
          type: "bar",
          data: results.map(function (r) {
            return r.nonzero_cells_mean;
          }),
          itemStyle: { color: "#91cc75" },
        },
      ],
      grid: { bottom: 100 },
      backgroundColor: chartBg,
    },
    true,
  );

  // Bucket heatmap
  if (results[0] && results[0].buckets && results[0].buckets.length > 0) {
    var buckets = results[0].buckets;
    var data = [];
    var mx = 0;
    var mn = Infinity;
    results.forEach(function (r, ri) {
      if (r.bucket_means) {
        r.bucket_means.forEach(function (v, bi) {
          data.push([ri, bi, v]);
          if (v > mx) mx = v;
          if (v > 0 && v < mn) mn = v;
        });
      }
    });
    if (mn === Infinity || mn >= mx) mn = 0;
    bktChart.setOption(
      {
        title: {
          text: "Per-Bucket Acceptance Rates",
          left: "center",
          top: 0,
          textStyle: { fontSize: 14 },
        },
        tooltip: {
          formatter: function (p) {
            var ri = p.value[0],
              bi = p.value[1],
              v = p.value[2];
            return (
              comboLabel(results[ri]) +
              "<br/>Bucket " +
              buckets[bi] +
              "m: " +
              (v * 100).toFixed(2) +
              "%"
            );
          },
        },
        xAxis: {
          type: "category",
          data: labels,
          axisLabel: { rotate: 45, fontSize: 10 },
        },
        yAxis: {
          type: "category",
          data: buckets.map(function (b) {
            return b + "m";
          }),
          name: "Range Bucket",
        },
        visualMap: {
          min: mn,
          max: mx || 1,
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
          formatter: function (v) {
            return (v * 100).toFixed(1) + "%";
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
      },
      true,
    );
  }

  // Track alignment chart (lower is better)
  alignChart.setOption(
    {
      title: {
        text: "Track Alignment (lower = better)",
        left: "center",
        top: 0,
        textStyle: { fontSize: 14 },
      },
      tooltip: {
        trigger: "axis",
        formatter: function (p) {
          var d = p[0];
          var r = results[d.dataIndex];
          return (
            comboLabel(r) +
            "<br/>Alignment: " +
            (r.alignment_deg_mean || 0).toFixed(1) +
            "\u00b0 \u00b1" +
            (r.alignment_deg_stddev || 0).toFixed(1) +
            "<br/>Misalignment: " +
            ((r.misalignment_ratio_mean || 0) * 100).toFixed(1) +
            "%"
          );
        },
      },
      xAxis: {
        type: "category",
        data: labels,
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: [
        { type: "value", name: "Alignment (\u00b0)", position: "left" },
        {
          type: "value",
          name: "Misalignment %",
          position: "right",
          axisLabel: {
            formatter: function (v) {
              return (v * 100).toFixed(0) + "%";
            },
          },
        },
      ],
      series: [
        {
          name: "Alignment",
          type: "bar",
          data: results.map(function (r) {
            return r.alignment_deg_mean || 0;
          }),
          itemStyle: { color: "#ee6666" },
        },
        {
          name: "Misalignment",
          type: "line",
          yAxisIndex: 1,
          data: results.map(function (r) {
            return r.misalignment_ratio_mean || 0;
          }),
          lineStyle: { color: "#fac858" },
          itemStyle: { color: "#fac858" },
        },
      ],
      legend: { bottom: 0 },
      grid: { bottom: 120, top: 40 },
      backgroundColor: chartBg,
    },
    true,
  );

  // Active tracks chart
  tracksChart.setOption(
    {
      title: {
        text: "Active Tracks",
        left: "center",
        top: 0,
        textStyle: { fontSize: 14 },
      },
      tooltip: { trigger: "axis" },
      xAxis: {
        type: "category",
        data: labels,
        axisLabel: { rotate: 45, fontSize: 10 },
      },
      yAxis: { type: "value", name: "Track Count" },
      series: [
        {
          name: "Mean",
          type: "bar",
          data: results.map(function (r) {
            return r.active_tracks_mean || 0;
          }),
          itemStyle: { color: "#73c0de" },
        },
      ],
      grid: { bottom: 100 },
      backgroundColor: chartBg,
    },
    true,
  );

  // Parameter heatmap: show acceptance rate for first two numerical params
  if (results[0] && results[0].param_values) {
    var pKeys = Object.keys(results[0].param_values);
    var numKeys = pKeys.filter(function (k) {
      return typeof results[0].param_values[k] === "number";
    });
    if (numKeys.length >= 2) {
      var xKey = numKeys[0],
        yKey = numKeys[1];
      var xSchema = PARAM_SCHEMA[xKey],
        ySchema = PARAM_SCHEMA[yKey];
      var xLabel = xSchema ? xSchema.label : xKey;
      var yLabel = ySchema ? ySchema.label : yKey;
      // Collect unique sorted axis values
      var xSet = {},
        ySet = {};
      results.forEach(function (r) {
        xSet[r.param_values[xKey]] = true;
        ySet[r.param_values[yKey]] = true;
      });
      var xVals = Object.keys(xSet)
        .map(Number)
        .sort(function (a, b) {
          return a - b;
        });
      var yVals = Object.keys(ySet)
        .map(Number)
        .sort(function (a, b) {
          return a - b;
        });
      var hmData = [];
      var hmMax = 0;
      var hmMin = Infinity;
      results.forEach(function (r) {
        var xi = xVals.indexOf(r.param_values[xKey]);
        var yi = yVals.indexOf(r.param_values[yKey]);
        var v = r.overall_accept_mean || 0;
        if (xi >= 0 && yi >= 0) {
          hmData.push([xi, yi, v]);
          if (v > hmMax) hmMax = v;
          if (v > 0 && v < hmMin) hmMin = v;
        }
      });
      if (hmMin === Infinity || hmMin >= hmMax) hmMin = 0;
      paramHeatmapChart.setOption(
        {
          title: {
            text: "Acceptance by " + xLabel + " vs " + yLabel,
            left: "center",
            top: 0,
            textStyle: { fontSize: 14 },
          },
          tooltip: {
            formatter: function (p) {
              return (
                xLabel +
                ": " +
                xVals[p.value[0]] +
                "<br/>" +
                yLabel +
                ": " +
                yVals[p.value[1]] +
                "<br/>" +
                "Accept: " +
                (p.value[2] * 100).toFixed(2) +
                "%"
              );
            },
          },
          xAxis: {
            type: "category",
            data: xVals.map(String),
            name: xLabel,
            axisLabel: { fontSize: 10 },
          },
          yAxis: {
            type: "category",
            data: yVals.map(String),
            name: yLabel,
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
            formatter: function (v) {
              return (v * 100).toFixed(1) + "%";
            },
          },
          series: [
            {
              type: "heatmap",
              data: hmData,
              emphasis: {
                itemStyle: { shadowBlur: 10, shadowColor: "rgba(0,0,0,0.5)" },
              },
            },
          ],
          grid: { bottom: 80, top: 60 },
          backgroundColor: chartBg,
        },
        true,
      );
      document.getElementById("param-heatmap").style.display = "";
    } else {
      document.getElementById("param-heatmap").style.display = "none";
    }
  } else {
    document.getElementById("param-heatmap").style.display = "none";
  }

  // Tracks heatmap and alignment heatmap (both use first two numerical params)
  if (results[0] && results[0].param_values) {
    var pKeys2 = Object.keys(results[0].param_values);
    var numKeys2 = pKeys2.filter(function (k) {
      return typeof results[0].param_values[k] === "number";
    });
    if (numKeys2.length >= 2) {
      var xKey2 = numKeys2[0],
        yKey2 = numKeys2[1];
      var xSchema2 = PARAM_SCHEMA[xKey2],
        ySchema2 = PARAM_SCHEMA[yKey2];
      var xLabel2 = xSchema2 ? xSchema2.label : xKey2;
      var yLabel2 = ySchema2 ? ySchema2.label : yKey2;
      var xSet2 = {},
        ySet2 = {};
      results.forEach(function (r) {
        xSet2[r.param_values[xKey2]] = true;
        ySet2[r.param_values[yKey2]] = true;
      });
      var xVals2 = Object.keys(xSet2)
        .map(Number)
        .sort(function (a, b) {
          return a - b;
        });
      var yVals2 = Object.keys(ySet2)
        .map(Number)
        .sort(function (a, b) {
          return a - b;
        });

      // Tracks heatmap
      var thmData = [];
      var thmMax = 0;
      var thmMin = Infinity;
      results.forEach(function (r) {
        var xi = xVals2.indexOf(r.param_values[xKey2]);
        var yi = yVals2.indexOf(r.param_values[yKey2]);
        var v = r.active_tracks_mean || 0;
        if (xi >= 0 && yi >= 0) {
          thmData.push([xi, yi, v]);
          if (v > thmMax) thmMax = v;
          if (v > 0 && v < thmMin) thmMin = v;
        }
      });
      if (thmMin === Infinity || thmMin >= thmMax) thmMin = 0;
      tracksHeatmapChart.setOption(
        {
          title: {
            text: "Active Tracks by " + xLabel2 + " vs " + yLabel2,
            left: "center",
            top: 0,
            textStyle: { fontSize: 14 },
          },
          tooltip: {
            formatter: function (p) {
              return (
                xLabel2 +
                ": " +
                xVals2[p.value[0]] +
                "<br/>" +
                yLabel2 +
                ": " +
                yVals2[p.value[1]] +
                "<br/>" +
                "Tracks: " +
                p.value[2].toFixed(1)
              );
            },
          },
          xAxis: {
            type: "category",
            data: xVals2.map(String),
            name: xLabel2,
            axisLabel: { fontSize: 10 },
          },
          yAxis: {
            type: "category",
            data: yVals2.map(String),
            name: yLabel2,
            axisLabel: { fontSize: 10 },
          },
          visualMap: {
            min: thmMin,
            max: thmMax || 1,
            calculable: true,
            orient: "horizontal",
            left: "center",
            bottom: 0,
            inRange: {
              color: ["#f7fcf5", "#c7e9c0", "#74c476", "#238b45", "#00441b"],
            },
          },
          series: [
            {
              type: "heatmap",
              data: thmData,
              emphasis: {
                itemStyle: { shadowBlur: 10, shadowColor: "rgba(0,0,0,0.5)" },
              },
            },
          ],
          grid: { bottom: 80, top: 60 },
          backgroundColor: chartBg,
        },
        true,
      );
      document.getElementById("tracks-heatmap").style.display = "";

      // Alignment heatmap
      var ahmData = [];
      var ahmMax = 0;
      var ahmMin = Infinity;
      results.forEach(function (r) {
        var xi = xVals2.indexOf(r.param_values[xKey2]);
        var yi = yVals2.indexOf(r.param_values[yKey2]);
        var v = r.alignment_deg_mean || 0;
        if (xi >= 0 && yi >= 0) {
          ahmData.push([xi, yi, v]);
          if (v > ahmMax) ahmMax = v;
          if (v > 0 && v < ahmMin) ahmMin = v;
        }
      });
      if (ahmMin === Infinity || ahmMin >= ahmMax) ahmMin = 0;
      alignHeatmapChart.setOption(
        {
          title: {
            text: "Alignment (\u00b0) by " + xLabel2 + " vs " + yLabel2,
            left: "center",
            top: 0,
            textStyle: { fontSize: 14 },
          },
          tooltip: {
            formatter: function (p) {
              return (
                xLabel2 +
                ": " +
                xVals2[p.value[0]] +
                "<br/>" +
                yLabel2 +
                ": " +
                yVals2[p.value[1]] +
                "<br/>" +
                "Alignment: " +
                p.value[2].toFixed(1) +
                "\u00b0"
              );
            },
          },
          xAxis: {
            type: "category",
            data: xVals2.map(String),
            name: xLabel2,
            axisLabel: { fontSize: 10 },
          },
          yAxis: {
            type: "category",
            data: yVals2.map(String),
            name: yLabel2,
            axisLabel: { fontSize: 10 },
          },
          visualMap: {
            min: ahmMin,
            max: ahmMax || 1,
            calculable: true,
            orient: "horizontal",
            left: "center",
            bottom: 0,
            inRange: {
              color: [
                "#00441b",
                "#238b45",
                "#74c476",
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
              data: ahmData,
              emphasis: {
                itemStyle: { shadowBlur: 10, shadowColor: "rgba(0,0,0,0.5)" },
              },
            },
          ],
          grid: { bottom: 80, top: 60 },
          backgroundColor: chartBg,
        },
        true,
      );
      document.getElementById("alignment-heatmap").style.display = "";
    } else {
      document.getElementById("tracks-heatmap").style.display = "none";
      document.getElementById("alignment-heatmap").style.display = "none";
    }
  } else {
    document.getElementById("tracks-heatmap").style.display = "none";
    document.getElementById("alignment-heatmap").style.display = "none";
  }
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
      "<th>Accept Rate</th><th>\u00b1 StdDev</th><th>Nonzero Cells</th><th>\u00b1 StdDev</th>";
    headerHtml +=
      "<th>Active Tracks</th><th>Alignment (\u00b0)</th><th>Misalignment</th>";
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
        '<td class="mono" style="color:var(--fg-faint)">\u00b1' +
        escapeHTML((r.overall_accept_stddev * 100).toFixed(2)) +
        "%</td>" +
        '<td class="mono">' +
        escapeHTML(r.nonzero_cells_mean.toFixed(0)) +
        "</td>" +
        '<td class="mono" style="color:var(--fg-faint)">\u00b1' +
        escapeHTML(r.nonzero_cells_stddev.toFixed(0)) +
        "</td>" +
        '<td class="mono">' +
        escapeHTML((r.active_tracks_mean || 0).toFixed(1)) +
        "</td>" +
        '<td class="mono">' +
        escapeHTML((r.alignment_deg_mean || 0).toFixed(1)) +
        "\u00b0</td>" +
        '<td class="mono">' +
        escapeHTML(((r.misalignment_ratio_mean || 0) * 100).toFixed(1)) +
        "%</td>";
    }

    tr.innerHTML = html;
    tbody.appendChild(tr);
  });
}

window.addEventListener("resize", function () {
  if (acceptChart) acceptChart.resize();
  if (nzChart) nzChart.resize();
  if (bktChart) bktChart.resize();
  if (paramHeatmapChart) paramHeatmapChart.resize();
  if (alignChart) alignChart.resize();
  if (tracksChart) tracksChart.resize();
  if (tracksHeatmapChart) tracksHeatmapChart.resize();
  if (alignHeatmapChart) alignHeatmapChart.resize();
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
    buildScenarioJSON: buildScenarioJSON,
    downloadScenario: downloadScenario,
    uploadScenario: uploadScenario,
    loadScenario: loadScenario,
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
    downloadCSV: downloadCSV,
    initCharts: initCharts,
    renderCharts: renderCharts,
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

  sensorId = document.querySelector('meta[name="sensor-id"]').content;

  // Initialise charts empty on page load
  initCharts();

  // Resize charts after layout settles (CSS grid may not have final dimensions at init time)
  setTimeout(function () {
    acceptChart.resize();
    nzChart.resize();
    bktChart.resize();
    paramHeatmapChart.resize();
    alignChart.resize();
    tracksChart.resize();
    tracksHeatmapChart.resize();
    alignHeatmapChart.resize();
  }, 100);

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

// ---- Page initialization (runs only in browser, not when required by Jest) ----
if (typeof document !== "undefined" && typeof module === "undefined") {
  init();
}
