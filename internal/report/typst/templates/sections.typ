// sections.typ — content sections matching the reference report layout.

#import "/preamble.typ": palette, fmt-speed, fmt-speed-bare, fmt-int, fmt-pct, fmt-pct-pp, mono, mono-nowrap

// Spanning title centered above the two-column body.
#let title-block(data) = {
  align(center)[
    #text(size: 22pt, weight: "bold")[#data.site.location] \
    #v(-0.05cm)
    #text(size: 11pt)[
      Surveyor: #emph(data.site.surveyor)
      #h(0.5em) • #h(0.5em)
      Contact: #link("mailto:" + data.site.contact)[#data.site.contact]
    ]
  ]
  v(0.3em)
}

// ─── Velocity Overview ────────────────────────────────────────────────────
#let velocity-overview(data) = {
  heading(level: 1)[Velocity Overview]
  let entries = if data.compare != none {
    let combined = data.overall.total_count + data.compare.overall.total_count
    (
      [#text(weight: "bold")[Site:] #data.site.location],
      [#text(weight: "bold")[Primary period (t1):] #mono(data.period.start_date) to #mono(data.period.end_date)],
      [#text(weight: "bold")[Comparison period (t2):] #mono(data.compare.start_date) to #mono(data.compare.end_date)],
      [#text(weight: "bold")[Total vehicle count:] #fmt-int(combined)],
    )
  } else {
    (
      [#text(weight: "bold")[Site:] #data.site.location],
      [#text(weight: "bold")[Period:] #mono(data.period.start_date) to #mono(data.period.end_date) (#fmt-int(data.overall.total_count) vehicles)],
    )
  }
  list(spacing: 0.35em, indent: 0.6em, ..entries)
}

// ─── Key Metrics ──────────────────────────────────────────────────────────
// Reference layout: 4 columns when comparing (Metric / Period t1 / Period t2 / Change),
// values rendered in monospace.
#let key-metrics(data) = {
  let units = data.period.units
  heading(level: 2)[Key Metrics]

  let header-cells = if data.compare != none {
    (
      [#text(weight: "bold")[Metric]],
      [#text(weight: "bold")[Period t1]],
      [#text(weight: "bold")[Period t2]],
      [#text(weight: "bold")[Change]],
    )
  } else {
    (
      [#text(weight: "bold")[Metric]],
      [#text(weight: "bold")[Value]],
    )
  }

  // mono-nowrap binds multi-word cells ("p50 Velocity") into one line.
  let cell(s) = mono-nowrap(s)
  let mk-row(label, t1, t2, delta) = if data.compare != none {
    ([#cell(label)], [#cell(t1)], [#cell(t2)], [#cell(delta)])
  } else {
    ([#cell(label)], [#cell(t1)])
  }

  let t1 = data.overall
  let t2 = if data.compare != none { data.compare.overall } else { none }
  let rows = (
    mk-row("p50 Velocity",
           fmt-speed(t1.p50, units),
           if t2 != none { fmt-speed(t2.p50, units) } else { "" },
           if t2 != none { fmt-pct(t1.p50, t2.p50) } else { "" }),
    mk-row("p85 Velocity",
           fmt-speed(t1.p85, units),
           if t2 != none { fmt-speed(t2.p85, units) } else { "" },
           if t2 != none { fmt-pct(t1.p85, t2.p85) } else { "" }),
    mk-row("p98 Velocity",
           fmt-speed(t1.p98, units),
           if t2 != none { fmt-speed(t2.p98, units) } else { "" },
           if t2 != none { fmt-pct(t1.p98, t2.p98) } else { "" }),
    mk-row("Max Velocity",
           fmt-speed(t1.max_speed, units),
           if t2 != none { fmt-speed(t2.max_speed, units) } else { "" },
           if t2 != none { fmt-pct(t1.max_speed, t2.max_speed) } else { "" }),
  )
  // Vehicle Count row: no Δ to avoid misleading comparison across unequal periods.
  let count-row = if data.compare != none {
    ([#cell("Vehicle Count")], [#cell(fmt-int(t1.total_count))], [#cell(fmt-int(t2.total_count))], [])
  } else {
    none
  }

  let cols = if data.compare != none { (auto, auto, auto, auto) } else { (auto, auto) }
  table(
    columns: cols,
    align: if data.compare != none { (left, right, right, right) } else { (left, right) },
    stroke: none,
    inset: (x: 4pt, y: 2pt),
    table.header(..header-cells),
    table.hline(stroke: 0.6pt),
    ..rows.flatten(),
    ..if count-row != none { count-row } else { () },
    table.hline(stroke: 0.6pt),
  )
  align(center)[#text(size: 8.5pt, weight: "bold")[Table 1: Key Metrics]]
}

// ─── Site Information ─────────────────────────────────────────────────────
#let site-information(data) = {
  let desc = data.site.at("site_description", default: "")
  let note = data.site.at("speed_limit_note", default: "")
  if desc == "" and note == "" { return }
  heading(level: 2)[Site Information]
  if desc != "" { par[#desc] }
  if note != "" { par[#note] }
}

// ─── Citizen Radar ────────────────────────────────────────────────────────
#let citizen-radar() = {
  heading(level: 2)[Citizen Radar]
  par[
    #link("https://velocity.report")[velocity.report] is a citizen radar
    tool designed to help communities measure vehicle speeds with affordable,
    privacy-preserving Doppler sensors. It's built on a core physical truth:
    kinetic energy scales with the square of speed.
  ]
  align(center)[$ K_E = 1/2 m v^2 $]
  align(center)[#text(size: 9pt)[where #math.italic[m] is the mass and #math.italic[v] is the velocity.]]
  par[
    A vehicle traveling at 40 mph has four times the crash energy of the
    same vehicle at 20 mph, posing exponentially greater risk to people
    outside the car. Even small increases in speed dramatically raise the
    likelihood of severe injury or death in a collision. By quantifying
    real-world vehicle speeds,
    #link("https://velocity.report")[velocity.report]
    produces evidence that exceeds industry standard metrics.
  ]
}

// ─── Aggregation and Percentiles ──────────────────────────────────────────
#let aggregation-and-percentiles() = {
  heading(level: 2)[Aggregation and Percentiles]
  par[
    This system uses Doppler radar to measure vehicle speed by detecting
    frequency shifts in waves reflected from objects in motion. This shift
    (known as the
    #link("https://en.wikipedia.org/wiki/Doppler_effect")[Doppler effect])
    is directly proportional to the object's relative velocity. When the
    sensor is stationary, the Doppler effect reports the true speed of an
    object moving toward or away from the radar.
  ]
  par[
    To structure this data, the velocity.report application first records
    individual radar readings, then applies a greedy, local, univariate
    #emph[Time-Contiguous Speed Clustering] algorithm to group log lines
    into sessions based on time proximity and speed similarity. Each
    session, or "transit," represents a short burst of movement consistent
    with a single passing object. This approach is efficient and
    reproducible, but in dense traffic or where objects overlap it may
    undercount vehicles by merging multiple objects into one transit.
  ]
  par[
    Undercounting can bias percentile metrics (like p85 and p98) downward,
    since fewer sessions can give disproportionate weight to slower
    vehicles. All reported statistics in this report are derived from these
    sessionised transits.
  ]
  par[
    Percentiles offer a structured way to interpret speed behaviour. The
    85th percentile (p85) indicates the speed at or below which 85% of
    vehicles traveled. The 98th percentile (p98) exceeds this
    industry-standard measure by capturing the fastest 2% of vehicle
    speeds, providing a more robust view into trends among top speeders. By
    extending beyond p85, p98 identifies an additional 13% of data that
    would otherwise be missed when trimming the top 15%, offering clearer
    insight into high-risk driving patterns without letting single
    anomalous readings dominate.
  ]
  par[
    However, percentile metrics can be unstable in periods with low sample
    counts. To reflect this, our charts flag low-sample segments in orange
    and suppress percentile points when counts fall below reliability
    thresholds (fewer than 50 samples per roll-up period).
  ]
}

// ─── Hardware Configuration ───────────────────────────────────────────────
#let hardware-configuration(data) = {
  heading(level: 2)[Hardware Configuration]
  let rows = (
    ("Radar Sensor:",          data.radar.sensor_model),
    ("Firmware version:",      data.radar.firmware_version),
    ("Transmit Frequency:",    data.radar.transmit_frequency),
    ("Sample Rate:",           data.radar.sample_rate),
    ("Velocity Resolution:",   data.radar.velocity_resolution),
    ("Azimuth Field of View:", data.radar.azimuth_fov),
    ("Elevation Field of View:", data.radar.elevation_fov),
  )
  let body = rows.map(((k, v)) => (
    [#text(weight: "bold")[#k]],
    [#mono(v)],
  )).flatten()
  table(
    columns: (auto, 1fr),
    align: (left, left),
    stroke: none,
    inset: (x: 0pt, y: 2pt),
    column-gutter: 6pt,
    ..body,
  )
}

// ─── Survey Parameters ────────────────────────────────────────────────────
// Reference renders this as a key:value list (left-aligned), not a table.
#let survey-parameters(data) = {
  heading(level: 2)[Survey Parameters]
  let p = data.period
  let r = data.radar
  let cmp = data.compare
  let rows = (
    ("Units:",                  p.units),
    ("Minimum speed (cutoff):", p.min_speed_str),
    ("Roll-up Period:",         p.group),
    ("Timezone:",               p.timezone),
    ("Start time (t1):",        p.start_iso),
    ("End time (t1):",          p.end_iso),
  )
  let cmp-rows = if cmp != none {
    (
      ("Start time (t2):",        cmp.start_iso),
      ("End time (t2):",          cmp.end_iso),
    )
  } else { () }
  let cosine-rows = if cmp != none {
    (
      ("Cosine Error Angle (t1):",   str(r.cosine_error_angle) + "°"),
      ("Cosine Error Factor (t1):",  str(r.cosine_error_factor)),
      ("Cosine Error Angle (t2):",   str(r.compare_cosine_error_angle) + "°"),
      ("Cosine Error Factor (t2):",  str(r.compare_cosine_error_factor)),
    )
  } else {
    (
      ("Cosine Error Angle:",   str(r.cosine_error_angle) + "°"),
      ("Cosine Error Factor:",  str(r.cosine_error_factor)),
    )
  }
  let all-rows = rows + cmp-rows + cosine-rows
  let body = all-rows.map(((k, v)) => (
    [#text(weight: "bold")[#k]],
    [#mono(v)],
  )).flatten()
  table(
    columns: (auto, 1fr),
    align: (left, left),
    stroke: none,
    inset: (x: 0pt, y: 2pt),
    column-gutter: 6pt,
    ..body,
  )
  let note = data.at("cosine_correction_note", default: "")
  if note != "" {
    v(0.2em)
    par[#text(size: 8.5pt)[#note]]
  }
}

// ─── Detailed Data Tables (heading) ───────────────────────────────────────
#let detailed-data-tables-heading() = {
  heading(level: 2)[Detailed Data Tables]
}

// ─── Table 2: Velocity Distribution ───────────────────────────────────────
// Reference shape: Bucket | t1 Count | t1 Percent | t2 Count | t2 Percent | Delta
#let velocity-distribution-table(data) = {
  let units = data.period.units
  let h1 = data.histogram.buckets
  let h2 = if data.compare != none { data.compare.histogram.buckets } else { () }
  // Build a label-keyed lookup for t2 buckets so unmatched labels still appear.
  let lookup-t2 = (:)
  if data.compare != none {
    for b in h2 {
      lookup-t2.insert(b.label, b)
    }
  }

  let hdr(top, bot) = align(right)[#text(weight: "bold")[#top#linebreak()#bot]]
  let hdr-l(top, bot) = align(left)[#text(weight: "bold")[#top#linebreak()#bot]]
  let header = if data.compare != none {
    (
      hdr-l[Bucket][(#units)],
      hdr[t1][Count],
      hdr[t1][Percent],
      hdr[t2][Count],
      hdr[t2][Percent],
      align(right)[#text(weight: "bold")[Delta]],
    )
  } else {
    (
      hdr-l[Bucket][(#units)],
      align(right)[#text(weight: "bold")[Count]],
      align(right)[#text(weight: "bold")[Percent]],
    )
  }

  let cell(s) = mono-nowrap(s)
  let pct(v) = if v == none { "" } else {
    // Always show one decimal so "28%" → "28.0%"
    let r = calc.round(v, digits: 1)
    let s = str(r)
    if not s.contains(".") { s + ".0%" } else { s + "%" }
  }
  let body = h1.map(b => {
    let t2 = lookup-t2.at(b.label, default: none)
    let t1pct = b.at("percent", default: none)
    let t2pct = if t2 != none { t2.at("percent", default: none) } else { none }
    if data.compare != none {
      (
        [#cell(b.label)],
        [#cell(fmt-int(b.count))],
        [#cell(pct(t1pct))],
        [#cell(if t2 != none { fmt-int(t2.count) } else { "" })],
        [#cell(pct(t2pct))],
        [#cell(fmt-pct-pp(t1pct, t2pct))],
      )
    } else {
      (
        [#cell(b.label)],
        [#cell(fmt-int(b.count))],
        [#cell(pct(t1pct))],
      )
    }
  }).flatten()

  let cols = if data.compare != none {
    (40pt, 1fr, 1fr, 1fr, 1fr, 1fr)
  } else {
    (40pt, 1fr, 1fr)
  }
  let aligns = if data.compare != none {
    (left, right, right, right, right, right)
  } else {
    (left, right, right)
  }
  table(
    columns: cols,
    align: aligns,
    stroke: none,
    inset: (x: 3pt, y: 2pt),
    table.header(..header),
    table.hline(stroke: 0.6pt),
    ..body,
    table.hline(stroke: 0.6pt),
  )
  align(center)[#text(size: 8.5pt, weight: "bold")[Table 2: Velocity Distribution (#units)]]
}

// ─── Table 3: Daily Percentile Summary (merged t1+t2) ─────────────────────
#let daily-summary(data) = {
  let units = data.period.units
  let merged = data.daily
  if data.compare != none {
    merged = data.daily + data.compare.daily
  }
  if merged.len() == 0 { return }

  let hdr(top, bot) = align(right)[#text(weight: "bold")[#top#linebreak()#bot]]
  let header = (
    align(left)[#text(weight: "bold")[Start#linebreak()Time]],
    align(right)[#text(weight: "bold")[Count]],
    hdr[p50][(#units)],
    hdr[p85][(#units)],
    hdr[p98][(#units)],
    hdr[Max][(#units)],
  )
  let cell(s) = mono-nowrap(s)
  let body = merged.map(row => (
    [#cell(row.date)],
    [#cell(fmt-int(row.count))],
    [#cell(fmt-speed-bare(row.p50))],
    [#cell(fmt-speed-bare(row.p85))],
    [#cell(fmt-speed-bare(row.p98))],
    [#cell(fmt-speed-bare(row.max_speed))],
  )).flatten()

  table(
    columns: (52pt, 32pt, 1fr, 1fr, 1fr, 1fr),
    align: (left, right, right, right, right, right),
    stroke: none,
    inset: (x: 3pt, y: 2pt),
    table.header(..header),
    table.hline(stroke: 0.6pt),
    ..body,
    table.hline(stroke: 0.6pt),
  )
  let title = if data.compare != none {
    "Table 3: Daily Percentile Summary (Comparison)"
  } else {
    "Table 3: Daily Percentile Summary"
  }
  align(center)[#text(size: 8.5pt, weight: "bold")[#title]]
}

// ─── Table 4: Granular Percentile Breakdown (merged t1+t2) ────────────────
#let granular-table(data) = {
  let units = data.period.units
  let merged = data.granular
  if data.compare != none {
    merged = data.granular + data.compare.granular
  }
  if merged.len() == 0 { return }

  let hdr(top, bot) = align(right)[#text(weight: "bold")[#top#linebreak()#bot]]
  let header = (
    align(left)[#text(weight: "bold")[Start#linebreak()Time]],
    align(right)[#text(weight: "bold")[Count]],
    hdr[p50][(#units)],
    hdr[p85][(#units)],
    hdr[p98][(#units)],
    hdr[Max][(#units)],
  )
  let cell(s) = mono-nowrap(s)
  let body = merged.map(row => (
    [#cell(row.bucket)],
    [#cell(fmt-int(row.count))],
    [#cell(fmt-speed-bare(row.p50))],
    [#cell(fmt-speed-bare(row.p85))],
    [#cell(fmt-speed-bare(row.p98))],
    [#cell(fmt-speed-bare(row.max_speed))],
  )).flatten()

  table(
    columns: (52pt, 32pt, 1fr, 1fr, 1fr, 1fr),
    align: (left, right, right, right, right, right),
    stroke: none,
    inset: (x: 3pt, y: 2pt),
    table.header(..header),
    table.hline(stroke: 0.6pt),
    ..body,
    table.hline(stroke: 0.6pt),
  )
  let title = if data.compare != none {
    "Table 4: Granular Percentile Breakdown (Comparison)"
  } else {
    "Table 4: Granular Percentile Breakdown"
  }
  align(center)[#text(size: 8.5pt, weight: "bold")[#title]]
}

// ─── Comparison histogram figure (column-width when present) ─────────────
#let histogram-figure(data) = {
  let h = data.charts.at("histogram", default: "")
  if h == "" { return }
  figure(
    image(h, width: 100%),
    caption: [Velocity Distribution Histogram],
    supplement: [Figure],
  )
}

// ─── Time-series chart figures (one per period) ──────────────────────────
// Each figure uses #place(scope: "parent") so the chart spans both columns.
#let timeseries-figures(data) = {
  let ts1 = data.charts.at("timeseries", default: "")
  let ts2 = data.charts.at("timeseries_compare", default: "")
  let pri-cap = if data.compare != none {
    [Velocity over time (t1: #data.period.start_date to #data.period.end_date)]
  } else {
    [Velocity over time (#data.period.start_date to #data.period.end_date)]
  }
  if ts1 != "" {
    figure(
      image(ts1, width: 100%),
      caption: pri-cap,
      supplement: [Figure],
    )
  }
  if ts2 != "" and data.compare != none {
    v(1em)
    figure(
      image(ts2, width: 100%),
      caption: [Velocity over time (t2: #data.compare.start_date to #data.compare.end_date)],
      supplement: [Figure],
    )
  }
}

// ─── Site map figure ─────────────────────────────────────────────────────
#let map-figure(data) = {
  let mp = data.charts.at("map", default: "")
  if mp == "" { return }
  figure(
    image(mp, width: 100%),
    caption: [Site Location Map with radar location (circle) and coverage area (red triangle)],
    supplement: [Figure],
  )
}
