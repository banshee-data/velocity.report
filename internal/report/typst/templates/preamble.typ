// preamble.typ — page setup, fonts, palette, formatting helpers.
// Targets visual parity with the LaTeX-rendered reference report.

#let palette = (
  p50:       rgb("#fbd92f"),
  p85:       rgb("#f7b32b"),
  p98:       rgb("#f25f5c"),
  max:       rgb("#2d1e2f"),
  count_bar: rgb("#a89c95"),
  rule:      rgb("#000000"),
  // Comparison-histogram bar colours — the LaTeX reference uses the p50
  // yellow for t1 and the p98 red for t2.
  cmp_t1:    rgb("#fbd92f"),
  cmp_t2:    rgb("#f25f5c"),
  link:      rgb("#0a4a8a"),
)

#let header-block(data) = {
  set text(size: 8pt)
  grid(
    columns: (1fr, 1fr),
    align: (left, right),
    [#text(weight: "bold")[#link("https://velocity.report")[velocity.report]]],
    [#emph(data.site.location)],
  )
  v(-0.45em)
  line(length: 100%, stroke: 0.8pt + palette.rule)
}

#let footer-block(data) = {
  let range-text = if data.compare != none {
    [#data.period.start_date to #data.period.end_date vs #data.compare.start_date to #data.compare.end_date]
  } else {
    [#data.period.start_date to #data.period.end_date]
  }
  line(length: 100%, stroke: 0.8pt + palette.rule)
  v(-0.4em)
  set text(size: 8pt)
  grid(
    columns: (1fr, 1fr),
    align: (left, right),
    [#range-text],
    [Page #context counter(page).display()],
  )
}

// apply-styles installs the document-wide text, paragraph, heading, link, and
// caption rules. It MUST be invoked at the top level of report.typ via
// `#show: apply-styles` (a show-everything rule) — set/show rules placed inside
// an ordinary function do not propagate to the document body, which previously
// left the report in Typst's default serif at the default size and margins.
#let apply-styles(body) = {
  set text(font: "Atkinson Hyperlegible", size: 8.7pt)
  set par(justify: true, leading: 0.5em, first-line-indent: 0pt)
  show heading.where(level: 1): it => {
    set text(size: 13.5pt, weight: "bold")
    block(above: 0.55em, below: 0.3em, it.body)
  }
  show heading.where(level: 2): it => {
    set text(size: 11.5pt, weight: "bold")
    block(above: 0.5em, below: 0.22em, it.body)
  }
  show link: set text(fill: palette.link)
  // Bold figure/table captions (matches the LaTeX reference).
  show figure.caption: it => text(weight: "bold", size: 8.5pt)[#it.supplement~#context it.counter.display(it.numbering): #it.body]
  body
}

// Mono "code-style" face for tabular data, like the reference's \AtkinsonMono.
// `nowrap` keeps multi-token cells like "p50 Velocity" or "6/2 00:00" on one
// line by replacing ordinary spaces with non-breaking spaces (U+00A0).
#let mono(body) = text(font: ("Atkinson Hyperlegible Mono", "DejaVu Sans Mono"), size: 8pt)[#body]
// mono-nowrap: a mono cell that prevents internal line breaks. We replace
// ordinary spaces with non-breaking spaces so phrases like "30.54 mph" or
// "6/2 00:00" stay on a single line within the cell.
#let mono-nowrap(s) = {
  let safe = if type(s) == str { s.replace(" ", "\u{00A0}") } else { s }
  box(text(font: ("Atkinson Hyperlegible Mono", "DejaVu Sans Mono"), size: 8pt)[#safe])
}

// fmt-speed: "30.54 mph" (always two decimals). Returns a string so that
// `mono-nowrap` can replace the space with a non-breaking one.
#let fmt-speed(value, units) = if value == none {
  "--"
} else {
  let r = calc.round(value, digits: 2)
  let s = str(r)
  if not s.contains(".") { s = s + ".00" }
  else {
    let parts = s.split(".")
    if parts.at(1).len() == 1 { s = s + "0" }
  }
  s + " " + units
}

// fmt-speed-bare: "30.54" — use for table cells where the column header
// already carries the unit.
#let fmt-speed-bare(value) = if value == none {
  "--"
} else {
  let r = calc.round(value, digits: 2)
  // Pad to 2 decimals so values line up in the mono font.
  let s = str(r)
  if not s.contains(".") { s = s + ".00" }
  else {
    let parts = s.split(".")
    if parts.at(1).len() == 1 { s = s + "0" }
  }
  s
}

// fmt-int: thousands-separated integer.
#let fmt-int(value) = if value == none {
  "--"
} else {
  let s = str(int(value))
  let parts = ()
  let i = s.len()
  while i > 3 {
    parts.insert(0, s.slice(i - 3, i))
    i = i - 3
  }
  if i > 0 { parts.insert(0, s.slice(0, i)) }
  parts.join(",")
}

// fmt-pct: signed percentage to one decimal, "+12.2%" / "-3.0%".
#let fmt-pct(a, b) = if a == none or b == none or a == 0 {
  ""
} else {
  let pct = (b - a) / a * 100
  let r = calc.round(pct, digits: 1)
  if r >= 0 { "+" + str(r) + "%" } else { str(r) + "%" }
}

// fmt-pct-pp: signed percentage-points (used for histogram comparison Δ),
// always with one decimal place so "+3" displays as "+3.0%".
#let fmt-pct-pp(a, b) = {
  if a == none { a = 0 }
  if b == none { b = 0 }
  let d = calc.round(b - a, digits: 1)
  let s = str(calc.abs(d))
  if not s.contains(".") { s = s + ".0" }
  if d >= 0 { "+" + s + "%" } else { "-" + s + "%" }
}
