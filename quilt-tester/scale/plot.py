#!/usr/bin/env python3

import argparse
import operator
import os
import os.path
import shlex
import subprocess
import sys
import time

def run_parser():
    parser = argparse.ArgumentParser(description="run the scale tests")
    parser.add_argument('--growth', default="exponential", type=str)
    parser.add_argument('--start', default=100, type=int)
    parser.add_argument('--modifier', default=1.5, type=float)
    parser.add_argument('--image', default='mmussomele/sleep', type=str)
    return parser

NAMESPACE = "scale-bd89e4c89f4d384e7afb155a3af99d8a6f4f5a06a9fecf0b6d220eb66e"
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
-out-file={2} \
-log-file={3} \
-quilt-log-file={4} \
-append -nostop \
"""
# -postboot-stitch={2}.tmp \

SWARM_CMD = """\
swarm -preboot-stitch={0}.tmp \
-image={1} \
-containers={2} \
-out-file={3} \
-log-file={4} \
-append -nostop \
"""

DEFAULT_CMD = SCALE_CMD.format(PREBOOT_SPEC, BOOT_SPEC, OUT_FILE + ".disconnect", LOG_FILE, LOG_FILE + "-quilt")
FULL_MESH_CMD = SCALE_CMD.format(PREBOOT_SPEC, FULL_MESH_BOOT_SPEC, OUT_FILE + ".connected", LOG_FILE, LOG_FILE + "-quilt")

def stop_namespace():
    quilt = subprocess.Popen(shlex.split("quilt daemon"))
    quilt_stop = subprocess.Popen(shlex.split("quilt stop {0}".format(NAMESPACE)))
    quilt_stop.wait()
    time.sleep(120)
    quilt.terminate()

def iter(start, modifier, op):
    while True:
        yield int(start)
        start = op(start, modifier)

def format_specs(count):
    for spec in [PREBOOT_SPEC, FULL_MESH_BOOT_SPEC, FULL_MESH_POST_SPEC, BOOT_SPEC, POSTBOOT_SPEC]:
        with open(spec, "r") as read_file, open(spec + ".tmp", "w") as write_file:
            write_file.write(read_file.read(-1).format(str(count)))

def make_scale_process(count, fullmesh, arg):
    if fullmesh:
        return subprocess.Popen(shlex.split(FULL_MESH_CMD + arg))
    return subprocess.Popen(shlex.split(DEFAULT_CMD + arg))

def make_swarm_process(count, image, arg):
    swarm_cmd_formatted = SWARM_CMD.format(PREBOOT_SPEC, image, count, OUT_FILE + ".swarm", LOG_FILE)
    return subprocess.Popen(shlex.split(swarm_cmd_formatted + arg))

def run_process(proc, count, opt, arg):
    format_specs(count)
    while True:
        process = proc(count, opt, arg)
        try:
            arg = ""
            returncode = process.wait()
            if returncode != 0:
                print("The scale tester exited with an error. Trying again.")
                stop_namespace() # stop the namespace so things restart
            else:
                return
        except KeyboardInterrupt:
            process.terminate()
            time.sleep(90) # wait to allow the process to finish its own shutdown
            sys.exit(0)

def run_test(count, image, arg=""):
    run_process(make_scale_process, count, True, "-ip-only") # run the full mesh test
    # run_process(make_swarm_process, count, image, "-ip-only") # we're pretty fast, so don't worry about swarm for now

def run_scale(args):
    options = run_parser().parse_args(args)
    if options.growth == "exponential":
        op = operator.mul
    elif options.growth == "linear":
        op = operator.add
    else:
        print("Unknown growth type: '{}'".format(options.growth))
        sys.exit(1)

    run_process(make_scale_process, 1, False, "")
    if os.path.exists(LOG_FILE):
        os.remove(LOG_FILE)
    for count in iter(options.start, options.modifier, op):
        run_test(count, options.image, "-ip-only")

def run(args):
    prog_name = args[0]
    args = args[1:]
    run_scale(args)

if __name__ == '__main__':
    run(sys.argv)
