#!/usr/bin/env python3
"""Bake both westbound Macalla landings into the Yerba Buena tile."""

import argparse
from pathlib import Path

from apply_road_contact_update import bake_contact_update

EXIT = "sf-contact/yerba-buena-east-westbound-off-ramp"
WEST = "sf-contact/yerba-buena-westbound-macalla-west-landing"
MAPPED = "sf-contact/yerba-buena-westbound-macalla-mapped-landing"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for argument in ("parent", "contact", "output"):
        parser.add_argument(argument, type=Path)
    args = parser.parse_args()
    print(bake_contact_update(
        args.parent,
        args.contact,
        args.output,
        road_ids={EXIT, WEST, MAPPED},
        token="80858024",
        source_key="macalla_mapped_landing_survey",
        receipt_key="macallaMappedLandingSurvey",
        new_connections=[(EXIT, WEST), (EXIT, MAPPED)],
    ))


if __name__ == "__main__":
    main()
