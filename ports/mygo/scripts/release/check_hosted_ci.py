#!/usr/bin/env python3
"""Require a successful main-branch development run for this exact commit."""
import json
import os
import subprocess
import sys
from release import REPOSITORY, require


def validate(runs, commit, repository):
    require(repository == REPOSITORY, 'publication is restricted to the authorized repository')
    require(any(r.get('head_sha') == commit and r.get('status') == 'completed' and r.get('conclusion') == 'success'
                and r.get('event') == 'push' and r.get('head_branch') == 'main'
                and r.get('head_repository', {}).get('full_name') == repository
                and r.get('path') == '.github/workflows/mygo.yml'
                for r in runs), 'exact commit has no successful main-branch MyGo development run; wait for CI before tagging')


if __name__ == '__main__':
    repo, commit = os.environ['GITHUB_REPOSITORY'], os.environ['GITHUB_SHA']
    response = subprocess.check_output(['gh', 'api', f'repos/{repo}/actions/workflows/mygo.yml/runs?head_sha={commit}&per_page=100'])
    validate(json.loads(response)['workflow_runs'], commit, repo)
    print('PASS: successful development workflow verified for exact main commit')
