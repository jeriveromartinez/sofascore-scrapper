#!/usr/bin/env python3
import ctypes
import os
import sys


AT_FDCWD = -100
RENAME_EXCHANGE = 2


def main() -> int:
    if len(sys.argv) != 3:
        print(f"usage: {sys.argv[0]} <current> <replacement>", file=sys.stderr)
        return 2
    if sys.platform != "linux":
        print("atomic directory exchange requires Linux", file=sys.stderr)
        return 1

    libc = ctypes.CDLL(None, use_errno=True)
    renameat2 = libc.renameat2
    renameat2.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    renameat2.restype = ctypes.c_int

    current = os.fsencode(sys.argv[1])
    replacement = os.fsencode(sys.argv[2])
    if renameat2(AT_FDCWD, current, AT_FDCWD, replacement, RENAME_EXCHANGE) != 0:
        error = ctypes.get_errno()
        raise OSError(error, os.strerror(error), f"{sys.argv[1]} <-> {sys.argv[2]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
