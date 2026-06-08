// report.typ — main entry for the velocity report layout.

#import "/preamble.typ": apply-styles, header-block, footer-block
#import "/sections.typ": *

#let data = json("/data.json")

// The page is single-column. The narrative runs in an explicit two-column block
// (so the prose wraps once, left column then right). The detailed tables and the
// figures are full-width: each long table balances its own rows across two
// halves, and the charts/map flow full width in document order.
#set page(
  paper: data.paper,
  margin: (top: 1.8cm, bottom: 1.0cm, left: 1.0cm, right: 1.0cm),
  columns: 1,
  header: header-block(data),
  footer: footer-block(data),
)

// Install document-wide text/heading/caption styling (see preamble).
#show: apply-styles

// Title spans the full page width.
#title-block(data)

// Narrative + small key/value tables flow through two balanced-width columns,
// wrapping once (column 1 → column 2).
#columns(2, gutter: 14pt)[
  #velocity-overview(data)
  #key-metrics(data)
  #histogram-figure(data)
  #site-information(data)
  #citizen-radar()
  #aggregation-and-percentiles()
  #hardware-configuration(data)
  #survey-parameters(data)
]

// Keep detailed tables off page 1. The data flow chooses a row-count-aware
// split so the granular breakdown continues into the second column instead of
// leaving that side empty.
#pagebreak()
#detailed-data-flow(data)

// Time-series charts, then the site map — full width, in document order, so the
// charts always come before the map.
#timeseries-figures(data)
#map-figure(data)
