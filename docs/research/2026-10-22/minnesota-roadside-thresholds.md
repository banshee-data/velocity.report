# Minnesota fixed-camera behavioral thresholds

**Evidence:** Shaer, Fielbaum and Levinson, *Journal of Transportation Safety & Security* (2026). The briefing describes 258 hours at 31 Minnesota road segments and historical crashes, using operational definitions: speeding ≥10 km/h above posted limit, short THW <2 s, and critical TTC <2 s. **Bibliographic record and full-text table need independent verification before public publication.**

These are *study-specific event screens*, not universally validated unsafe cutoffs. The reported study context is straight road segments, dry conditions and uncongested traffic; these inclusion rules materially constrain comparisons with urban San Francisco intersections.

**Implementation assessment:** speeding requires a time-valid posted speed limit and valid passage speed; THW requires correctly assigned same-path leader and follower plus bumper gap; TTC additionally requires reliable closing velocity. Distinguish events, time-at-risk, and eligible opportunity denominators. Do not label an episode aggressive merely because one threshold is crossed.

**Next experiment:** reproduce all three screens on labeled fixed-sensor episodes; compare sensitivity to thresholds and confidence/occlusion filters. Store threshold profile identifier, citation, revision, units and exact eligibility. No production threshold changes in this PR.
