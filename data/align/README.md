# Data Alignment Tools

This directory contains Python tools for aligning and processing velocity report data.

## Setup

1. Create a virtual environment:
```bash
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
```

2. Install the project (modern approach):
```bash
pip install -e .
```

3. Or install development dependencies:
```bash
pip install -e ".[dev]"
```

## Usage

```bash
python cctv.py
```

## Modern Python Project Structure

This project uses `pyproject.toml` (PEP 621) for dependency management - the modern standard that replaces `requirements.txt` and `setup.py`.
