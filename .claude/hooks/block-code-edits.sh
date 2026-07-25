#!/usr/bin/env bash
# Guard: flow-state is a hands-on learning project. Claude never writes code here —
# the user types everything themselves. This hook hard-blocks Edit/Write/NotebookEdit
# on any file in the repo EXCEPT Claude's own operating files (.claude/**, CLAUDE.md).
#
# Wired as a PreToolUse hook in .claude/settings.json.
# To relax temporarily: disable via /hooks, or edit .claude/settings.json.

input=$(cat)
fp=$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')
[ -z "$fp" ] && exit 0

# Repo root: prefer the env var Claude Code sets for hooks; fall back to this
# script's own location (.claude/hooks/ -> repo root) for standalone/testing.
# Never hardcode an absolute path — it would leak the machine username.
repo="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

# Normalize to an absolute path.
case "$fp" in
  /*) abs="$fp" ;;
  *)  abs="$repo/$fp" ;;
esac

# Anything outside the repo (e.g. the scratchpad) is fine.
case "$abs" in
  "$repo"/*) ;;
  *) exit 0 ;;
esac

rel="${abs#"$repo"/}"

# Claude may manage its own scaffolding.
case "$rel" in
  .claude/*) exit 0 ;;
  CLAUDE.md) exit 0 ;;
esac

reason="flow-state is a hands-on learning project — Claude does not edit files in this repo. Generate the code in chat and the user will type it in themselves. (Blocked: $rel. Allowed for Claude: .claude/**, CLAUDE.md. Guard: .claude/hooks/block-code-edits.sh.)"

jq -n --arg r "$reason" '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "deny",
    permissionDecisionReason: $r
  }
}'
exit 0
