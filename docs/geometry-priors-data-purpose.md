# Geometry priors: data purpose and interpretation

The geometry-priors dataset records observed physical features of the built environment to support
spatial localisation, registration, mapping, and geometric analysis.

## What the records describe

Records may identify buildings, bridges, trees, poles, and other stationary features, along with
measured or inferred positions, extents, surfaces, planes, openings, recesses, and projections.
Geometry is represented as georeferenced coordinates and geometric primitives derived from sensor
observations or appropriately licensed source data.

The dataset is a **measurement-oriented representation of observable physical conditions**, not a
set of construction documents, architectural plans, engineering specifications, or instructions for
constructing a structure. It does not aim to capture hidden structural elements, materials,
finishes, interiors, or methods of construction. Records state measurements; they carry no
direction for recreating a structure.

## Temporal and measurement context

Geometry describes conditions observed or estimated at the stated acquisition time. Features may
have changed since collection. Position, shape, and completeness depend on sensor characteristics,
coverage, occlusion, registration, processing, and source quality. An observed surface does not
establish the presence or absence of unobserved features.

Where available, records should include:

- Acquisition date or time interval and source provenance.
- Coordinate reference system and units.
- Whether geometry is directly observed or inferred.
- Positional uncertainty, coverage, and quality indicators.
- Processing or dataset version.

## Appropriate interpretation

The data is intended for localisation, spatial indexing, alignment, change detection, and related
research and engineering analysis. It should not be relied on as a boundary survey, structural
assessment, construction specification, or authoritative statement of current site conditions.

## Wording

Describe the data by what was measured. The same words apply in documentation, code comments, user
interface copy, dataset cards, and the public site.

| Say                                                              | Avoid                                             |
| ---------------------------------------------------------------- | ------------------------------------------------- |
| measured, observed, or inferred geometry                         | model, reconstruction, or replica of a structure  |
| footprint, extent, height, surface, plane                        | plan, drawing, blueprint, as-built, specification |
| feature (building, bridge, tree, pole), with its evidence status | digital twin                                      |
| position, extent, and uncertainty at an acquisition time         | dimensions presented as what to build from        |

Evidence status uses the words of the physical reference records
(`internal/lidar/annotation/physical.go`): observed, inferred, prior-only, or unknown.

Attributes stay with geometry. A published record has no material, finish, interior, or
construction-method attribute, and a schema that lists properties lists only geometry and its
measurement context.

## Where this is applied

- File format, evidence status, and the property allow-list:
  [geometry-prior-service.md](lidar/architecture/geometry-prior-service.md) and
  [vector-scene-map.md](lidar/architecture/vector-scene-map.md).
- Published scene assets:
  [web scene export,
  section 12](plans/lidar-web-scene-export-plan.md#12-context-travels-with-the-export) and the
  Hugging Face [dataset card](../tools/s2-archive/huggingface-dataset-README.md).
- Agent and contributor wording: [AGENTS.md](../AGENTS.md#purpose-of-geometry-and-scene-data), with
  the decision recorded in [DECISIONS.md](DECISIONS.md).

## Publication checklist

- [ ] Source and redistribution terms recorded for each input dataset.
- [ ] Coordinate system, units, and acquisition dates documented.
- [ ] Observed and inferred geometry distinguishable.
- [ ] Accuracy, uncertainty, and known coverage gaps documented.
- [ ] Attributes limited to geometry and its measurement context.
- [ ] Public descriptions consistently characterise the product as measured spatial data.
