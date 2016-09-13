#!/usr/bin/env python3

import argparse
import csv
import math
import os
import os.path
import re
import sys

import matplotlib
matplotlib.use('PDF')
matplotlib.rcParams['text.usetex'] = True
matplotlib.rcParams.update({'font.size': 20, 'font.family': 'serif'})
import matplotlib.pyplot as plt
import matplotlib.lines as mlines

TIME_REGEX = re.compile('(?:(\\d+)h)?(?:(\\d+)m)?(\\d+)(?:.\\d+)?s')

def run_parser():
    parser = argparse.ArgumentParser(description="plot scale test results")
    parser.add_argument('--disconnect', default="", type=str, required=True)
    parser.add_argument('--connect', default="", type=str, required=True)
    parser.add_argument('--swarm', default="", type=str, required=True)
    parser.add_argument('--outfile', default="", type=str, required=True)
    parser.add_argument('--xtick', default=1, type=int)
    parser.add_argument('--ytick', default=5, type=int)
    return parser

def roundup(x, factor):
    ceil = int(math.ceil(x / factor)) * int(factor)
    # Close to the edge
    # if (x % factor) > (0.5 * factor):
    #     return ceil + factor
    return ceil

def make_plot(data, out_file, x_tick_size, y_tick_size):
    #disconnect_data = data['Disconnected']
    #disconnect_line = mlines.Line2D(disconnect_data[0], disconnect_data[1], \
    #    color='#87C4A8', marker='>', markersize=12, linewidth=4, zorder=2)

    connect_data = data['Connected']
    connect_line = mlines.Line2D(connect_data[0], connect_data[1], \
        color='#E1BCBC', marker='s', markersize=12, linewidth=4, zorder=2)

    #swarm_data = data['Swarm']
    #swarm_line = mlines.Line2D(swarm_data[0], swarm_data[1], \
    #    color='#BCC3E1', marker='o', markersize=12, linewidth=4, zorder=2)

    fig, ax = plt.subplots(figsize=(8, 4.25)) # Tweak me for height/width

    ax.yaxis.grid(True, linestyle='dotted', which='major', color='black',
                   alpha=0.5)
    ax.xaxis.grid(True, linestyle='dotted', which='major', color='black',
                   alpha=0.5)

    ax.set_axisbelow(True)
    #ax.add_line(disconnect_line)
    ax.add_line(connect_line)
    #ax.add_line(swarm_line)

    #plt.legend([disconnect_line, connect_line, swarm_line], ['Disconnected', 'Connected ', 'Swarm'],
    #        loc=(0.01,0.64), fontsize=19, labelspacing=0.19, borderpad=None)
    #leg = plt.gca().get_legend()
    #leg.draw_frame(False)

    #x_data = disconnect_data[0] + connect_data[0] + swarm_data[0]
    #y_data = disconnect_data[1] + connect_data[1] + swarm_data[1]
    x_data = connect_data[0]
    y_data = connect_data[1]
    max_x = roundup(max(x_data), x_tick_size)
    max_y = roundup(max(y_data), y_tick_size)

    plt.xticks(range(0, max_x + (1 * x_tick_size), x_tick_size))
    plt.yticks(range(0, max_y + (2 * y_tick_size), y_tick_size))

    ax.set_ylabel('Minutes')
    ax.set_xlabel('Thousands of Containers')

    out_dir = os.path.dirname(out_file)
    if not os.path.exists(out_dir):
        os.makedirs(out_dir)

    if os.path.exists(out_file):
        os.remove(out_file)

    plt.savefig(out_file, bbox_inches='tight', pad_inches=0.1)

#def get_data(disconnect_path, connect_path, swarm_path):
def get_data(connect_path):
    #for path in [disconnect_path, connect_path, swarm_path]:
    if not os.path.exists(connect_path):
        print("Bad path: {}".format(connect_path))
        sys.exit(1)

    #columns = parse_data(disconnect_path, 'Disconnected', {})
    columns = parse_data(connect_path, 'Connected', {})
    #columns = parse_data(swarm_path, 'Swarm', columns)

    for col in columns:
        columns[col] = [z for z in zip(*columns[col])]

    return columns

def parse_data(file_path, name, columns):
    with open(file_path, 'r') as f:
        reader = csv.reader(f, delimiter=",")
        for row in reader:
            n, t = row
            time = TIME_REGEX.match(t)
            hours, minutes, seconds = time.groups(0)
            minutes = float(hours) * 60 + float(minutes) + float(seconds) / 60
            if name in columns:
                columns[name].append((float(n) / 1000, minutes))
            else:
                columns[name] = [(float(n) / 1000, minutes)]
    return columns

def run(args):
    options = run_parser().parse_args(args[1:])
    data = get_data(options.connect)
    make_plot(data, options.outfile, options.xtick, options.ytick)

if __name__ == '__main__':
    run(sys.argv)
