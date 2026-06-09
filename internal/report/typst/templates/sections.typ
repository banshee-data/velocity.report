// sections.typ — content sections matching the reference report layout.

#import "/preamble.typ": palette, fmt-speed, fmt-speed-bare, fmt-int, fmt-pct, fmt-pct-pp, mono, mono-nowrap

// Shared styled data table: fills the container width, faint zebra striping on
// alternating body rows, a rule under the header and at the bottom, and a bold
// centered caption. `columns` should use fr units so the table fills the
// column/page width.
#let data-table(columns: (), aligns: auto, header: (), body: (), caption: none) = {
  table(
    columns: columns,
    align: aligns,
    stroke: none,
    inset: (x: 3pt, y: 2.4pt),
    // y == 0 is the header row; stripe alternating body rows beneath it.
    fill: (_, y) => if y > 0 and calc.even(y) { palette.zebra },
    table.header(..header),
    table.hline(stroke: 0.6pt),
    ..body,
    table.hline(stroke: 0.6pt),
  )
  if caption != none {
    align(center)[#text(size: 8.5pt, weight: "bold")[#caption]]
  }
}

// kv-table renders a headerless two-column key/value list with the same faint
// zebra striping as the data tables (used by Hardware Configuration and Survey
// Parameters). `pairs` is an array of (label, value) string tuples.
#let kv-table(pairs) = {
  let body = pairs.map(((k, v)) => (
    [#text(weight: "bold")[#k]],
    [#mono(v)],
  )).flatten()
  table(
    columns: (auto, 1fr),
    align: (left, left),
    stroke: none,
    inset: (x: 3pt, y: 2.4pt),
    // No header row, so stripe odd rows (first row stays white).
    fill: (_, y) => if calc.odd(y) { palette.zebra },
    ..body,
  )
}

// Spanning title centered above the two-column body. The surveyor/contact line
// (and its bullet separator) render only when the corresponding fields are
// present, so an empty surveyor or contact never leaves a dangling label or "•".
#let title-block(data) = {
  let surveyor = data.site.at("surveyor", default: "")
  let contact = data.site.at("contact", default: "")
  align(center)[
    #text(size: 22pt, weight: "bold")[#data.site.location]
    #if surveyor != "" or contact != "" {
      linebreak()
      v(-0.05cm)
      text(size: 11pt)[
        #if surveyor != "" [Surveyor: #emph(surveyor)]
        #if surveyor != "" and contact != "" [#h(0.5em) • #h(0.5em)]
        #if contact != "" [Contact: #link("mailto:" + contact)[#contact]]
      ]
    }
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
      [#text(weight: "bold")[Period:] #mono(data.period.start_date) to #mono(data.period.end_date)],
      [#text(weight: "bold")[Total vehicle count:] #fmt-int(data.overall.total_count)],
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

  // mono-nowrap binds multi-word cells ("p50 Velocity") into one line.
  let cell(s) = mono-nowrap(s)
  let t1 = data.overall
  let t2 = if data.compare != none { data.compare.overall } else { none }

  if data.compare != none {
    let header = (
      [#text(weight: "bold")[Metric]],
      align(right)[#text(weight: "bold")[Period t1]],
      align(right)[#text(weight: "bold")[Period t2]],
      align(right)[#text(weight: "bold")[Change]],
    )
    let mk(label, a, b) = (
      [#cell(label)], [#cell(fmt-speed(a, units))], [#cell(fmt-speed(b, units))], [#cell(fmt-pct(a, b))],
    )
    let body = (
      mk("p50 Velocity", t1.p50, t2.p50),
      mk("p85 Velocity", t1.p85, t2.p85),
      mk("p98 Velocity", t1.p98, t2.p98),
      mk("Max Velocity", t1.max_speed, t2.max_speed),
      // Vehicle Count: no Δ to avoid misleading comparison across unequal periods.
      ([#cell("Vehicle Count")], [#cell(fmt-int(t1.total_count))], [#cell(fmt-int(t2.total_count))], []),
    ).flatten()
    data-table(
      columns: (1.4fr, 1fr, 1fr, 1fr),
      aligns: (left, right, right, right),
      header: header,
      body: body,
      caption: "Table 1: Key Metrics",
    )
  } else {
    let header = ([#text(weight: "bold")[Metric]], align(right)[#text(weight: "bold")[Value]])
    let mk(label, v) = ([#cell(label)], [#cell(fmt-speed(v, units))])
    let body = (
      mk("p50 Velocity", t1.p50),
      mk("p85 Velocity", t1.p85),
      mk("p98 Velocity", t1.p98),
      mk("Max Velocity", t1.max_speed),
    ).flatten()
    data-table(
      columns: (1fr, auto),
      aligns: (left, right),
      header: header,
      body: body,
      caption: "Table 1: Key Metrics",
    )
  }
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
  )
  // Firmware is optional; omit the row entirely when unknown (matches the
  // hardware table's behaviour when no firmware version was recorded).
  if data.radar.at("firmware_version", default: "") != "" {
    rows.push(("Firmware version:", data.radar.firmware_version))
  }
  rows += (
    ("Transmit Frequency:",    data.radar.transmit_frequency),
    ("Sample Rate:",           data.radar.sample_rate),
    ("Velocity Resolution:",   data.radar.velocity_resolution),
    ("Azimuth Field of View:", data.radar.azimuth_fov),
    ("Elevation Field of View:", data.radar.elevation_fov),
  )
  kv-table(rows)
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
  // Cosine figures arrive as preformatted strings; an empty string means no
  // angle correction was applied, so the rows are omitted.
  let cosine-rows = ()
  if cmp != none {
    if r.at("cosine_error_angle", default: "") != "" {
      cosine-rows += (
        ("Cosine Error Angle (t1):",  r.cosine_error_angle + "°"),
        ("Cosine Error Factor (t1):", r.cosine_error_factor),
      )
    }
    if r.at("compare_cosine_error_angle", default: "") != "" {
      cosine-rows += (
        ("Cosine Error Angle (t2):",  r.compare_cosine_error_angle + "°"),
        ("Cosine Error Factor (t2):", r.compare_cosine_error_factor),
      )
    }
  } else if r.at("cosine_error_angle", default: "") != "" {
    cosine-rows += (
      ("Cosine Error Angle:",  r.cosine_error_angle + "°"),
      ("Cosine Error Factor:", r.cosine_error_factor),
    )
  }
  kv-table(rows + cmp-rows + cosine-rows)
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

#let render-table-part(parts, start: 0, end: none, show-caption: true) = {
  if parts == none { return }
  let rows = parts.rows
  if end == none { end = rows.len() }
  if start >= end { return }
  data-table(
    columns: parts.columns,
    aligns: parts.aligns,
    header: parts.header,
    body: rows.slice(start, end).flatten(),
    caption: if show-caption { parts.caption } else { none },
  )
}

#let table-fragment-weight(rows, show-caption: true) = {
  if rows <= 0 {
    0
  } else {
    // Header/rules/caption cost enough vertical space that row count alone
    // leaves the second column underfilled. These weights are deliberately
    // simple and deterministic so source zips recompile identically.
    rows + 2 + if show-caption { 2 } else { 0 }
  }
}

#let table-part-weight(parts) = if parts == none {
  0
} else {
  table-fragment-weight(parts.rows.len())
}

#let choose-granular-split(hist, daily, granular) = {
  if granular == none {
    0
  } else {
    let n = granular.rows.len()
    if n <= 1 {
      n
    } else {
      let fixed-left = table-part-weight(hist) + table-part-weight(daily)
      let best = 1
      let best-diff = 100000
      // Bias continuation rows into the right column. The left column already
      // carries the velocity distribution and daily summary, so a strictly even
      // split can leave the second column ending too high above Figure 2.
      let right-bias = 3
      for i in range(1, n) {
        let left = fixed-left + table-fragment-weight(i, show-caption: false)
        let right = table-fragment-weight(n - i)
        let diff = calc.abs((left + right-bias) - right)
        if diff < best-diff {
          best = i
          best-diff = diff
        }
      }
      best
    }
  }
}

// ─── Table 2: Velocity Distribution ───────────────────────────────────────
// Reference shape: Bucket | t1 Count | t1 Percent | t2 Count | t2 Percent | Delta
#let velocity-distribution-parts(data) = {
  let units = data.period.units
  let h1 = data.histogram.buckets
  if h1.len() == 0 {
    none
  } else {
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
    let rows = h1.map(b => {
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
    })

    let cols = if data.compare != none {
      (1.2fr, 1fr, 1fr, 1fr, 1fr, 1fr)
    } else {
      (1fr, 1fr, 1fr)
    }
    let aligns = if data.compare != none {
      (left, right, right, right, right, right)
    } else {
      (left, right, right)
    }
    (
      columns: cols,
      aligns: aligns,
      header: header,
      rows: rows,
      caption: "Table 2: Velocity Distribution (" + units + ")",
    )
  }
}

#let velocity-distribution-table(data) = {
  render-table-part(velocity-distribution-parts(data))
}

// ─── Table 3: Daily Percentile Summary (merged t1+t2) ─────────────────────
#let rollup-table-parts(rows, units, first-column, caption) = {
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
  let table-rows = rows.map(row => (
    [#cell(row.at(first-column))],
    [#cell(fmt-int(row.count))],
    [#cell(fmt-speed-bare(row.p50))],
    [#cell(fmt-speed-bare(row.p85))],
    [#cell(fmt-speed-bare(row.p98))],
    [#cell(fmt-speed-bare(row.max_speed))],
  ))
  (
    columns: (auto, auto, 1fr, 1fr, 1fr, 1fr),
    aligns: (left, right, right, right, right, right),
    header: header,
    rows: table-rows,
    caption: caption,
  )
}

#let daily-summary-parts(data) = {
  let units = data.period.units
  let merged = data.daily
  if data.compare != none {
    merged = data.daily + data.compare.daily
  }
  if merged.len() == 0 {
    none
  } else {
    let title = if data.compare != none {
      "Table 3: Daily Percentile Summary (Comparison)"
    } else {
      "Table 3: Daily Percentile Summary"
    }
    rollup-table-parts(merged, units, "date", title)
  }
}

#let daily-summary(data) = {
  render-table-part(daily-summary-parts(data))
}

// ─── Table 4: Granular Percentile Breakdown (merged t1+t2) ────────────────
#let granular-table-parts(data) = {
  let units = data.period.units
  let merged = data.granular
  if data.compare != none {
    merged = data.granular + data.compare.granular
  }
  if merged.len() == 0 {
    none
  } else {
    // Single-period reports have no daily table, so the granular breakdown is
    // Table 3; in comparison mode it follows the daily table as Table 4.
    let title = if data.compare != none {
      "Table 4: Granular Percentile Breakdown (Comparison)"
    } else {
      "Table 3: Granular Percentile Breakdown"
    }
    rollup-table-parts(merged, units, "bucket", title)
  }
}

#let granular-table(data) = {
  render-table-part(granular-table-parts(data))
}

#let detailed-data-flow(data) = {
  let hist = velocity-distribution-parts(data)
  let daily = daily-summary-parts(data)
  let granular = granular-table-parts(data)
  let split = choose-granular-split(hist, daily, granular)

  detailed-data-tables-heading()
  grid(
    columns: (1fr, 1fr),
    gutter: 16pt,
    [
      #render-table-part(hist)
      #render-table-part(daily)
      #render-table-part(granular, end: split, show-caption: false)
    ],
    [
      #if granular != none {
        render-table-part(granular, start: split)
      }
    ],
  )
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

// wide-figure is emitted after the detailed table block on a one-column page.
// Keeping it in normal flow lets the map sit directly below the chart when it
// fits, or at the top of the next page when it does not.
#let wide-figure(path, caption) = {
  v(0.45em)
  figure(image(path, width: 100%), caption: caption, supplement: [Figure])
}

// ─── Time-series chart figures (one per period) ──────────────────────────
#let timeseries-figures(data) = {
  let ts1 = data.charts.at("timeseries", default: "")
  let ts2 = data.charts.at("timeseries_compare", default: "")
  let pri-cap = if data.compare != none {
    [Velocity over time (t1: #data.period.start_date to #data.period.end_date)]
  } else {
    [Velocity over time (#data.period.start_date to #data.period.end_date)]
  }
  if ts1 != "" {
    wide-figure(ts1, pri-cap)
  }
  if ts2 != "" and data.compare != none {
    wide-figure(ts2, [Velocity over time (t2: #data.compare.start_date to #data.compare.end_date)])
  }
}

// ─── Site map figure ─────────────────────────────────────────────────────
// A full-width bottom float, emitted after the time-series charts. Because
// bottom floats are placed in declaration order, the map always follows the
// charts. Returns nothing when no map was supplied.
#let map-figure(data) = {
  let mp = data.charts.at("map", default: "")
  if mp == "" { return }
  wide-figure(
    mp,
    [Site Location Map with radar location (circle) and coverage area (red triangle)],
  )
}
