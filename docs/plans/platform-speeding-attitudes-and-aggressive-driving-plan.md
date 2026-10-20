# Public attitudes to speeding and aggressive driving in reports plan

How velocity.report should use two AAA Foundation for Traffic Safety studies, _Attitudes Towards
Speeding_ (2026) and _Aggressive Driving and Road Rage_ (2025): which of their findings a speed
report can cite, which of their terms the project can adopt, which behaviours the sensor can and
cannot observe, and where the two vocabularies collide. The studies measure what people say and
believe; the sensor measures what passes. The plan keeps the two apart on every page and uses each
to explain the other in the docs.

Both report summaries were read in full. The attitudes technical report was read in full on
October 12, 2026, from the AAA Foundation's PDF (196 pages), including the attitudinal-group
profiles, the Discussion, Figure 16 and Appendix D, and every figure this plan takes from it was
checked against that text; its state-level estimates were read for their method and for California.
The aggressive-driving report was read from a text extraction the owner supplied: its front matter,
definitions and frameworks only. A figure no read source carries is marked **Confirm**.

- **Status:** Parked (D-32, October 20, 2026): its rows are in v0.6.9 (safety reporting) and later; revisit when v0.6.9 is next. Was: Proposed; no code written
- **Layers:** Cross-cutting (PDF report copy and `data.json`, wording guard, safety-reference edition, L8 behaviour benchmarks, documentation)
- **Target:** v0.5.10 for the shared wording guard, the percentile copy, the pace-of-traffic line, the limit-relative shares against the operator-supplied limit, and the attitudes entries in the safety-reference edition; v1.0 for shares per limit segment and schedule; research notes deferred
- **Companion plans:** [crash data and published risk models](platform-crash-data-integration-plan.md), [behaviour analytics](lidar-behaviour-analytics-plan.md), [posted speed limits](posted-speed-limits-plan.md), [traffic description language](data-traffic-description-language-plan.md), [platform vocabulary](platform-vocabulary-and-data-model-plan.md)
- **Related:** [human crash baselines analysis](../platform/operations/human-crash-baselines-analysis-2026-10.md), [percentile aggregation semantics](../radar/architecture/percentile-aggregation-semantics.md), [identifiability analysis](../platform/architecture/identifiability-analysis.md), [editorial role](../../.github/knowledge/role-editorial.md), [research briefing, January 2025 to October 2026](../lidar/operations/brief/research-briefing-2025-01-to-2026-10.md), [TENETS](../../TENETS.md)
- **Canonical:** [PDF reporting](../platform/operations/pdf-reporting.md)

## Motivation

A speed report prints p85 against a limit and leaves the reader to decide what the gap means. The
attitudes study has now described how the American public decides: its focus groups converged on a
social threshold for speeding about 10 mph over the posted limit, its questionnaire put the
threshold at 8 to 10 mph over by road type and the expected start of enforcement at 10 to 12, and
49% of the people asked agreed that driving with the flow of traffic is safer for them than obeying
the limit. On those figures, a report that prints p85 of 33 mph in a 25 mph zone will read as
normal to many of its readers, as evidence the limit is wrong to some (a fifth would set limits at
least 10% above what they expect), and to a traffic engineer as the number that sets limits. The
report can meet all three where they stand: print the pace of traffic beside the limit, print the
share of passages above the threshold the public itself uses, and cite where that threshold comes
from.

The aggressive-driving study defines the behaviours the public calls aggressive, grouped into
seven themes. Most of them are gestures, sounds and confrontations a radar or LiDAR cannot see, by
design. Two of them, driving well above the flow of traffic and following too closely, are what the
sensor measures or plans to measure, and the study gives the public's own words for them. The
project can use those words for what it observes without adopting the study's verdicts, because
its wording guard already refuses every verdict word the study is built on.

The consequence of inaction is small and specific: the percentile section keeps calling p98 a view
into "top speeders" and "high-risk driving patterns", two labels on people the sensor never sees;
"survey" keeps meaning a deployment in this repository and a questionnaire in the sources it will
cite; and the one finding that answers the most common objection to a speed report, that everyone
drives with the flow, stays uncited while the flow itself goes unprinted.

## Current state

- **Report copy.** `aggregation-and-percentiles()` in
  [sections.typ](../../internal/report/typst/templates/sections.typ) says p98 gives "a more robust
  view into trends among top speeders" and "clearer insight into high-risk driving patterns". The
  crash-data plan replaces the kinetic-energy paragraph beside it and leaves this one.
- **Speed limit.** A unitless per-request integer, `ReportConfig.SpeedLimit`, default 25, printed
  beside a free-text note. The [posted speed limits plan](posted-speed-limits-plan.md) returns a
  signed limit per road segment at v1.0.
- **Percentiles.** p50, p85 and p98 over per-transit maximum speeds, per
  [percentile aggregation semantics](../radar/architecture/percentile-aggregation-semantics.md).
  Nothing is printed relative to the limit, and nothing is printed relative to the traffic around
  a passage.
- **Direction.** The transit worker reads `ABS(speed)`, so a radar transit has no direction. A
  pace of same-direction traffic is a LiDAR quantity until that changes.
- **Wording guard.** `headway.VerdictPattern` in
  [wording.go](../../internal/report/headway/wording.go) refuses twelve stems: `tailgat`,
  `aggress`, `driver`, `risk`, `score`, `verdict`, `unsafe`, `danger`, `violat`, `offend`,
  `propensity`, `profil`. The API tests in `internal/api/server_scenes_headway_test.go` keep a
  second list of eight (`tailgat|risk|driver|score|unsafe|danger|aggressi|verdict`). Neither
  contains `speeder`.
- **Behaviour plan.** [Section 8.1](lidar-behaviour-analytics-plan.md#81-speed-behaviour)
  specifies `speed_over_limit`, `speeding_exposure_{5,10,15}` and `speed_relative_to_stream`, the
  last defined as speed less the median speed of contemporaneous same-direction traffic. None is
  implemented. Following gap and net time gap (8.3) are built with descriptive bands at 2.0, 1.5
  and 1.0 s, kind `no_established_threshold`. Lane changes and cut-ins (8.8), stop behaviour (8.11)
  and induced evasive response (8.12) are deferred.
- **Traffic description language.** The [TDL plan](data-traffic-description-language-plan.md)
  defines the token `speeding` as `speed.max_mph > posted_limit` and gives the example
  "percentage of vehicles speeding on weekdays".
- **Vocabulary.** V12 in the [vocabulary plan](platform-vocabulary-and-data-model-plan.md) defines
  a **survey** as one deployment's measurement over a period. Both studies call their
  questionnaire a survey.
- **Editorial stance.** The [editorial role](../../.github/knowledge/role-editorial.md) says the
  product is "a tool for community empowerment, not for enforcement or surveillance". Terry's
  brief names "a resident worried about speeding traffic" as a reader never to mock.
- **Temporal strata.** The crash-data plan's Item 2a prints counts and percentiles in the day,
  night, weekday and weekend cells. The shares this plan adds fit the same table.
- **References.** `data/maths/references.bib` has no AAA Foundation entry. The crash-data plan
  cites one AAA Foundation paper, Tefft 2013, for the impact-speed curve.

## Findings

### The two studies

| Study                                                                                                                                                    | Method                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | What it measures                                                                                                                                                                                                        | What it is not                                                                                   |
| -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Steinbach, McDonough, Hungund, Kasha and Bai, _Attitudes Toward Speeding: Insights from Focus Groups and a National Survey of U.S. Drivers_, August 2026 | A literature review of 102 papers; eight online focus groups with 58 people from 30 states, February to March 2025, judging point-of-view video vignettes; a questionnaire to 16,598 drivers: 11,560 from NORC's probability-based AmeriSpeak panel, aged 16 and over and screened for having driven in the past 30 days, and 5,038 aged 18 and over from Dynata's non-probability panel for state estimates, fielded December 8, 2025 to January 12, 2026, weighted by NORC's TrueNorth method to a margin of error of ±1.08% with a design effect of 2.0; latent class analysis over seven attitude dimensions | How people define speeding by road type, which limits they expect and would set, how they rank its risk against other behaviours, why they speed and refrain, how they justify it, and what they support doing about it | A measurement of any speed. Part 4, a driving-simulator study, is a separate forthcoming report  |
| Steinbach, Kasha, Svancara and Parker, _Aggressive Driving and Road Rage_, September 2025                                                                | A literature review of 620 articles from 2013 to 2023 and a five-member expert panel; eight focus groups with 53 people who admitted to aggressive driving or road rage; a questionnaire to a probability-based panel of 3,020 aged 16 and over, per the summary (the extraction lacks the method section)                                                                                                                                                                                                                                                                                                       | Which behaviours people call aggressive, how often they report doing them, and what predicts doing them                                                                                                                 | An observation of any behaviour. Every prevalence figure is self-reported over the previous year |

### Attitudes towards speeding: what the report states

From the summary and the technical report's text; figure and table numbers are the report's own.

**How speeding is defined.** Shown a road with its limit and asked how fast a vehicle must go to be
speeding, respondents' means were 33 mph on a 25 mph residential street, 45 on a 35 mph arterial,
80 on a 70 mph rural highway and 78 on a 70 mph urban highway (Figure 4). The means at which they
expected police to act were 35, 47, 81 and 80, and the means they called dangerous were 44, 56,
88 and 85. The cumulative curve matters more than the means (Figure 5): on the residential street,
40% call 30 mph speeding, 85% call 35 mph speeding, and nearly all call 45 mph speeding; on the
arterial, about a quarter call 40 speeding and 72% call 45 speeding; on the highways, 34% call 75
speeding and 94% call 85 (rural) or 80 (urban) speeding. The report notes that 35 in a 25 "is often
used by road safety professionals as a reference threshold", and calls 85 on a 70 mph rural
highway a "commonly used threshold reference"; it measures no speeds and never mentions the 85th
percentile. For its own classification of respondents it counts how often each travels 10 mph or
more over the limit on residential roads and arterials and 15 mph over on highways, the limit
being the one they expect for the pictured road, asked before any posted limit is shown (Figure
8's method). On the arterial picture that expectation averaged 45.8 mph against a posted 35.

**Speed limit credibility.** Before seeing a limit, respondents expected 25.8 mph on the
residential street and would set 26.2; on the arterial, 45.8 and 47.9; the gap was largest on
rural highways, 4 mph (Figure 6). Those who would set a limit at least 10% above what they expected
were 24% on residential roads, 33% on arterials, 28% on rural and 21% on urban highways. The focus
groups found limits generally credible; one participant summed up "25 miles an hour means 30" and
another called 5 over "like nothing".

**Technical and acceptable speeding.** Every focus group separated technical speeding, any speed
above the posted limit even by 1 mph, from acceptable speeding, set by situational, environmental
and social conditions. The most common upper bound of acceptable was 10 mph over, read as an
unofficial margin of tolerance that police themselves drive within, though some placed police
tolerance lower ("they'll give you a 7, 8, 9 [MPH] over", page 32); above 10 over, speed was
acceptable when passing a large or unsafe vehicle, in emergencies, with no other vehicles on the
road, or "when there's nobody watching", and some cited up to 25 over by road, weather and time of
day. Judging the vignettes, which hid the speedometer but often showed a limit sign, participants
mentioned the speed of other vehicles "by far" the most, then external markers, following distance,
lane keeping, curvature, grade and visibility, weather, and the presence of pedestrians, cyclists
and crosswalks.

**Risk.** 74% said they often or always see vehicles much faster than the flow on highways and 45%
see residential speeding that often (Table 9); the two items use different references, the traffic
on the highway and the limit on the residential street. 55% were very or extremely concerned by
residential speeding, below impaired, distracted and drowsy driving; 56% thought a residential
speeder unlikely to be stopped. By the report's risk score, 16% rated residential speeding more
dangerous than the other behaviours and 9% less; for highway speeding the figures were 6% and 24%
(Figure 7). The questionnaire's own aggressive-driving item, a car "following you very closely"
that "switches lanes and speeds by you", is seen often or always by 53% and concerns 40% very or
extremely (Table 9).

**Behaviour and situations.** 21% reported never speeding on any road type, 34% rarely, 30%
sometimes and 15% frequently (Figure 8). People were more likely to speed with no traffic (52%),
on a long trip (45%) and on a familiar road (45%), and less likely in rain (84%), on an unfamiliar
road (76%) and in the dark (73%) (Figure 9).

**Motivations.** To speed, often or always: getting around slow vehicles (31%), reaching the
destination faster (25%), not holding up traffic (21%), not annoying others (15%), the way the
road is built (13%), the car's design (6%) (Figure 10). To refrain: a ticket (79%), a crash (72%),
bad weather (60%), too much traffic (46%), insurance premiums (35%), travel cost (8%) (Figure 11).
13% always stop themselves exceeding the limit, 34% often, 39% sometimes, 10% rarely, 4% never.

**Moral disengagement** (Table 10, a bespoke scale from the focus groups). Agreement: "if people
want to drive the speed limit, they shouldn't be in the left lane" 59%; "it's safer for me to
drive with the flow of traffic than obey the speed limit" 49%, mean 3.3 of 5; "going a little bit
over the speed limit isn't really speeding" 49%; "no one actually drives the speed limit" 30%;
"it's okay to go over the speed limit because speed limits were set with the expectation that
people will drive faster than them" 11%, with 56% disagreeing. 10% scored low on the scale, 47%
moderate and 43% high (the text; Table 13 prints 10%, 46% and 44%).

**Countermeasures** (Table 11, affect own speed, affect others, support in own neighbourhood, where
crashes have happened, where they could): increased police presence 85, 86, 79, 87, 88; automated
cameras 80, 73, 59, 71, 73; electronic warning signs 74, 63, 41, 43, 44; speed humps 91, 86, 37,
43, 43; rumble strips 62, 54, 35, 40, 40; roundabouts 82, 78, 35, 39, 40. 62% thought police
presence exists to make roads safer and 33% to raise revenue; for cameras 48% and 46%. 87% take
some action to avoid enforcement and 42% warn others; the most common action is adjusting speed to
match the surrounding vehicles (58%), then cruise control (43%), apps (27%), making sure "there's
always another vehicle going faster" (21%), routes that avoid cameras (16%) and a radar detector
(8%) (Figures 12 and 13). In the focus groups, feedback signs drew a mostly apathetic response and
doubts about the displayed speed, and one supporter valued that "it's on the record"; cameras drew
"Big Brother" and a broader mistrust of the technology (pages 39 and 40).

**Intelligent speed assistance** (Table 12): an advisory alert would affect 58% of respondents'
own speed and 51% want one in their next car; an overridable limiter 62% and 35%; a
non-overridable limiter 61% and 23%. 40% want an overridable limiter in other people's cars but
not their own (Figure 15), most of all for people with a record of impaired driving or serious
speeding offences (Figure 14).

**Mindsets.** Five latent classes over seven dimensions, each with its own speeding threshold:
Strict Compliance Advocates (21%, low thresholds), Safety-Focused Realists (14%, high thresholds),
Principled Pragmatists (26%, moderate or high), Tolerant but Non-Compliant (23%, high) and
Autonomy-First (16%, high). The report concludes that one-size-fits-all speed management is
unlikely to be optimal. The profiles, Table 13 and Appendix D are under
[the five mindsets](#the-five-mindsets-what-the-report-states).

**Literature the report cites.** NHTSA's 2022 to 2023 national survey: only about a third think
limits should always be enforced by police, 63% think more cameras a good idea; Peterson and
Gaugler 2021: up to about 10 mph over is seen as typical and safe; NHTSA's 2013 latent classes:
non-speeders 39%, sometimes 44%, speeders 17%. The report observes that definitions of speeding in
the literature are "threshold based or derived from sensor-based driving data" and that virtually
no study asks what people themselves count as speeding.

**Limitations the report states.** A non-probability supplement for state estimates; self-report,
with potential under-reporting; no coverage of people who read neither English nor Spanish; and
highway images that may have pulled the expected and preferred highway limits below the real ones.

### The five mindsets: what the report states

From the attitudinal-group section (pages 72 to 92), Table 13 and Appendix D, read in full on
October 12, 2026.

**How the groups were made.** The latent class analysis took fifteen indicators in seven
dimensions (Table D1): the relative risk of highway and of residential speeding; limit
credibility on four road types, from whether the limit a respondent would set is at least 10%
below or above the one they expect; support for infrastructure and enforcement countermeasures
where crashes have happened; support for an overridable and a non-overridable speed limiter, in
their own car, only in others', or in none; the speeding threshold on four road types; enforcement
avoidance; and the mean of the thirteen moral-disengagement items. A threshold is the speed a
respondent calls speeding as a share of the limit: low is within 10% over, medium 10% to 20%, high
more than 20%. On a 25 mph street a high threshold means nothing at or below 30 mph is speeding;
the plan's 5 mph edge on that street is the report's 20% boundary. The information criteria kept
falling past five classes; five were chosen over four and seven for interpretability (Table D2),
and moral disengagement and speed assistance discriminated most (page 86). Each respondent was
assigned to their most likely class. The names and mottos are the authors' readings of response
patterns, not respondents' words.

| Indicator (source)                                                | Strict Compliance Advocates                      | Safety-Focused Realists                           | Principled Pragmatists                          | Tolerant but Non-Compliant                                            | Autonomy-First                                               | All drivers         |
| ----------------------------------------------------------------- | ------------------------------------------------ | ------------------------------------------------- | ----------------------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------ | ------------------- |
| Share of drivers                                                  | 21%                                              | 14%                                               | 26%                                             | 23%                                                                   | 16%                                                          | 100%                |
| Motto (Executive Summary)                                         | "Driving safely is part of being a good person." | "The risks of speeding aren't worth the rewards." | "Rules matter, but context matters too."        | "Everyone speeds, and it's usually not a big deal."                   | "The driver—not the system—should decide how fast to drive." |                     |
| Speed sometimes or frequently; frequently (D4)                    | 33%; 11%                                         | 33%; 9%                                           | 26%; 2%                                         | 75%; 32%                                                              | 55%; 19%                                                     | 45%; 15% (Figure 8) |
| Low threshold on residential streets; high on urban highways (13) | 40%; 1%                                          | 11%; 20%                                          | 3%; 2%                                          | 4%; 47%                                                               | 4%; 28%                                                      | 12%; 19%            |
| Residential limit should be raised (13)                           | 19%                                              | 13%                                               | 15%                                             | 40%                                                                   | 29%                                                          | 24%                 |
| High support for traditional countermeasures (13)                 | 59%                                              | 82%                                               | 63%                                             | 44%                                                                   | 24%                                                          | 54%                 |
| Overridable limiter wanted in own car; only in others' (13)       | 51%; 27%                                         | 94%; 5%                                           | 17%; 70%                                        | 28%; 65%                                                              | 1%; 4% (95% want it in none)                                 | 35%; 40%            |
| Avoid enforcement and warn others (13)                            | 43%                                              | 41%                                               | 26%                                             | 59%                                                                   | 43%                                                          | 42%                 |
| High moral disengagement (13)                                     | 34%                                              | 23%                                               | 28%                                             | 68%                                                                   | 63%                                                          | 44%                 |
| More likely to speed when there is no traffic (D4)                | 30%                                              | 27%                                               | 53%                                             | 69%                                                                   | 61%                                                          | 52% (Figure 9)      |
| Likely to get a ticket for residential speeding (D6)              | 53%                                              | 65%                                               | 32%                                             | 44%                                                                   | 30%                                                          |                     |
| Distinguishing features (D3)                                      | Never in a crash 41%; below high school 33%      | Women 58%; never in a crash 43%                   | Aged 60 and over 53%; three or more crashes 28% | Aged 16 to 44 63%; crash in the last two years 25%; camera ticket 11% | Men 62%                                                      |                     |

**The report's reading of each group** (pages 72 to 88 and 103):

- **Strict Compliance Advocates** treat nearly all speeding as unacceptable; their speed is set
  by their own comfort and standard of safe driving more than by law, enforcement or social
  pressure, an "absolutist view of safety". A relatively high share want limits lowered (12% to
  26% by road type), and half want speed assistance in their own cars.
- **Safety-Focused Realists** accept that drivers can exceed the limit safely at times, and
  comply anyway because they weigh both crash risk and enforcement. They give enforcement,
  infrastructure and speed assistance the highest support of any group "by a wide margin", and
  are the group least likely to avoid enforcement (21% do not).
- **Principled Pragmatists** accept the limits' legitimacy, rarely speed, and make situational
  exceptions for traffic flow: 49% agree it is safer to drive with the flow than to obey the
  limit. They want speed assistance in other drivers' cars, not their own, and the report reads
  them as the group with "the greatest degree of cognitive dissonance from speeding". Their
  acceptance may follow social norms: they may support speed assistance once it is "widely adopted
  and applied consistently across drivers".
- **Tolerant but Non-Compliant** drivers have normalised speeding; their speed choice is "more
  likely to be influenced by emotional or reactive reasons", and despite recent tickets and
  crashes "they had poorly calibrated opinions about the risks of speeding". They feel the most
  time pressure (a chronic time-pressure mean of 41.2 against 36.4 to 38.6).
- **Autonomy-First** drivers hold "a more fundamental belief that speed choice should remain
  under the driver's control", show "very low perceived risk", and oppose enforcement and speed
  assistance alike.

**What the report concludes** (pages 103 to 106). Infrastructure is "among the few interventions
capable of reducing speeding across all driver mindsets", and may matter most for Autonomy-First
drivers "precisely because they change the driving environment rather than attempting to change
attitudes"; its acceptance improves with "proactive communication explaining why specific
locations were selected". Enforcement may work best on Safety-Focused Realists and Tolerant but
Non-Compliant drivers, through the certainty of being caught rather than the severity of the
penalty. Speed assistance is misunderstood: only about 60% think a limiter would affect their own
or others' speed, and every configuration is rated less effective than any other measure but
rumble strips. Normalisation should start where acceptance already is, with Safety-Focused
Realists and with fleets such as New York City's municipal vehicles. The report did not study
education or campaigns, and notes that "collecting the data needed to classify drivers is onerous
and not easily replicated".

**Where the report disagrees with itself.** Ten places, recorded so no figure is taken from the
wrong one:

1. The overview says Strict Compliance Advocates "did not avoid speeding enforcement themselves"
   (page 72); their profile and Table 13 say 81% avoid it.
2. The profiles say Strict Compliance Advocates and Safety-Focused Realists "do not speed"; a
   third of each reports speeding sometimes or frequently (Table D4), and Principled Pragmatists
   have the fewest frequent speeders (2%).
3. The profiles of both speeding groups say they do not refrain from speeding "to avoid the risk
   of getting a ticket" (pages 81 and 84), yet 80% of Tolerant but Non-Compliant and 79% of
   Autonomy-First drivers name not wanting a speeding ticket as a reason not to speed, more than
   either compliant group (Table D5). The conclusions side with the table, crediting Tolerant but
   Non-Compliant drivers with "a strong sensitivity to the prospect of being caught speeding"
   (page 104); they also avoid enforcement and warn others more than any group (59%).
4. The Autonomy-First summary says limits "should be higher"; most of the group calls the
   current limit about right on every road type (page 85, Table 13), with 28% to 40% wanting it
   raised.
5. Table D5 prints "Because going over the speed limit is against the law" twice with different
   values; the second row is probably another reason mislabelled.
6. Figure 16 gives speed humps 33% support in areas with a crash history; Table 11 and the text
   give 43% (page 68). The plan uses 43%.
7. The text gives 34% and 22% wanting an overridable and a non-overridable limiter in their next
   car (page 71); Table 12 and Figure 16 give 35% and 23%. The plan uses the table.
8. The text puts moral disengagement at 10% low, 47% moderate and 43% high (page 66); Table 13
   prints 10%, 46% and 44%.
9. The method cites "Appendix E" for the latent class variables (page 56) and says Appendix D
   lists the state-level design effects and margins of error (page 53). There is no Appendix E;
   the variables are Table D1, and the state figures are in the separate state document.
10. The overview gives Tolerant but Non-Compliant drivers high speeding thresholds (page 73); their
    profile says "Moderate to High" (page 81).

**What this changes here.** No mindset can be read from a passage, and the accepted residual
stands. Two things transfer. Of the situations asked, the groups split most sharply on speeding
when there is no traffic (27% to 69%), which is the free-flow opportunity the deployment observes:
the strongest support yet for stratifying the pace and the limit-relative shares by traffic, as the
deferred research note proposes. And the report's prescription for infrastructure is to explain why
a location was chosen, which is what a before-and-after report from a site documents, without
recommending anything.

### The mindsets in the lead author's words

Rebecca Steinbach, the attitudes report's lead author, walked through the five groups with Kea
Wilson on Streetsblog's _The Brake_ ([Wilson 2026][brake], released October 6, 2026, 26 minutes).
It is a spoken, secondary account: her characterisations, the host's glosses, and no figure beyond
the group shares the summary already gives. The report's own profiles are under
[the five mindsets](#the-five-mindsets-what-the-report-states), and the list after the table says
where she and the report differ. Times in parentheses are positions in the episode audio. The
full transcript is archived outside the repository (see [Sources checked](#sources-checked)); this
section is the project's summary of it.

Why speeding (02:36 to 04:04): in the Traffic Safety Culture Index, speeding was the behaviour
people rated least dangerous and least disapproved of, the one they reported doing most, and for
many the only risky behaviour they reported (03:47). She frames the study as the "safer people
aspect" of the safe system approach (05:41): a speed hump or a camera works where it is built, and
everywhere else relies on drivers.

| Mindset, share of drivers        | How she describes the group                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | What she thinks would reach it                                                                                                                                                                                                                                                         |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Strict Compliance Advocates, 21% | Anything over the limit is speeding (10:22). The only group to equate driving safely with being a good person and a responsible citizen (07:26), and to see road safety as a shared duty: "you're driving for someone else" (07:49). Its speeding is, by her own conjecture, accidental: focus-group members described not watching the speedometer and slowing as soon as they noticed (08:46)                                                                                                         | Help not to speed by mistake: infrastructure that does not invite speed, a vehicle alert at the limit such as intelligent speed assistance, and vehicles built so that speeding is harder to miss (08:55 to 09:17)                                                                     |
| Safety-Focused Realists, 14%     | Not really speeding until close to 10 mph over (10:38), yet it seldom speeds, because it recognises the crash risk to itself and others and fears tickets (10:52)                                                                                                                                                                                                                                                                                                                                       | Enforcement: one of the groups "more susceptible to enforcement-related countermeasures" (11:15)                                                                                                                                                                                       |
| Principled Pragmatists, 26%      | Limits are generally right and should be followed, but autonomy and control over its own vehicle matter more (12:53), and it wants "to be the ones to decide how fast to go in particular conditions" (13:00). The conditions she lists next are where people in general say speeding is acceptable, not this group's own: faster, highway-like roads, light traffic, "no cops", no pedestrians or cyclists, "no one to see them" (13:06 to 13:35)                                                      | Intelligent speed assistance as "another tool to help them follow speed limits", once people who already accept it make it normal (20:18 to 20:35)                                                                                                                                     |
| Tolerant but Non-Compliant, 23%  | "The most worrying group by far" (14:55): poorly calibrated about consequences; more likely than any other group to have had speeding tickets and crashes, and still speeding more than the three groups before it (15:06); high moral disengagement, with three stories: the flow is safer than the limit, speeding is less dangerous than other behaviours, and "I'm not going so fast that I'm actually going to hurt anybody" (15:24 to 15:47). The host called it "tolerant but compliant" (14:15) | Not messaging, which "is not likely to work" (16:04), and not enforcement, which it does not fear: infrastructure and vehicle technology, to "constrain the environment and the vehicles that they are driving" (16:20)                                                                |
| Autonomy-First, 16%              | Does not want to be told what to do "by the system, by their vehicle, by police, by speed limit signs, by anyone" (17:02); raised the Autobahn at every chance in the focus groups (17:17); rejected in-vehicle technology more than any other group and found it "particularly offensive" (17:31); ideological rather than pragmatic (17:50)                                                                                                                                                           | It takes its own responsibility seriously, so messages that put "the onus on them to behave responsibly" may land better here than with other groups (18:18); it will avoid enforcement, offer little support for infrastructure, and never choose a car with speed assistance (18:28) |

**Where she and the report differ.**

- Principled Pragmatists and speed assistance: she hopes it can reach them. The report finds 17%
  want an overridable limiter in their own car and 2% a non-overridable one, against 70% and 73%
  for other drivers' cars (Table 13); her route is the report's, normalisation through use that
  looks widespread and consistent (pages 104 to 105).
- Tolerant but Non-Compliant drivers and enforcement: she says they do not fear it. The report's
  conclusions expect enforcement to work on them through the certainty of being caught (page
  104), and 80% name not wanting a speeding ticket as a reason not to speed (Table D5).
- Autonomy-First drivers: she suggests messages about personal responsibility. The report did not
  study messaging (page 105), points to infrastructure for this group (page 103), and reads its
  responsibility less generously: high moral disengagement and "very low perceived risk".
- Strict Compliance Advocates: "by accident" is her conjecture. The report says they are not
  influenced by situational factors, while a third of them report speeding sometimes or
  frequently (Table D4).

**What reaches more than one group** (19:51 to 22:12). Two lessons she draws for speed management.
Social norms: acceptance of intelligent speed assistance was higher than she expected (20:10), and
the focus groups tolerated it more in fleets and professional driving, "constraining professional
rather than personal freedom" (20:51), so fleets adopting it could make it normal. Transparency:
people accepted speed humps around schools and lane narrowing near work zones, where the reason is
plain, and resisted measures where it "wasn't obvious why they were there" (21:36 to 21:44). Hence
"something for everyone" (21:57): the report's conclusion against one-size-fits-all, in her words.

**What surprised her** (23:04 to 24:30). She expected people who drive well over the limit to think
it set wrong. Instead people "had a lot of faith in the speed limit and in the way that it was set"
(23:46), did not want it much higher, and read it as a safe speed that is part of being a good
driver. Against the limit stand other pressures: keeping up with traffic (24:01), showing that
"you're the master of your vehicle" (24:05), and boredom on routine drives (24:08). Speeding, in
her phrase, is "a moral negotiation of what you think is the right thing for you to do in the
moment" (24:21).

**What this changes here.**

- Nothing about any passage. A mindset belongs to a driver, and the sensor records passages, not
  who drove them; the accepted residual against mindset classification stands.
- The pace line gains a second reason. Keeping up with traffic is the first pressure she names,
  beside the 49% who agree the flow is safer than the limit (Table 10).
- The conditions people give for acceptable speeding (13:06) are states the deployment observes
  over time: light traffic as the free-flow opportunity, people on foot or on bicycles in the
  LiDAR scene, the hour, and darkness. They are hypotheses for the deferred research note on the
  pace by stratum, never a reading of who was driving; of them, no traffic splits the report's
  groups most.
- The transparency finding is the case for the before-and-after comparison: evidence of why a
  measure is there, which is what the focus groups asked for and what the report itself recommends
  (page 104), without the speed report recommending one.
- The interview shows the vocabulary the guard exists for. The host's "speeders" and "lawless
  maniac" (01:20, 01:39) and "maniacs" (16:30) are press words; the report labels mindsets, and
  the author speaks of groups and folks.

### Aggressive driving and road rage: what the report states

From the summary and the extracted parts of the technical report: its front matter, Part 1 and a
few figure captions. Parts 2 and 3 are not in the extraction.

- The definition the study adopts, from Finley et al. 2023 and the AAA Foundation's own 2022
  wording: any unsafe driving behaviour performed deliberately, with ill intention or disregard
  for safety, that impacts others. The expert panel debated speeding under it and
  concluded that speeding on an empty open road is not aggressive, while deliberately speeding
  around others with disregard for their safety is.
- Road rage is behaviour intended to cause physical, psychological or emotional harm; the panel
  proposed "violent driving" for the hostile end of the continuum, because the public and road
  safety practitioners do not separate it from aggressive driving.
- The literature review placed contributing factors at three levels of a socio-ecological model:
  individual, relational and community, the last including the built environment. The frameworks
  it surveyed are about aggression, emotion, and goal attainment or control.
- Previous AAA Foundation work found that in 2014 more than 78% reported at least one aggressive
  behaviour in the past year.
- Focus groups produced seven themes: putting others at risk, getting ahead, stealing space,
  controlling other road users' behaviour, expressions of displeasure, provoking reactions, and
  violence. Participants described anxiety, fear and pleasure as well as anger.
- Reported emotions while driving: annoyed or frustrated 72%, anxious 49%, angry 48%, calm 37%,
  empowered 17% (Figure 5). 80% drive less aggressively in rain; 18% more in rush hour and 16% more
  in unexpected traffic (Figure 8). 56% see sports cars drive aggressively always or often
  (Figure 9).
- The questionnaire measured 21 behaviours. 96% reported at least one in the previous year; trying
  to get ahead (92%) and putting others at risk (92%) were the most prevalent themes; 11% reported
  violent behaviours. Higher engagement among younger and male respondents; valuing road etiquette
  was protective; the most salient predictor was "aggressive driving culture", how much other
  people in one's area drive aggressively.
- Both reports carry the same rights statement: free to copy in whole or part and distribute at no
  charge with credit to the Foundation; not to be resold or used for commercial purposes without
  its permission.

### Figures still to confirm

What neither summary nor extraction carries. The press figures the plan first relied on are kept
only where nothing better exists.

| Figure                                                                                                               | Where it is                                    | Reported by                                                                                                                                                                                                               |
| -------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The 21 behaviours with their prevalence and theme, and the sample and weighting                                      | Aggressive-driving report, Part 3 and Figure 3 | Press: top five as speeding up at a yellow light (82%), using the right lane to pass (68%), honking at another road user (66%), glaring (65%), driving 15 mph faster than the normal flow (58%) ([AAA Northeast][aaa-ne]) |
| Changes since 2016: cutting off up 67%, honking up 47%, tailgating down 24%, yelling down 17%                        | Aggressive-driving report, Part 3              | [ConsumerAffairs][consumeraffairs]                                                                                                                                                                                        |
| The state-level estimates for states other than California                                                           | The attitudes report's separate state document | Not read; California's pages are under [what the technical reports add](#what-the-technical-reports-add)                                                                                                                  |
| A 2025 AAA Foundation telephone survey: 37% exceeded the limit by 10 mph on a residential street in the past 30 days | A different study                              | [IIHS speed page][iihs]                                                                                                                                                                                                   |

### What the technical reports add

Seven things the summaries did not carry, each with a consequence for the design.

1. **A curve, not an edge.** The public's threshold is a distribution by road type (Figure 5). On
   a 25 mph residential street, 5 over is speeding to 40% and 10 over to 85%; on a 35 mph arterial,
   5 over to a quarter and 10 over to 72%. The edition holds the curve's points per road type, and
   the shares table's bands gain a cited reading: the share of respondents who would call each band
   speeding.
2. **The report's own edge matches one of the plan's.** It classifies respondents by how often
   they travel 10 mph or more over the limit they expect on residential roads and arterials, and
   15 over on highways. The plan's 10 mph edge is that edge; the 5 and 10 mph edges also mark the
   residential curve (40% and 85%); the 15 mph edge is the report's for highways only. The edges
   were chosen to match the behaviour plan.
3. **Road type is an input.** The curve, the mean threshold and the edge all depend on it, and
   the site has no road class. The operator states it beside the limit.
4. **Three thresholds, one printable.** The means for speeding, for police action and for danger
   (33, 35 and 44 on a 25 mph street) are a finding worth the docs; only the first reaches a page,
   because the second is enforcement and the third fails the guard.
5. **The public's instrument is the pace.** 58% avoid enforcement by matching the vehicles around
   them, and the speed of other vehicles was "by far" what participants mentioned most in judging
   the vignettes. The pace line prints the quantity the public already uses.
6. **The sensor is read as a feedback sign.** 74% say an electronic sign would change their own
   speed and 41% would support one in their own neighbourhood; the focus groups doubted the
   displayed speed, and point 9 gives the report's own barriers. A radar on a pole invites the same
   reading, so the guide says what this one is not: no display, no record of any vehicle, and the
   calibration printed on the report.
7. **"Aggressive" is defined by intent.** Deliberately, with ill intention or disregard, and
   impacting others: three of the four clauses are mental states. That is the whole case for never
   printing the word, stated in the source's own terms.

Two hypotheses for the research notes also arrive: 73% say they speed less in the dark and 52%
more when there is no traffic, which the night stratum and the opportunity denominator observe
directly. The second divides the five mindsets more than any other situation asked, from 27% to
69% (Table D4).

The full report adds four more points the extractions lacked.

8. **Two ways drivers hide in traffic.** Among strategies to avoid a ticket (Figure 12), 21% make
   sure "there's always another vehicle going faster", and 8% use a radar detector. The first is a
   following behaviour the LiDAR can look for in aggregate: a vehicle above the limit with a
   faster vehicle ahead in its lane. The second bears on the radar itself: the OPS243-A transmits
   in the 24.0 to 24.25 GHz band, which overlaps police K-band, so a detector may alert near a
   deployment and its driver slow down. Whether a module of this power is detected at roadside
   range is an open question.
9. **Feedback signs, in the report's own synthesis** (Figure 16): support 41% in one's own
   neighbourhood, 43% and 44% in crash-prone and high-risk areas; 74% say a sign would change
   their own speed. Its barriers are "reduced effectiveness after driver habituation" and
   "skepticism related to sensor accuracy", and it recommends portable signs. The guide's
   statement of what this sensor is not, and its printed calibration, answer the second barrier.
10. **The de facto limit.** The Discussion states the mechanism the pace line and the shoulder
    research note rest on: drivers calibrate to the speed at which they expect enforcement, and
    "that threshold can become the de facto speed limit" (page 94). The Conclusions repeat that
    drivers tend to "evaluate speeding relative to surrounding traffic rather than posted speed
    limits" (page 106).
11. **California.** The state-level estimates give California a random sample of 1,056 drivers, a
    design effect of 1.66 and a margin of error of ±3.89 points: 54% agree it is safer to drive
    with the flow than to obey the limit (49% nationally), the top reasons to speed are getting
    around slow drivers (28%), not holding up traffic (24%) and arriving sooner (24%), 35% avoid
    enforcement and warn others, and support for speed humps in one's own neighbourhood is 40%
    (37% nationally). No California figure is marked significantly different from the national
    one.

### What the evidence establishes and does not

| Question a report reader asks                                | What the studies establish                                                                                                                                                                                                                                                                          | What they do not establish                                                                                                                                                                                                                                              |
| ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Where does the public draw the line for speeding?            | A curve by road type: on a 25 mph residential street, 40% of respondents call 30 mph speeding and 85% call 35 mph speeding; the report's own measure is 10 mph over on residential streets and arterials and 15 on highways: cited bin edges, and a cited reading of each band                      | Any harm at those speeds. The harm curves in the crash-data plan answer that, and a bin edge is never a safety threshold                                                                                                                                                |
| Why is this report's p85 read as normal?                     | 49% hold that the flow is safer than the limit, 49% that a little over is not really speeding, and the speed of other vehicles is what people mention most when judging a speed, and matching it the most common way to avoid enforcement: the reason to print the pace of traffic beside the limit | Anything about the people who passed this site. The findings are about a national population's beliefs                                                                                                                                                                  |
| Which "aggressive" behaviours can a roadside sensor observe? | A public vocabulary for two measurable behaviours, driving well above the flow and following too closely, and a definition whose load-bearing clauses are intent                                                                                                                                    | A classification of any passage. Prevalence is self-reported per person per year; the sensor counts passages per period                                                                                                                                                 |
| What do people support doing about speeding?                 | Support by countermeasure and by place, the gap between believed effect and support on one's own street, and the split on whether cameras exist for safety or revenue                                                                                                                               | What any community should ask for. The AAA report recommends infrastructure, targeted enforcement and speed assistance in general terms (pages 103 to 106); velocity.report takes no position, and the before-and-after comparison is its contribution to that question |

### What the sensor can observe of the public's vocabulary

The summary names the seven themes and the press names several of the 21 behaviours; the full
list is **Confirm**. The mapping is to the behaviour plan's feature matrix, which owns every
definition.

| Behaviour as the public names it                                                                 | Theme                                           | Sensor       | Behaviour plan feature                                                                  | Status                                                                  |
| ------------------------------------------------------------------------------------------------ | ----------------------------------------------- | ------------ | --------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Driving 15 mph faster than the normal flow of traffic                                            | Getting ahead                                   | Radar, LiDAR | `speed_relative_to_stream` (8.1); the pace of traffic and the share above it, this plan | Specified, unbuilt                                                      |
| Speeding                                                                                         | Putting others at risk                          | Radar, LiDAR | `speed_over_limit`, `speeding_exposure_{5,10,15}` (8.1); the limit-relative shares      | Waits on the posted limit; operator-supplied limit until then           |
| Tailgating, following too closely                                                                | Putting others at risk, controlling others      | LiDAR        | Following gap and net time gap (8.3)                                                    | Built, gated on G-SMO-1                                                 |
| Cutting off, weaving, changing lanes without signalling                                          | Getting ahead, stealing space                   | LiDAR        | Lane changes, merges and cut-ins (8.8)                                                  | Deferred: lane geometry and a completeness gate                         |
| Using the right lane to pass                                                                     | Getting ahead                                   | LiDAR        | 8.8                                                                                     | Deferred                                                                |
| Speeding up as a light turns, running a red                                                      | Getting ahead                                   | LiDAR        | Stop behaviour (8.11)                                                                   | Deferred: stop-line geometry; the signal state is never observed        |
| Brake-checking                                                                                   | Controlling others, provoking                   | LiDAR        | Induced evasive response (8.12)                                                         | Deferred: estimation plan Phase 8                                       |
| Blocking a merge, taking a space                                                                 | Stealing space                                  | LiDAR        | Yielding behaviour (8.10)                                                               | Later                                                                   |
| Honking, glaring, gesturing, yelling, flashing lights, confronting, bumping, leaving the vehicle | Expressions of displeasure, provoking, violence | None         | None                                                                                    | Unobservable by design: no camera, no microphone, no identity (Tenet 1) |

Three consequences follow.

1. **The sensor sees passages, not people.** 58% of respondents say they drove 15 mph above the
   flow at least once in a year. A site that sees 2% of passages 15 mph above the pace is not
   contradicting that, and a site that sees 20% is not confirming it. One figure is persons per
   year, self-reported; the other is passages per period, observed. The two are never printed side
   by side as a check on each other. The only legitimate comparison is the shape of a multi-site
   distribution, and that is a research note.
2. **"Aggressive driving culture" is a community-level variable** in the study, measured by asking
   people what others in their area do. At one street, the observed distribution of passages
   above the pace and of close following is the nearest observable thing. The docs may say so.
   No page names a culture, an index or a level of aggression, and the guard refuses the stem.
3. **The protective and predictive factors are out of reach.** Etiquette, feelings about one's
   vehicle, age and sex are properties of people. The sensor has none of them and, by Tenet 1, no
   means of acquiring them.

The attitudes report's vignettes add how people judge a speed with the speedometer hidden and
before the limit is given: the speed of other vehicles most, then external markers, following
distance (two participants counting seconds as driver's education taught), lane keeping (swerving
read as speeding), curvature, grade and visibility, weather, and the presence of pedestrians,
cyclists and crosswalks. Three of those are the sensor's: the pace (8.1), the following gap (8.3)
and, once lane geometry exists, lane keeping (8.7); the vulnerable-road-user criteria are the 8.9
and 8.10 interactions. Weather and visibility are context the site does not record.

### Terminology: what transfers and what does not

| Term in the studies                                                                                         | In docs and plans                                                                                 | On a report page or in `data.json`                                                                                      | Reason                                                                                                                               |
| ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| Speeding                                                                                                    | Yes, as the public's word                                                                         | "Above the posted limit", with the limit, its unit and its provenance                                                   | "Speeding" alone is a judgement without a limit. The TDL token `speeding` keeps its definition, maximum speed above the posted limit |
| Social threshold, 10 mph over                                                                               | Yes                                                                                               | A bin edge, "10 mph or more above the limit", with the citation in prose                                                | A focus-group finding about acceptance, not a safety threshold; the questionnaire puts it at 8 to 10 mph over by road type           |
| Flow of traffic, keeping pace                                                                               | Yes                                                                                               | "Pace of traffic", a measured median                                                                                    | The one term that names a quantity the sensor measures directly                                                                      |
| Enforcement threshold, 10 to 12 mph over                                                                    | Yes                                                                                               | Never                                                                                                                   | The report takes no position on enforcement                                                                                          |
| Driver mindsets; the five class names                                                                       | Yes, as the study's classes of respondents                                                        | Never                                                                                                                   | Classes of people from attitude items; the sensor has no people, and `driver` fails the guard                                        |
| Aggressive driving, road rage                                                                               | Yes, attributed to the studies                                                                    | Never                                                                                                                   | Verdict words; `aggress` fails the guard and `rage` should                                                                           |
| Aggressive driving culture                                                                                  | Yes, as above                                                                                     | Never                                                                                                                   | A community variable measured by asking; the observed distribution is reported as a distribution                                     |
| Putting others at risk, and the other six themes                                                            | Yes, attributed                                                                                   | Never                                                                                                                   | `risk` fails the guard; a theme is a label                                                                                           |
| Safe speeding, dangerous speeding                                                                           | Only as the study's description of moral disengagement                                            | Never                                                                                                                   | `danger` and `unsafe` fail the guard; the distinction is the one the study identifies as a justification                             |
| Countermeasures: infrastructure-based, automated enforcement, police presence, intelligent speed assistance | Yes, in a guide, neutrally, with the project's stance stated                                      | Never                                                                                                                   | Editorial stance; the before-and-after comparison is the product's answer to "did it work"                                           |
| Survey                                                                                                      | "Questionnaire" or "attitudes questionnaire" for theirs; "speed survey" for ours when both appear | "Survey" keeps the V12 meaning                                                                                          | V12 defines a survey as a deployment's measurement                                                                                   |
| Violation, offender, non-compliant                                                                          | Only when quoting                                                                                 | Never                                                                                                                   | `violat` and `offend` fail the guard; "non-compliant" labels a person                                                                |
| Compliance                                                                                                  | Yes                                                                                               | As the behaviour plan's `legal` kind prints it: the speed and the limit, not the word                                   | The measurement is the two numbers                                                                                                   |
| Top speeders, high-risk driving patterns (ours, today)                                                      | Remove                                                                                            | Remove                                                                                                                  | Labels on people; `speeder` joins the guard                                                                                          |
| Technical speeding, acceptable speeding                                                                     | Yes: the focus groups' own pair                                                                   | "Above the posted limit" for the first; the bands, with the share of respondents who call each speeding, for the second | The report's distinction between the law and the norm; the page prints both without a verdict on either                              |
| Speed limit credibility; expected and preferred limits                                                      | Yes                                                                                               | Never                                                                                                                   | A property of respondents' beliefs about limits, not of a street                                                                     |
| Non-speeder, rare, sometimes and frequent speeder                                                           | Only as the report's classes                                                                      | Never                                                                                                                   | Classes of people; `speeder` joins the guard                                                                                         |
| Speed feedback sign, electronic warning sign                                                                | Yes, in the guide, to say what the sensor is not                                                  | Never                                                                                                                   | A feedback sign shows each vehicle its speed; this sensor shows nothing and records no vehicle                                       |

### The wording guard and the sources' own titles

Both titles fail the guard as written: _Attitudes Toward Speeding: Insights from Focus Groups and a
National Survey of U.S. Drivers_ contains `Driver`, as does the series label on its cover,
`Driver Behavior & Performance`, and _Aggressive Driving and Road Rage_ contains `Aggress`. The
crash-data plan met the same problem with a table title and settled it: a page cites author and
year, the full title lives in the edition field the guard skips, and a test proves the citation
renders. The same rule applies here, and it is a reason to keep one stem list rather than two. The
report's own words trip the guard too: its behaviour classes are "non-speeders, rare speeders,
sometimes speeders, and frequent speeders" (page 55), and Figure 14's group is "Drivers with a
record of serious speeding offenses", so a page that quotes them cites the figure instead.

### What a speed survey gives back

1. **The pace against the limit, by road type.** The attitudes study reports that acceptance is
   roadway-dependent and that keeping pace is what the public mentions most, without measuring what
   the pace is. Pooled across sites, the distribution of p50 less the posted limit by road type is
   that measurement, and it says whether the social threshold describes where the flow actually
   sits on residential streets. A research note in `data/explore/`, after the posted limits plan
   gives sites a limit.
2. **A shoulder in the upper tail.** If speed choice is calibrated to an expected enforcement
   threshold of 10 to 12 mph over, the upper tail of a site's speed distribution should fall more
   steeply past that point than a smooth tail would. A site distribution with enough transits can
   test for that shoulder. It is a hypothesis for a research note, not a report feature, and
   finding it would say nothing about any passage.
3. **Night against day.** The crash-data plan's strata already split the week the way the crash
   literature does. The share above the social threshold by stratum is the observed side of a
   belief the studies record and cannot measure.

What a survey cannot give back: any attitude, any motivation, any mindset, any self-report, and
any count of a behaviour the sensor cannot see. The studies' prevalence figures are not a
calibration target for the sensor, and the sensor's shares are not a check on the questionnaire.

## Design / approach

```mermaid
flowchart LR
  S["AAA Foundation summaries<br>and technical reports"] --> E["Safety-reference edition<br>attitudes entries: figure, population,<br>year, item, citation"]
  S --> D["Docs: terminology contract,<br>behaviour mapping, guide copy"]
  T["Site transits<br>per-transit maxima, start times"] --> P["Pace of traffic<br>p50 per bucket and per stratum"]
  T --> B["Limit-relative shares<br>above the limit; by 5, 10, 15<br>in the sign's unit; masked under 50"]
  L["Limit<br>operator-supplied today,<br>limit segment at v1.0"] --> B
  P --> R["Report: pace line, shares table,<br>cited attitudes paragraph"]
  B --> R
  E --> R
  R --> G["One wording guard<br>author-year on the page,<br>titles in the skipped field"]
```

### Terminology contract

Five rules, applied to every page, API payload, guide and plan this work touches.

1. A page describes passages and distributions. It never names a person, a class of person, a
   trait, a culture or a mindset. The guard enforces the stems; the rule covers the rest.
2. Every figure from a study is an attitude or a self-report, labelled as such, with its
   population, year and citation. No such figure is printed beside an observed share as a
   comparison.
3. "Pace of traffic" is the median of per-transit maximum speeds over a stated window and
   population. "Above the limit" always carries the limit, its unit and its provenance. "Speeding"
   stays a docs word and a TDL token.
4. "Survey" means a deployment's measurement. The studies' instruments are questionnaires.
5. The report takes no position on enforcement or countermeasures. The guide may report what the
   public supports, with the project's stance beside it.

### The pace of traffic

The pace is p50 over per-transit maximum speeds, the statistic the report already computes per
rollup bucket and over the period. The line prints the period pace beside the limit, in the sign's
unit, with the limit's provenance: "Pace of traffic 31 mph; posted limit 25 mph
(operator-supplied)".

A passage's distance above the pace is its maximum speed less the p50 of the transits that started
in the same clock hour, in the site's zone. The window is fixed at one hour and is independent of
the chart grouping, which the operator chooses from 15 minutes to 28 days: a 28-day bucket holds
no contemporaneous traffic. An hour with fewer transits than the chart's `LowSampleThreshold` (50)
gives its passages no distance, and they leave both the numerator and the denominator of the
share. The share of transits 10 mph or more above the pace is printed with its N. The per-transit
distance is computed for the share and never stored, served or exported: a passage's position
against its neighbours is one step from a label on it.

Radar transits have no direction, so the radar pace mixes both directions and the line says so.
The behaviour plan's `speed_relative_to_stream` is the same-direction LiDAR quantity; when the
transit worker keeps direction, the radar pace follows the same definition and the caveat goes.

### Limit-relative shares

For the period and for each of the crash-data plan's four strata, the share of transits at or below
the limit, and the cumulative shares above it by 5, 10 and 15 mph where the sign is in mph. The
10 mph edge is the report's own: it classifies respondents by how often they travel 10 mph or more
over the limit they expect on residential streets and arterials (15 on highways), and on a 25 mph
residential street 40% of its respondents call 5 over speeding and 85% call 10 over speeding
(Figures 5 and 8). They also
match the behaviour plan's `speeding_exposure_{5,10,15}` edges, so there is one set of edges in the
project. The quantities differ and are named apart: `speeding_exposure_*` is free-flow time on a
LiDAR passage; a share here is a fraction of transits by maximum speed. Each cell carries its N and
is masked under 50 transits, the stratum table's rule, and the identifiability analysis's
small-cell rule applies to the complement. The boundary-hour filter applies to every cell exactly
as it does to the headline numbers, the crash-data plan's Item 2a rule. The pace and the shares use
the same speed expression as the headline percentiles, cosine correction included: a mount angle of
20° understates an uncorrected speed by 6%, and a share against a limit is the figure whose reading
that understatement changes.

The curve the paragraph cites and the edge it names depend on the site's road type, residential,
arterial or highway, which the site does not record. The operator states it beside the limit, in
the report configuration today and on the site after the vocabulary plan's sites merge, and the
report prints it as operator-stated. Without one, the table prints its bands and the paragraph
cites the flow finding alone.

The comparison is with the limit, so the benchmark kind is `legal`, with the limit's jurisdiction
and effective date once the posted limits plan supplies them. The 10 mph edge is a reporting
choice, and the edition records why it was chosen and what it cites. A bin edge is not a
`research_threshold`; the behaviour plan's `no_established_threshold` carries no citation, so the
provenance of the edge lives in the edition entry and the prose, not on the benchmark.

Edges are in the sign's unit. The curve and the edges are findings about the United States. A site
whose limit is signed in km/h, or in a jurisdiction with no cited edges, prints the share above the
limit and nothing finer until the edition carries an entry for it.

### Attitudes entries in the safety-reference edition

The crash-data plan's pinned edition gains a `surveys` group, one edition rather than two, with the
same provenance fields: each entry carries the figure, the population and its size, the instrument
and fieldwork dates, a paraphrase of the item, the report's figure or table number, the citation
with its full title in the field the guard skips, and the date the source was read. The entries
the pace line and the shares need:

- 49% agreed that it is safer to drive with the flow of traffic than obey the speed limit
  (Table 10; mean 3.3 on a five-point scale).
- The curve of respondents who call a speed speeding, by road type (Figure 5): residential
  25 mph, 40% at 30, 85% at 35, nearly all at 45; arterial 35 mph, about 25% at 40, 72% at 45,
  nearly all at 60; rural highway 70 mph, 34% at 75, 94% at 85; urban highway 70 mph, 34% at 75,
  94% at 80.
- The mean speeds respondents called speeding by road type (Figure 4): 33, 45, 80 and 78 mph
  against limits of 25, 35, 70 and 70.
- The report's own definition of speeding for classifying respondents: 10 mph over the expected
  limit on residential roads and arterials, 15 on highways, which is the provenance of the shares'
  edges.
- Expected and preferred residential limits, 25.8 and 26.2 mph, and the 24% who would set a
  residential limit at least 10% above what they expected (Figure 6).
- Kept in the edition and never printed, for the docs: the mean speeds at which respondents
  expected police to act (35, 47, 81 and 80) and the speeds they called dangerous (44, 56, 88 and
  85).

The mindsets, the countermeasure and ISA support figures, the moral-disengagement items beyond the
flow item, and every aggressive-driving figure stay out of the edition. They have no report
surface.

### The cited attitudes paragraph

It replaces the "top speeders" paragraph and passes the guard. A draft for a residential site:

> Many people judge a speed against the traffic around it as much as against the sign. In a
> national questionnaire of 16,598 people who drive, 49% agreed that keeping pace with traffic is
> safer for them than obeying the limit (Steinbach et al. 2026). Shown a 25 mph residential street,
> 40% called 30 mph speeding and 85% called 35 mph speeding, and the report itself counts 10 mph
> over as speeding when it classifies the people it asked. This report prints the pace of traffic
> beside the limit, and the share of passages 5, 10 and 15 mph above it, so a reader can see where
> this street's pace sits against both. Those thresholds are where acceptance ends, not where harm
> begins; the harm figures are in the section above, with their sources.

The third sentence is chosen by the stated road type and omitted without one. The paragraph is
driven from `data.json` and the edition, so the figures and the edition identifier render from data
rather than template text.

### One wording guard

`internal/api`'s `verdictWords` becomes `headway.VerdictPattern`, so the headway report, its field
run, the charts and the API scan one list. The list gains `speeder` and a word-bounded `\brage\b`:
the bare stem would refuse "average", "coverage" and "storage", words a served payload can
legitimately hold. `reckless`, `careless` and `mindset` are candidates for the behaviour plan's
owner. `compliant` is an open question below. The citation test from the crash-data plan covers the
new paragraph and the shares table.

The guard is a test over template-driven text, `data.json` and the edition's strings. Operator-
entered fields, the site description, the surveyor and the speed-limit note, are the operator's
words: they are not scanned, and the report prints them marked as operator-supplied.

### Documentation surfaces

- A guide section for people reading a report: why many readers will call the pace normal; what
  the bands mean and do not mean, with the curve; what the public says it supports, printed
  neutrally with the project's stance; and what the sensor is not. It is not a speed feedback sign,
  which 74% say would change their own speed and 41% would support in their own neighbourhood,
  whose displayed speeds the focus groups doubted, and whose barriers the report names as
  habituation and doubts about the sensor's accuracy (Figure 16); and it is not a camera, which
  focus groups called "Big Brother". The sensor shows nothing, records no vehicle, and prints its
  calibration provenance on the report. Target `public_html/src/guides/reports.md`, a guide the
  index does not yet have.
- A one-line entry in the behaviour plan's Section 8.1 pointing at the shares and the pace, and in
  the vocabulary plan's next round for **pace**, **questionnaire** and **road type**.
- `references.bib` entries `Steinbach2025` and `Steinbach2026`.

### Boundaries

No classification of a passage or a person by any study's categories. No "aggressive" on a page.
No culture, index or level. No self-reported prevalence printed beside an observed share. No
enforcement threshold on a page. No countermeasure recommendation in a report. No band finer than
the limit for a unit or jurisdiction the edition has no cited edges for. No claim that a share
above the social threshold is a share above a harm threshold.

## Scope

### Item 1: read the rest of the technical reports and record the rights

**Summary:** Replace the remaining **Confirm** figures from the parts of the reports the
extractions did not carry, and settle the rights.

**Steps:**

1. The aggressive-driving report's Parts 2 and 3: the 21 behaviours with their prevalence and
   theme, the sample and weighting, the 2014 and 2016 comparison method, and the aggressive
   driving culture items; the wording of the "15 mph faster than the normal flow" item, which
   decides the pace-above edge.
2. Done for the attitudes report on October 12, 2026: the profiles (pages 72 to 92 and Appendix
   Tables D3 to D7), the LCA inputs (Table D1; the report has no Appendix E), the Discussion and
   its countermeasure synthesis (Figure 16). The state-level document was read for its method and
   California; the other states remain.
3. Rights: the attitudes report "may not be resold or used for commercial purposes without the
   explicit permission of the foundation. It may, however, be copied in whole or in part and
   distributed for free via any medium, provided the Foundation is given appropriate credit as the
   source of the material" (page 3). Settle whether this project's use is non-commercial, the same
   question the crash-data plan asks of CC BY-NC-ND 4.0, and record the answer in both plans.
4. Add both sources to `data/maths/references.bib` and a dated verified-sources table to this
   plan.

**Acceptance:** no **Confirm** mark remains in this plan, and the rights row states what may be
reproduced.

**Milestone:** v0.5.10, before Item 3b

### Item 2: one wording guard and the percentile copy

**Summary:** One stem list across the headway report and the API, with `speeder` and a
word-bounded `rage`, and the percentile paragraph rewritten without labels on people.

**Steps:**

1. `verdictWords` in `internal/api` replaced by `headway.VerdictPattern`; the catch-the-words test
   extended to the new stems and to both source titles, which must be refused, and to "average"
   and "coverage", which must pass.
2. The percentile paragraph in `sections.typ` rewritten to describe what p98 measures, with the
   cited attitudes paragraph of Item 3 taking the explanatory role, and one clause saying that p85
   is a statistic of operating speeds and not a rule for setting limits; the operator guide carries
   the ITE and MUTCD citations ([D-28][d28]).
3. A guard test over the full rendered report's `data.json` and text, as the headway report's
   `TestNoVerdictLanguage` does, since the speed report has none today.

**Milestone:** v0.5.10

### Item 3a: the pace line and the limit-relative shares

**Summary:** Print the pace of traffic and the limit-relative shares per period and stratum
against the operator-supplied limit, with no external data.

**Steps:**

1. The pace per hour and per period in `data.json`, each transit's distance above its hour's pace
   computed and not stored, and the share 10 mph or more above the pace with its N.
2. The shares table per period and per stratum, edges from the edition for the sign's unit, masked
   under 50, the boundary-hour filter applied as to the headline numbers, the limit's provenance
   printed as operator-supplied, and the direction caveat for radar.
3. An operator-stated road type, residential, arterial or highway, in the report configuration
   beside the limit, printed with it and absent by default.
4. The same speed expression as the headline percentiles, cosine correction included, proved by a
   test on a site with a mount angle.
5. Tests: the Friday 18:00 stratum boundary shared with the crash-data plan's Item 2a; an hour
   under 50 transits, whose passages leave the pace share; a limit in km/h, which prints the share
   above the limit only; a site with no limit, which prints no shares and no pace-against-limit
   line; the guard over the table.

**Acceptance:** a report at a site with a limit prints the pace line and the shares table, a report
without one prints neither, and the headline numbers do not change.

**Milestone:** v0.5.10; the period-level table depends on nothing, the stratum rows on the
crash-data plan's Item 2a

### Item 3b: the attitudes entries and the cited paragraph

**Summary:** The `surveys` group in the safety-reference edition and the paragraph that cites it.

**Steps:**

1. The `surveys` group with the entries the design lists, the flow item, the curve and the mean
   thresholds by road type, the self-report edges and the credibility figures, each validated like
   the edition's other groups: citation, population, fieldwork dates, figure or table, the item's
   paraphrase, and the title in the skipped field. The Traffic Safety Culture Index is the series
   source for the speeding items, one entry per edition year, with the one-off study as the
   explanatory source; the observed shares of Campolettano et al. 2025 are cited in the guide
   only ([D-28][d28]).
2. The paragraph, driven from `data.json` and the edition, its road-type sentence chosen by the
   stated road type and omitted without one, and its population described as the report describes
   it: people who had driven in the previous month.
3. Tests: the guard over the paragraph; both source titles refused and the author-year citation
   rendered; the "acceptance ends, not where harm begins" sentence printed; the police and danger
   thresholds present in the edition and absent from every rendered page.

**Acceptance:** the paragraph renders from the edition with its identifier, and no figure in it
exists outside the edition.

**Milestone:** v0.5.10, after Item 1 and after the crash-data plan's Item 2 ships the edition

### Item 4: guide and plan cross-references

**Summary:** The guide section, including what the sensor is not, and the behaviour and vocabulary
plan entries.

**Acceptance:** the guide section is linked from the guide index; the behaviour plan's Section 8.1
and the vocabulary plan's next round name the pace, the questionnaire and the road type;
`references.bib` carries both entries.

**Milestone:** v0.5.10, after Item 1

### Item 5: shares per limit segment and schedule

**Summary:** When the posted limits plan gives each transit a limit segment, with its unit,
jurisdiction and schedule, the shares split at limit changes and the operator-supplied caveat
goes; a school-zone schedule gives the shares during active hours.

**Milestone:** v1.0, after the posted speed limits plan and D-16

## Dependencies

- The crash-data plan's Item 2 for the edition package and Item 2a for the strata the shares are
  printed in.
- The [posted speed limits plan](posted-speed-limits-plan.md) for any share that is more than
  operator-supplied, and the [schedules design](../radar/architecture/speed-limit-schedules.md)
  for school-zone hours.
- A stated road type, residential, arterial or highway, for the curve the paragraph cites and the
  edge it names: in the report configuration until the vocabulary plan's sites merge gives it a
  home on the site.
- The transit source's cosine correction. The PDF's transit statistics are queried without the
  cosine join while the page prints the corrected note, an open task; a share against a limit does
  not print until the pace and the shares use the corrected speed.
- The transit worker keeping direction, for a same-direction pace on radar.
- The behaviour plan's owner for the guard stems beyond `speeder` and `rage`, and for the LiDAR
  `speed_relative_to_stream`, which this plan does not build.
- Item 1's reading of the reports' remaining parts before Item 3b prints a figure.

## Risks

| Risk                                                                                            | Likelihood | Impact | Mitigation                                                                                                                                                              |
| ----------------------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The 10 mph band is read as a harm threshold                                                     | High       | High   | The paragraph says where acceptance ends is not where harm begins; a test checks the sentence; the harm paragraph in the crash-data plan sits above it with its sources |
| A self-reported prevalence is printed beside an observed share                                  | Medium     | High   | No prevalence figure enters the edition; the terminology contract's rule 2; review of every `surveys` entry against it                                                  |
| The guard refuses the citation                                                                  | High       | Low    | Author and year on the page, the title in the skipped field, and a test that both titles are refused and the citation renders                                           |
| A reviewer wants the mindsets as a segmentation of a street's traffic                           | Medium     | Medium | The sensor has no people; the boundary is stated; `driver` and `mindset` in the guard make the label unprintable                                                        |
| The radar pace mixes directions and is read as one stream                                       | High       | Low    | The caveat prints until the worker keeps direction; the LiDAR definition is same-direction                                                                              |
| Press figures are wrong                                                                         | Medium     | Medium | Item 1 first; nothing from the press table reaches the edition                                                                                                          |
| The report is read as recommending cameras or police presence                                   | Low        | High   | Nothing about countermeasures on a page; the guide states the project's stance beside any support figure                                                                |
| A km/h site gets US bin edges                                                                   | Medium     | Medium | Edges are an edition entry per unit and jurisdiction; without one, only the share above the limit prints                                                                |
| "Survey" is read as the questionnaire in a plan, or as the deployment in a citation             | Medium     | Low    | The contract's rule 4; the style pass can flag "national survey" without "questionnaire" nearby                                                                         |
| One site's shares are quoted as the national figure's local value                               | Medium     | Medium | The pace line and the shares carry the site and period; the paragraph names the population the 49% describes                                                            |
| The bare stem `rage` refuses "average", "coverage" and "storage"                                | High       | Medium | A word-bounded `\brage\b`, and a test that those words pass                                                                                                             |
| Uncorrected transit speeds at an angled mount understate every share against the limit          | High       | High   | The shares use the headline speed expression; the cosine task is a dependency; a test on an angled site                                                                 |
| The pace window follows the chart grouping and loses contemporaneity                            | Medium     | Medium | A fixed one-hour window independent of the grouping; passages in thin hours leave the share                                                                             |
| The guard refuses an operator's own note                                                        | Medium     | Low    | Operator-entered fields are not scanned; the report marks them operator-supplied                                                                                        |
| The site's road type is stated wrong, and an arterial's curve is cited for a residential street | Medium     | Medium | The road type prints beside the limit as operator-stated; without one the paragraph cites the flow finding only                                                         |
| The curve is read as the share of passages that are speeding                                    | Medium     | Medium | The paragraph says the curve is the share of respondents who would call that speed speeding, in a sentence apart from the passages' shares                              |

## Open questions

- Does a radar detector alert on the OPS243-A? The report finds 8% of drivers use one (Figure
  12), and the module transmits inside police K-band. A site with both sensors can test it: the
  LiDAR's speed profile on the approach, compared between the radar on and off, at the same hours.
- Does `compliant` join the guard? It would block "compliant" as a label on a passage while leaving
  "compliance" for the `legal` kind's name. Decided by the behaviour plan's owner with the other
  candidate stems.
- Which edges for km/h jurisdictions, and is an enforcement guideline such as the UK's "10% plus
  2 mph" a legitimate cited edge when the report takes no position on enforcement? The alternative
  is the share above the limit only until an attitudes source for that jurisdiction exists.
- Where does road type live, and which of the report's four types does a site take? The report's
  residential image is a 25 mph street and its arterial image is posted at 35, so a 30 mph
  collector sits between the curves and the report has none for it; its respondents also expected
  the arterial's limit to be 45.8 mph before they were shown the sign.
- Does the share above the pace use 10 mph or 15? The aggressive-driving item is "15 mph faster
  than the normal flow of traffic" (press; **Confirm**), the only pace-relative threshold either
  study names. The plan prints 10 until the item's wording is read.
- The report's definition of speeding is measured from the limit respondents expected, not the
  posted one; a site uses the posted limit. Does the paragraph say so, or is the difference small
  enough on a 25 mph street (expected 25.8) to leave to the docs?
- Is 50 the right floor for an hour's pace? A median of 20 values is steadier than an 85th
  percentile of 20, so the hour floor may sit lower than the chart's;
  [Q24](../../data/QUESTIONS.md) asks the same of p85, and the floor follows its answer.
- Should the radar pace line wait for direction, or print mixed-direction with the caveat? The
  plan prints with the caveat.
- Does the `surveys` group sit in `internal/report/safetyref` with the harm and rate entries, or
  does the package take a wider name? One edition either way.
- Does the guide show the danger and police thresholds (44 and 35 on a 25 mph street) at all,
  given that the guard keeps both off every page? The docs are not a page, so the plan says yes,
  labelled as what respondents said.
- 84% say they speed less in rain and 73% less in the dark; the site records no weather. Does a
  report ever take a weather record, or does the stratum table say only that the site cannot tell
  a wet night from a dry one? Out of this plan's scope; logged for the crash-data plan's strata.
- Reproduction is settled in part: both reports may be copied in whole or in part and distributed
  for free with credit, and may not be resold or used commercially without the Foundation's
  explicit permission. Whether this project's use is commercial is the open half, shared with the
  crash-data plan's CC BY-NC-ND question, and Item 1 records one answer in both plans.
- The behaviour plan's 8.3 bands were chosen without the aggressive-driving study; its
  self-reported tailgating decline since 2016 is not evidence about a band. Should 8.3's prose
  say so, to stop the comparison being made later?
- Decision to record on acceptance, in `DECISIONS.md`: the report takes no position on
  enforcement or countermeasures. The guide and every later surface inherit it, so it belongs in
  the register rather than in one plan.
- Does the annual Traffic Safety Culture Index, on a constant questionnaire since 2008, enter the
  edition's attitudes entries beside the one-off attitudes study? A series is what any
  before-and-after reading of attitudes would need, and the one-off study has no before. Raised by
  the [October 2026 briefing][rb-2026-10]. **Answered** by [D-28][d28]: yes, as the series
  source for the speeding items, one entry per edition year, with the one-off study as the
  explanatory source.
- Do Campolettano, Kusano and Victor's observed shares above the limit in San Francisco and
  Phoenix enter the pace line as an `external_distribution` comparator, with the mobile population
  and the two cities stated, or only as context in the guide? Raised by the same briefing.
  **Answered** by [D-28][d28]: guide only; nothing on the report page.
- Does the percentile paragraph cite ITE's position and the MUTCD edition on what the 85th
  percentile is used for, so that a printed p85 is never read as the limit-setting rule? Raised
  by the same briefing. **Answered** by [D-28][d28]: one clause on the page, that p85 is
  a statistic of operating speeds and not a rule for setting limits; the citations in the guide.

## Sources checked

Checked on October 9, 2026. Both summaries were read in full from the uploaded PDFs; the technical
reports were read from text extractions the owner supplied, in which figures are placeholders and
the numbers in a figure come from its extracted axis labels. The AAA Foundation site, the AAA
newsroom and their mirrors were unreachable from the review environment. **Confirm** marks a claim
whose primary text was not read. The lead author's interview was added on October 11, 2026. On
October 12, 2026 the attitudes technical report was read in full from the AAA Foundation's PDF,
which the Foundation's site served again, and its state-level estimates were fetched.

| Source                                                                        | What was read or found                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Status                    |
| ----------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------- |
| Attitudes Toward Speeding, report summary, August 2026                        | Three pages: introduction, method, every key finding and the five-mindset table; the Foundation's PDF has SHA-256 `e4e93038491a6777212e39d25a1c62dd17b5c18c970d166e0bcbbde386156238`                                                                                                                                                                                                                                                                                                                                                                                             | Read                      |
| [Attitudes Toward Speeding, technical report][aaafts-speed-pdf]               | All 196 pages, from the Foundation's PDF (SHA-256 `4ee60963d9bc6af4ef0cdf83f949b58cce9370cea6edf6cc7a057f39ee4f0c98`, the same file the project's paper archive holds as `Steinbach2026.pdf`): Parts 1 to 3, the five attitudinal groups with Table 13, the Discussion, Figure 16, the Conclusions, the questionnaire (Appendix C) and Appendix D; every figure in this plan checked against it                                                                                                                                                                                  | Read                      |
| [Attitudes Toward Speeding, state-level estimates][aaafts-speed-states]       | 103 pages, SHA-256 `ab0576dc46e56af4469b0efb39ccbd4797e203ab036226334d301f8c7cb2d261`: per state, the sample (random and opt-in), design effect and margin of error, countermeasure support against the national figure with significance marks, and charts of reasons to speed and not, moral beliefs and enforcement avoidance; read for its method and for California                                                                                                                                                                                                         | Read in part              |
| [_The Brake_, "The Real Reasons Drivers Speed"][brake]                        | Kea Wilson's interview with the lead author, October 6, 2026, 25 min 47 s: the five mindsets in her words, two lessons for speed management, and what surprised her. Transcribed by two machine passes (Whisper and Parakeet) reconciled by an agent and checked against both, not proofread against the audio; the transcript is archived outside the repository as `transcripts/the-brake/2026-10-06-the-real-reasons-drivers-speed-rebecca-steinbach/transcript.md` on the project archive volume, SHA-256 `6849ae438ef39245175e2d516fbf7fc5197291eb5b72af170b4df937e345afe3` | Heard, via transcript     |
| Aggressive Driving and Road Rage, report summary, September 2025              | Two pages: introduction, methodology, every key finding                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Read                      |
| [Aggressive Driving and Road Rage, technical report][aaafts-aggr-pdf]         | Front matter, the rights statement, Part 1's definitions and frameworks, and figure captions for Figures 5, 8 and 9; Parts 2 and 3 not extracted                                                                                                                                                                                                                                                                                                                                                                                                                                 | Read in part              |
| [AAA newsroom release, August 18, 2026][aaa-speed-release]                    | Figures since confirmed against Tables 10 and 11 and Figures 10 and 11                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Superseded                |
| [AAA newsroom release, September 2025][aaa-aggr-release]                      | 96% any behaviour; 11% violent; behaviours contagious; matches the summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Superseded                |
| [AAA Northeast][aaa-ne]                                                       | The top five behaviours with percentages                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | **Confirm**; via search   |
| [ConsumerAffairs][consumeraffairs]                                            | The four changes since 2016                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | **Confirm**; via search   |
| [CBS Detroit][cbs-detroit]                                                    | The 44, 85 and 88 mph danger thresholds; confirmed against Figure 4                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Superseded                |
| [IIHS speed topic page][iihs]                                                 | A 2025 AAA Foundation survey: 37% exceeded the limit by 10 mph on a residential street in the past 30 days. The figure is the 2025 Traffic Safety Culture Index's, a web panel and not a telephone survey, per the [October 2026 briefing][rb-2026-10]                                                                                                                                                                                                                                                                                                                           | Checked against the index |
| [AAA Foundation research page, attitudes][aaafts-speed]                       | Fetched October 12, 2026: links the technical report, the summary and the state-level estimates                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Read                      |
| [AAA Foundation research page, aggressive driving][aaafts-aggr]               | Lists the report and the summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | **Confirm**; via search   |
| [AAA Foundation, 2025 Traffic Safety Culture Index][tsci-2025]                | Zhang and Steinbach, June 2026; a probability web panel of 2,699 licensed drivers, July 31 to August 13, 2025: 61% rate 10 mph over on a residential street very or extremely dangerous and 48% rate 15 over on a freeway so; 37% and 48% report doing each at least once in 30 days; speeding has the lowest social disapproval of every behaviour asked; 47% support cameras for more than 10 over on residential streets; the same copying terms as the two studies; read in part by the [October 2026 briefing][rb-2026-10]                                                  | Read in part              |
| [NHTSA, 2022-2023 National Survey of Speeding Attitudes and Behaviors][nssab] | DOT HS 813 594, December 2024, 5,680 respondents by web and mail; 87% agree everyone should obey the limit because it is the law and 85% call more than 20 mph over unacceptable; the series the AAA report cites                                                                                                                                                                                                                                                                                                                                                                | **Confirm**; To fetch     |
| [NHTSA, Speeding: 2024 Data][nhtsa-813823]                                    | DOT HS 813 823, July 2026: a speeding-related crash is one where a driver was charged with a speeding offence or an officer recorded racing, too fast for conditions or exceeding the limit; 11,288 fatalities, 29% of the total, 28% of fatal crashes; the primary behind the "nearly 30%" the cited paragraph quotes, and an officer attribution, never a share of passages; read in full                                                                                                                                                                                      | Read                      |
| [Campolettano, Kusano and Victor 2025, TIP][campolettano-tip]                 | Observed shares above the posted limit on surface streets in San Francisco and Phoenix from a million free-flow traversals seen by a ride-hailing fleet: 33% to 49%, p85 3.6 to 7.2 mph over; a mobile-sensor population and the only observed comparator for Item 3a's shares                                                                                                                                                                                                                                                                                                   | **Confirm**               |
| [ITE, Setting Speed Limits][ite-ssl]                                          | The 85th percentile describes existing operating behaviour, is one input and does not by itself set the posted speed; the national MUTCD is the 11th Edition with Revision 1, December 2025; the sentence Item 2's percentile paragraph lacks                                                                                                                                                                                                                                                                                                                                    | Read                      |

[brake]: https://podcasts.apple.com/us/podcast/the-real-reasons-drivers-speed-rebecca-steinbach/id1610029625?i=1000793374806
[aaafts-speed-pdf]: https://aaafoundation.org/wp-content/uploads/2026/08/202608_AAAFTS-Attitudes-towards-Speeding.pdf
[aaafts-speed-states]: https://aaafoundation.org/wp-content/uploads/2026/08/202608-AAAFTS-Attitudes-Toward-Speeding-State-Level-Estimates.pdf
[aaafts-aggr-pdf]: https://aaafoundation.org/wp-content/uploads/2026/01/202509-AAAFTS-Aggressive-Driving.pdf
[aaafts-speed]: https://aaafoundation.org/research/attitudes-toward-speeding-insights-from-focus-groups-and-a-national-survey-of-u-s-drivers/
[aaafts-aggr]: https://aaafoundation.org/research/aggressive-driving-and-road-rage-2/
[aaa-speed-release]: https://newsroom.aaa.com/2026/08/new-research-finds-most-drivers-think-speeding-is-normal-putting-everyone-at-risk/
[aaa-aggr-release]: https://newsroom.aaa.com/2025/09/study-finds-almost-all-drivers-experience-road-rage-but-it-can-be-stopped/
[aaa-ne]: https://magazine.northeast.aaa.com/daily/newsroom/aaa-study-finds-96-of-drivers-admit-to-driving-aggressively/
[consumeraffairs]: https://www.consumeraffairs.com/news/new-aaa-study-finds-nearly-all-drivers-engage-in-road-rage-092325.html
[cbs-detroit]: https://www.cbsnews.com/detroit/news/aaa-study-driving-say-speeding-normal/
[iihs]: https://www.iihs.org/topics/speed
[tsci-2025]: https://aaafoundation.org/wp-content/uploads/2026/05/202606-AAAFTS-2025-TSCI.pdf
[nssab]: https://rosap.ntl.bts.gov/view/dot/78928
[nhtsa-813823]: https://crashstats.nhtsa.dot.gov/Api/Public/ViewPublication/813823
[campolettano-tip]: https://waymo.com/research/potential-safety-benefits-associated-with-speed-limit-compliance-in-san/
[ite-ssl]: https://www.ite.org/technical-resources/topics/speed-management-for-safety/setting-speed-limits/
[rb-2026-10]: ../lidar/operations/brief/research-briefing-2025-01-to-2026-10.md

## Checklist

### Complete

- [x] Both report summaries read and recorded; press figures tabled with their status
- [x] Technical reports read from the owner's extractions: the attitudes report's methods and results through its attitudinal groups, the aggressive-driving report's definitions; press figures replaced where the text carries the number
- [x] The behaviour vocabulary mapped to the behaviour plan's feature matrix and to what the sensor cannot see
- [x] Terminology contract: what transfers to docs, what reaches a page, and the V12 collision
- [x] The attitudes technical report read in full (October 12, 2026): the five mindsets, the Discussion, Figure 16 and Appendix D summarised, ten internal inconsistencies recorded, and every figure the plan takes from it checked against the text
- [x] The lead author's interview on the five mindsets summarised with timestamps; its transcript archived outside the repository

### Outstanding

- [ ] Item 1: the aggressive-driving report's unextracted parts read, every **Confirm** replaced, rights settled, `references.bib` entries; the attitudes report read in full on October 12, 2026 (`S`)
- [ ] Item 2: one wording guard with `speeder` and a word-bounded `rage`, the percentile copy rewritten, a guard test over the speed report (`S`)
- [ ] Item 3a: pace line and limit-relative shares per period and stratum, the operator-stated road type, the cosine-corrected speed expression, tests (`M`)
- [ ] Item 3b: `surveys` group in the edition and the cited attitudes paragraph, tests (`S`)
- [ ] Item 4: guide section and plan cross-references (`S`)

### Deferred

- [ ] Item 5: shares per limit segment and schedule, tracked by [posted-speed-limits-plan](posted-speed-limits-plan.md)
- [ ] Research note: the pace against the posted limit by road type across sites, in `data/explore/`, after the posted limits plan (`S`)
- [ ] Research note: a shoulder in the upper tail at 10 to 12 mph over the limit as an enforcement-expectation signature, in `data/explore/` (`S`)
- [ ] LiDAR `speed_relative_to_stream`, tracked by the [behaviour analytics plan](lidar-behaviour-analytics-plan.md)

### Accepted residuals (no action planned)

- [ ] Mindset classification of a street's traffic: not possible, the sensor has no people
- [ ] Honking, gestures, confrontation and the other unobservable behaviours: not observable by design
- [ ] Countermeasure recommendations in a report: outside the product's stance
- [ ] Self-reported prevalence as a calibration target for observed shares: different populations and units
      [d28]: ../DECISIONS.md#d-28--research-briefing-decisions-october-2026
