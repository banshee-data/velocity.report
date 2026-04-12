# Track description language (TDL)

- **Status:** Proposed
- **Layers:** L8 Analytics

The Track Description Language (TDL) is a natural-language query interface that lets neighbourhood advocates describe traffic questions without writing SQL.

## Summary

The Track Description Language is a natural-language query interface over
the transit database. It allows users and report generators to express
questions like:

- "What percentage of eastbound vehicles exceed 30 mph between 07:00–09:00?"
- "Show transits where a vehicle passed within 1.5 m of a cyclist."
- "Average speed profile for vehicles classified as lorry."

TDL is not SQL. It uses human-readable terms grounded in traffic-engineering
vocabulary so that neighbourhood advocates and community groups can describe
what they want to see without learning a query language.

## Design decisions

| Decision          | Choice                                                | Rationale                                     |
| ----------------- | ----------------------------------------------------- | --------------------------------------------- |
| Syntax family     | Natural language                                      | Target users are change-makers, not engineers |
| Execution target  | Go query builder → parameterised SQLite               | Safe, parameterised SQL; no raw SQL exposed   |
| Schema exposure   | Abstract                                              | Users see domain concepts, not table names    |
| Aggregation model | Per-transit max speed + on-demand dataset percentiles | Percentiles reserved for grouped summaries    |

## History

- Full specification: [track-description-language plan](../../plans/data-track-description-language-plan.md)
