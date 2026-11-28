#!/usr/bin/env python3
"""Bake the reviewed westbound Macalla exit into its owning SF game tile."""

import argparse
from pathlib import Path

from apply_road_contact_update import bake_contact_update


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for argument in ("parent", "contact", "output"):
        parser.add_argument(argument, type=Path)
    args = parser.parse_args()
    print(bake_contact_update(
        args.parent,
        args.contact,
        args.output,
        road_ids={"sf-contact/yerba-buena-east-westbound-off-ramp"},
        token="80858024",
        source_key="macalla_westbound_exit_survey",
        receipt_key="macallaWestboundExitSurvey",
    ))


if __name__ == "__main__":
    main()
