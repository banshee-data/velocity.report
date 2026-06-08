// report.typ — main entry, mirrors the LaTeX reference layout.

#import "/preamble.typ": apply-styles, header-block, footer-block
#import "/sections.typ": *

#let data = json("/data.json")

// Page geometry matches the LaTeX reference (geometry: top=1.8cm, bottom=1.0cm,
// left=1.0cm, right=1.0cm). The whole body is a two-column page (LaTeX multicol
// equivalent): narrative and every table flow continuously through the two
// columns, wrapping once. The wide figures float across both columns.
#set page(
  paper: "us-letter",
  margin: (top: 1.8cm, bottom: 1.0cm, left: 1.0cm, right: 1.0cm),
  columns: 2,
  header: header-block(data),
  footer: footer-block(data),
)

// Install document-wide text/heading/caption styling (see preamble).
#show: apply-styles

// Title spans both columns (floats to the parent/page scope).
#place(top + center, scope: "parent", float: true, clearance: 12pt, title-block(data))

#velocity-overview(data)
#key-metrics(data)
#histogram-figure(data)
#site-information(data)
#citizen-radar()
#aggregation-and-percentiles()
#hardware-configuration(data)
#survey-parameters(data)

// All detailed tables flow through the same two columns, continuously, so the
// data wraps once (column 1 → column 2 → next page) rather than each table
// splitting on its own.
#detailed-data-tables-heading()
#velocity-distribution-table(data)
#daily-summary(data)
#granular-table(data)

// Wide figures float full width across both columns (LaTeX figure* equivalent),
// dropping into the free space after the tables or onto the next page.
#timeseries-figures(data)
#map-figure(data)
