#!/usr/bin/env python3
"""Joins thin 64-bit Mach-O executables into one universal (fat) binary,
like Apple's lipo -create, so the Mac app can be built on any machine.

usage: lipo.py OUT IN...
"""
import struct
import sys

ALIGN = 14  # slices start on 16 KiB boundaries, as lipo does for arm64


def main():
    out, *ins = sys.argv[1:]
    slices = []
    for path in ins:
        data = open(path, "rb").read()
        if data[:4] != b"\xcf\xfa\xed\xfe":
            sys.exit(f"{path}: not a 64-bit Mach-O executable")
        cputype, cpusub = struct.unpack("<ii", data[4:12])
        slices.append((cputype, cpusub & 0xFFFFFF, data))

    page = 1 << ALIGN
    entries, body, pos = b"", b"", page
    for cputype, cpusub, data in slices:
        entries += struct.pack(">iiIII", cputype, cpusub, pos, len(data), ALIGN)
        pad = -len(data) % page
        body += data + b"\0" * pad
        pos += len(data) + pad
    head = struct.pack(">II", 0xCAFEBABE, len(slices)) + entries
    with open(out, "wb") as f:
        f.write(head + b"\0" * (page - len(head)) + body)


if __name__ == "__main__":
    main()
