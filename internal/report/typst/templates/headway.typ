// headway.typ — the headway report: observed following exposure.
//
// The data is the headway_report_v1 contract (internal/report/headway,
// model.go). Every number, name and suppression arrives as the exact text to
// print, so this file is layout only: it formats no value and names no
// metric, reason or status itself. The status label is printed in the page
// header, the footer, a background mark and a banner, and on every chart.

#import "/preamble.typ": apply-styles, palette, mono, mono-nowrap
#import "/sections.typ": data-table, kv-table

#let data = json("/data.json")

#set document(
  title: data.title + " (" + data.status_label + ")",
  keywords: ("status:" + data.status, "contract:" + data.contract),
)

#let status-box(size: 9pt) = box(
  stroke: 1.2pt + black,
  inset: (x: 5pt, y: 3pt),
  text(size: size, weight: "bold")[#data.status_label],
)

#set page(
  paper: data.paper,
  margin: (top: 1.9cm, bottom: 1.3cm, left: 1.0cm, right: 1.0cm),
  header: {
    set text(size: 8pt)
    grid(
      columns: (1fr, auto),
      align: (left + horizon, right + horizon),
      [#text(weight: "bold")[#link("https://velocity.report")[velocity.report]] · #data.title],
      status-box(size: 8pt),
    )
    v(-0.3em)
    line(length: 100%, stroke: 0.8pt + palette.rule)
  },
  footer: {
    line(length: 100%, stroke: 0.8pt + palette.rule)
    v(-0.4em)
    set text(size: 8pt)
    grid(
      columns: (1fr, 1fr),
      align: (left, right),
      [#text(weight: "bold")[#data.status_label] · #mono(data.contract)],
      [Page #context counter(page).display()],
    )
  },
  background: place(center + horizon, rotate(-40deg, box(
    text(size: 54pt, weight: "bold", fill: luma(238), hyphenate: false)[#data.status_label],
  ))),
)

#show: apply-styles
// Table cells hold ids and preformatted values; justification would stretch
// the spaces inside them.
#show table: set par(justify: false)

// ─── Helpers ──────────────────────────────────────────────────────────────

// tok prints a registry id, token, track id or locator in the mono face,
// with a break opportunity after each "/", "." and "_" so a long id wraps
// inside its cell instead of running into the next one. Hyphenation is off:
// a hyphen inserted into a token would print a name the registry lacks.
#let tok(s) = text(hyphenate: false, mono(s.replace("/", "/\u{200B}").replace(".", ".\u{200B}").replace("_", "_\u{200B}")))

// A list of strings joined with commas, or "none" for an empty list (an
// empty array joins to none in Typst).
#let listing(xs) = tok(if xs.len() == 0 { "none" } else { xs.join(", ") })

// A reason tally, "reason × n", or "none".
#let reason-counts(xs, key) = listing(xs.map(x => x.reason + " × " + str(x.at(key))))

#let bold(s) = text(weight: "bold")[#s]
#let num(s) = mono-nowrap(s)
#let head(..items) = items.pos().map(x => bold(x))

// ─── Title and status ─────────────────────────────────────────────────────

#align(center)[
  #text(size: 20pt, weight: "bold")[#data.title]
  #v(0.2em)
  #status-box(size: 12pt)
]
#v(0.4em)
#block(width: 100%, stroke: 1pt + black, inset: 7pt)[
  #bold(data.status_label) · #data.status_note
]

#heading(level: 1)[Scope]
#list(spacing: 0.5em, ..data.statements.map(s => [#s]))

#heading(level: 2)[Method versions]
#kv-table((
  ("Contract:", data.contract),
  ("Status:", data.status),
  ("Encounter method:", data.methods.encounter),
  ("Local path:", data.methods.local_path),
  ("Leader choice:", data.methods.pairing),
  ("Synchronisation:", data.methods.sync),
  ("Pointwise equations:", data.methods.pointwise),
  ("Exposure:", data.methods.exposure),
))

#heading(level: 2)[Named bands]
#data-table(
  columns: (auto, 1fr, auto),
  aligns: (left, left, left),
  header: head[Band][Metric][Benchmark],
  body: data.bands.map(b => (
    num(b.display), tok(b.duration_metric), tok(b.duration_benchmark),
    [], tok(b.rate_metric), tok(b.rate_benchmark),
  )).flatten(),
  caption: "Descriptive bins with their registered duration and rate metrics",
)

// ─── Aggregates ───────────────────────────────────────────────────────────

#heading(level: 1)[Aggregates]
#par[
  Encounters are pooled only within one version group, and only where a
  value is supported. Suppressed encounters are counted by reason. The
  distribution covers the valid following time of encounters whose band
  exposure is supported; every other second of the group's accounted
  encounter time is shown beside it under its reason.
]

#for a in data.aggregates {
  heading(level: 2)[Version group #a.id]
  kv-table((
    ("Estimate stage:", a.version.estimate_stage),
    ("Estimator:", a.version.estimator_id),
    ("Observation model:", a.version.obs_model_id),
    ("Estimator parameters:", a.version.param_hash),
    ("Method and parameters:", a.version.method_id),
    ("Values read from:", a.value_block),
    ("Encounters:", a.encounter_ids.join(", ")),
  ))
  data-table(
    columns: (auto, auto, auto, auto, 1fr, 1fr),
    aligns: (left, right, right, right, right, left),
    header: head[Band][Encounters][Time below][Valid following time][Pooled rate][Excluded encounters],
    body: a.bands.map(b => (
      num(b.display), num(str(b.encounters)), num(b.below_display), num(b.valid_display),
      if b.rate_value == none { tok(b.rate_display) } else { num(b.rate_display) },
      reason-counts(b.excluded, "encounters"),
      table.cell(colspan: 6, text(size: 7pt)[#tok(b.duration_metric) · #tok(b.rate_metric)]),
    )).flatten(),
    caption: "Band exposure, " + a.id + ": time below each band over valid following time",
  )
  data-table(
    columns: (auto, auto, 1fr, auto, auto, auto),
    aligns: (left, right, left, right, right, right),
    header: head[Metric][Supported][Suppressed][Min][p50][Max],
    body: a.metrics.map(d => {
      let row = (tok(d.metric), num(str(d.supported)), reason-counts(d.suppressed, "encounters"))
      if d.supported == 0 {
        row + (table.cell(colspan: 3, tok(d.min_display)),)
      } else {
        row + (num(d.min_display), num(d.p50_display), num(d.max_display))
      }
    }).flatten(),
    caption: "Encounter values, " + a.id + ": supported values only; no uncertainty is propagated to this level",
  )
  let h = a.histogram
  figure(
    image(a.chart, width: 100%),
    caption: [
      Valid following time by #tok(h.metric) over #h.encounters encounters with supported band
      exposure, as a share of #h.denominator_display of accounted encounter time, #a.id.
      Suppressed time is shown by reason; nothing is folded into zero.
    ],
    supplement: [Figure],
  )
  data-table(
    columns: (1fr, auto, auto),
    aligns: (left, right, right),
    header: head[Suppressed time][Time][Share],
    body: if h.excluded.len() == 0 { (tok("none"), [], []) } else {
      h.excluded.map(x => (tok(x.reason), num(x.display), num(x.share_display))).flatten()
    },
    caption: "Accounted encounter time outside the distribution, " + a.id,
  )
}

// ─── Encounters ───────────────────────────────────────────────────────────

#pagebreak(weak: true)
#heading(level: 1)[Encounters]
#par[
  One row per leader and follower pair. Each value is a registered
  measurement with its uncertainty, or a suppression with its reason and no
  number. Unsupported intervals are listed rather than dropped, and the
  review-only predicted gap is shown apart with its coast age.
]
#data-table(
  columns: (auto, 1fr, 1.2fr, 1.2fr, auto),
  aligns: (left, left, left, left, left),
  header: head[ID][Capture][Leader][Follower][Values read from],
  body: data.encounters.map(e => (
    num(e.id), tok(e.capture_id), tok(e.leader.track_id), tok(e.follower.track_id),
    tok(e.estimate_stage + " / " + e.value_block),
  )).flatten(),
  caption: "Encounter index",
)

#for e in data.encounters {
  pagebreak(weak: true)
  heading(level: 2)[Encounter #e.id: #e.capture_id]
  kv-table((
    ("Leader:", e.leader.track_id + " (" + e.leader.motion_class + ")"),
    ("Leader locator:", e.leader.locator),
    ("Follower:", e.follower.track_id + " (" + e.follower.motion_class + ")"),
    ("Follower locator:", e.follower.locator),
    ("Directed path:", e.path.geometry_id),
    ("Path extent:", e.path.length_display + ", " + str(e.path.knots) + " knots, " + str(e.path.bridged_knots) + " bridged"),
    ("Window (UTC):", e.first_utc + " to " + e.last_utc + " (" + e.duration_display + ")"),
    ("Estimate stage:", e.estimate_stage + "; values read from " + e.value_block),
    ("Estimator:", e.version.estimator_id + ", " + e.version.obs_model_id + ", " + e.version.param_hash),
    ("Method:", e.version.method_id),
    ("Frames:", str(e.input.observed_frames) + " observed, " + str(e.input.coasted_frames) + " coasted; planar fallback "
      + (if e.input.planar_fallback { "true" } else { "false" })),
  ))
  data-table(
    columns: (auto, auto, 1fr, auto),
    aligns: (left, right, left, right),
    header: head[Metric][Value][Uncertainty][Opportunity],
    // A suppression spans the value, uncertainty and opportunity columns: it
    // has none of the three, and a narrow cell must not squeeze its reason.
    body: e.measurements.map(m => if m.suppressed {
      (tok(m.metric), table.cell(colspan: 3, tok(m.display)))
    } else {
      (
        tok(m.metric),
        num(m.display),
        text(hyphenate: false, mono(m.uncertainty_display)),
        num(m.at("opportunity_display", default: "")),
      )
    }).flatten(),
    caption: "Measurements, " + e.id,
  )

  let ep = e.endpoints
  par[
    #bold[Endpoint sources.] Leader trailing:
    #listing(ep.leader_trailing_sources.map(s => s.source + " × " + str(s.instants))),
    follower leading:
    #listing(ep.follower_leading_sources.map(s => s.source + " × " + str(s.instants))).
  ]
  let m = ep.at("at_minimum_gap", default: none)
  if m != none {
    data-table(
      columns: (auto, 1fr, auto, auto, auto),
      aligns: (left, left, left, left, left),
      header: head[Endpoint][Track][Arc along path][Source][Support],
      body: (m.leader, m.follower).map(p => (
        tok(p.extremity), tok(p.track_id), num(p.display), tok(p.source), tok(p.support),
      )).flatten(),
      caption: "Endpoints at the minimum supported spatial gap (" + m.gap_display + ", at " + m.offset_display + "), " + e.id,
    )
  }

  let acc = e.accounting
  data-table(
    columns: (1fr, auto, auto),
    aligns: (left, right, right),
    header: head[Time][Instants][Duration],
    body: (
      (tok("valid"), [], num(acc.valid_display)),
      ..acc.suppressions.map(s => (tok(s.reason), num(str(s.instants)), num(s.display))),
      (bold[accounted], [], num(acc.accounted_display)),
      ([unobserved, within the suppressed time], [], num(acc.unobserved_display)),
      ([record gaps, standing for nothing], [], num(acc.record_gap_display)),
    ).flatten(),
    caption: "Where the encounter's time went, " + e.id,
  )

  if e.unsupported.len() > 0 {
    data-table(
      columns: (auto, auto, auto, 1fr, auto, auto),
      aligns: (left, right, right, left, left, left),
      header: head[From the first instant][Instants][Time][Reason][Condition][Leader role],
      body: e.unsupported.map(u => (
        num(u.range_display), num(str(u.instants)), num(u.duration_display),
        tok(u.reason), tok(u.at("condition", default: "")), tok(u.role),
      )).flatten(),
      caption: "Unsupported intervals: not valid following time, " + e.id,
    )
  }

  if e.predicted.len() > 0 {
    data-table(
      columns: (auto, auto, auto, 1fr, auto),
      aligns: (left, right, left, left, left),
      header: head[From the first instant][Instants][Coast age][Predicted gap][Largest sigma],
      body: e.predicted.map(p => (
        num(p.range_display), num(str(p.instants)), num(p.coast_age_display), num(p.gap_display), num(p.sigma_max_display),
      )).flatten(),
      caption: "Predicted-only intervals (" + e.predicted.at(0).metric + ", " + e.predicted.at(0).visibility
        + "), excluded from every aggregate, " + e.id,
    )
  }

  figure(
    image(e.chart, width: 100%),
    caption: [
      Encounter #e.id. Upper: spatial gap, observed (solid, one-sigma band) and review-only
      predicted (dashed, hollow). Lower: valid net time gap against the named bands. Shaded
      intervals are not valid following time.
    ],
    supplement: [Figure],
  )
}

// ─── Captures and followers ───────────────────────────────────────────────

#pagebreak(weak: true)
#heading(level: 1)[Captures]
#data-table(
  columns: (auto, 1fr, auto, 1.4fr),
  aligns: (left, left, left, left),
  header: head[Capture][Source][Parameters][Description],
  body: data.captures.map(c => (
    tok(c.id), tok(c.source), tok(c.params_hash), [#c.description],
  )).flatten(),
  caption: "Captures, each analysed with its own parameters",
)

#heading(level: 2)[Every track as a follower]
#par[
  Each track is taken in turn as a follower. A refused path is listed with
  its conditions; free flow is time with no body ahead, which is neither
  following nor a suppression.
]
#for c in data.captures {
  data-table(
    columns: (auto, 1fr, auto, auto, 1fr, auto),
    aligns: (left, left, right, right, left, right),
    header: head[Track][Path][Leader][Free flow][Suppressed][Record gaps],
    body: c.followers.map(f => (
      tok(f.track_id),
      tok(if f.at("path_conditions", default: ()).len() > 0 { "refused: " + f.path_conditions.join(", ") } else { f.geometry_id }),
      num(f.leader_display),
      num(f.free_flow_display),
      listing(f.suppressed.map(s =>
        s.reason + (if s.at("condition", default: "") != "" { " " + s.condition } else { "" }) + " " + s.display)),
      num(f.record_gap_display),
    )).flatten(),
    caption: "Followers, " + c.id,
  )
}
