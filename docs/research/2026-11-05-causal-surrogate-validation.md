# Crash-surrogate validity: hard braking, causality and intersection comparisons (2026-11-05)

This note evaluates a newly surfaced causal-validation framework for roadside driving-behaviour research. It distinguishes accurate observation from predictive association and from evidence that changing the observable changes crash outcomes.

- **Status:** Research assessment only. No safety threshold, per-driver verdict, metric promotion or pipeline change.
- **References:** `Wu2026` (new), `Nasri2026` (existing context); [canonical bibliography](../../data/maths/references.bib).
- **Canonical:** [behaviour analytics plan](../plans/lidar-behaviour-analytics-plan.md), [state-estimation plan](../plans/lidar-state-estimation-plan.md), [crash-data context plan](../plans/platform-crash-data-integration-plan.md) and [backlog](../BACKLOG.md).

## Evidence

### Wu and Flannagan (2026)

Kun-Feng (Ken) Wu and Carol Flannagan, _Accident Analysis & Prevention_, **“Validating crash surrogates: a causal graphical framework and a SHRP2 case study of hard braking”**, [DOI: 10.1016/j.aap.2026.108773](https://doi.org/10.1016/j.aap.2026.108773), [PubMed 42858486](https://pubmed.ncbi.nlm.nih.gov/42858486/). Published online October 10, 2026; indexed as volume 239, article 108773 (January 2027 issue).

The authors propose a three-part procedure for validating a candidate _causal_ crash surrogate: (1) justify the causal-identification assumptions for the surrogate–crash relationship; (2) estimate how much of an intervention's effect on crashes the surrogate captures; and (3) assess the surrogate paradox and comparability/transportability. Their worked example uses SHRP2 naturalistic-driving data and tests hard braking as a candidate surrogate for **rear-end crashes**.

**Evidence boundary:** the indexed abstract describes the method and example, not a universal hard-braking threshold or a quantified finding that hard braking is causally valid. This assessment does not reproduce the full paper's model or infer a numerical effect. A study of rear-end crash surrogacy does not establish the same relation for pedestrian or cyclist conflicts, fatal crashes, or every intersection geometry.

A causally valid surrogate is specific to the intervention, target outcome and population. A reduction in the measured surrogate alone does not establish that an intervention prevented crashes. The _surrogate paradox_ is the possibility that the surrogate improves while the desired crash outcome fails to improve or becomes worse, including because effects bypass the surrogate.

## Three distinct claims for velocity.report

| Claim                             | What must be shown                                                                                                                                                                                                                                | What the current field pipeline can support                                                                                                                                |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Physical measurement validity** | Braking and acceleration represent vehicle motion rather than LiDAR viewpoint drift, centroid shifts, fragment reassociation or derivative noise. Timing, longitudinal direction, uncertainty, suppression and observation coverage are explicit. | A development/validation question; use reviewed physical references, the existing geometry and estimation gates, and supplemental GNSS/IMU passes.                         |
| **Predictive safety association** | A declared event definition and exposure denominator associate with independently sourced crashes or qualified conflicts in held-out sites and periods, while traffic mix, road class, signal state and opportunity are accounted for.            | Not established by an accurate braking measurement alone. Requires multi-site outcome and exposure evidence with appropriate spatial/temporal matching.                    |
| **Causal surrogate validity**     | With a clearly defined intervention and crash outcome, causal assumptions, estimable mediation, surrogate-paradox checks and cross-setting comparability hold.                                                                                    | Not established. Cannot be inferred from a cross-site ranking or single-site before/after braking change. Requires a separate future research design and outcome evidence. |

The first claim is a **prerequisite for** the other two, but passing it is not evidence that the others passed. In particular, a LiDAR-derived hard-braking frequency is not automatically a _rear-end crash probability_.

## Existing complementary evidence (not newly published)

[Nasri et al. (2026), _Journal of Safety Research_](https://doi.org/10.1016/j.jsr.2026.04.008), reviewed in [the October 22 DRAC/TTC/PET note](2026-10-22/drac-and-conflict-metrics.md), analyzed **54.8 million one-second trajectory samples at 139 signalized intersections in Tucson**, linked to five years of police-reported crashes. DRAC was the strongest of the tested surrogate predictors across their model classes. In negative-binomial models, 1,000 additional _critical DRAC events_ were associated with about **5–10% higher expected counts** of selected crash types. TTC contributed less; PET had little value at their one-second sampling resolution.

This is **observational crash-count association, not causal mediation validation**. DRAC denotes a _required_ deceleration to avoid a modeled collision under declared assumptions; it must not be relabeled as a measured hard-braking maneuver. The effect size is specific to the authors' event definitions, observation density, crash outcomes, model and study network; it is not a portable threshold for a 10 Hz nominal / lower effective-rate roadside LiDAR survey. Cross-study event rates cannot be compared without compatible exposure and sampling.

## Implications for approach stability and spatial braking maps

- Report interpretable measurements: approach speed distribution, braking-onset and peak-deceleration locations, qualified event counts per **eligible observed passage/opportunity**, missingness and confidence bounds. Keep the physical reference point, longitudinal axis and map registration version explicit.
- Continue the geometry-first work: foreground retention, stable body-center/facet measurement, identity continuity, held-out position and motion accuracy, then qualified derivatives. Smoothing or constant-velocity predictions cannot turn systematic measurement bias into valid braking.
- Keep **measurement language** distinct from **safety claims**. A braking hot spot can justify investigating sightlines, crossings, signal timing, congestion, turning behavior or conflicting movements; it is not a stand-alone finding of dangerous design.
- Preserve crash-surrogate and conflict distributions separately. Do not invent a universal threshold for acceleration, jerk, hard braking, TTC, PET or DRAC, and do not publish an aggregate “aggressive driver” or “danger score.”
- Before/after survey reports may describe changes in observed braking, location and exposure. They must not state that crashes were prevented or causal risk fell without external outcome evidence and a study design that supports such inference.
- Apply [the crash-data plan's existing scope boundary](../plans/platform-crash-data-integration-plan.md): neither a site crash rate nor a site-level surrogate-to-crash calibration is established by the fixed sensor.

## Deferred research protocol (not a release gate)

**Trigger:** revisit only when (a) G-GEO-1, G-UNC-1 and G-SMO-1 and derivative-specific motion gates support a stable physical braking record, and (b) multiple sites/periods have credible independent crash or conflict outcomes, comparable exposure and a prespecified intervention/comparator question.

If triggered, design a falsifiable study:

1. **Predeclare the estimand:** one intervention, target crash type/severity, observation population, follow-up period and a specific surrogate definition (threshold, event window, spatial bin, eligible exposure). Do not choose the definition by maximizing retrospective correlation.
2. **Draw a causal graph:** include exposure and demand, entry speed, traffic mix, congestion, signal phase, geometry, weather, enforcement, site selection and time trends; identify unmeasured confounding and post-treatment adjustment hazards. Separate intervention-induced changes in opportunity from changed behaviour.
3. **Quantify measurement fidelity:** held-out trajectory errors, false braking on true steady passes, effective sample intervals, quality masks, sensitivity to derivative bandwidth and multi-pass/session dependence.
4. **Test prediction on genuinely held-out sites/time:** calibration, predictive error, crash-type specificity, between-city transportability, and uncertainty/suppression; distinguish association from causal identification.
5. **Only then test mediation:** specify and justify the causal assumptions, estimate uncertainty in the mediated effect, search for direct/unmediated effects, and check scenarios where the event rate improves while crash severity, conflicts or other outcomes worsen.
6. **Record failed claims:** report insufficient power, missing crash data, incompatible exposure or untestable assumptions as **not validated**, rather than converting exploratory associations into a public risk verdict.

No new field collection, crash database import or production metric is authorized by this note. Existing 05x physical measurement and 07x behavior schedules remain unchanged.

## Open questions

- What numerical surrogacy result, if any, does the full Wu–Flannagan paper obtain for hard braking and rear-end crashes? What exact hard-braking definition and treatment variable does it use?
- Which assumptions can be tested or bounded using anonymized fixed-sensor surveys, and which require external crash/outcome and intervention records?
- How does predictive performance change with LiDAR-specific censoring, short passages and sampling rates compared with SHRP2 in-vehicle observations?
- Are different surrogates needed for crossing, turning, cyclist-passing and pedestrian conflicts rather than transferring the rear-end example?

**Decision:** retain causal surrogate validation as a deferred research question, not a reason to revise the current tracker backlog or report safety thresholds.
