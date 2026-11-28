#!/usr/bin/env python3
"""Bake the reviewed eastbound Southgate exit into the Yerba Buena game tile."""

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
        road_ids={
            "yerba-buena-lower-tunnel-east-of-exit",
            "sf-contact/yerba-buena-east-eastbound-exit-tunnel",
            "sf-contact/yerba-buena-east-eastbound-off-ramp",
        },
        token="80858024",
        source_key="eastbound_southgate_mapped_exit_survey",
        receipt_key="eastboundSouthgateMappedExitSurvey",
    ))


if __name__ == "__main__":
    main()
