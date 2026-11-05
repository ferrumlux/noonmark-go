#!/usr/bin/env python3
"""Stream stage output and synchronously retain only its bounded final bytes."""
import sys
from pathlib import Path

LIMIT = 262144
remaining = bytearray()
while block := sys.stdin.buffer.read1(65536):
    sys.stdout.buffer.write(block)
    sys.stdout.buffer.flush()
    remaining.extend(block)
    if len(remaining) > LIMIT:
        del remaining[:-LIMIT]
Path(sys.argv[1]).write_bytes(remaining)
