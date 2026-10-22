"""Bounded two-way workflow dispatch. Never starts a scan or accepts arbitrary URLs."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time

SOURCE = 'reihlinstinct/ginot-hair-plans'


def api(path, payload=None):
    cmd = ['gh', 'api', path]
    if payload is not None:
        cmd += ['-X', 'POST', '--input', '-']
    result = subprocess.run(cmd, input=json.dumps(payload) if payload else None,
                            text=True, capture_output=True, check=True)
    return json.loads(result.stdout) if result.stdout.strip() else None


def validate(sha, run, metadata):
    if not re.fullmatch(r'[0-9a-f]{40}', sha) or not re.fullmatch(r'[0-9]+', run):
        raise ValueError('Invalid source identifiers')
    if metadata.get('conclusion') != 'success' or metadata.get('path') != '.github/workflows/permit-feed.yml':
        raise ValueError('Source is not a successful permit-feed workflow')


def fetch():
    sha, run = os.environ['SOURCE_SHA'], os.environ['SOURCE_RUN']
    if not re.fullmatch(r'[0-9a-f]{40}', sha) or not run.isdigit():
        raise ValueError('Invalid source identifiers')
    for _ in range(30):
        metadata = api(f'repos/{SOURCE}/actions/runs/{run}')
        if metadata.get('status') == 'completed':
            break
        time.sleep(10)
    validate(sha, run, metadata)
    # Only allow a commit already reachable from current public main.
    comparison = api(f'repos/{SOURCE}/compare/{sha}...main')
    if comparison.get('status') not in ('identical', 'ahead'):
        raise ValueError('Source SHA is not a main ancestor')
    subprocess.run(['git', 'clone', '--filter=blob:none', '--no-checkout',
                    'https://github.com/' + SOURCE + '.git', 'source'], check=True)
    subprocess.run(['git', '-C', 'source', 'checkout', sha, '--',
                    'data/ginot_hair_parcels.geojson', 'data/permit_requests.json'], check=True)


def callback():
    report = json.loads(Path('audit.json').read_text())
    report['schema_version'] = 1
    report['source_commit'] = os.environ['SOURCE_SHA']
    report['source_run'] = os.environ['SOURCE_RUN']
    report['auditor_run'] = os.environ['GITHUB_RUN_ID']
    report['source_digest'] = hashlib.sha256(Path('source/data/permit_requests.json').read_bytes()).hexdigest()
    # No permit row, address, owner or unrelated field is sent in callback.
    text = json.dumps(report, separators=(',', ':'))
    if len(text) > 6000:
        raise ValueError('Callback exceeds bounded report size')
    api(f'repos/{SOURCE}/actions/workflows/audit-result.yml/dispatches',
        {'ref': 'main', 'inputs': {'report': text}})


if __name__ == '__main__':
    if sys.argv[1:] == ['fetch']:
        fetch()
    elif sys.argv[1:] == ['callback']:
        callback()
    else:
        raise SystemExit('Expected fetch or callback')
