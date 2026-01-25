#!/usr/bin/env python3
"""
order-schema-tables.py - Reorder SQL schema tables by foreign key dependencies

This script reads a SQL schema file and reorders CREATE TABLE statements
so that tables are defined before any tables that reference them via foreign keys.

Usage:
    python3 order-schema-tables.py <schema_file>

The script:
1. Parses CREATE TABLE statements and their foreign key references
2. Builds a dependency graph
3. Performs topological sort to determine correct order
4. Outputs reordered schema to stdout

Exit codes:
    0 - Success
    1 - Error (circular dependencies, parse error, etc.)
"""

import re
import sys
from typing import Dict, List, Set, Tuple


def extract_table_name(create_statement: str) -> str:
    """Extract table name from CREATE TABLE statement."""
    # Match: CREATE TABLE [IF NOT EXISTS] ["table_name" | table_name]
    match = re.search(
        r'CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+(?:"([^"]+)"|(\w+))',
        create_statement,
        re.IGNORECASE,
    )
    if match:
        return match.group(1) or match.group(2)
    return None


def extract_foreign_keys(create_statement: str) -> List[str]:
    """Extract referenced table names from FOREIGN KEY clauses."""
    # Match: FOREIGN KEY (...) REFERENCES ["]table_name["] (...)
    pattern = r'FOREIGN\s+KEY\s*\([^)]+\)\s*REFERENCES\s+(?:"([^"]+)|(\w+))'
    matches = re.finditer(pattern, create_statement, re.IGNORECASE)
    return [match.group(1) or match.group(2) for match in matches]


def parse_schema(schema_content: str) -> Tuple[Dict[str, str], Dict[str, List[str]]]:
    """
    Parse schema file into table definitions and dependencies.

    Returns:
        Tuple of (table_statements, dependencies) where:
        - table_statements: Dict mapping table name to full CREATE TABLE statement
        - dependencies: Dict mapping table name to list of tables it depends on
    """
    table_statements = {}
    dependencies = {}

    # Split on CREATE TABLE to get individual statements
    # Keep everything from CREATE TABLE until the next CREATE (TABLE, INDEX, TRIGGER) or end
    parts = re.split(
        r'(?=CREATE\s+(?:TABLE|INDEX|TRIGGER|UNIQUE\s+INDEX))',
        schema_content,
        flags=re.IGNORECASE,
    )

    for part in parts:
        part = part.strip()
        if not part:
            continue

        # Only process CREATE TABLE statements
        if not re.match(r'CREATE\s+TABLE', part, re.IGNORECASE):
            continue

        table_name = extract_table_name(part)
        if not table_name:
            continue

        # Store the full statement (up to and including the closing semicolon)
        # Find the end of the CREATE TABLE statement (before any CREATE INDEX/TRIGGER)
        match = re.search(
            r'(CREATE\s+TABLE.*?;\s*)',
            part,
            re.IGNORECASE | re.DOTALL,
        )
        if match:
            table_statement = match.group(1)
            table_statements[table_name] = table_statement
            dependencies[table_name] = extract_foreign_keys(table_statement)

    return table_statements, dependencies


def topological_sort(
    table_statements: Dict[str, str], dependencies: Dict[str, List[str]]
) -> List[str]:
    """
    Perform topological sort on tables based on foreign key dependencies.

    Returns:
        List of table names in dependency order (parents before children)
    """
    # Build in-degree map (number of dependencies for each table)
    in_degree = {table: 0 for table in table_statements}
    adj_list = {table: [] for table in table_statements}

    for table, deps in dependencies.items():
        for dep in deps:
            # Only count dependencies that exist in our schema
            if dep in table_statements:
                in_degree[table] += 1
                adj_list[dep].append(table)

    # Start with tables that have no dependencies
    queue = [table for table, degree in in_degree.items() if degree == 0]
    result = []

    while queue:
        # Sort queue for deterministic output
        queue.sort()
        table = queue.pop(0)
        result.append(table)

        # Reduce in-degree for dependent tables
        for dependent in adj_list[table]:
            in_degree[dependent] -= 1
            if in_degree[dependent] == 0:
                queue.append(dependent)

    # Check for circular dependencies
    if len(result) != len(table_statements):
        remaining = set(table_statements.keys()) - set(result)
        print(
            f"Error: Circular dependency detected among tables: {remaining}",
            file=sys.stderr,
        )
        sys.exit(1)

    return result


def reorder_schema(schema_content: str) -> str:
    """
    Reorder schema tables by foreign key dependencies.

    Returns:
        Reordered schema content
    """
    # Parse schema
    table_statements, dependencies = parse_schema(schema_content)

    # Get ordered table names
    ordered_tables = topological_sort(table_statements, dependencies)

    # Extract non-table statements (indexes, triggers, etc.)
    non_table_parts = []
    parts = re.split(
        r'(?=CREATE\s+(?:TABLE|INDEX|TRIGGER|UNIQUE\s+INDEX))',
        schema_content,
        flags=re.IGNORECASE,
    )

    for part in parts:
        part = part.strip()
        if not part:
            continue

        # Skip CREATE TABLE statements (we'll add them in order)
        if re.match(r'CREATE\s+TABLE', part, re.IGNORECASE):
            continue

        non_table_parts.append(part)

    # Build output: ordered CREATE TABLE statements followed by other statements
    output_parts = []

    # Add ordered CREATE TABLE statements
    for table in ordered_tables:
        output_parts.append(table_statements[table])

    # Add indexes and triggers
    output_parts.extend(non_table_parts)

    return '\n'.join(output_parts)


def main():
    if len(sys.argv) != 2:
        print("Usage: python3 order-schema-tables.py <schema_file>", file=sys.stderr)
        sys.exit(1)

    schema_file = sys.argv[1]

    try:
        with open(schema_file, 'r') as f:
            schema_content = f.read()
    except FileNotFoundError:
        print(f"Error: File not found: {schema_file}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error reading file: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        reordered_schema = reorder_schema(schema_content)
        print(reordered_schema)
    except Exception as e:
        print(f"Error reordering schema: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
