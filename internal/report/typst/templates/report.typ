// report.typ — main entry for the velocity report layout.

#import "/preamble.typ": apply-styles, header-block, footer-block
#import "/sections.typ": *

#let data = json("/data.json")

// The whole body is a two-column page: the narrative and every table flow
// continuously through the two columns, wrapping once (column 1 → column 2 →
// next page) so each table stays one column wide. The title, the detailed-data
// heading, and the figures span both columns as full-width floats.
#set page(
  paper: data.paper,
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
// data wraps once (column 1 → column 2) rather than each table stretching the
// full page width. The heading floats full width above the flow.
#detailed-data-tables-heading()
#velocity-distribution-table(data)
#daily-summary(data)
#granular-table(data)

// Figures span both columns as bottom floats, declared in reading order:
// time-series charts first, then the site map. Bottom floats stack in
// declaration order, so the charts always precede the map.
#timeseries-figures(data)
#map-figure(data)
