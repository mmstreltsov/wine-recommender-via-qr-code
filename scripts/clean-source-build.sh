#!/bin/bash
# NFR-05: build current tracked/non-ignored sources, not HEAD; no ignored local assets.
set -euo pipefail
git ls-files -z --cached --others --exclude-standard |
  tar --null -T - -cf - |
  docker build --no-cache --pull -t "${1:?usage: bash scripts/clean-source-build.sh IMAGE_TAG}" -
