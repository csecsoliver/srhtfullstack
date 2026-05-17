#!/usr/bin/env python3
"""
This program reads licenses.json from the current directory and prints out a
Python dict with just the information needed for git.sr.ht. The licenses.json
file can be found in the "license-list-data" repository published by the
SPDX team, currently hosted here:

https://raw.githubusercontent.com/spdx/license-list-data/master/json/licenses.json
"""

import json


def main():
    with open('./licenses.json') as f:
        license_data = json.load(f)
        print('SPDX_LICENSES = {')
        for l in license_data['licenses']:
            l_id = l['licenseId']
            l_name = l['name'].replace('"', '\\"')
            print(f'    "{l_id}": "{l_name}",')
        print('}')


if __name__ == '__main__':
    main()
