"""Go JSON test events to private-data-free GitHub summary, stdlib only."""
import html
import json
import os
from pathlib import Path
import re


def render(events, coverage, source):
    states = {}
    for line in events.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'):
            states[event['Test']] = event['Action']
    lines = ['## Go audit tests', '', '| Tests | Passed | Failed | Skipped |',
             '| ---: | ---: | ---: | ---: |',
             f'| {len(states)} | {list(states.values()).count("pass")} | '
             f'{list(states.values()).count("fail")} | {list(states.values()).count("skip")} |',
             '', 'Coverage: ' + coverage.strip(), '',
             '<details><summary>Per-test results</summary>', '']
    matches = list(re.finditer(r'^func (Test\w+)\(', source, re.M))
    sources = {m[1]: source[m.start():matches[i+1].start() if i+1 < len(matches) else len(source)]
               for i, m in enumerate(matches)}
    for name, state in sorted(states.items(), key=lambda item: ({'fail': 0, 'skip': 1, 'pass': 2}[item[1]], item[0])):
        lines += [f'<p><b>{state.upper()}</b> {html.escape(name)}</p>',
                  '<details><summary>Exact synthetic inputs, steps and assertions</summary>',
                  '<p>Checked-in source only. Actual passing operand values not captured. '
                  'Skipped means checks were not executed; no alternative run assumed. '
                  'Failures: consult job log for actual diagnostic.</p>',
                  '<pre>' + html.escape(sources.get(name, 'Source unavailable.')) + '</pre>', '</details>']
    return '\n'.join(lines + ['</details>', ''])


if __name__ == '__main__':
    events = Path('test-events.json').read_text() if Path('test-events.json').exists() else ''
    coverage = Path('coverage-total.txt').read_text() if Path('coverage-total.txt').exists() else 'unavailable'
    text = render(events, coverage, '\n'.join(p.read_text() for p in sorted(Path('.').glob('*_test.go'))))
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as out:
        out.write(text)
