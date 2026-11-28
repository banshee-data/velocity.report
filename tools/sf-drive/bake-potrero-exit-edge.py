#!/usr/bin/env python3
"""Apply the reviewed pavement edge to the northbound Potrero exit."""

import argparse
from pathlib import Path

from apply_road_contact_update import bake_contact_update


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    for name in ("parent", "contact", "output"):
        cli.add_argument(name, type=Path)
    args = cli.parse_args()
    print(bake_contact_update(
        args.parent,
        args.contact,
        args.output,
        road_ids=["sf-contact/us101-north-potrero-exit"],
        token="808f7e34",
        source_key="potrero_exit_edge_survey",
        receipt_key="potreroExitEdgeSurvey",
    ))


if __name__ == "__main__":
    main()
