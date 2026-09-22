# Identifiability: what a published observation actually reveals

- **Status:** Proposed analysis. Illustrative arithmetic, no measured parameters
- **Layers:** L9 endpoints (export), reporting, public web site
- **Related:** [vehicle encyclopedia](vehicle-encyclopedia.md), [vehicle taxonomy](../../lidar/architecture/vehicle-taxonomy.md), [TENETS](../../../TENETS.md)

Publishing what passed a kerb is a privacy question with a numeric answer, not a
prohibition. This note replaces "never name a model" with a model of who can be
singled out, by whom, and under what conditions. The conclusion is that class
granularity is the binding control in one deployment and barely matters in
another, which a blanket rule would have hidden.

---

## 1. Why a rule was the wrong shape

The earlier position was that a published scene must never carry a make and
model. It is defensible and it is too crude in both directions. It forbids
things that are plainly safe, such as naming a common model among ten thousand
passes, and it permits things that are not, such as publishing a timestamped
pass on a street where four vehicles an hour go by.

The risk is not a property of the label. It is a property of the **anonymity
set**: how many real vehicles could have produced what was published. That is a
number, it depends on traffic volume, class rarity, time resolution and
repetition, and it can be estimated per site.

## 2. The threat that matters

The realistic adversary is a neighbour, a landlord, an ex-partner or a curious
member of the public who already knows what somebody drives. Not a state
actor and not a data broker: those have better sources.

That shapes everything. The adversary has strong side information about _one_
vehicle and wants to confirm or track it. They do not need to identify everyone;
they need one match. The design cannot defeat somebody who already knows the
target's car and their schedule, and should not pretend to. What it can do is
refuse to hand them the confirmation for free, and refuse to make tracking
easier than standing at a window.

## 3. Two deployments, two exposures

|                                   | Survey                           | Continuous                      |
| --------------------------------- | -------------------------------- | ------------------------------- |
| Pattern                           | About 20 minutes, irregular days | Always on, typically a home     |
| Sees a daily commuter             | Occasionally, by chance          | Every day                       |
| Observations linkable across time | Essentially not                  | Trivially                       |
| Subject knows they were observed  | Sometimes                        | Rarely                          |
| Binding privacy control           | Class granularity                | Time resolution and aggregation |

These are not two settings on a slider. They are different problems, and the
rest of this note treats them separately.

## 4. A single observation

Let $N$ be the passes in a published window and $p_c$ the share of local traffic
in class $c$. The expected number of class-$c$ passes is $N p_c$, and that is the
anonymity set for one sighting: the number of passes a reader cannot tell apart
from the one they are looking at.

Require $N p_c \ge k$ for a chosen $k$. If classes subdivide by a branching
factor $b$ as depth increases, so that $p_c \approx b^{-d}$, then

$$
d_{\max} \approx \log_b \left( \frac{N}{k} \right)
$$

**Publishable depth grows logarithmically with traffic volume.** A hundredfold
difference in volume buys roughly three and a half extra levels at $b = 3.5$.

### Worked illustration

Assumed class shares by depth, which are a guess pending real fleet data:
25%, 8%, 2%, 0.4%, 0.08%. Expected matching passes in the window:

| Depth        | Share | Quiet, $N=100$ | Moderate, $N=1000$ | Busy, $N=10{,}000$ |
| ------------ | ----- | -------------- | ------------------ | ------------------ |
| 1 gross form | 25%   | 25             | 250                | 2500               |
| 2 silhouette | 8%    | 8              | 80                 | 800                |
| 3 profile    | 2%    | **2**          | 20                 | 200                |
| 4 family     | 0.4%  | 0.4            | **4**              | 40                 |
| 5 entry      | 0.08% | 0.08           | 0.8                | **8**              |

At $k=5$: a quiet street supports depth 2, a moderate street depth 3 to 4, and a
busy street reaches depth 5. On a genuinely busy road, publishing a common
model against a single pass is not the disclosure the earlier rule assumed it
was: eight other passes that hour looked identical.

Two cautions. $N$ is the count in the **published window**, so a scene covering
twenty minutes of a busy road is not a busy-road sample. And the expectation is
not the realisation: a Poisson count with mean 8 is below 4 about one time in
twenty, so the rule should be applied to the realised count in the artefact
being published, not to the average.

## 5. Repeated observations

This is where a rule about labels stops helping.

Suppose $|A_c|$ vehicles of class $c$ plausibly use the street, each appearing on
a given day with probability $q$. The number appearing on all $M$ published days
is about $|A_c| q^M$. Requiring $k$ survivors:

$$
M_{\max} \approx \frac{\ln |A_c| - \ln k}{-\ln q}
$$

| $|A_c|$ | $q$ | $M_{\max}$ at $k=5$ |
| ------- | --- | ------------------- |
| 50 | 0.3 | 1.9 days |
| 200 | 0.3 | 3.1 days |
| 200 | 0.6 | 7.2 days |
| 2000 | 0.3 | 5.0 days |
| 2000 | 0.6 | 11.7 days |

**Anonymity collapses geometrically with repetition and only logarithmically
with class size.** Coarsening the class buys days; repetition spends them. Under
generous assumptions a continuously published, timestamped per-pass stream
singles out regular users inside a fortnight, whatever the label says.

This is the composition problem that differential privacy describes formally:
each release leaks a little, and the leaks add up. The practical reading needs
no formalism. A survey spends almost nothing from that budget. Continuous
publication spends it every day.

## 6. When the timestamp is the identifier

The sharper version. An adversary looking for a commuter does not search the
whole day, they search a ten-minute window. Assume a fifth of daily volume falls
in a two-hour peak, and take a ten-minute slice of it:

| Street   | $N$/day | Peak 2 h | 10-min window  | Depth 1 matches | Depth 2 matches |
| -------- | ------- | -------- | -------------- | --------------- | --------------- |
| Quiet    | 100     | 20       | **1.7 passes** | 0.42            | 0.13            |
| Moderate | 1000    | 200      | 17 passes      | 4.2             | 1.3             |
| Busy     | 10,000  | 2000     | 167 passes     | 42              | 13              |

On a quiet residential street, fewer than two vehicles pass in the ten minutes
around any given time. **The timestamp alone identifies; the class adds almost
nothing to a risk that is already total.** Publishing "a vehicle passed at 08:12"
with no class at all is enough, because the reader already knows whose car is on
the road at 08:12.

That inverts the design for a home deployment. Arguing about class depth there
is arguing about the wrong control. What matters is whether a timestamped
per-pass record is published at all, and at what time resolution.

## 7. Which control binds where

| Deployment | Street | Binding control       | Practical rule                                                                            |
| ---------- | ------ | --------------------- | ----------------------------------------------------------------------------------------- |
| Survey     | Busy   | Class depth           | Per-pass class at the depth the realised count supports                                   |
| Survey     | Quiet  | Class depth, and time | Per-pass class, coarsened time, or no per-pass times                                      |
| Continuous | Busy   | Aggregation           | No per-pass stream; periodic aggregates, any depth with cell suppression                  |
| Continuous | Quiet  | Aggregation and time  | Aggregates only, over periods long enough that a single regular user does not move a cell |

The common thread: what makes an observation dangerous is being **an event with
a time**, not being **a count with a label**.

## 8. Reports and scenes are different artefacts

This is the distinction that gives the per-model breakdown back.

|                          | A labelled pass                     | A counted period               |
| ------------------------ | ----------------------------------- | ------------------------------ |
| Unit                     | One event                           | One population                 |
| Carries a time           | Yes                                 | Only the period                |
| Linkable across releases | Yes                                 | No                             |
| Reveals                  | That _this_ vehicle was _here_ then | That the fleet looks like this |

A per-make-and-model breakdown in a report is a **fleet census**: "over thirty
days, 412 passes, of which 38 were Model A". Nobody is in it. There is no time to
link on and no pass to point at. That is a different artefact from a scene
carrying a model name against a moving box at 08:12, and it should be governed
differently.

So: **a report may carry model-level detail; a scene carries a class.** This is
the line worth holding, and it delivers most of what model-level detail was
wanted for, because the arguments that need model names are population arguments.

Standard statistical disclosure control applies to the report:

- Suppress cells below a threshold count, and suppress complements so a
  suppressed cell cannot be recovered by subtraction.
- Watch the time series of a cell, not just its value: a monthly count that
  reads 1, 1, 1, 1 is a regular user, and publishing it repeatedly is the
  linkage problem in aggregate clothing.
- Lengthen the period rather than suppressing the model, where possible. A year
  of a quiet street may support what a month does not.

## 9. A class code people can hold

A published class needs a name a reader can carry away. Rental category codes
are the right instinct: a few characters a traveller reads at a glance,
encoding size and body type.

They are the wrong vocabulary to adopt directly. Their categories are commercial
rather than physical, so the line between "premium" and "luxury" is a price tier
that no sensor can see and that means nothing for road safety. Half their
positions encode transmission and air conditioning. And they are somebody's
standard, which makes adoption a licensing question before it is a design one.

The [vehicle taxonomy](../../lidar/architecture/vehicle-taxonomy.md) specifies
the code this project uses instead: fixed-width, every position a property the
sensor measures, and an unresolved position written as a wildcard so the code
shows its own resolution. Crosswalks to rental categories, European segments and
the federal class definitions are published as approximate conveniences, because
a class derived from what a sensor resolves will not align exactly with one
derived from interior volume or from price.

## 10. The tradeoff: model-level detail

| Gained                                                             | Cost                                                              |
| ------------------------------------------------------------------ | ----------------------------------------------------------------- |
| Mass to a few per cent rather than a wide band, and energy with it | A rare model is identifying wherever it appears                   |
| The actual bonnet height, braking distance and loss record         | Model plus time plus place is a recognised quasi-identifier       |
| Rhetorical force: a named vehicle lands harder than a category     | Invites "that is my neighbour's car", which is the realistic harm |
| Accountability: manufacturers respond to being named               | A named model in a small community implies a named household      |

The band cost is the one to weigh carefully. A class spanning 2.0 to 3.0 tonnes
gives a kinetic energy estimate with roughly fifty per cent uncertainty, which is
wide enough to weaken the argument it exists to support. Three middle grounds
recover most of it without publishing an entry against a pass:

- **Attach the distribution, not the entry.** "Vehicles in this class here weigh
  2.0 to 3.0 tonnes, and the most common weighs 2.4" is a class-level statement
  carrying model-level information.
- **Narrow the class where it matters.** A size-banded class is much tighter
  than a body-form class, and depth 3 is often available where depth 5 is not.
- **Put the entry in the report.** Almost every argument that needs model names
  is a population argument, and the report is where population arguments live.

## 11. What this does not defend against

Somebody who already knows the target's vehicle and their routine. No
publication rule helps there, and an honest note should say so rather than
implying protection it cannot provide.

Nor does it address what an operator can see on their own device. This note is
about what is **published**. Local visibility is an operator's own business and
their own responsibility, and they should be told plainly that a continuously
recording sensor on a quiet street knows their neighbours' routines whether or
not anything is ever published.

## 12. Parameters to measure

Everything above is arithmetic over assumed numbers. Six quantities turn it into
an analysis.

| Symbol    | What                                             | How                                                      |
| --------- | ------------------------------------------------ | -------------------------------------------------------- |
| $N$       | Passes per period, per site                      | Already measured                                         |
| $p_c$     | Class share of local traffic                     | From the site's own observations, not a national average |
| $b$       | Branching factor of the class tree               | Falls out of the taxonomy once built                     |
| $|A_c|$   | Class-$c$ vehicles plausibly using the street    | Hardest of the six; bounded by local registrations       |
| $q$       | Per-day appearance probability of a regular user | Estimable from repeat structure in the site's own data   |
| $k$       | Chosen anonymity floor                           | A decision, informed by the above                        |

$q$ is measurable without identifying anybody: the repeat structure of anonymous
observations tells you how regular the traffic is, which is exactly the input
needed to decide how much regularity a publication would expose.

## 13. Open questions

1. What is $k$? It appears in every threshold and is still a decision, not a
   measurement. The measurements above inform it; they do not set it.
2. Is the per-pass time in a survey export needed at all? Playback needs
   relative timing, not absolute wall-clock time. If absolute times can be
   dropped or coarsened without hurting the scene, most of section 6 stops
   binding.
3. What period length makes a quiet-street aggregate safe? Section 8 says
   lengthen rather than suppress, but not by how much.
4. Should the publishable depth be computed per site and per artefact from the
   realised counts, rather than configured? Computed is right and needs a place
   to live.
5. Does an operator's own local view need any limits, or is disclosure enough?
