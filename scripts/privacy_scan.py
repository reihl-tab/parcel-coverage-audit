"""Reject accidental private inputs or recognizable credentials before publication."""
from pathlib import Path
import re

patterns = {
    'GitHub credential': r'\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{30,})\b',
    'private key': r'-----BEGIN (?:RSA |OPENSSH |EC )?PRIVATE KEY-----',
    'email': r'[A-Za-z0-9_.+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}',
    'phone': r'\+\d[\d -]{8,}\d',
    'plate-shaped number': r'(?<!\d)\d{2,3}-\d{2,3}-\d{2,3}(?!\d)',
}
files = [p for p in Path('.').rglob('*') if p.is_file() and '.git' not in p.parts and '__pycache__' not in p.parts]
errors = []
for path in files:
    if path.name in ('privacy_scan.py',):
        continue
    if path.suffix in ('.geojson', '.out') or path.name.endswith('.local.json'):
        errors.append(f'{path}: prohibited input/output data file')
    text = path.read_text()
    for label, regex in patterns.items():
        if re.search(regex, text):
            errors.append(f'{path}: {label}')
if errors:
    raise SystemExit('\n'.join(errors))
print(f'PASS: {len(files)} text source/config/doc files scanned; no detected credentials, contacts, plates or private input files.')
