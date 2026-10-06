# Resolution-limited vehicle taxonomy and explainable retrieval

- **Status:** Proposal Math (Not Active in Current Runtime)
- **Related:**

- [Vehicle taxonomy](../../../docs/lidar/architecture/vehicle-taxonomy.md): the taxonomy this note derives
- [Classification maths](../classification-maths.md): the road-user classifier this sits beneath
- [Clustering maths](../clustering-maths.md): cluster geometry the descriptor is built from
- [Scene vehicle identification](../../../docs/plans/scene-vehicle-identification-plan.md): delivery plan

---

## 1. Problem statement

Naming the vehicle behind a cluster of returns has two properties that a
conventional classifier handles badly.

The answer space is large and unbalanced: thousands of catalogue entries, most
of which will never be observed, and no prospect of enough labelled examples per
entry to learn a decision boundary for each. And the achievable specificity is
not a property of the algorithm but of the instrument: at 40 m a Pandar40P
returns a handful of points from a vehicle, and no amount of modelling recovers
a distinction the returns do not contain.

This note sets out the retrieval formulation that addresses both, the
uncertainty model that makes its threshold meaningful, the decomposition that
makes each decision explainable without a post-hoc attribution method, and the
construction of a taxonomy whose depth is derived from measured resolving power
rather than asserted.

## 2. Notation

| Symbol                        | Meaning                                               | Unit     |
| ----------------------------- | ----------------------------------------------------- | -------- |
| $d$                           | Descriptor dimension count                            | -        |
| $x \in \mathbb{R}^d$          | Descriptor of an observed cluster                     | mixed    |
| $p \in \mathbb{R}^d$          | Descriptor prototype generated from a catalogue shell | mixed    |
| $r$                           | Range from sensor to object centroid                  | m        |
| $n$                           | Return count in the cluster                           | -        |
| $\sigma^{\text{meas}}_j(r,n)$ | Measurement standard deviation of dimension $j$       | dim unit |
| $\sigma^{\text{within}}_j$    | Within-model variation of dimension $j$               | dim unit |
| $\sigma^{\text{shell}}_j$     | Prototype error of dimension $j$                      | dim unit |
| $\tilde\sigma_j(r,n)$         | Total uncertainty of dimension $j$                    | dim unit |
| $D(x,p)$                      | Normalised squared distance                           | -        |
| $\tau$                        | Rejection threshold on $D$                            | -        |
| $k$                           | Depth in the taxonomy tree                            | -        |

## 3. The uncertainty model

Three independent contributions determine how precisely a dimension can be
compared. They combine in quadrature:

$$
\tilde\sigma_j^2(r,n) =
  \underbrace{\sigma^{\text{meas}}_j(r,n)^2}_{\text{sensor}}
+ \underbrace{\sigma^{\text{within}}_j{}^2}_{\text{the model varies}}
+ \underbrace{\sigma^{\text{shell}}_j{}^2}_{\text{the prototype is wrong}}
$$

Each term is separately measurable and separately meaningful.
$\sigma^{\text{meas}}$ is the spread of a dimension across repeated observations
of the same vehicle, binned by range and return count, taken from reviewed
masks. $\sigma^{\text{within}}$ is the spread of that dimension across real
examples of one catalogue entry: trim, load, wear, tyre choice.
$\sigma^{\text{shell}}$ is the residual between a prototype and dense
observations of the vehicle it claims to describe.

### 3.1 The resolution floor

Only the first term depends on range. As $r \to 0$,

$$
\tilde\sigma_j \to \sqrt{\sigma^{\text{within}}_j{}^2 + \sigma^{\text{shell}}_j{}^2}
$$

which is strictly positive. Identification therefore does **not** improve
without bound as a vehicle approaches: it approaches a floor set by how much a
model varies within itself and how accurate its shell is, neither of which is a
property of the sensor.

This is the note's most useful prediction, and it is falsifiable. It says the
taxonomy has a maximum useful depth even at zero range, that this depth is a
property of the catalogue rather than the instrument, and that improving the
sensor past a certain point buys nothing without also improving the shells.

## 4. Distance

$$
D(x,p;r,n) = \sum_{j=1}^{d} \left( \frac{x_j - p_j}{\tilde\sigma_j(r,n)} \right)^2
$$

A Mahalanobis distance with a diagonal covariance. Two choices in that sentence
carry weight.

**There are no free weights.** A per-dimension weight and a per-dimension
tolerance are the same parameter written twice: dividing by $\tilde\sigma_j$ and
multiplying by $w_j$ are interchangeable. Writing it as an uncertainty rather
than a weight matters because an uncertainty is a _measurement_ with a unit and
a procedure, while a weight is a preference. The $d$ numbers the system needs
are variances of physical quantities, obtainable with a tape measure and a
corpus, not coefficients fitted to make a score come out right.

**The covariance is diagonal on purpose.** A full covariance is more accurate
and destroys the explanation: with off-diagonal terms, one dimension's
contribution depends on every other dimension's residual and no per-dimension
sentence exists. What is given up is the correlation between descriptor
dimensions. That cost should be measured rather than assumed, and if it is
large the correct response is to choose a less redundant descriptor, not to
adopt a dense metric.

## 5. Explainability

### 5.1 Attribution is exact, not estimated

$D$ is a sum of $d$ non-negative terms. The share of the mismatch attributable
to dimension $j$ is therefore

$$
c_j = \frac{1}{D}\left( \frac{x_j - p_j}{\tilde\sigma_j} \right)^2, \qquad \sum_{j=1}^{d} c_j = 1
$$

**Explainer point 1.** No attribution method is required. Techniques such as
SHAP and LIME exist because a model's output is not decomposable into
per-feature contributions, so an approximation has to be constructed after the
fact. Here the decomposition _is_ the model, evaluated term by term. The
explanation and the computation are the same arithmetic, which means an
explanation cannot drift from the decision it describes.

### 5.2 The margin says which dimension decided

With best match $p^{(1)}$ and runner-up $p^{(2)}$, the margin is

$$
\Delta = D(x,p^{(2)}) - D(x,p^{(1)}) = \sum_{j=1}^{d} \delta_j,
\qquad
\delta_j = \frac{(x_j - p^{(2)}_j)^2 - (x_j - p^{(1)}_j)^2}{\tilde\sigma_j^2}
$$

**Explainer point 2.** The choice between two candidates decomposes the same
way, and a signed $\delta_j$ identifies which dimension favoured which
candidate. That yields a sentence a member of the public can check: "it is A
rather than B because the roofline break sits 0.4 m further forward, which is
3.1 standard deviations at this range and accounts for 78% of the margin."

### 5.3 The threshold has a meaning

Under a correct match with the uncertainty model of section 3, each normalised
residual is approximately standard normal, so

$$
D \sim \chi^2_d, \qquad \tau = F^{-1}_{\chi^2_d}(1-\alpha)
$$

**Explainer point 3.** The rejection threshold is stated as a false-rejection
rate rather than a tuned constant. "Reject above the 99th percentile of $\chi^2$
with 18 degrees of freedom" is checkable; a bare number is not.

**And the discrepancy is a diagnostic.** The residuals are not exactly
independent, not exactly unbiased, and not Gaussian at low return counts, so the
empirical threshold calibrated against reviewed masks will differ from the
$\chi^2$ value. The size of that gap measures how wrong the uncertainty model
is. A large gap is not a reason to abandon $\chi^2$ and tune a constant: it is
evidence that one of the three terms in section 3 is mis-measured, and it points
at which.

### 5.4 Published depth is the minimum of two measured limits

$$
k^*(x,r) = \min\big( k_{\text{res}}(x,r),\; k_{\text{floor}}(\cdot) \big)
$$

where $k_{\text{res}}$ is the deepest node whose match survives rejection and
$k_{\text{floor}}$ is the deepest node whose local registration count reaches
the anonymity threshold.

**Explainer point 4.** The published answer is the lesser of what the instrument
can support and what the population can hide. Both are numbers with units and
procedures behind them. Neither is a policy switch, and neither can be relaxed
by a preference.

## 6. Resolvability and the tree

### 6.1 Indistinguishability

Two prototypes are indistinguishable at range $r$ when their own separation
falls inside the tolerance:

$$
p \sim_r q \iff D(p,q;r) < \tau
$$

Note $\tau$ does not depend on $r$: the range dependence lives entirely in
$\tilde\sigma_j(r)$, which is what normalising the distance buys.

### 6.2 Monotone merging

$\sigma^{\text{meas}}_j$ is non-decreasing in $r$, so $\tilde\sigma_j$ is
non-decreasing and $D(p,q;r)$ is non-increasing in $r$ for every pair. Hence

$$
r_1 < r_2 \implies \{(p,q) : p \sim_{r_1} q\} \subseteq \{(p,q) : p \sim_{r_2} q\}
$$

Pairs merge as range grows and never separate again. This holds whatever the
per-dimension degradation profile looks like, which is why it can be relied on.

### 6.3 Building the tree

Cluster on $\sqrt{D}$ rather than $D$: in the scaled coordinates
$u_j = p_j / \tilde\sigma_j$, $\sqrt{D}$ is ordinary Euclidean distance and
therefore a proper metric, while $D$ itself violates the triangle inequality.

Use **complete linkage**. The semantics required is "every pair within this
cluster is indistinguishable", which complete linkage guarantees and single
linkage does not: under single linkage, $p \sim q$ and $q \sim s$ merges
$\{p,q,s\}$ even when $p$ and $s$ are plainly distinguishable, producing classes
whose members can be told apart.

### 6.4 The nesting assumption, and what to do if it fails

Section 6.2 guarantees the pairwise relation nests. It does not by itself
guarantee that complete-linkage _partitions_ at different ranges nest, because
$\tilde\sigma_j(r)$ rescales each axis by a different factor and so changes the
geometry of the space, not merely its scale.

Partitions nest automatically when degradation is uniform, that is when
$\tilde\sigma_j(r) = \tilde\sigma_j(r_0)\,g(r)$ for a single scalar $g$. Then a
change of range is a uniform scaling, one dendrogram serves every range, and the
cut height alone selects the depth. Whether that holds is an empirical question
and is worth measuring early, because the convenient construction depends on it.

Where it does not hold, do not assume it. Build the tree **top-down**:
partition at the coarsest range bin first, then subdivide only within each
parent at the next bin. Nesting then holds by construction, at the cost of
occasionally splitting a group the bottom-up dendrogram would have kept
together. Build bottom-up to discover the structure; enforce top-down to
guarantee the tree property.

### 6.5 Conservation

Because the partitions nest, counts at depth $k$ sum exactly to counts at depth
$k-1$. A population chart drawn at any depth is internally consistent with one
drawn at any other, and observations resolved to different depths aggregate by
rolling up to their coarsest common ancestor. Averaging across depths without
rolling up is the error this property exists to prevent.

## 7. Assumptions and failure modes

| Assumption                                   | If it fails                                                                            | Detection                                                          |
| -------------------------------------------- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| Descriptor dimensions are weakly correlated  | Diagonal covariance understates distance; $D$ is optimistic                            | Empirical threshold far below the $\chi^2$ value                   |
| Residuals are unbiased                       | A systematic shell error biases every observation of that model                        | Per-model residual mean is non-zero across many observations       |
| Synthetic prototypes match real observations | $\sigma^{\text{shell}}$ dominates and the catalogue stops supplying prototypes cheaply | Prototype-to-observation residual exceeds within-model spread      |
| Degradation is uniform across dimensions     | Partitions at different ranges do not nest                                             | Direct check: cluster per bin and test containment                 |
| The full silhouette is observed              | Occlusion truncates $x$; a partial vehicle matches a shorter prototype                 | Compare only observed stations, and reject on low visible fraction |
| Return count is sufficient                   | $\sigma^{\text{meas}}$ is unestimable and $D$ is meaningless                           | Floor on $n$, itself a function of $r$                             |

Occlusion is the one most easily underestimated. At a junction vehicles hide one
another constantly, and a truncated silhouette is a _confident_ match to a
shorter vehicle unless the sum in section 4 runs only over dimensions the sensor
could actually observe, with $d$ reduced accordingly in the threshold.

## 8. What this does not claim

It does not claim the descriptor is discriminative enough to reach any
particular depth. That is a measurement, and a shallow answer is a legitimate
result rather than a failure: publishing the resolvable bucket count per range
is the honest output whatever it turns out to be.

It does not claim the tree is a natural or semantic taxonomy. It is the
partition the instrument can justify. Human names attached to its nodes are an
editorial act performed afterwards, and a node that resists naming is evidence
about the descriptor, not a naming problem.
