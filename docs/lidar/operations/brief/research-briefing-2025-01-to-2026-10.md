# Research briefing, January 2025 to October 2026

- **Status:** Complete. No definition changed and no threshold adopted. Edits applied: two evaluation-source rows and a reference group in the behaviour plan; ten source rows, one row correction and two open questions in the crash-data plan; five source rows, one row correction and three open questions in the attitudes plan; one backlog item's reading list extended.
- **Scope:** Primary work published from January 1, 2025 to October 10, 2026 on roadside and infrastructure LiDAR perception and on the safety and behaviour literature the analytics cite, plus older work newly surfaced in that window. The October 2026 sources already judged by the repository (the Waymo sober-baseline and fatal-rate papers, Valgo's client README, the two AAA Foundation studies and FMCSA's handheld programme) are taken as read and only their new neighbours are evaluated. The weekly briefing of the same date, written by the Monday routine from search summaries alone, judged Baumann, PRISA, the rectangle-fitting paper, Schäfer and Monfort and Mueller as newly surfaced and adopted the last as a cited source; this briefing reads those primaries where they could be reached and extends the window. No third-party briefing was supplied.
- **Related:** [Behaviour analytics plan, Sections 5, 11 and 16](../../../plans/lidar-behaviour-analytics-plan.md#5-benchmark-taxonomy), [crash-data plan](../../../plans/platform-crash-data-integration-plan.md), [attitudes plan](../../../plans/platform-speeding-attitudes-and-aggressive-driving-plan.md), [human crash baselines analysis](../../../platform/operations/human-crash-baselines-analysis-2026-10.md), [geometry convergence plan](../../../plans/lidar-tracker-geometry-convergence-plan.md), [the weekly briefing of October 3 to 10](research-briefing-2026-10-10.md), [the October 9 fixed-sensor briefing](fixed-sensor-research-briefing-2026-10-09.md)

A twenty-one month window was searched through the organisation and journal watchlists and the
topic queries of the `/research-briefing` skill, and every source worth more than a line was read
as far as the review environment allowed and judged against the benchmark taxonomy, the metrics
registry, the three plans the sources touch and the wording guard. The window holds no study that
changes a definition the repository already has. It holds four things the repository lacked: an
independent construction of the status-quo human crash rate for the areas the Waymo papers cover,
a modern United States pedestrian harm curve with the vehicle-height covariate, two tests of the
speed-change crash model at the low speeds and small reductions a residential site sees, and a
published worked example of that model applied to an observed free-flow speed distribution. On
the LiDAR side it holds a roadside sensor node whose error budget reads like this project's own,
three post-processing papers on the fragmentation problem the convergence plan measures, a
cross-sensor background-subtraction benchmark with released static and dynamic point labels, and
two roadside datasets with tracking identities that are closer analogues to a pole-mounted sensor
than the drone sets the behaviour plan lists.

## Answer

Nothing in the window moves a benchmark kind, renames a metric or justifies a threshold. The
behaviour plan's Section 5 rule, that a quantity with no defensible universal threshold prints as
a value and a percentile, is reinforced rather than weakened: the one new threshold candidate, a
predicted post-encroachment time of 1.5 s, is chosen by its authors as a warning operating point
from a sensitivity sweep and is not a severity validation, so it stays out of Section 5.1.

Four sources are adopted as references with their figures, and three of them open questions the
plans had not logged. The IIHS preprint of July 2026 builds the human crash involvement rate for
Phoenix, San Francisco, Los Angeles and Austin from state police-reported crashes and FHWA
urbanised-area vehicle-miles, with no in-transport, vehicle-type or time-of-day filter and no
under-reporting correction; it is the first construction of that rate not authored by Waymo or
built on Waymo's method, and its per-area rates belong in the safety-reference edition beside the
Waymo rows with their definitions stated. Monfort and Mueller's 2025 curve from 202 United States
pedestrian crashes puts the 50% fatality point where Tefft 2013 put it, at about 42 mph, and
moves the serious-injury curve and its low-speed end; which curve the edition prints, and whether
both print with their fleet years, is now an open question in the crash-data plan. Ambros and
Kieć's 2025 test of the exponential speed-crash model on 238 low-speed treatment sites found that
it overpredicts crash change at the micro scale and that half of the treatment categories show no
significant relationship at all, which bounds the plan's before-and-after model section on
exactly the streets this project surveys. Campolettano, Kusano and Victor's 2025 application of
the same model to a million free-flow segment traversals in two cities is the worked example the
plan's cited harm paragraph needs, and its observed limit-relative shares are the first comparator
for the attitudes plan's pace line that is observed rather than self-reported.

On the LiDAR side nothing is adopted into code. Schäfer, Alrifaee and Hashemi's sensor node
reports a centre error of 0.52 m RMSE that is dominated by a 0.46 m dimension error and an
orientation error of 7.7° RMSE worst at the field-of-view edges, which is the geometry convergence
plan's finding in another laboratory; its one-dimensional grid-map extent estimator with log-odds
and lock-on is the design that plan's extent belief should be compared against. Fontalvo and
colleagues' range-adaptive clustering tolerance is a candidate for the annotation-scored sweep,
where the project's own fragmentation gate can score it. The two fragment-stitching papers do not
change the behaviour plan's rule that coasting is not observation: a stitched gap stays suppressed
in every following metric, and the papers' own authors call the interpolated segment a visual
representation of an association decision. Baumann, Vosshans and Dang's benchmark releases the
static and dynamic point labels across three sensing technologies that an evaluation of this
project's L3 foreground extraction has lacked. R-LiViT and HetroD enter the behaviour plan's
evaluation table with what each can and cannot validate.

## Genuinely new in the window

Published between January 1, 2025 and October 10, 2026, by date of first listing.

- Luo et al., _Vehicle trajectory repair under full occlusion and limited datapoints with roadside LiDAR_, Sensors, February 2025.
- Mirlach et al., _R-LiViT_, arXiv, March 2025, accepted at ICCV 2025.
- Campolettano, Kusano and Victor, _Potential safety benefits associated with speed limit compliance in San Francisco and Phoenix_, Traffic Injury Prevention, online August 2025.
- Ambros and Kieć, _Are the speed-crash models applicable for low speeds?_, European Transport Research Review, 2025.
- Monfort and Mueller, _A modern injury risk curve for pedestrian injury in the United States_, Journal of Safety Research, online July 2025, print September 2025.
- Lee and Kim, _Impact of lowering the maximum speed limit of city roads on pedestrian traffic accident patients_, PLoS One, June 2025.
- Scanlon et al., _From Stoplights to On-Ramps_, arXiv, August 2025, since published as SAE International Journal of Transportation Safety 14(2).
- Schäfer, Alrifaee and Hashemi, _Lidar-based tracking of traffic participants with sensor nodes in existing urban infrastructure_, arXiv, September 2025; Advanced Intelligent Systems, May 2026.
- Shahbaz and Agarwal, _UrbanTwin_, arXiv, September 2025; IEEE Open Journal of ITS, 2026.
- Iglesias et al., _A fully interpretable statistical approach for roadside LiDAR background subtraction_, ICVES 2025, arXiv October 2025.
- European Commission, _Road safety thematic report, Speed and speeding_, version November 2025, listed 2026.
- Feng, Park and Mondschein, _Analyzing residential speeding using connected vehicle data_, arXiv, January 2026.
- Chen et al., _HetroD_, arXiv, February 2026, accepted at ICRA 2026.
- Ambros, Šípek, Křivánek and Valentová, _Exploring the heterogeneity in impacts of 30 km/h speed interventions_, Accident Analysis and Prevention, April 2026.
- Shang and Li, _Roadside LiDAR for cooperative safety auditing at urban intersections_, arXiv, April 2026, CVPR DriveX workshop; and the City College of New York final report, ROSA P, May 2026.
- Cui et al., _MR-LiDAR_, arXiv, May 2026.
- Fontalvo et al., _Spatial geometry analysis of roadside LiDAR for improved vehicle clustering accuracy_, Sensors, June 2026.
- Dong et al., _Occlusion-aware trajectory discontinuity correction for roadside LiDAR using time-space analysis_, Sensors, June 2026.
- Katz, Delecki, Qian and Moss, _Human Crash Baselines for Robotaxis and Robotrucks_, Valgo white paper, June 2026; and the AAA Foundation's _2025 Traffic Safety Culture Index_, June 2026.
- Ding et al., _RESOLVE_, arXiv, June 2026, accepted at ECCV 2026.
- Kusano, Scanlon, McMurry and Victor, _Do time of day and day of week matter when comparing Waymo's automated driving system and human crash involvement rates?_, Traffic Injury Prevention, in press, surfaced July 2026.
- Teoh, Kidd and Riexinger, _Rise of the machines: crash experiences of highly automated vehicles and human drivers_, IIHS preprint, July 2026.
- NHTSA, _Speeding: 2024 Data_, DOT HS 813 823, July 2026.
- Bang et al., _PRISA_, arXiv, July 2026, accepted at IEEE ITSC 2026.
- Baumann, Vosshans and Dang, _Beam-wise statistical background subtraction for static roadside LiDAR_, arXiv, August 2026, accepted at IEEE ITSC 2026.
- _Vehicle speed estimation using infrastructure-mounted LiDAR via rectangle edge matching_, Applied Sciences 16(5):2513, March 2026; authors unread, see To fetch.

## Newly surfaced older work

- Chen, Scanlon, Kusano, McMurry and Victor, _Dynamic benchmarks: spatial and temporal alignment for ADS performance evaluation_, arXiv October 2024, now Transportation Research Record 2025; surfaced as the lineage the Valgo white paper and Waymo's data hub cite.
- NHTSA, _2022-2023 National Survey of Speeding Attitudes and Behaviors_, DOT HS 813 594, December 2024; surfaced through the AAA Foundation's August 2026 report, which cites it.
- NHTSA's LIDAR speed-measuring device performance specification, DOT HS 809 811 (revised 2013), and the Conforming Product List of September 15, 2025; surfaced while chasing the prior briefing's open question on what a handheld following-distance unit measures.
- Laureshyn, Svensson and Hydén's 2010 following and crossing taxonomy by conflict angle, surfaced as the angle convention PRISA adopts; relevant to B5.
- The IIHS news release of December 10, 2024 on Monfort and Mueller's curve, which carries the by-speed and by-vehicle figures the paper's abstract does not.
- Waymo's safety impact hub, updated to June 2026 data on October 2, 2026, with downloadable benchmark files; its methodology page was read, its files were not.

## Teoh, Kidd and Riexinger, IIHS, July 2026

_Rise of the machines: crash experiences of highly automated vehicles and human drivers_, a
30-page preprint "submitted to a journal", read in full.

1. **What it measures.** Police-reportable crash involvements per million vehicle-miles for Waymo
   vehicles in driverless operation against police-reported involvements of human drivers in the
   same urbanised areas and years, 2021 to 2024, interstates and freeways excluded. The automated
   side is NHTSA's Standing General Order: 736 public-road crashes with automation engaged, of
   which the authors coded 159 (22%) as police-reportable or maybe so from the narratives, 89 of
   them Waymo and 64 of those driverless. The human side is state police-reported crash databases
   for Arizona, California and Texas, located to Census urbanised areas, with vehicle-miles from
   FHWA Highway Statistics Table HM-71 by urbanised area and functional class. No vehicle-type or
   time-of-day restriction was applied because HM-71 cannot be split that way; no under-reporting
   correction was applied. This is a continuous census of police records against a hand-coded
   sample of narratives, not a naturalistic or self-reported population.
2. **The maths.** The headline reproduces: 64 involvements over 49.957 million miles is 1.28 per
   million, 901,525 over 221,992 million is 4.06, and the ratio 0.32 with its interval (0.25,
   0.40) is what an exact Poisson interval on 64 events gives. The per-area human rates reproduce
   from the appendix: Phoenix 4.63, San Francisco 2.40, Los Angeles 4.01, Austin 3.48. Austin's
   "3% higher" rests on two Waymo involvements in 0.555 million miles and is not a finding. The
   rear-end split is the useful detail: striking another vehicle 0.09 of the human rate (0.02,
   0.37), being struck 0.60 (0.35, 1.03), the second interval crossing one. The uncertainty that
   dominates is not sampling but coding: the authors say so, call the narrative method
   "unsustainable", and note that Arizona's property-damage reporting threshold is $300 against
   $1,000 in the other states, with no adjustment made.
3. **What the sensor can observe.** Nothing of a crash rate; the analysis of October 8 showed a
   residential site needs decades of counts. What the paper could not do is the part a site can:
   nearly half of Waymo's involvements were on roads posted at 25 mph or below against 8% of the
   human ones, and the authors state that vehicle-miles were not available to turn that into a
   rate. A velocity.report site on a 25 mph street measures passages by hour on exactly the road
   class HM-71 cannot resolve by posted limit.
4. **Where it lands.** The crash-data plan's area benchmark context block and the safety-reference
   edition's area-rate table hold rates keyed by area, road type, temporal stratum, severity,
   cohort, crash year and source edition, today from Waymo's papers only. This paper's rates are
   a second construction of the status-quo rate for the same areas under a different definition:
   urbanised area not county, all vehicles not in-transport passenger vehicles, police-reportable
   not any-injury with a 32% correction. Its Austin human rate of 3.48 against Valgo's 4.055 for
   Travis County under the client README's default definition is the specification-curve point
   made with real numbers.
5. **Verdict.** New. Adopted as a source row and a reference in the crash-data plan; its per-area
   human rates are candidate edition rows with their definition carried, which the plan's rule
   already requires. The posture, that a report quotes a rate with its definition and never one
   number, already holds. Not adopted: any comparison of the automated and human rates, which is
   not this project's question.
6. **Rights.** The preprint carries IIHS's copyright notice and no licence. Figures are quotable
   with citation; tables are not reproduced. **Confirm** reuse terms with IIHS before an edition
   row cites the preprint rather than the published article.
7. **What a survey could give back.** A research note: the hourly passage profile on a 25 mph
   residential street, multiplied by a stated segment length, is a local vehicle-miles estimate by
   posted limit that neither HM-71 nor the SGO reporting provides, and the share of a site's
   passages at night is the exposure term the paper's dark-condition percentages (41% of Waymo
   involvements, 27% of human) could not be normalised by.

## Monfort and Mueller, Journal of Safety Research, 2025

_A modern injury risk curve for pedestrian injury in the United States: the combined effects of
impact speed and vehicle front-end height_, volume 94, pages 235 to 241. The abstract and the
IIHS release of December 10, 2024 were read in full; the paper was not reachable, so every figure
below that is not in the abstract is **Confirm**.

1. **What it measures.** Injury outcome against reconstructed impact speed for 202 pedestrians
   aged 16 or over struck by passenger vehicles, from Michigan 2015 to 2022 and from California,
   New Jersey and Texas in 2022, at three thresholds (MAIS 2+F, MAIS 3+F and fatal), with the
   striking vehicle's hood leading-edge height estimated from photographs of the model as a
   moderator, and formulas by pedestrian age and sex. Per the release: at 20 mph 46% at least
   moderate, 18% serious, 1% fatal; at 35 mph 86%, 67% and 19%; at 50 mph fatality above 80%;
   serious-injury risk 50% at 30 mph and 32% at 25 mph; at 30 mph a median car 37% serious
   against a median pickup 76%, the pickup's front end 13 inches higher. The 50% points per the
   abstract's indexing: 35, 49 and 68 km/h (22, 30 and 42 mph) for the three thresholds. The
   height effect on fatality was not statistically significant.
2. **The maths.** The curve form cannot be re-derived without the paper. Against the curves the
   crash-data plan already cites: Tefft 2013 puts 50% fatality at about 42 mph and 50% severe
   injury at about 31 mph; this paper puts 50% fatal at 42 mph and 50% MAIS 3+F at 30 mph. The
   50% points agree. They diverge below 25 mph, where Tefft's fatality curve is already at 10% by
   23 mph and this paper's is at 1% by 20 mph, and that is the region with the fewest fatal cases
   in a sample of 202. The paper's own comparison is to contemporary European estimates, which it
   says sit to the right of its curves. The serious-injury curve, not the fatal one, is where the
   new sample is strongest, and the height covariate is where it adds something no older curve
   has.
3. **What the sensor can observe.** Travel speed, never impact speed: the plan's caveat stands and
   this paper does not loosen it. The sensor can, however, observe the covariate this curve says
   matters: the body height of a passing vehicle, at the resolution of a class rather than a
   model. No camera is needed for that; L6's classes and the vehicle taxonomy already separate a
   car from a tall-fronted truck.
4. **Where it lands.** The crash-data plan's Item 1 reads Tefft 2013, Rosén and Sander 2009 and
   Hussain et al. 2019; the backlog's cited-harm item names the first two. None cites this paper.
   Its standardisation (2015 to 2022 United States fleet) is more recent than Tefft's (2007 to
   2009), which the plan's own sources-checked row flags as a limitation.
5. **Verdict.** New. Adopted as a reference and a source row in the crash-data plan, and the
   backlog item's reading list is extended to it. Logged as an open question there: whether the
   edition prints Tefft 2013, this curve, or both with their fleet years, and whether the
   all-vehicle curve or the per-class curves are the ones a report can use given that a class is
   what the sensor sees. Not adopted: any per-vehicle harm statement on a report page, which the
   wording guard and Section 1 of the behaviour plan already forbid.
6. **Rights.** Elsevier copyright; the abstract and the figures the authors published in the IIHS
   release are citable with attribution; reproduction of the paper's figures or formulas needs
   the publisher's terms. **Confirm** on reading.
7. **What a survey could give back.** The share of passages by class and hour at a site is the
   fleet-mix term this curve needs and that a crash record lacks; a research note could report it
   beside the speed distribution without ever multiplying the two on a page.

## Ambros and Kieć 2025, and the speed-change model at low speed

_Are the speed-crash models applicable for low speeds?_, European Transport Research Review
17:59, CC BY 4.0, read in full. With it, two sources that bear on the same section of the
crash-data plan: Ambros, Šípek, Křivánek and Valentová, _Exploring the heterogeneity in impacts of
30 km/h speed interventions_, Accident Analysis and Prevention 232:108547, April 2026, read from
search summaries only; and the European Commission's ERSO thematic report on speed, version
November 2025, read in full.

1. **What it measures.** The 2025 paper collects 238 traffic-calming locations from published and
   unpublished European before-and-after studies (30 km/h zones in Switzerland, Germany, Norway and
   the Netherlands, 20 mph zones in Britain, 20 and 30 km/h zones in Poland, Czech village
   gateways and more), with mean speeds before and after and injury crash counts, and asks whether
   the crash change the exponential model predicts from the speed change (coefficient 0.06 per
   km/h for injury crashes, from Elvik et al. 2019) matches the crash change observed. Mean speeds
   before ran from 33 to 56 km/h by original limit; 97% of sites carried under 10,000 vehicles a
   day. The 2026 meta-analysis pools 30 km/h intervention studies with random effects and reports
   17% fewer crashes, 18% fewer injuries, speed down 4.68 km/h, no significant volume change and
   about 1 dB less noise, with larger crash effects for physical calming than signs alone, for
   whole areas, and where baseline speeds were higher; all **Confirm**. The ERSO report is a
   secondary synthesis that repeats Yannis and Michelaraki's unweighted 2024 figure of 37% fewer
   fatalities across 17 cities, which the 2026 meta-analysis says overstates the pooled effect.
2. **The maths.** The model reproduces as stated: a 10 km/h fall in mean speed predicts 45% fewer
   injury crashes, 5 km/h predicts 26%, and the 1.6 km/h a 25 mph street might move predicts 9%.
   The test is a regression of observed on expected change. Over all locations the slope is
   positive and significant but below one across its confidence interval, so the model
   overpredicts; in five of nine treatment categories there is no significant relationship, and in
   the four that show one the interval spans both under- and overprediction. The explanatory
   speed model has R² of 0.44. The dominant uncertainty is the assignment of crashes to sites and
   the absence of volume control, which the authors state. Their conclusion is that the model "may
   present the trend of crash changes, but should not be taken as a predictive tool for
   estimating the exact values", and "particularly" so at low speeds.
3. **What the sensor can observe.** The speed term, continuously, with free-flow opportunity
   separated, which the paper says its sources lacked ("further details, such as free-flow speed
   considerations, were not available") and which it names as the data future studies need. It
   cannot observe the crash term, and the site arithmetic of October 8 shows no window changes
   that.
4. **Where it lands.** The crash-data plan's before-and-after model section (Item 3) prints the
   relative change in crashes by severity that the pinned model predicts from a change in mean
   speed between two periods at one site. Those sites are the micro-scale, low-speed,
   small-reduction case this paper tested and found the model least valid for. The plan's Sources
   checked row for Elvik et al. 2019 is **Confirm coefficients** and says nothing about validity
   at this scale.
5. **Verdict.** New. Adopted as references and source rows in the crash-data plan, and logged
   there as an open question: whether the before-and-after section prints a predicted number at
   all on streets under 50 km/h, or only the model's direction with the interval and this
   paper's finding, and whether the 2026 meta-analysis's pooled 30 km/h effect enters the edition
   as a `published_model` entry of a different kind, an intervention effect rather than a
   speed-change model. ERSO is superseded on the 30 km/h effect size and is not a source of
   figures. Not adopted: any change to the model's coefficients, which the plan pins from Elvik.
6. **Rights.** The 2025 paper is CC BY 4.0 and may be reproduced with attribution. The ERSO
   report states "reproduction of this document is allowed with due acknowledgement". The 2026
   paper's terms are unread; **Confirm**.
7. **What a survey could give back.** A before-and-after speed record with its interval and its
   free-flow fraction at one street is the input the 2025 paper asked for; paired over years with
   a municipality's crash record for the same street, it tests the model at the scale where the
   paper could not. A research note should state at the outset that the crash side needs the
   municipality, not the sensor.

## Campolettano, Kusano and Victor, Traffic Injury Prevention, 2025

_Potential safety benefits associated with speed limit compliance in San Francisco and Phoenix_,
online August 2025. Waymo's research page was read; the article was not reachable.

1. **What it measures.** Speeds of human-driven vehicles estimated from opposite-direction traffic
   observed by Waymo's ride-hailing fleet, over a million unique vehicle-segment traversals in
   free-flow conditions on surface streets in the two cities, with speeding defined against the
   posted limit by segment. Per the page: 33% to 49% of observed vehicles above the limit, 85th
   percentile speeds 3.6 to 7.2 mph above it, and an exponential speed-crash model stratified by
   posted limit giving 18% to 30% fewer serious injuries and 27% to 43% fewer fatalities under
   full compliance, about 82 lives a year from FARS counts. All **Confirm** against the article.
2. **The maths.** With Elvik's fatality coefficient of 0.08 per km/h, the stated fatality range
   corresponds to mean-speed reductions between 3.9 and 7.0 km/h, and with the injury coefficient
   of 0.06 the injury range to 3.3 to 5.9 km/h. Both sit inside the observed 85th-percentile
   excess of 5.8 to 11.6 km/h, so the headline is internally consistent with the model the plan
   pins. What cannot be re-derived from the page is the share of vehicle-miles in each
   speed-limit stratum, which sets the weights.
3. **What the sensor can observe.** The same quantities, at a point rather than along segments:
   the share of passages above the limit, the 85th percentile against the limit, and free-flow
   opportunity. The populations differ: these are traversals seen from a moving vehicle, across
   every segment the fleet drives; a site sees every passage at one place. Section 4 of the
   behaviour plan keeps fixed and mobile observations apart for this reason.
4. **Where it lands.** The attitudes plan's Item 3a prints the pace of traffic and the share of
   transits above the limit by 5, 10 and 15 mph; its only comparators today are self-reported
   shares from questionnaires, which the plan forbids printing beside an observed share. This is
   an observed share, from the only other study in the window that reports one for United States
   streets. The crash-data plan's cited harm paragraph has no worked example of the model applied
   to a measured distribution; this is one, by speed-limit stratum.
5. **Verdict.** New. Adopted as a reference and a source row in both plans. Logged in the
   attitudes plan as an open question: whether the observed shares enter the pace line as an
   `external_distribution` comparator, with the mobile population and the two cities stated, or
   only as context in the guide.
6. **Rights.** Traffic Injury Prevention; the article's licence is unread. **Confirm**.
7. **What a survey could give back.** The fixed-point version of the same measurement on a street
   the fleet does not drive, and the time-of-day split the fleet's sampling cannot give evenly.

## The benchmark-alignment family: Kusano et al. 2026, Chen et al. 2025, and Waymo's hub

Kusano, Scanlon, McMurry and Victor, _Do time of day and day of week matter when comparing
Waymo's automated driving system and human crash involvement rates?_, accepted at Traffic Injury
Prevention, read in full from the preprint on Waymo's site; Chen, Scanlon, Kusano, McMurry and
Victor, _Dynamic benchmarks_, Transportation Research Record 2025, read from the arXiv abstract;
Waymo's safety impact hub methodology page as of October 2, 2026, read; the October 8, 2026 blog
on the sober paper, read; the July 7, 2026 blog on time and place, read.

1. **What they measure.** Kusano 2026 takes 2023 state crash and county vehicle-miles for
   Maricopa, San Francisco, Los Angeles and Travis counties and allocates the county total across
   hours and days with Inrix volume profiles at 15-minute resolution, aggregated to the hour; it
   finds surface-street benchmark rates between midnight and 03:59 of 2.00 to 4.82 times the
   average on weekdays and 2.67 to 6.23 times on weekends, and a benchmark matched to the fleet's
   place and time mix 1.41 to 1.71 times the population rate. Chen 2025 does the spatial half
   with level-13 S2 cells and finds the reweighted benchmark 10% to 47% higher in San Francisco,
   12% to 20% in Maricopa and 7% lower to 34% higher in Los Angeles. The hub publishes 271.3
   million miles by area to June 2026 and four downloadable files, including the geographic
   distribution the adjustment uses, with a 32% under-reporting correction on the any-injury
   benchmark and none on the airbag or serious-injury ones.
2. **The maths.** The reweighting is a weighted average of cell or hour rates by the fleet's
   share of miles; the magnitudes are plausible and were not re-derived, since the inputs are not
   published at cell level. Kusano 2026 says in its own words that "there are limited validation
   studies of the absolute accuracy of hourly VMT estimates by Inrix", which the October 8
   analysis had already identified as the least validated input of the fatal-rate paper.
3. **What the sensor can observe.** The hourly volume profile of one road link, directly, which is
   the quantity Inrix models from probes. This is the strongest give-back in the window and it
   was already in the analysis's recommendation; the new paper states the validation gap itself.
4. **Where it lands.** The analysis's Finding 5 adopted the four temporal strata and Item 2a of
   the crash-data plan builds them into the report; the lineage rows for Chen et al. 2025 were
   **Confirm**. Nothing here changes a stratum: the edition's area rates are published at four
   strata, and hourly benchmarks exist only for four counties.
5. **Verdict.** Already holds for the strata and the posture. Adopted as references and source
   rows in the crash-data plan, with the Chen row confirmed. Not adopted: the hub's running
   comparison, which is a vendor surface; its benchmark files are listed under To fetch so their
   terms can be read before any row cites them.
6. **Rights.** Kusano 2026 is a preprint on Waymo's site with no stated licence; Chen 2025 on arXiv
   is CC BY-NC-ND 4.0; the hub states no licence for its downloads and says the data is public for
   replication. **Confirm** each before use.
7. **What a survey could give back.** A research note comparing a site's hourly share profile with
   Inrix's for its link class, which neither paper could do without a fixed counter.

## Valgo's white paper, June 2026, and the client at 0.4.0

Katz, Delecki, Qian and Moss, _Human Crash Baselines for Robotaxis and Robotrucks_, 14 pages,
read in full. The PyPI release history was read: 0.1.0 on June 17, 2026, 0.3.0 on September 29
(adding a fatal definition), 0.4.0 on October 1, which is the README the October 8 analysis read.

1. **What it measures.** The methodology behind humanbaselines.com as of June 2026: a matched
   crash count over matched exposure, with every filter on the numerator mirrored on the
   denominator. Numerator from state police-reported extracts; outcomes any reported injury (K, A,
   B or C), serious or fatal (K or A), fatal (K), and airbag deployment as a physical proxy;
   under-reporting correction optional, scaling property-damage-only by 2.48 and non-fatal injury
   by 1.47 after Blincoe et al. 2023; location from coordinates or geocoded against OpenStreetMap.
   Denominator from state roadway inventories where complete, else HPMS 2023, split by vehicle
   class with FHWA VM-4 fractions and by weather with NOAA station observations. Geofence mode
   uses level-13 S2 cells and an exact Poisson interval; route mode uses corridor segments and an
   empirical-Bayes gamma-Poisson interval. Ten metropolitan areas across Texas, California,
   Arizona and Nevada, and interstate corridors across four states. Its Figure 1 shows Austin's
   2022 any-injury rate spanning roughly 0.5 to 8 crashes per million vehicle-miles across 96
   specifications, and its Figure 2 the Travis County 2022 to 2024 cascade from 97,309
   police-reported crashes to 67,447 after location, road type, in-transport, vehicle type and
   weather filters.
2. **The maths.** The cascade and the rate algebra are the sober paper's and reproduce. The paper
   says the denominator carries more uncertainty than the numerator, "especially for local roads
   where traffic counts are sparse and the share of travel must be estimated rather than
   measured".
3. **What the sensor can observe.** The local-road count the paper says is sparse.
4. **Where it lands.** The October 8 analysis listed six claims about Valgo as **Confirm** from
   search summaries: the recreation of Waymo's baselines, the specification-curve posture, the
   lineage, the data sources, the Blincoe multipliers and the homepage example. The white paper
   confirms the first five in its own words; the homepage example was not in it. The terms of
   use remain unread: the paper states no licence for itself and notes only OpenStreetMap's ODbL
   for the base map.
5. **Verdict.** Already holds. The crash-data plan's Valgo row is corrected to say what is now
   read, and the paper is added as a reference. Not adopted: any API result, pending the terms.
6. **Rights.** No statement in the paper. **Confirm**.
7. **What a survey could give back.** As for Kusano 2026: a measured local-road volume where the
   denominator is modelled.

## Questionnaires and the officer-attributed share

The AAA Foundation's _2025 Traffic Safety Culture Index_ (Zhang and Steinbach, June 2026), read
in part: front matter, rights, method, Tables 2 and 5 and the neighbourhood comparison. NHTSA's
_2022-2023 National Survey of Speeding Attitudes and Behaviors_ (DOT HS 813 594, December 2024),
record page read, report not reachable. NHTSA's _Speeding: 2024 Data_ (DOT HS 813 823, July 2026),
read in full. Nationwide's Agency Forward driving behaviours questionnaire of January 2026, press
coverage only.

1. **What they measure.** The index is a probability-based web panel (Ipsos KnowledgePanel) of
   2,699 licensed drivers who drove in the past 30 days, fielded July 31 to August 13, 2025. On
   speeding it reports perceived danger (61% rate 10 mph over the limit on a residential street
   very or extremely dangerous; 48% for 15 mph over on a freeway), perceived apprehension (62%
   think police would catch 15 over on a freeway), social disapproval (speeding has the lowest of
   every behaviour asked), self-report (48% drove 15 over on a freeway and 37% drove 10 over on a
   residential street at least once in the past 30 days), perception of neighbours (91% and 86%
   say drivers in their neighbourhood drive too fast on freeways and residential roads), and
   support for countermeasures (47% for cameras ticketing more than 10 mph over on residential
   streets). The NHTSA survey is the fourth in a series: 5,680 respondents by web and mail;
   search summaries give 87% agreeing that everyone should obey the limit because it is the law
   and 85% calling more than 20 mph over unacceptable, both **Confirm**. The fact sheet defines a
   speeding-related crash as one in which any driver was charged with a speeding offence or an
   officer indicated racing, driving too fast for conditions or exceeding the limit; in 2024,
   28% of fatal crashes and 13% of injury crashes were speeding-related, 11,288 fatalities were
   29% of the 39,254 total, down 5% from 2023, and 87% of speeding-related fatalities were on
   non-interstate roads. Nationwide's questionnaire asks drivers what they think of other
   drivers.
2. **The maths.** The index's 37% and 48% are sums of its Table 5 rows (36.5% and 47.7% at least
   once); the margin of error it states for the full sample applies. The fact sheet's shares are
   officer attributions on FARS and CRSS records, not measured speeds.
3. **What the sensor can observe.** Passages above a limit, never what anyone says, believes or
   was charged with. The attitudes plan's rule that a self-reported share and an observed share
   are never printed side by side as one holds; this briefing adds that the officer-attributed
   share is a third population and gets the same treatment.
4. **Where it lands.** The attitudes plan's sources table carries "a 2025 AAA Foundation telephone
   survey: 37% exceeded the limit by 10 mph on a residential street in the past 30 days" from the
   IIHS speed page, marked as a different study and **Confirm**. The 37% is this index's figure,
   from a web panel, not a telephone survey; the row is corrected. The plan's cited attitudes
   paragraph quotes "nearly 30% of traffic fatalities in 2024" through the AAA report; the fact
   sheet is the primary with the definition. The index's title passes the wording guard; its
   item wording does not, and never reaches a page.
5. **Verdict.** New. Adopted as source rows in the attitudes plan. Logged there as an open
   question: whether the index, an annual series on a constant questionnaire since 2008, enters
   the safety-reference edition's attitudes entries beside the one-off attitudes study, since a
   series is what any before-and-after reading of attitudes would need. The NHTSA survey is a
   reference, To fetch. Nationwide's questionnaire is not adopted: it measures perceptions of
   others with no published method.
6. **Rights.** The index "may be copied in whole or in part and distributed for free via any
   medium, provided the Foundation is given appropriate credit", and "may not be resold or used
   for commercial purposes without the explicit permission of the Foundation", the same terms as
   the two studies the plan already holds and the same open half. NHTSA documents are United
   States government works.
7. **What a survey could give back.** Nothing to a questionnaire. The limit-relative shares the
   index asks about (10 mph over on a residential street) are the bands Item 3a prints, so the
   guide can place the two side by side as different populations with the same band, which is
   the one legitimate juxtaposition.

## The 85th percentile in practice: ITE, MUTCD and Caltrans

ITE's _Setting Speed Limits_ page (copyright 2026) and Caltrans's _Setting Speed Limits_ page,
read; the MUTCD 11th Edition Revision 1 (December 2025), the California Manual for Setting Speed
Limits update and NCHRP 17-133 (started December 2025) known from those pages and search
summaries only.

1. **What they state.** ITE: the 85th percentile speed describes existing operating behaviour,
   is one input and should not by itself set the target or posted speed; an observed speed above
   the target should prompt review of the roadway, not a higher limit; the current national
   manual is the 11th Edition with Revision 1, dated December 2025. Caltrans: the 85th percentile
   rounded to the nearest 5 mph remains the method, with a documented 5 mph reduction on an
   engineer's finding, and the manual's update was targeted for March 2026. NCHRP 17-133 is a
   two-year study of the 85th percentile's applicability on freeways and rural highways.
2. **The maths.** None to check.
3. **What the sensor can observe.** The 85th percentile of free-flow passages, which is exactly
   what these pages say is an operating-speed statistic and not a limit-setting rule.
4. **Where it lands.** The attitudes plan's Item 2 rewrites the report's percentile paragraph to
   describe what p98 measures; the percentile semantics doc defines p85 and p98 as aggregates
   over vehicle maximum speeds. Neither cites a practice document saying what p85 is used for.
5. **Verdict.** Already holds for the definition. Logged in the attitudes plan as an open question:
   whether the percentile paragraph cites ITE's position and the MUTCD edition so that a reader
   never reads a printed p85 as the limit-setting rule. The MUTCD revision itself is To fetch.
6. **Rights.** ITE and Caltrans pages are cited, not reproduced.
7. **What a survey could give back.** Nothing beyond the statistic itself.

## Feng, Park and Mondschein, arXiv, January 2026

_Analyzing residential speeding using connected vehicle data: a case study in Charlottesville, VA
area_, read from the HTML full text.

1. **What it measures.** Connected-vehicle trajectory records at 3 s intervals over 14 days in
   April 2022, supplied through the Virginia DOT, on OpenStreetMap ways tagged residential in
   Charlottesville and Albemarle County, with the limit from the way's maxspeed tag where present
   and 25 mph imputed where absent, which was 97.2% of the 3,754 segments kept. "Aggressive" is
   at or above the limit plus 10 mph and "reckless" at or above the limit plus 20, the statutory
   term; 38% of segments had at least one of the first and 20% of the second. The 27-fold
   night-to-day figure is, by inference from its Table 4, a ratio of segment counts (1,243 ways
   with a higher night rate against 46 with a higher day rate), not a rate.
2. **The maths.** Per-segment rates are records above the band over records, for segments with at
   least 100 observations; the 27 is not a prevalence ratio and should not be quoted as one.
3. **What the sensor can observe.** The same bands at a point, with a posted limit the operator
   confirmed rather than one imputed to 97% of segments.
4. **Where it lands.** The posted speed limits plan's open question on who confirms a limit read
   from a sign or an OpenStreetMap prior; this paper is the cautionary case. Its labels are
   verdict words the wording guard refuses.
5. **Verdict.** New. Not adopted. Noted here as the reason the posted-limits plan does not impute.
6. **Rights.** arXiv non-exclusive licence.
7. **What a survey could give back.** A night-to-day ratio that is a ratio of rates.

## Schäfer, Alrifaee and Hashemi, Advanced Intelligent Systems, 2026

_Lidar-based tracking of traffic participants with sensor nodes in existing urban
infrastructure_, read in part from the September 2025 preprint (abstract, the dimension
estimator, the results); the version of record of May 18, 2026 was not reachable.

1. **What it measures.** A single LiDAR and a CPU-only edge computer on an existing pole, running
   an extended Kalman filter for state, a one-dimensional grid map with binary Bayes updates in
   0.1 m cells for each of length, width and height, a class from a footprint lookup, and an
   existence probability from track age and box consistency; boxes from an L-shape fit (the
   Autoware implementation). Ground truth from RTK GPS at about 5 cm on a reference car over 19
   passes in urban-like test scenes. Results: pose RMSE 0.52 m, dimension RMSE 0.46 m, orientation
   RMSE 7.69°, the class right in 99.6% of frames, and 99.88% of messages within 100 ms end to
   end. The sensor model was not captured from the text and is **Confirm**.
2. **The maths.** The authors attribute the pose error to the dimension error because pose is the
   box centre, and the orientation error to the field-of-view edges where the L-shape is partly
   visible. Both are the mechanisms the geometry convergence plan measured on kirk0 in October:
   believed extents short of the labelled span, and a heading that does not converge because an
   end face's axis seeds it.
3. **What the sensor can observe.** The same things; this node is the nearest published analogue
   to the project's pipeline in sensor count, compute class and estimator family.
4. **Where it lands.** The convergence plan's extent belief (§9.2 of the state-estimation plan)
   accumulates admitted spans; this paper's estimator discretises each dimension, updates
   log-odds and locks on after sufficient evidence, and reports a standard deviation from the
   grid. The plan has no comparison against a grid-map estimator.
5. **Verdict.** New. Adopted as a reference in the behaviour plan's new roadside-perception group.
   Proposed, not applied: the convergence plan compares its extent accumulation with the
   grid-map and log-odds scheme on kirk0's reviewed references, where the same RMSE decomposition
   (centre error against extent error) is computable today.
6. **Rights.** arXiv non-exclusive licence for the preprint; Wiley's terms for the article are
   unread; "code published on acceptance" per the preprint, repository unverified. **Confirm**.
7. **What a survey could give back.** The hypothesis that at kirk0 the current solver's centre
   error is dominated by its extent error, as it is in this node, scored against the three fitted
   references.

## Fontalvo et al., Sensors, June 2026

_Spatial geometry analysis of roadside LiDAR for improved vehicle clustering accuracy_, Sensors
26(13):4068, CC BY 4.0, read in full.

1. **What it measures.** A neighbourhood tolerance for clustering that grows with radial distance
   and the vertical beam separation of the sensor (a Velodyne VLP-32C at a fixed height), applied
   by normalising pairwise distances so DBSCAN and hierarchical clustering run unchanged. On a
   15-minute controlled recording with one reference vehicle, DBSCAN accuracy rose from 85.2% to
   97.4% and MOTA from 0.766 to 0.960, with false negatives from 1,060 to 304 and identity
   switches unchanged at about 20; at two Lubbock intersections 86.6% to 90.8% and 83.9% to
   84.0%. The scaling factor is stable between 1.7 and 2.4; the cost is under 2.2 ms a frame.
2. **The maths.** The tolerance is a geometric statement about angular sampling, not a fit, and
   its parameters are tied to the sensor's beam table. The paper reports no fragmentation count
   and one merged-vehicle example.
3. **What the sensor can observe.** The Pandar40P's non-uniform vertical spacing makes the same
   range-dependent gap the paper models.
4. **Where it lands.** L4's DBSCAN takes one eps for the whole field
   ([dbscan_clusterer.go](../../../../internal/lidar/l4perception/dbscan_clusterer.go)), the
   sweep tunes it, and the annotation-scored tuning recorded recall falling with range. The
   backlog's W3 retention and fragmentation gate scores, per reviewed mask, the share of returns
   in the associated cluster and the number of clusters its returns fall in: the fragmentation
   measure this paper lacks.
5. **Verdict.** New. Adopted as a reference. Proposed, not applied: a range-adaptive eps as a
   sweep arm scored by W3 on kirk0, with the two trucks as the merging test.
6. **Rights.** CC BY 4.0.
7. **What a survey could give back.** The per-mask fragmentation count at 20 to 50 m before and
   after, which this paper could not report.

## Fragment stitching: Dong et al. 2026 and Luo et al. 2025

_Occlusion-aware trajectory discontinuity correction for roadside LiDAR using time-space
analysis_, Sensors 26(12):3755, and _Vehicle trajectory repair under full occlusion and limited
datapoints with roadside LiDAR_, Sensors 25(4):1114, both CC BY 4.0, both read in full.

1. **What they measure.** Dong et al. stitch track fragments in a lane's time-space diagram with
   a temporal gap up to 2.5 s (3.5 s when the window shows negative velocity), a spatial gap up to
   20 m and linear interpolation, on four 30-minute peaks at one Reno intersection against manual
   annotation: precision 0.989, recall 0.916, F1 0.948, and count error from 14.5% to 2.6% in a
   50 m zone, from 35.8% to 5.3% in a 150 m zone. Luo et al. detect full occlusion from jumps in
   the gaps to the neighbours ahead and behind, link the fragments, and refine the representative
   point from the most complete frame's box: 82% to 95% of occlusions correctly reconnected on
   straight and curved roads, against 30% to 84% for two earlier methods; no numerical speed
   error reported.
2. **The maths.** Both are post-processing with site-tuned thresholds; Dong et al. tuned on the
   evaluation data and say so. Dong et al. also state that the interpolated segment "is only a
   visual representation of the association decision, not a reconstruction of the true path".
3. **What the sensor can observe.** Nothing during the occlusion; both papers agree.
4. **Where it lands.** Section 9.2 of the behaviour plan, "coasting is not observation": a
   stitched gap contributes no valid following time and no instant. B1 asks how often both
   parties are observed; the count-error numbers here show what fragmentation does to volume on
   a long zone, which is the radar transit worker's problem too.
5. **Verdict.** Already holds for every metric. Adopted as references. Not adopted: interpolation
   across a gap, in any metric; the identity link alone is the part the convergence plan's
   reacquisition work may take, scored by the W5 identity gate.
6. **Rights.** CC BY 4.0.
7. **What a survey could give back.** The count error on a 50 m zone at kirk0 before and after
   reacquisition, which these papers report for other sites.

## Background subtraction benchmarks: Baumann, Vosshans and Dang 2026, Iglesias et al. 2025

_Beam-wise statistical background subtraction for static roadside LiDAR: a cross-sensor benchmark
study_, arXiv August 2026, accepted at IEEE ITSC 2026, read from the HTML full text; _A fully
interpretable statistical approach for roadside LiDAR background subtraction_, ICVES 2025, read
from the abstract.

1. **What they measure.** Baumann et al. treat background estimation as per-beam temporal
   modelling and compare a cell-free-time baseline, a density-aware variant, a
   quantile-constrained Gaussian, a beam-wise Gaussian mixture and an image-based mixture, with
   and without spatial consistency filtering, on a new HighwayScene set (Blickfeld QB2, Ouster
   OS0 and Aeva Aeries II; 5,997 frames split 4,000, 1,000 and 997) and on 76 newly annotated
   CoopScenes frames (Ouster OS2; 2,455 objects, 857 dynamic), with point-wise precision, recall
   and F1 on dynamic points. No foreground-free frames are required. Best F1 with filtering:
   99.4% on the Ouster, 97.0% on the Blickfeld, 97.6% on the Aeva; no CoopScenes scene below 90%.
   Filtering costs 6 to 39 ms a frame on a desktop CPU. Data, annotations and code are to be
   released at the HighwayScene page. Iglesias et al. build a Gaussian grid from background-only
   scans and evaluate on RCooper.
2. **The maths.** The models are the same family as L3's per-cell EMA with Welford variance and
   closeness thresholds; the paper's statement that per-beam statistics transfer across sensing
   technologies is the claim to test, not accept.
3. **What the sensor can observe.** The same.
4. **Where it lands.** L3's foreground extraction has unit tests and the kirk0 reviewed masks, but
   no public point-labelled benchmark across sensors; the evaluation table in Section 11 has no
   row for foreground extraction at all because the table is about behaviour.
5. **Verdict.** New. Adopted as references. Proposed, not applied: run L3 on CoopScenes' annotated
   frames once released and report point-wise precision and recall beside the paper's table.
6. **Rights.** arXiv non-exclusive licence; the datasets' terms are unread. **Confirm**.
7. **What a survey could give back.** A fourth sensor and a residential scene to the cross-sensor
   table, if the project's masks were published in the same point-label form.

## PRISA and the New York auditing work

Bang et al., _PRISA: proactive infrastructure LiDAR framework for intersection safety
assessment_, arXiv July 2026, IEEE ITSC 2026, read from the HTML full text; Shang and Li,
_Roadside LiDAR for cooperative safety auditing at urban intersections_, CVPR DriveX 2026, CC BY
4.0, read from the abstract; Li, Shang, Feng, Wei and Kamga, City College of New York final report,
ROSA P, May 2026, read from the record.

1. **What they measure.** PRISA fuses two Ouster LiDARs at 5 m through the vendor's backend at
   10 Hz, downsampled to 5 Hz, predicts trajectories with FlowChain over a 1.6 s history and a
   2.4 s horizon, and raises conflicts on time to collision for following geometry (within 30° of
   the leader's heading) and on predicted post-encroachment time for crossing geometry (beyond
   60°), both at 1.5 s, with a 2 m/s speed floor and a 15 m region. TTC's 1.5 s is cited to SSAM;
   PPET's 1.5 s is chosen from a sweep over 0.5 to 2.4 s as the operating point that "captures
   most detected conflicts" and stays stable. On R-LiViT the predictor's displacement errors are
   0.49 to 0.65 m average and 0.71 to 1.11 m final. In Chattanooga on a Jetson AGX Thor the
   pipeline runs in 194 ms a frame, and the evaluation window produced 1,190 vehicle-vehicle TTC
   conflicts, 1,139 PPET conflicts and six involving a vulnerable road user (window length
   **Confirm**). The New York work builds an 8,000-frame annotated roadside set at one signalised
   intersection, finds that direction-agnostic TTC drops below 1 s while longitudinal TTC stays
   above braking thresholds in a lateral-intrusion conflict, and reports from a beam-loss study on
   public sets that vulnerable road user detection is stable to about 20% beam loss and degrades
   faster when the lost beams are contiguous.
2. **The maths.** A 1.5 s TTC threshold that fires over a thousand times in one evaluation window
   at one intersection is a candidacy screen, which is what Section 5.2 of the behaviour plan
   says SSAM's thresholds are. PPET is a predicted quantity; the plan's PET is observed.
3. **What the sensor can observe.** Observed PET and TTC on the same geometry; not a prediction.
4. **Where it lands.** Section 5.1's threshold table and B5, which asks whether interaction type
   can be classified from conflict angle: PRISA's 30° and 60° bands come from Laureshyn,
   Svensson and Hydén 2010, which the plan does not cite.
5. **Verdict.** New. Not adopted as a threshold: PPET's 1.5 s is an operating point, not a
   severity finding, and the quantity is not one the plan computes. Adopted as references;
   Laureshyn et al. 2010 noted for B5. The beam-loss finding is noted for the sweep's occlusion
   arms.
6. **Rights.** arXiv non-exclusive licence (PRISA); CC BY 4.0 (Shang and Li); the ROSA P report's
   rights statement is unread.
7. **What a survey could give back.** Observed PET distributions at a stop-controlled residential
   crossing, where PRISA's six vulnerable-road-user conflicts suggest how rare the event is.

## Roadside and drone datasets

R-LiViT (Mirlach, Wan, Wiedholz, Keen and Eich; XITASO and LiangDao; ICCV 2025), HetroD (Chen et
al.; ICRA 2026; levelXdata), MR-LiDAR (Cui et al.; May 2026), RESOLVE (Ding, Song, De Vincenzi
and Suo; ECCV 2026) and UrbanTwin (Shahbaz and Agarwal; IEEE OJ-ITS 2026), all read from
abstracts and project pages.

1. **What they hold.** R-LiViT: three intersections in two German cities recorded in late spring
   2024, day and night, 10,000 LiDAR frames, 2,400 aligned RGB and thermal pairs, 150 scenarios,
   seven LiDAR classes with per-object tracking identities, public; the LiDAR model and the
   dataset licence are **Confirm**. HetroD: drone recordings at six Taiwanese sites, 17.5 hours,
   65,400 trajectories of which 70% are vulnerable road users, HD maps and signal states, free for
   non-commercial use. MR-LiDAR: 16, 32, 80 and 128-beam sensors in identical roadside scenes,
   finding that an 80-beam sensor with an optimised beam distribution matches or beats a uniform
   128-beam one, with a selection guide; access unstated. RESOLVE: one urban intersection at
   three LiDAR resolutions with cameras, 26,000 point-cloud frames, 220,000 boxes, ten classes,
   on GitHub. UrbanTwin: synthetic replicas of LUMPI, V2X-Real-IC and TUMTraf-I with 10,000
   frames each from emulated sensors in digital twins, on Harvard Dataverse, CC BY-NC-ND.
2. **The maths.** None to check beyond counts.
3. **What the sensor can observe.** R-LiViT is the one set recorded from the sensor's own vantage,
   a pole at an urban intersection, with identities for pedestrians and cyclists; the behaviour
   plan's note that drone sets have better occlusion characteristics than ours applies to HetroD
   as it does to inD.
4. **Where it lands.** Section 11's evaluation table, which lists highD, inD, rounD, openDD and
   Waymo Open Motion and calls inD the closest external analogue.
5. **Verdict.** New. R-LiViT and HetroD adopted as Section 11 rows with what each can and cannot
   validate; MR-LiDAR, RESOLVE and UrbanTwin adopted as references only. RESOLVE's camera fusion
   is outside Tenet 1 for the product; its LiDAR-only baselines at three resolutions are not.
6. **Rights.** HetroD and UrbanTwin non-commercial; R-LiViT and MR-LiDAR **Confirm**; RESOLVE's
   repository terms unread.
7. **What a survey could give back.** A residential-street roadside set with reviewed masks, which
   none of these is.

## Rectangle edge matching, Applied Sciences, March 2026

_Vehicle speed estimation using infrastructure-mounted LiDAR via rectangle edge matching_,
16(5):2513. Unreachable; search summaries only, every claim **Confirm**: boundary points from a
cluster, outliers removed, a 2D rectangle fitted by Gauss-Newton with regularisers that hold
width and height stable, on a 40-channel sensor at an intersection in Hwaseong, speed scored
against CAN-bus ground truth, with fits sustained when about two rings are observed. The
convergence plan's W1a fits orientation by the closeness criterion of Zhang 2017 and sets its
variance from the line fits; a regularised rectangle fit scored against vehicle-bus speed is the
nearest published comparator. Not adopted; a reference pending reading, listed under To fetch.

## Handheld instruments and the FMCSA programme

Stalker's LidarCam 2 page states a "Following-Too-Close Mode" that "calculates the distance
between two moving vehicles in the same lane", with no accuracy or method given; NHTSA's
Conforming Product List of September 15, 2025 lists lidar devices from six manufacturers; the
performance specification those devices meet is DOT HS 809 811 (revised 2013), which a secondary
source summarises as distance accuracy of ±0.3 m at 90 m, **Confirm**. A sponsored article of
July 2026 says the Mississippi programme's effects remain anecdotal and no report exists. This
partly answers the prior briefing's open question: the instrument's mode is a distance between
two vehicles along the officer's beam, and the tolerance comes from a specification document
rather than the programme. Nothing changes in the behaviour plan's row; the specification is
listed under To fetch.

## Not adopted, one line each

- Lee and Kim 2025, PLoS One: 43% fewer pedestrian emergency patients in seven Korean cities one year after Safe-Speed-5030, with higher adjusted odds of ICU admission after; no speed or exposure term, overlapped by COVID-19, no control. A count change, not a rate.
- Chun, Abdel-Aty and Wang 2026, Transportation Research Part F 118:103542: visual distraction and speeding co-occur in naturalistic data; needs in-cabin observation, outside Tenet 1.
- IATSS Research 2026, young novice driver speed and headway at signalised intersections: in-vehicle naturalistic population; lower posted limits correlated with longer following distances; not a roadside population.
- arXiv 2602.09425, vehicle classification from roadside LiDAR with a vision-language model: a runtime model class the classifier plan's transparent tiers exclude.
- GSV2X (CVPR 2026) and CLIFE (arXiv 2607.16154): camera-LiDAR fusion, outside Tenet 1.
- Sensors 26(3):998, multi-RSU tracking under fog with a particle PHD filter: multi-sensor, unread.
- Expert Systems with Applications 262 (March 2025), interacting multiple model for low-channel roadside LiDAR: 98.8% detection, 97.4% association, 0.23 m position MAE, MOTA 75.5% and 73.6% on a custom set and V2X-Seq-SPD, all **Confirm**; unread.
- ISPRS Journal 2026 review of roadside LiDAR scene understanding: tracking-before-detection suits occlusion and fragmentation per its summary; unread, To fetch.
- Ouster's 2026 ITS trends post and Hesai's stationary-applications page: deployment counts and sensor specifications, no roadside measurement claim.
- Campolettano, Scanlon, Kusano and Victor, IRCOBI 2026 (IRC-26-75), a Safe System attribution of 37,654 FARS 2023 fatal cases: not this project's question; unread.
- Waymo Open Dataset April 2025 and WOD-E2E October 2025 updates: vehicle-side; no change to the motion set's use in Section 11.
- Vision Zero Network 2026 posts and city high-injury network refreshes: no methodology change.
- SWOV, ITF and FHWA: no new speed publication in the window; FHWA-HRT-24-175 (January 2025) and FHWA-HRT-25-007 on LiDAR and thermal detection of vulnerable road users at the Turner-Fairbank test bed are in window, unreachable, To fetch.
- NHTSA Countermeasures That Work: no edition in the window; the 11th stands.

## What changes

1. The behaviour plan's Section 11 gains rows for R-LiViT and HetroD, and Section 16 gains a
   "Roadside LiDAR perception" group (Schäfer et al., Fontalvo et al., Dong et al., Luo et al.,
   Baumann et al., Iglesias et al., Bang et al., Shang and Li, Cui et al., Ding et al., Shahbaz and
   Agarwal) and the two datasets in its trajectory group, each linked to this briefing.
2. The crash-data plan's Sources checked table gains rows for Teoh et al. 2026, Monfort and
   Mueller 2025, Ambros and Kieć 2025, Ambros et al. 2026, Campolettano et al. 2025, Kusano et al.
   2026, Chen et al. 2025, the Valgo white paper, the ERSO report and Lee and Kim 2025; the Valgo
   row says what is now read; two open questions are logged: the printed harm curve, and whether
   the before-and-after section prints a number under 50 km/h.
3. The attitudes plan's Sources checked table gains rows for the 2025 index, the NHTSA survey,
   the NHTSA fact sheet, Campolettano et al. 2025 and ITE's page; the IIHS row is corrected from
   "telephone survey" to the index; three open questions are logged: the index series in the
   edition, the observed shares as a comparator, and the percentile paragraph's citation.
4. The backlog's cited-harm item reads Monfort and Mueller 2025 beside Tefft 2013 and Rosén and
   Sander 2009.
5. Nothing is renamed, no benchmark kind changes, no threshold enters Section 5.1, and no code
   changes.

Proposals that need a plan's owner, recorded here and not applied: the grid-map extent estimator
comparison and the regularised rectangle fit for the convergence plan; the range-adaptive eps arm
for the sweep; L3 on CoopScenes' released labels; a research note on hourly volume profiles
against Inrix for a site's link class.

## Open questions

- Which pedestrian harm curve the safety-reference edition prints: Tefft 2013, Monfort and Mueller 2025, or both with their fleet years; and whether a report may use the per-class curves given that a class is what the sensor sees. Logged in the crash-data plan.
- Whether the before-and-after model section prints a predicted number at all on streets under 50 km/h, given Ambros and Kieć's overprediction finding, or only the direction with the interval and the finding stated. Logged in the crash-data plan.
- Whether the 2026 pooled 30 km/h effect enters the edition as an intervention effect, a different kind from a speed-change model, once read. Logged in the crash-data plan.
- Whether the annual Traffic Safety Culture Index enters the edition's attitudes entries beside the one-off attitudes study. Logged in the attitudes plan.
- Whether Campolettano et al.'s observed limit-relative shares enter the pace line as an `external_distribution` comparator with the mobile population stated. Logged in the attitudes plan.
- Whether the percentile paragraph cites ITE's position and the MUTCD edition on what p85 is for. Logged in the attitudes plan.
- Whether the convergence plan compares its extent belief against a grid-map, log-odds estimator on kirk0's references, and whether W1a's orientation fit is scored against a regularised rectangle fit. For the convergence plan's owner.
- Whether a range-adaptive eps is worth a sweep arm scored by W3. For the sweep's owner.
- The prior briefing's B10 and its instrument question stand; the instrument half is now a specification document to read.

## Decisions

Decided by the owner on October 10, 2026 and recorded as [D-28][d28].

| Question                                        | Decision                                                                                                                                                                                                     |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Which harm curve the edition prints             | Both, Tefft 2013 and Monfort and Mueller 2025, each with its fleet years and sample; the report's paragraph uses the all-vehicle curves; per-class curves stay in the docs until L6 class recall is accepted |
| The before-and-after model under 50 km/h        | Print the relative change with its interval as a model trend, with a one-sentence low-speed caveat citing Ambros and Kieć 2025; never a count, never suppressed                                              |
| The 2026 pooled 30 km/h effect                  | A reference now; an intervention-effect entry in the edition only after the paper is read, and off every page until the `published_model` kind is decided                                                    |
| The Traffic Safety Culture Index in the edition | Yes, as the series source for the speeding items, one entry per edition year, with the one-off attitudes study as the explanatory source                                                                     |
| Campolettano et al.'s observed shares           | Operator guide only, as context with the mobile population and the two cities stated; nothing on the report page                                                                                             |
| The percentile paragraph                        | One clause on the page, that p85 is a statistic of operating speeds and not a rule for setting limits; the ITE and MUTCD citations in the guide                                                              |
| The convergence plan comparison                 | Two `S` items: the centre-versus-extent error decomposition on kirk0's references, and a grid-map extent estimator as a shadow arm; the rectangle fit waits on reading                                       |
| A range-adaptive eps                            | One sweep arm scored by the W3 gate once it exists, kirk0's two trucks as the merging test; defaults unchanged; `S`                                                                                          |
| The prior briefing's questions                  | B10 deferred until B1 is measured; the instrument question open until the specification is read                                                                                                              |

## Sources checked

Status is one of Read, Read in part, Superseded or **Confirm**. A **Confirm** row's figures come
from search summaries or a secondary page and are not presented as read.

| Source                                                                     | What was read or found                                                                                                          | Status                   |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| [Teoh, Kidd and Riexinger 2026, IIHS preprint][iihs-pre]                   | Full 30-page PDF: abstract, method, Tables 2 to 6, discussion, Appendix A2                                                      | Read                     |
| [IIHS release, July 23, 2026][iihs-news]                                   | Headline figures, caveats, author                                                                                               | Read                     |
| [Monfort and Mueller 2025, JSR 94][monfort-iihs]                           | IIHS bibliography abstract; article unreachable                                                                                 | **Confirm**              |
| [IIHS release, December 10, 2024][monfort-news]                            | By-speed and by-vehicle figures, sample, data sources                                                                           | Read                     |
| [IIHS pedestrians page][iihs-ped]                                          | Monfort and Mueller figures; Hu et al. 2026 on front-end shape, no figures                                                      | Read                     |
| [Ambros and Kieć 2025, ETRR 17:59][ambros]                                 | Full PDF: data, Tables 3 to 6, discussion, limitations                                                                          | Read                     |
| [Ambros, Šípek, Křivánek and Valentová 2026, AAP 232:108547][ambros26]     | Pooled estimates and moderators from search summaries; article unreachable                                                      | **Confirm**              |
| [European Commission, ERSO speed report, November 2025][erso]              | Full 29-page PDF; a secondary synthesis, superseded on the 30 km/h effect by the 2026 meta-analysis                             | Read; Superseded in part |
| [Campolettano, Kusano and Victor 2025, TIP][campo]                         | Waymo research page summary; article unreachable                                                                                | **Confirm**              |
| [Kusano, Scanlon, McMurry and Victor 2026, TIP in press][kusano-tod]       | Full preprint PDF from Waymo's site                                                                                             | Read                     |
| [Chen, Scanlon, Kusano, McMurry and Victor 2025, TRR][chen]                | arXiv abstract with headline reweighting effects                                                                                | Read in part             |
| [Scanlon et al. 2025, From Stoplights to On-Ramps][stoplights]             | arXiv abstract: freeway and surface-street benchmarks; Atlanta 2.4 against Phoenix 0.7 any-injury freeway IPMM; CC BY-NC-SA 4.0 | Read in part             |
| [Kusano et al. 2025, crash type at 56.7 million miles][kusano-type]        | arXiv abstract; CC BY-NC-ND 4.0; the Valgo lineage's second paper                                                               | Read in part             |
| [Waymo safety impact hub][hub]                                             | Methodology page: benchmark construction, 32% correction, dynamic adjustment, 271.3 million miles, four files, no licence       | Read                     |
| [Waymo blog, October 8, 2026][waymo-oct]                                   | The sober paper's status and figures, already held; the IRCOBI link                                                             | Read                     |
| [Waymo blog, July 7, 2026][waymo-jul]                                      | Time and place effects; links to the two papers                                                                                 | Read                     |
| [Valgo white paper, June 2026][valgo-wp]                                   | Full 14-page PDF                                                                                                                | Read                     |
| [Valgo news, June 19, 2026][valgo-news]                                    | Announcement, specification-curve example, data sources, no licence                                                             | Read                     |
| [humanbaselines on PyPI][valgo-pypi]                                       | Release history 0.1.0 to 0.4.0 via the JSON index                                                                               | Read                     |
| [AAA Foundation, 2025 Traffic Safety Culture Index][tsci]                  | Front matter, rights, method, Tables 2 and 5, neighbourhood comparison                                                          | Read in part             |
| [NHTSA, 2022-2023 NSSAB, DOT HS 813 594][nssab]                            | ROSA P record: authors, date, sample, mode; report blocked                                                                      | **Confirm**              |
| [NHTSA, Speeding: 2024 Data, DOT HS 813 823][nhtsa-sp]                     | Full fact sheet                                                                                                                 | Read                     |
| Nationwide Agency Forward survey, January 2026                             | Press coverage only                                                                                                             | **Confirm**; not adopted |
| [ITE, Setting Speed Limits][ite]                                           | Position statements, MUTCD edition, guidance list                                                                               | Read                     |
| [Caltrans, Setting Speed Limits][caltrans]                                 | Method, rounding, 5 mph reduction, manual update target                                                                         | Read                     |
| NCHRP 17-133                                                               | Project listing from search summaries                                                                                           | **Confirm**              |
| [Feng, Park and Mondschein 2026, arXiv][feng]                              | HTML full text                                                                                                                  | Read                     |
| [Schäfer, Alrifaee and Hashemi 2025, arXiv preprint][schafer]              | Abstract, Section 5.6, Section 6 results; Wiley version of record unreachable                                                   | Read in part             |
| [Fontalvo et al. 2026, Sensors][fontalvo]                                  | Full text via PMC                                                                                                               | Read                     |
| [Dong et al. 2026, Sensors][dong]                                          | Full text via PMC                                                                                                               | Read                     |
| [Luo et al. 2025, Sensors][luo]                                            | Full text via PMC                                                                                                               | Read                     |
| [Baumann, Vosshans and Dang 2026, arXiv][baumann]                          | HTML full text: models, sensors, splits, results table, runtime, release page                                                   | Read                     |
| [Iglesias et al. 2025, arXiv][iglesias]                                    | Abstract and journal reference                                                                                                  | Read in part             |
| [Bang et al. 2026, PRISA, arXiv][prisa]                                    | HTML full text: sensors, thresholds and their justification, R-LiViT errors, deployment                                         | Read                     |
| [Shang and Li 2026, arXiv][shang]                                          | Abstract                                                                                                                        | Read in part             |
| [Li, Shang, Feng, Wei and Kamga 2026, ROSA P][ccny]                        | Record page: beam-loss finding, dataset size, pipeline                                                                          | Read in part             |
| [Mirlach et al. 2025, R-LiViT, arXiv][rlivit]                              | Abstract; counts; licence of the paper CC0, dataset terms unstated                                                              | Read in part             |
| [Chen et al. 2026, HetroD, arXiv][hetrod] and [levelXdata page][hetrod-lx] | Abstract and page: counts, classes, terms                                                                                       | Read in part             |
| [Cui et al. 2026, MR-LiDAR, arXiv][mrlidar]                                | Abstract                                                                                                                        | Read in part             |
| [Ding et al. 2026, RESOLVE, arXiv][resolve]                                | Abstract and repository link                                                                                                    | Read in part             |
| [Shahbaz and Agarwal 2026, UrbanTwin, arXiv][urbantwin]                    | Abstract, journal reference, Dataverse link                                                                                     | Read in part             |
| [Applied Sciences 16(5):2513, rectangle edge matching][rect]               | Search summaries; publisher page blocked twice                                                                                  | **Confirm**              |
| [Stalker LidarCam 2][stalker]                                              | Mode description, no specification                                                                                              | Read                     |
| [NHTSA Conforming Product List, September 15, 2025][cpl]                   | Manufacturer list from search summaries; the LTI 20/20 TruVISION's listing unverified                                           | **Confirm**              |
| NHTSA LIDAR performance specification, DOT HS 809 811                      | Named by a 2016 NHTSA guide per search summaries; the ±0.3 m at 90 m figure is from a tertiary page                             | **Confirm**              |
| [Laser Technology sponsored article, July 2026][lti-july]                  | Programme effects anecdotal, no report                                                                                          | **Confirm**              |
| [Lee and Kim 2025, PLoS One][leekim]                                       | Full text via PMC                                                                                                               | Read; not adopted        |
| [Chun, Abdel-Aty and Wang 2026, TR-F][chun]                                | Search summary                                                                                                                  | **Confirm**; not adopted |
| [ISPRS Journal 2026 review][isprs]                                         | Search summary; publisher blocked                                                                                               | **Confirm**; To fetch    |
| [FHWA-HRT-25-007][fhwa25] and [FHWA-HRT-24-175][fhwa24]                    | Search summaries; downloads returned HTML                                                                                       | **Confirm**; To fetch    |
| ESWA 262 (2025), IMM for low-channel roadside LiDAR                        | Search summary                                                                                                                  | **Confirm**; not adopted |
| [Yannis and Michelaraki 2024, Sustainability][yannis24]                    | Figures as repeated by ERSO and search summaries                                                                                | **Confirm**; Superseded  |

### To fetch

Primaries the review environment could not reach, with the URL to attach to a follow-up session.
An attached PDF or Markdown extraction is read in full.

- Monfort and Mueller 2025: <https://doi.org/10.1016/j.jsr.2025.06.030>
- Ambros et al. 2026: <https://www.sciencedirect.com/science/article/pii/S0001457526001569>
- Campolettano, Kusano and Victor 2025: <https://www.tandfonline.com/doi/full/10.1080/15389588.2025.2538726>
- Schäfer et al. 2026, version of record: <https://advanced.onlinelibrary.wiley.com/doi/10.1002/aisy.202501102>
- Applied Sciences 16(5):2513: <https://www.mdpi.com/2076-3417/16/5/2513>
- ISPRS Journal review: <https://www.sciencedirect.com/science/article/pii/S0924271626000122>
- FHWA-HRT-25-007: <https://highways.dot.gov/sites/fhwa.dot.gov/files/FHWA-HRT-25-007.pdf>
- FHWA-HRT-24-175: <https://highways.dot.gov/media/58566>
- NHTSA NSSAB 2022-2023: <https://rosap.ntl.bts.gov/view/dot/78928/dot_78928_DS1.pdf>
- NHTSA DOT HS 809 811, LIDAR speed-measuring device performance specifications, and the September 2025 Conforming Product List: <https://www.nhtsa.gov/document/conforming-product-list-cpl-speed-measuring-devices>
- MUTCD 11th Edition Revision 1 (December 2025) and the 2026 California Manual for Setting Speed Limits
- Waymo safety impact hub benchmark files and release notes: <https://waymo.com/safety/impact/>
- Scanlon et al. 2026 SAE Int. J. Transp. Safety 14(2) published version: <https://doi.org/10.4271/09-14-02-0003>
- Folla et al. 2025, observed speed compliance in 20 European countries, as cited by ERSO; no URL found
- MR-LiDAR's data access statement: <https://arxiv.org/pdf/2605.24777>
- Sensors 26(3):998, multi-RSU tracking under fog: <https://www.mdpi.com/1424-8220/26/3/998>

## Provenance

- Window: January 1, 2025 to October 10, 2026, invoked as `/research-briefing --since 2025-01-01`.
- Searches: 72 `WebSearch` queries in extended mode, all run on October 10, 2026 (UTC), listed below in the order run. A source found by more than one route counts once.
- Reads: `WebFetch` for pages and arXiv; `curl` into an empty scratch directory and `pdftotext` for the IIHS preprint, the Valgo white paper, the ERSO report, Ambros and Kieć, the Schäfer preprint, the 2025 index and the NHTSA fact sheet; the PyPI JSON index for release dates. Downloads were treated as untrusted and read as text only.
- Unreachable from the review environment: ScienceDirect, Taylor and Francis, Wiley, MDPI, PubMed, Springer Link, ROSA P and highways.dot.gov PDF downloads, Stalker's main site. aaafoundation.org, waymo.com, iihs.org, arxiv.org, valgo.ai, pmc.ncbi.nlm.nih.gov and levelxdata.com were reachable.
- Repository revision `a7df7cf`: the behaviour plan's Sections 5, 9.2, 11, 13 and 16; the crash-data plan's Findings, Open questions and Sources checked; the attitudes plan's Items 2 and 3a, Open questions and Sources checked; the posted-limits plan; the human crash baselines analysis; the headway wording guard; L3's closeness thresholds; L4's DBSCAN clusterer and its oriented-box estimator; the geometry convergence plan.
- Queries, by route. Organisation watchlist: "Waymo safety research publications 2026 human baseline crash rate"; "Valgo humanbaselines.com crash baselines 2026"; "AAA Foundation for Traffic Safety research reports 2026 speeding"; "NHTSA Countermeasures That Work 2025 edition speeding"; "SWOV 2026 publication speed road safety fact sheet update"; "ITF OECD 2026 report speed management road safety"; "FHWA safety research 2026 roadside LiDAR intersection safety report"; "NHTSA research 2026 speeding report roadside speed measurement"; "Hesai Ouster application note roadside LiDAR measurement accuracy traffic 2026"; "SafeTREC 2026 research report speed LiDAR pedestrian"; "Vision Zero Network high injury network methodology update 2026"; "TRID ROSAP 2026 roadside LiDAR trajectory report"; "IIHS 2026 research speed limits pedestrian deaths vehicle front-end height study"; "FMCSA Mississippi LiDAR headway following too closely project update 2026 results report"; "NHTSA conforming products list lidar speed measuring devices specification distance accuracy following too close mode".
- Journal and venue watchlist: "Transportation Research Part C 2026 roadside LiDAR vehicle tracking occlusion fragmentation"; "IEEE Transactions on Intelligent Transportation Systems 2026 infrastructure LiDAR multi-object tracking"; "Accident Analysis and Prevention 2026 time headway following distance distribution roadside sensor"; "SAE technical paper 2026 crash rate benchmark automated driving human"; "SSRN preprint 2026 speeding crash risk speed limit"; "CVPR ICRA IROS 2026 roadside LiDAR perception paper"; "Transportation Research Part F 2026 speeding behaviour roadside observed"; "Traffic Injury Prevention 2026 speed pedestrian injury vehicle front-end"; "IEEE ITSC 2026 IV 2026 roadside LiDAR tracking paper accepted"; "Journal of Safety Research 2026 speed pedestrian headway following published"; "arXiv eess.SP 2026 roadside LiDAR background model foreground extraction"; "roadside LiDAR tracking preprint arXiv 2026 occlusion identity switch roadside multi-object".
- Topic queries, year and preprint forms: "roadside LiDAR tracking 2026"; "roadside LiDAR tracking 2025 journal vehicle pedestrian trajectory extraction accuracy evaluation"; "infrastructure LiDAR vehicle extent estimation 2025"; "infrastructure LiDAR vehicle extent estimation preprint bounding box length width sparse roadside 2026"; "LiDAR headway measurement 2025"; "LiDAR headway measurement 2026 roadside vehicle gap time headway extraction"; "LiDAR headway measurement preprint roadside time gap vehicles"; "following distance enforcement LiDAR 2026"; "following distance enforcement LiDAR preprint time gap handheld validation accuracy study"; "crash involvement rate per vehicle mile baseline 2026 preprint"; "crash involvement rate per vehicle mile baseline 2025 human driver benchmark police-reported"; "sober driving baseline human crash rate ADS comparison 2026"; "sober driving baseline preprint alcohol-free crash rate reconstruction"; "human crash baseline ADS preprint 2026 arXiv benchmark retrospective evaluation"; "speeding attitudes national survey 2025 preprint"; "speeding attitudes national survey 2026"; "aggressive driving survey 2026 national"; "aggressive driving survey preprint 2025 2026 self-report prevalence questionnaire"; "impact speed pedestrian fatality curve 2025"; "impact speed pedestrian fatality curve preprint 2026"; "speed change crash model 2025 power model exponential update Elvik"; "speed change crash model preprint 2026 speed limit reduction crash effect meta-analysis"; "85th percentile speed practice 2026 speed limit setting"; "85th percentile speed practice preprint 2025 operating speed target speed research"; "85th percentile speed practice 2025 speed limit setting research report"; "post-encroachment time threshold 2025 surrogate safety measure LiDAR intersection"; "post-encroachment time threshold 2026 LiDAR roadside conflict validation crash correlation"; "stop sign compliance LiDAR 2025"; "stop sign compliance rolling stop roadside sensor study 2026 preprint"; "stop sign compliance LiDAR 2026 stop-controlled intersection roadside sensor rolling stop measurement"; "free-flow speed speeding exposure opportunity naturalistic 2026 roadside"; "new roadside infrastructure LiDAR trajectory dataset 2025 2026 release vehicles pedestrians cyclists"; "highD inD rounD exiD uniD levelXdata dataset 2025 2026 new release"; "Waymo Open Motion Dataset 2025 2026 update release".
- Follow-up queries on a found source: "IIHS study Waymo driverless crashes 68% fewer per vehicle mile July 2026"; "Monfort Mueller 2025 modern injury risk curve pedestrian impact speed front-end height Journal of Safety Research"; "Katz Delecki Qian Moss human crash baselines white paper specification curve analysis"; "R-LiViT roadside LiDAR visual thermal dataset Mirlach 2025 arXiv"; "Zhang Steinbach 2025 speeding national survey drivers unacceptable yet engage"; "Chen Kusano Scanlon 2025 Transportation Research Record human crash benchmark Kusano 2025 Traffic Injury Prevention"; "hetroD dataset arXiv ICRA 2026 levelXdata Taiwan intersections drone trajectories paper"; "\"Potential safety benefits associated with speed limit compliance in San Francisco and Phoenix\" arXiv preprint authors"; "Yannis Michelaraki 2025 effectiveness of 30 km/h speed limit meta-analysis city-wide fatalities"; "IRCOBI 2026 paper 2675 Waymo FARS 2023 fatal crash risk time of day geography sober"; "\"Exploring the heterogeneity in impacts of 30 km/h speed interventions\" subgroup meta-analysis Accident Analysis and Prevention 2026 authors pooled estimate".

[iihs-pre]: https://www.iihs.org/api/datastoredocument/bibliography/2374
[iihs-news]: https://www.iihs.org/news/detail/waymos-driverless-cars-crash-less-often-than-people
[monfort-iihs]: https://www.iihs.org/research-areas/bibliography/ref/2322
[monfort-news]: https://www.iihs.org/news/detail/vehicle-height-compounds-dangers-of-speed-for-pedestrians
[iihs-ped]: https://www.iihs.org/research-areas/pedestrians-and-bicyclists
[ambros]: https://doi.org/10.1186/s12544-025-00753-6
[ambros26]: https://www.sciencedirect.com/science/article/pii/S0001457526001569
[erso]: https://road-safety.transport.ec.europa.eu/document/download/9826c063-bc55-423e-84a3-24200dca3547_en?filename=ERSO-TR-speed_2026.pdf
[campo]: https://waymo.com/research/potential-safety-benefits-associated-with-speed-limit-compliance-in-san/
[kusano-tod]: https://storage.googleapis.com/waymo-uploads/files/documents/safety/Kusano%202026%20-%20Effects%20of%20Day%20of%20Week%20and%20Time%20of%20Day%20on%20Benchmark%20Crash%20Risk.pdf
[chen]: https://arxiv.org/abs/2410.08903
[stoplights]: https://arxiv.org/abs/2508.19425
[kusano-type]: https://arxiv.org/abs/2505.01515
[hub]: https://waymo.com/safety/impact/
[waymo-oct]: https://waymo.com/blog/2026/10/sober-driving-benchmarks/
[waymo-jul]: https://waymo.com/blog/2026/07/time-geo-crash-risk-effect/
[valgo-wp]: https://valgo.ai/pdf/valgo-humanbaselines-2026.pdf
[valgo-news]: https://valgo.ai/news/humanbaselines/
[valgo-pypi]: https://pypi.org/project/humanbaselines/
[tsci]: https://aaafoundation.org/wp-content/uploads/2026/05/202606-AAAFTS-2025-TSCI.pdf
[nssab]: https://rosap.ntl.bts.gov/view/dot/78928
[nhtsa-sp]: https://crashstats.nhtsa.dot.gov/Api/Public/ViewPublication/813823
[ite]: https://www.ite.org/technical-resources/topics/speed-management-for-safety/setting-speed-limits/
[caltrans]: https://dot.ca.gov/programs/safety-programs/setting-speed-limits
[feng]: https://arxiv.org/abs/2601.10974
[schafer]: https://arxiv.org/abs/2509.20009
[fontalvo]: https://pmc.ncbi.nlm.nih.gov/articles/PMC13363753/
[dong]: https://pmc.ncbi.nlm.nih.gov/articles/PMC13307123/
[luo]: https://pmc.ncbi.nlm.nih.gov/articles/PMC11859876/
[baumann]: https://arxiv.org/abs/2608.14868
[iglesias]: https://arxiv.org/abs/2510.22390
[prisa]: https://arxiv.org/abs/2607.16156
[shang]: https://arxiv.org/abs/2604.10419
[ccny]: https://rosap.ntl.bts.gov/view/dot/91974
[rlivit]: https://arxiv.org/abs/2503.17122
[hetrod]: https://arxiv.org/abs/2602.03447
[hetrod-lx]: https://levelxdata.com/hetrod-dataset/
[mrlidar]: https://arxiv.org/abs/2605.24777
[resolve]: https://arxiv.org/abs/2606.31895
[urbantwin]: https://arxiv.org/abs/2509.06781
[rect]: https://doi.org/10.3390/app16052513
[stalker]: https://stalkerradar.com/lidarcam-2
[cpl]: https://www.nhtsa.gov/document/conforming-product-list-cpl-speed-measuring-devices
[lti-july]: https://www.officer.com/sponsored/article/55390832/from-observation-to-evidence-modernizing-tailgating-enforcement
[leekim]: https://pmc.ncbi.nlm.nih.gov/articles/PMC12157059/
[chun]: https://www.sciencedirect.com/science/article/abs/pii/S1369847826000379
[isprs]: https://www.sciencedirect.com/science/article/pii/S0924271626000122
[fhwa25]: https://highways.dot.gov/sites/fhwa.dot.gov/files/FHWA-HRT-25-007.pdf
[fhwa24]: https://highways.dot.gov/media/58566
[yannis24]: https://www.nrso.ntua.gr/geyannis/pub/pj251-review-of-city-wide-30-km-h-speed-limit-benefits-in-europe/
[d28]: ../../../DECISIONS.md#d-28--research-briefing-decisions-october-2026
