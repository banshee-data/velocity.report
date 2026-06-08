// report.typ — main entry, mirrors the LaTeX reference layout.

#import "/preamble.typ": apply-styles, header-block, footer-block
#import "/sections.typ": *

#let data = json("/data.json")

// Page geometry matches the LaTeX reference (geometry: top=1.8cm, bottom=1.0cm,
// left=1.0cm, right=1.0cm). Columns are applied to the body via an explicit
// #columns block (not set page columns) so the wide figures after the block
// render full page width, in document order, with no floats.
#set page(
  paper: "us-letter",
  margin: (top: 1.8cm, bottom: 1.0cm, left: 1.0cm, right: 1.0cm),
  header: header-block(data),
  footer: footer-block(data),
)

// Install document-wide text/heading/caption styling (see preamble).
#show: apply-styles

// Spanning title above the two-column body.
#title-block(data)

#columns(2, gutter: 14pt)[
  #velocity-overview(data)
  #key-metrics(data)
  #histogram-figure(data)
  #site-information(data)
  #citizen-radar()
  #aggregation-and-percentiles()
  #hardware-configuration(data)
  #survey-parameters(data)

  #detailed-data-tables-heading()
  #velocity-distribution-table(data)
]

// Long percentile tables (Daily / Granular) render full page width as balanced
// two-column split grids — both halves fill evenly and end mid-page, leaving
// room for the time-series chart to flow up onto the same page.
#daily-summary(data)
#granular-table(data)

// Wide figures (time-series, map) flow full page width, in order — the chart
// lands right after the tables and the map follows.
#timeseries-figures(data)
#map-figure(data)
