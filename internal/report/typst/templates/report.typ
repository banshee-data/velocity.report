// report.typ — main entry, mirrors the LaTeX reference layout.

#import "/preamble.typ": setup, header-block, footer-block
#import "/sections.typ": *

#let data = json("/data.json")

#setup(data)

// Bold figure captions (matches the LaTeX reference's `\textbf{Figure N: ...}`).
#show figure.caption: it => text(weight: "bold")[#it.supplement~#context it.counter.display(it.numbering): #it.body]

// Two-column body for pages 1 and 2 (reference style). `set page` clears
// unspecified fields, so re-pass header/footer to keep them on every page.
#set page(
  paper: "us-letter",
  columns: 2,
  header: header-block(data),
  footer: footer-block(data),
)

// Title spans both columns by floating to the parent scope.
#place(top + center, scope: "parent", float: true, clearance: 14pt, title-block(data))

#velocity-overview(data)
#key-metrics(data)
#histogram-figure(data)
#site-information(data)
#citizen-radar()
#aggregation-and-percentiles()
#hardware-configuration(data)

#colbreak(weak: true)

#survey-parameters(data)

#detailed-data-tables-heading()
#velocity-distribution-table(data)
#daily-summary(data)
#granular-table(data)

// Trailing pages: drop back to single column for the full-width charts.
#pagebreak()
#set page(
  paper: "us-letter",
  columns: 1,
  header: header-block(data),
  footer: footer-block(data),
)

#timeseries-figures(data)

#pagebreak()
#map-figure(data)
