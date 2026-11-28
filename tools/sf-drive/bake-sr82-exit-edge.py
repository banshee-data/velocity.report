#!/usr/bin/env python3
"""Apply the reviewed pavement edge to the southbound I-280 exit to SR 82."""

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
        road_ids=["sf-contact/i280-south-sr82-exit"],
        token="808f7c24",
        source_key="sr82_exit_edge_survey",
        receipt_key="sr82ExitEdgeSurvey",
    ))


if __name__ == "__main__":
    main()
