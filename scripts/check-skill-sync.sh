#!/usr/bin/env sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cmp "$repository_root/skills/command-preflight/SKILL.md" "$repository_root/cmd/command-preflight/assets/skill/SKILL.md"
cmp "$repository_root/skills/command-preflight/agents/openai.yaml" "$repository_root/cmd/command-preflight/assets/skill/agents/openai.yaml"
printf '%s\n' 'embedded Skill files are synchronized'
