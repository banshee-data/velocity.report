# Platform hub

Stable documentation for shared codebase structure and cross-cutting development
concerns. Anything that is not specific to a single sensor or to the user interface
lives here.

| Subdirectory                   | Contents                                                                          |
| ------------------------------ | --------------------------------------------------------------------------------- |
| [architecture/](architecture/) | Go package design, DB schema shape, logging model, ID conventions, metrics naming |
| [operations/](operations/)     | Deployment, release lifecycle, documentation standards, tooling, reporting        |

## Scope

This hub owns topics where the lasting value is a structural property of the shared
codebase **or** a development methodology that applies regardless of which sensor or
UI surface is being changed:

### Codebase structure

- Go package boundaries and import rules
- Database schema design (two-package SQL boundary)
- Traffic Description Language (TDL)
- Structured logging model
- Typed UUID prefix conventions
- Platform library design (TicTacTail)

### Deployment and release

- Single-binary distribution packaging
- Raspberry Pi imager
- Schema migrations
- Release lifecycle and backward-compatibility management
- TLS certificate strategy for local appliances

### Development methodology

- Documentation and plan hygiene standards
- Metrics naming and observability contracts
- Code coverage methodology
- Data science reproducibility principles
- Python developer tooling (scripts and shared venv — not on RPi)
- Code-formatting standards
- PDF reporting infrastructure
- AI agent architecture

## Platform vocabulary

These are the agreed nouns under [D-31](../DECISIONS.md#d-31--renames-are-cutovers-tombstones-for-people-aliases-for-the-app).
Definitions describe the concepts; they do not claim that future storage or relationships exist.
Phase 1 cuts over clip, label and site terminology. Capture digests, one label write path,
unified sites, deployments and surveys as stored relationships remain Phase 2.
Current sites still use separate radar and LiDAR tables; a configuration period describes a
sensor deployment, and a published recording describes a survey only where that meaning fits.

| Noun                     | Definition                                                                                                                                                                                                                                                     |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **sensor**               | One physical instrument, identified by `sensor_id`; `sensor_model` is its kind                                                                                                                                                                                 |
| **port**                 | The address a listener binds                                                                                                                                                                                                                                   |
| **listener**             | The open live connection on a port                                                                                                                                                                                                                             |
| **packet**               | The wire unit as the sensor sent it                                                                                                                                                                                                                            |
| **reading**              | One decoded measurement                                                                                                                                                                                                                                        |
| **detection**            | The radar sensor's own on-board classifier report of a passage                                                                                                                                                                                                 |
| **frame**                | One LiDAR rotation assembled from packets (L2)                                                                                                                                                                                                                 |
| **capture**              | A static file of packets, identified by its content digest                                                                                                                                                                                                     |
| **capture sequence**     | The derived run of abutting captures from one sensor                                                                                                                                                                                                           |
| **capture period**       | A motion or static stretch of a sequence, derived                                                                                                                                                                                                              |
| **capture volume**       | A configured directory that holds captures                                                                                                                                                                                                                     |
| **capture time**         | The data's own clock: when the sensor captured it, as opposed to wall time                                                                                                                                                                                     |
| **source**               | What feeds the pipeline: a listener (`live`), a `capture`, a `vrlog`, or a `synthetic` generator                                                                                                                                                               |
| **part**                 | One file of a survey's capture export: a capture holding the whole frames between two lines of a 300 s grid, cut between packets at frame boundaries, at most 4 GB; the set is the survey's **capture export**                                                 |
| **run**                  | One pass of the pipeline over one source at one configuration                                                                                                                                                                                                  |
| **vrlog**                | The container a run writes: stream kinds `observation`, `analysis`, `display`; a profile; capabilities                                                                                                                                                         |
| **evidence**             | The contents of a vrlog's observation stream: clusters and the retained points behind them                                                                                                                                                                     |
| **observation**          | One L4 cluster record inside evidence                                                                                                                                                                                                                          |
| **extraction**           | The act, configuration and identity of producing evidence from a source (`extractor_id`, `extraction_id`)                                                                                                                                                      |
| **recording**            | Verb only: writing a vrlog; the REC state                                                                                                                                                                                                                      |
| **replay**               | Feeding a capture or a vrlog through the pipeline                                                                                                                                                                                                              |
| **playback**             | The visualiser's transport over a vrlog: pause, play, seek, rate                                                                                                                                                                                               |
| **track**                | One road user's passage through the survey's observation frame: the temporal collection of readings (radar) or cluster observations (LiDAR) that a tracker built for one road user; a **track point** is one frame's measurement of a LiDAR track              |
| **tracker**              | The software that builds tracks from readings or observations; each sensor has one, at the same level                                                                                                                                                          |
| **track build**          | One tracker pass over a range of readings, run as a job                                                                                                                                                                                                        |
| **estimate**             | The tracker's revisable account of a track's state over an interval, at stage `online`, `fixed_lag` or `final`: what we currently think moved through the evidence                                                                                             |
| **revision**             | An estimate that replaces a stated interval of an earlier one, with lineage when tracks split or join                                                                                                                                                          |
| **checkpoint**           | The tracker's resumable state, committed with an analysis generation                                                                                                                                                                                           |
| **frontier**             | How far a stream has got: captured, durable, processed, finalised, reported                                                                                                                                                                                    |
| **sweep**                | A set of runs at varied parameters, with rounds and combos                                                                                                                                                                                                     |
| **job**                  | One unit of background work in either queue: a kind (verb-object), a subject, an executor, an output                                                                                                                                                           |
| **worker**               | A process that executes jobs                                                                                                                                                                                                                                   |
| **label**                | A person's verdict on one track: a class plus quality flags                                                                                                                                                                                                    |
| **class**                | The vocabulary of object kinds (`ObjectClass`: car, bus, pedestrian, …)                                                                                                                                                                                        |
| **quality flag**         | A per-track quality verdict (good, noisy, merge, split, …)                                                                                                                                                                                                     |
| **annotation**           | A reviewed point mask over a pack, with its object identity and review status                                                                                                                                                                                  |
| **pack**                 | An immutable excerpt of a run's vrlog for annotation, identified by its digest (`pack_id`)                                                                                                                                                                     |
| **gap mark**             | A region and time a person says the pipeline missed                                                                                                                                                                                                            |
| **clip**                 | A window within a survey chosen for evaluation or annotation, with its selection provenance, reference run, recommended parameters and pack                                                                                                                    |
| **selector**, **finder** | A configured way of ranking candidate windows; the algorithm it uses                                                                                                                                                                                           |
| **site**                 | A physical place with a name and position. S2 cells index the place; the current radar and LiDAR site stores remain separate until Phase 2.                                                                                                                    |
| **district**             | An S2 L10 cell, the filesystem roll-up; never a map extent                                                                                                                                                                                                     |
| **neighbourhood**        | An S2 L13 cell, the fine partition for joins and datasets                                                                                                                                                                                                      |
| **site cell**            | An S2 L16 cell, a junction and its approaches; where a site sits, not what it is                                                                                                                                                                               |
| **coordinates**          | A WGS84 latitude and longitude, with altitude when known                                                                                                                                                                                                       |
| **heading**              | Degrees clockwise from north of a sensor's zero: the LiDAR azimuth zero, the centre of the radar beam. Held by the deployment's pose for both sensors                                                                                                          |
| **yaw**                  | Rotation about Z inside a named frame, for tracks, boxes and OBBs                                                                                                                                                                                              |
| **pose**                 | Coordinates plus heading; later roll, pitch and altitude. A **local pose** is x, y, z plus yaw in a named frame                                                                                                                                                |
| **trajectory**           | Time-stamped local poses with a **placement** pose that maps the local frame to WGS84                                                                                                                                                                          |
| **frame** (coordinate)   | One of `sensor`, `site`, `wgs84`; the `frame` column, distinct from `frame_id`                                                                                                                                                                                 |
| **deployment**           | One sensor at one site, or on a moving platform, over a period, with its mounting and pose                                                                                                                                                                     |
| **survey**               | One deployment's measurement over a period, at a site (static) or along a route (moving); may be published                                                                                                                                                     |
| **report**               | A generated document from a survey                                                                                                                                                                                                                             |
| **vantage**              | A named viewpoint of a site, served by every survey at that site                                                                                                                                                                                               |
| **scene** (geometry)     | The physical environment a sensor observes: the L3 scene signature, the L7 Scene layer's persistent geometry, the vector scene map. Never a product or storage noun                                                                                            |
| **dataset**              | The report parameter naming which rows a report aggregates: `detections`, `readings`, `tracks`                                                                                                                                                                 |
| **tuning config**        | The whole configuration a run records: `version`, `l1`, `l3`, `l4`, `l5`, `pipeline`; stored by `lidar_run_configs` and `lidar_param_sets`; hashed as the perf gate's **fingerprint**                                                                          |
| **preset**               | A named tuning config file in `config/presets/` that sets the pipeline depth by the per-layer engine selectors (`detect`, `l3-only`, `full`); a preset is a tuning config, not a second kind of thing                                                          |
| **profile**              | A vrlog's level of detail: `display`, `foreground-reduced`, `foreground-complete`; chosen in the tuning config at `l4.evidence.profile`. The code today knows `reduced-cluster-sample` and `foreground-complete`; the legacy name is read by name until v0.7.1 |
| **capability**           | A fact the writer records in a vrlog manifest about what the container holds, derived from the profile and what the run produced, never configured; a reader names the capabilities it **requires** and is refused by name                                     |
| **sensors**              | The route that reports each sensor's status                                                                                                                                                                                                                    |
| **points**               | The survey export tier holding a short foreground point-cloud excerpt                                                                                                                                                                                          |

Compatibility treatments and expiry are owned by the [compatibility register](operations/compatibility-register.md).

## Existing place surfaces

A **deployment** is one sensor's placement over a period, with its mounting and pose.
The Sites screen calls the existing radar configuration periods radar deployments;
this does not add a deployment table or sensor relationship. A **survey** is that
sensor's measurement over a period. Reports select measurement periods, and the
existing published-recording catalogue is shown as Surveys.

The catalogue editor retains its current database/API field contract until Phase 2.
Its `Survey` frontend type does not claim that the future unified surveys table exists.
Clip locations use districts, sites and site cells; scene is reserved for physical geometry.
