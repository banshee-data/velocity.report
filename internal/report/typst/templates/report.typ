// report.typ — main entry for the velocity report layout.

#import "/preamble.typ": apply-styles, header-block, footer-block
#import "/sections.typ": *

#let data = json("/data.json")

// Page 1 uses a two-column narrative. The detailed tables start after an
// explicit page break and switch to a full-width page with their own manual
// two-column table flow, followed by figures in normal document flow.
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

#pagebreak()
#set page(columns: 1)

#detailed-data-flow(data)

// Figures follow the table block in source order. If the chart and map fit
// together they sit together; otherwise the map naturally moves to the top of
// the next page instead of being pinned to the bottom.
#timeseries-figures(data)
#map-figure(data)
