# Citation rubric

- **Scope:** What enters [references.bib](references.bib), how the bibliography is kept to a
  budget, and how the [reference graph](reference-graph.md) ranks papers to read next.
- **Status:** Adopted October 20, 2026.
- **Related:** [Reference audit](../../docs/lidar/operations/brief/reference-audit.md),
  [budget](bibliography-budget.json), [candidate exclusions](reference-graph-curation.json),
  [research-briefing skill](../../.claude/skills/research-briefing/SKILL.md),
  [TENETS.md](../../TENETS.md)

The bibliography records every work the repository names, so the reference audit can show where
each name came from. That stays the rule. Without a limit, though, every briefing adds entries
and nothing leaves: the bibliography grew from 94 to 166 entries in ten days. The
[citation graph](reference-graph.json) makes the pressure worse, because each paper we hold
points to dozens we do not. This rubric keeps the rule and adds a budget and a way out.

## What enters the bibliography

A work gets an entry when a checked-in document or source file names it, whatever the verdict
on it. Adopted, cited for context and read but not adopted all count. The
[reference audit](../../docs/lidar/operations/brief/reference-audit.md) maps each entry to the
mention that brought it in. Because naming a work commits it to the bibliography, the budget
starts in the document: name a paper when the document needs it. Before adding an entry, check
these three things:

1. **One entry per work.** A preprint and its published version share an entry. A press article
   about a paper is not a second source.
2. **Primary over secondary.** Cite the paper, not the review that summarises it, unless the
   review is what the document relies on.
3. **Fits the budget.** See below.

## The budget

[bibliography-budget.json](bibliography-budget.json) gives each bibliography section a soft cap.
The first caps (October 20, 2026) are the section's size then plus two, rounded up to the next
multiple of five: 235 places against 166 entries. A change that takes a section over its cap
does one of these, and says which in its reference-audit row:

- **Prune:** remove an entry that nothing now cites (see the next section).
- **Merge:** fold a duplicate or a superseded version into one entry.
- **Raise the cap:** with a sentence in the reference audit saying why this section needs more
  room, such as a new area of work. Raising a cap is a choice, not a default.

`make check-bibliography` prints each section's count against its cap and the entries nothing
cites. It is advisory; `--strict` fails on a section over its cap, a section with no cap, or a
cap that names no section.

## Pruning

An entry is a candidate for removal when no tracked file names it by key or DOI and the
reference audit maps no author-and-year mention to it. `make check-bibliography` lists the first
condition; check the second by hand, because documents often cite by author and year. Review the
list at the weekly retro and remove entries that fail both. The entry stays in Git history and
the citation graph keeps the paper as an uncited work, so removing it loses nothing that cannot
be restored.

On October 20, 2026, seven entries were not named by key or DOI: Bogoslavskyi2016,
AutowareNebula, Yan2018SECOND, Besl1992ICP, Zhang1994ICP, Biber2003NDT and Wilson2026. Most are
probably cited by author and year; none has been removed.

## Papers to read next

The [reference graph page](reference-graph.md) lists works our papers cite that we do not hold.
They are candidates, ranked for reading, never entries. A candidate becomes an entry only by
being read, evaluated and named by a document, under the rules above.

**Score.** A candidate's score is the number of our papers that cite it, plus the number of
bibliography sections those papers come from. A work cited by four papers in three sections
scores 7. The section count rewards work that connects parts of the project over work that one
cluster cites heavily. The page shows both parts of each score and offers a minimum.

**Exclusions.** [reference-graph-curation.json](reference-graph-curation.json) lists candidates
the page leaves out, each with one reason:

| Reason        | Leaves out                                                                                   |
| ------------- | -------------------------------------------------------------------------------------------- |
| `camera-only` | Image or video methods and datasets with no LiDAR or radar part, which Tenet 1 excludes      |
| `generic-ml`  | Optimisers, backbones, frameworks and sequence models cited for tooling, not for measurement |
| `indoor`      | Indoor RGB-D scenes, which share neither the sensor nor the scene                            |

An excluded work stays in the graph and can be shown with a toggle; exclusion only removes it
from the reading list. Add an exclusion when a reason fits plainly. A method that fuses camera
and LiDAR is not `camera-only`. Remove an exclusion, rather than adding a reason, when the
project starts to need the work.

## What this does not settle

- The caps are a starting point, set from the current sizes, not from any measure of what a
  section needs.
- The score counts citations. It says nothing about a paper's quality, and a paper cited by many
  of ours may be cited only in passing.
- The citation graph has gaps: 14 of our 164 papers had no reference list from any source when
  it was built, and some works appear twice under different identifiers.
