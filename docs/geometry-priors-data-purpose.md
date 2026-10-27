# Geometry priors: data purpose and interpretation

The geometry-priors dataset records observed physical features of the built environment to support spatial localization, registration, mapping, and geometric analysis.

## What the records describe

Records may identify buildings, bridges, trees, poles, and other stationary features, along with measured or inferred positions, extents, surfaces, planes, openings, recesses, and projections. Geometry is represented as georeferenced coordinates and geometric primitives derived from sensor observations or appropriately licensed source data.

The dataset is a **measurement-oriented representation of observable physical conditions**, not a set of construction documents, architectural plans, engineering specifications, or instructions for constructing a structure. It does not aim to capture hidden structural elements, materials, finishes, interiors, or methods of construction.

## Temporal and measurement context

Geometry describes conditions observed or estimated at the stated acquisition time. Features may have changed since collection. Position, shape, and completeness depend on sensor characteristics, coverage, occlusion, registration, processing, and source quality. An observed surface does not establish the presence or absence of unobserved features.

Where available, records should include:
- Acquisition date or time interval and source provenance.
- Coordinate reference system and units.
- Whether geometry is directly observed or inferred.
- Positional uncertainty, coverage, and quality indicators.
- Processing or dataset version.

## Appropriate interpretation

The data is intended for localization, spatial indexing, alignment, change detection, and related research and engineering analysis. It should not be relied on as a boundary survey, structural assessment, construction specification, or authoritative statement of current site conditions.

## Publication checklist

- [ ] Source and redistribution terms recorded for each input dataset.
- [ ] Coordinate system, units, and acquisition dates documented.
- [ ] Observed and inferred geometry distinguishable.
- [ ] Accuracy, uncertainty, and known coverage gaps documented.
- [ ] Public descriptions consistently characterize the product as measured spatial data.
