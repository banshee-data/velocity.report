---
title: Addresses you can calculate
description: "Why velocity.report names places with S2 cells: L10 for the coarse partition, L13 for the fine one, and no map server anywhere in the loop."
date: 2026-09-01T12:00:00Z
topics: [geospatial, s2, data]
og_image: /img/s2-10-sf.jpg
---

Every measurement this project takes happens somewhere, and sooner or later that somewhere needs a name. Not a pretty name: a name you can compute, compare, and sort, on a Raspberry Pi that may never see the internet again.

We settled on [S2 cells](https://s2geometry.io/). This note explains why, and what the two levels we care about are for. The exact formatting rules live in the [S2 addressing profile](/reference/s2-addressing/); this page is the reasoning behind them. <!-- link-ignore -->

## The problem with the names we had

A sensor records a week of traffic on a street. That week is a dataset. So are the replay captures taken alongside it, the site metadata, and any third-party figures you later want to compare against. All of it belongs to a place. None of the obvious ways of saying which place survives contact with a second deployment.

**Street addresses** need a geocoder, which means a server, which means a network dependency for something as basic as filing a capture. They are also not a grid: you cannot group by them, and "Sutro Street" exists in more towns than you would like.

**Latitude and longitude** are precise and useless for grouping. Two readings thirty metres apart share no useful prefix, no common bucket, and no cheap test for "these belong together". Precision is not the same as structure.

**Site names** work beautifully until the second person picks one. Then you have `clarendon`, `Clarendon Ave`, and `clarendon-avenue-take-2`, describing the same kerb.

What we actually wanted was narrower than a mapping stack: a way to name a patch of ground, calculate that name offline, group things by it, and have the name mean the same thing to someone else's tools.

## What S2 is, in one paragraph

S2 projects the globe onto a cube, divides each of the six faces into quadrants, divides those quadrants into quadrants, and keeps going for thirty levels until the cells are a couple of centimetres across. Every cell has exactly four children and exactly one parent. Each cell gets a 64-bit integer identifier, and those identifiers are assigned along a Hilbert curve that visits every child of a cell before it leaves for the next one. Google's [developer guide](https://s2geometry.io/devguide/s2cell_hierarchy) covers the construction properly, and the [cell statistics table](https://s2geometry.io/resources/s2cell_statistics) gives the sizes at every level. We are not going to redo either here.

## Why S2 and not one of the others

[Geohash](https://en.wikipedia.org/wiki/Geohash) has the friendliest property of the three: a shorter string really is the ancestor of a longer one. Its cells are rectangles in latitude and longitude, though, so they stretch and shrink alarmingly as you move away from the equator, and two adjacent places can sit either side of a prefix boundary and look completely unrelated.

[H3](https://h3geo.org/) is hexagonal, which makes adjacency and movement analysis genuinely nicer. Hexagons do not tile hierarchically, however, so its parent and child relationships are approximate. Ours need to be exact, because we use containment to decide what belongs in which bucket.

S2 gives exact containment, integer identifiers, and locality from the Hilbert ordering. It is also already inside tools we might want to talk to: BigQuery's geography functions and MongoDB's spherical indexes both use it underneath. That matters less than it sounds, but it does mean a cell identifier is a thing other people's systems already understand.

The deciding property was simpler than any of that. S2 is arithmetic. Given a latitude, a longitude, and a level, you get the same cell identifier on a Pi in a garage as you do in a data warehouse, with no map server, no boundary file, and no lookup table. For a project whose fourth tenet is that it has to work offline, that settled it.

## L10: the coarse partition

An S2 level 10 cell covers somewhere between fifty and a hundred square kilometres depending on where it lands, which is roughly seven to ten kilometres across. The four over San Francisco are about 8.2 km on a side.

That is the scale of a town, or of a distinct district in a larger city. We use it as the top bucket: the thing that answers "which body of data is this part of" before anything finer gets involved. A whole deployment programme in one city fits inside a handful of L10 cells, and those cells are stable for as long as the Earth is.

L10 identifiers are written with a five-character prefix and one further character, as in `80858-1`.

## L13: the fine layer, and where interoperability happens

Three levels down from L10 sits L13, and because each level quarters the one above it, every L10 cell contains exactly 64 of them. An L13 cell is around a square kilometre: about a kilometre across.

That turns out to be the natural unit for a deployment. A street, its junctions, and the blocks either side fit inside one. Two sites in the same neighbourhood usually land in different cells, which is what you want when you are comparing them, and a single site does not sprawl across four of them, which is what you want when you are filing its data.

It is also the level we quote when talking to anything outside the project. An L13 cell is fine enough to be meaningful and coarse enough not to be a location fix: naming one says "this part of town", not "this house". Given the first tenet of this project, that is a feature we chose rather than a limitation we tolerate.

L13 identifiers are written with the same five-character prefix and three further characters, as in `80858-064`.

## The picture

<figure class="figure-wide">
  <a href="/img/s2-10-sf.jpg" target="_blank" rel="noopener noreferrer">
    <img src="/img/s2-10-sf.jpg" alt="Map of San Francisco divided into four large S2 level 10 cells labelled 80858-1, 80858-7, 808f7-d, and 808f7-f. A brown Hilbert path steps through each cell's sixteen level 12 children; a blue path traces the sixty-four level 13 children of the north-eastern cell, with labels such as 80858-064." loading="lazy" decoding="async" width="4325" height="4975">
  </a>
  <figcaption>Four adjacent L10 cells over San Francisco. Brown traces the 16-step L12 path through each; blue traces the 64-step L13 path through the hero cell.</figcaption>
</figure>

Three things in that image are worth pointing out.

The cells are parallelograms, not squares. That is not a drawing error or a projection artefact added on the way to the page: S2 cells are faces of a projected cube, and this far from a face centre their edges genuinely do not line up with meridians and parallels.

The curves were not drawn by hand. Every vertex comes from a real CellID, sorted into the library's own Hilbert order, so the figure is a rendering of the data structure rather than an artist's impression of it. The generator lives in [tools/s2-hilbert](https://github.com/banshee-data/velocity.report/tree/main/tools/s2-hilbert) and regenerates every asset from `make render-s2-hilbert`.

The four cells are neighbours on the ground, but the curve does not run through them one after another. Only one pair of them is a true succession, `808f7-d` followed by `808f7-f`. For the other two, both the cell before and the cell after lie outside the frame entirely. Space-filling curves keep nearby things nearby most of the time, which is not the same as always.

## Hilbert order, briefly

The useful consequence of the ordering is that cells close together on the ground usually get identifiers close together as integers. A range scan over identifiers is therefore a decent approximation of "everything near here", which is why S2 is a good spatial index and not merely a good naming scheme.

The curve rotates and reflects as it descends, so how it runs through a cell depends on where that cell's parent sits. This is the part people write papers about; the [Hilbert curve](https://en.wikipedia.org/wiki/Hilbert_curve) article is a better place to start than anything we would write, and Google's [S2 reference implementation](https://github.com/google/s2geometry) is the authority on the exact traversal.

## Calculating it offline

None of this needs a network. A device knows its latitude and longitude, the level is a constant, and the rest is integer arithmetic that any of the S2 ports will do in microseconds. There is no tile server to be unreachable, no API key to expire, and no boundary file to go stale in the two years between a deployment and someone asking what the data means.

For a project that is meant to keep running on a residential street after everybody has stopped paying attention to it, "the answer is computable from first principles" is worth more than any amount of convenience elsewhere.

## What we use it for

**Grouping capture artefacts.** Replay material, `.pcap` and `.vrlog` alike, is filed under the cell it was recorded in. A corpus then becomes addressable by place rather than by whatever the person holding the laptop called the folder that afternoon.

**Aligning datasets.** Our own site metadata, collision records, traffic counts, and municipal boundary data rarely agree on how to spell a street name and never agree on how to identify a junction. They can all be reduced to a cell identifier, and cell identifiers join cleanly.

**Talking about places in public.** Issues, reports, and this website can name an L13 cell and be precise about the area without publishing anybody's address.

The convention is settled and the drawing tools are in the repository. Computing cell identifiers inside the service itself is still ahead of us: this note describes the addressing scheme the project has adopted, not a feature you will find in the current release.

## The exact rules live next door

Deciding to use S2 is one document. Writing down precisely how a token is formatted, what the display hyphen means, and how to check that your implementation agrees with ours is a different one, with different obligations: it has to stay stable.

See the [S2 addressing profile](/reference/s2-addressing/) for exact token formatting, terminology, and interoperability test vectors. <!-- link-ignore -->
