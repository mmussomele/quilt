#!/usr/bin/env python3

import argparse
from functools import reduce
import os
import os.path
import signal
import shlex
import subprocess
import sys
import time

def run_parser():
    parser = argparse.ArgumentParser(description="run the scale tests", usage="%(prog)s run [-h] [-start N] [-factor M]")
    parser.add_argument('--start', default=100, type=int)
    parser.add_argument('--factor', default=1.5, type=float)
    parser.add_argument('--fullmesh', default=False, type=bool)
    return parser

PREBOOT_SPEC = "./plot/preboot-scale.spec"
BOOT_SPEC = "./plot/scale.spec"
FULL_MESH_BOOT_SPEC = "./plot/scale-full-mesh.spec"
POSTBOOT_SPEC = "./plot/postboot-scale.spec"
FULL_MESH_POST_SPEC = "./plot/post-scale-full-mesh.spec"
OUT_FILE = "./tmp/scale-output"
LOG_FILE = "./tmp/scale-logs"
SCALE_CMD = """\
scale -preboot-stitch={0}.tmp \
-stitch={1}.tmp \
-postboot-stitch={2}.tmp \
-out-file={3} \
-log-file={4} \
-append -nostop \
"""

DEFAULT_CMD = SCALE_CMD.format(PREBOOT_SPEC, BOOT_SPEC, POSTBOOT_SPEC, OUT_FILE, LOG_FILE)
FULL_MESH_CMD = SCALE_CMD.format(PREBOOT_SPEC, FULL_MESH_BOOT_SPEC, FULL_MESH_POST_SPEC, OUT_FILE, LOG_FILE)

def exp_iter(start, factor):
    while True:
        yield int(start)
        start *= factor

def make_scale_process(count, fullmesh, arg):
    for spec in [PREBOOT_SPEC, FULL_MESH_BOOT_SPEC, FULL_MESH_POST_SPEC, BOOT_SPEC, POSTBOOT_SPEC]:
        with open(spec, "r") as read_file, open(spec + ".tmp", "w") as write_file:
            write_file.write(read_file.read(-1).format(str(count)))

    if fullmesh:
        return subprocess.Popen(shlex.split(FULL_MESH_CMD + arg))
    return subprocess.Popen(shlex.split(DEFAULT_CMD + arg))

def run_process(count, fullmesh, arg=""):
    scale_process = make_scale_process(count, fullmesh, arg)
    try:
        returncode = scale_process.wait()
        if returncode != 0:
            print("The scale tester exited with an error. Trying again")
            time.sleep(30)
            run_process(count, fullmesh, arg)
    except KeyboardInterrupt:
        scale_process.terminate()
        sys.exit(0)

def run_scale(args):
    options = run_parser().parse_args(args)
    bootcounts = exp_iter(options.start, options.factor)
    run_process(1, False) # boot one container to ensure that the machines are fully booted
    if os.path.exists(LOG_FILE):
        os.remove(LOG_FILE)
    for count in bootcounts:
        run_process(count, options.fullmesh, "-ip-only")

def run(args):
    prog_name = args[0]
    args = args[1:]
    run_scale(args)

if __name__ == '__main__':
    run(sys.argv)
