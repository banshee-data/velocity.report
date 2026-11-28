#!/usr/bin/env python3
"""Bake the reviewed Harrison loop pavement into its owning SF game tile."""

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
        road_id="sf-contact/i80-westbound-harrison-loop",
        token="8085807c",
        source_key="i80_harrison_loop_pavement_survey",
        receipt_key="i80HarrisonLoopPavementSurvey",
    ))


if __name__ == "__main__":
    main()
