---
layout: page.njk
title: "Reference graph: velocity.report"
description: "The papers velocity.report's research cites, the works they cite in turn, and which to read next, on a time axis."
eleventyExcludeFromCollections: true
noindex: true
---

# Reference graph

- **Scope:** The papers in [references.bib](https://github.com/banshee-data/velocity.report/blob/main/data/maths/references.bib), the works they cite,
  and the candidates worth reading next, on a time axis.
- **Data:** [graph.json](graph.json), built by
  citation-needed from OpenAlex, Crossref,
  OpenCitations, Semantic Scholar and GROBID; section lanes from
  [references.bib](https://github.com/banshee-data/velocity.report/blob/main/data/maths/references.bib); exclusions from [curation.json](curation.json).
- **Rules:** [Citation rubric](https://github.com/banshee-data/velocity.report/blob/main/data/maths/citation-rubric.md)

Each dot is a paper, placed by year (older on the left) in the lane of the bibliography section
that cites it. Filled dots are our papers; open dots are works our papers cite that we do not
hold, sized by their candidate score. Select a paper to draw its links: solid lines go to what
it cites, dashed lines come from what cites it. The panel links out to the DOI, OpenAlex,
Semantic Scholar and Google Scholar. Years before 1960 share the left edge of the axis.

<div data-reference-graph data-graph="graph.json" data-bib="references.bib" data-curation="curation.json"></div>
<script type="module" src="reference-graph.mjs"></script>
<noscript>This page needs JavaScript. The data is in <a href="graph.json">graph.json</a> beside it.</noscript>

## How to read it

- **Historical precedence.** A link always runs from the citing paper back to an earlier one, so
  solid lines from a selected paper point left. A candidate far to the left that many of our
  papers cite is foundational work we have not read.
- **Candidate score.** The number of our papers citing a work, plus the number of bibliography
  sections those papers come from. The "Read next" table lists the top 40 that the rubric does
  not exclude; the section filter narrows it to one part of the project.
- **Exclusions.** Camera-only, generic machine-learning and indoor works are left off the
  reading list, with the reasons in the [rubric](https://github.com/banshee-data/velocity.report/blob/main/data/maths/citation-rubric.md#papers-to-read-next). Turn
  on "Include excluded candidates" to see them.
- **Status.** "Downloaded" and "indexed" come from the paper collection on the LiDAR volume when
  the graph was built: a verified PDF, and Markdown checked against it.

## Limits

- The graph is a snapshot. It changes only when citation-needed exports it again.
- 14 of our papers had no reference list from any source when it was built. Their dots have no
  outgoing links, which is missing data, not a finding.
- Some works appear twice under different identifiers, and a few records carry impossible years;
  those are shown as "n.d." and the panel says what the graph recorded.
- A citation count measures attention, not quality or relevance.
