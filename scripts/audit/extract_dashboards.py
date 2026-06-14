#!/usr/bin/env python3
"""Extract distinct pflex_* metric names referenced across grafana/** dashboards.

Usage: python3 scripts/audit/extract_dashboards.py grafana
Prints one metric name per line, sorted, plus a per-file breakdown to stderr.
"""
import json
import os
import re
import sys

METRIC_RE = re.compile(r"pflex_[a-z0-9_]+")


def main(root):
    all_metrics = set()
    for dirpath, _, files in os.walk(root):
        for fn in files:
            if not fn.endswith(".json"):
                continue
            full = os.path.join(dirpath, fn)
            blob = open(full).read()
            found = set(METRIC_RE.findall(blob))
            if found:
                print(f"{full}: {len(found)} metrics", file=sys.stderr)
            all_metrics |= found
    for m in sorted(all_metrics):
        print(m)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "grafana")
