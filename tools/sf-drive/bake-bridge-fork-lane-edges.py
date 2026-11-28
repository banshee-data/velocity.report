#!/usr/bin/env python3
"""Apply the SoMa and Alexander pavement reviews to the game tiles."""

import argparse
from pathlib import Path
from tempfile import TemporaryDirectory

from apply_road_contact_update import bake_contact_update


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    for name in ("parent", "soma_contact", "alexander_contact", "output"):
        cli.add_argument(name, type=Path)
    args = cli.parse_args()
    with TemporaryDirectory(dir=args.output.parent) as directory:
        soma = Path(directory) / "soma"
        bake_contact_update(
            args.parent, args.soma_contact, soma,
            road_ids=["fremont-exit", "sf-contact/i80-eastbound-lower-climb"],
            token="8085807c",
            source_key="soma_fork_pavement_survey",
            receipt_key="somaForkPavementSurvey",
        )
        print(bake_contact_update(
            soma, args.alexander_contact, args.output,
            road_ids=["sf-contact/us101-south-alexander-exit", "sf-contact/us101-north-alexander-exit"],
            token="8085842c",
            source_key="alexander_exit_pavement_survey",
            receipt_key="alexanderExitPavementSurvey",
        ))


if __name__ == "__main__":
    main()
