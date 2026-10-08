# Agent skills

Canonical skills live in `.agents/skills/` (the cross-tool location read by
Codex, OpenCode, Cursor, Copilot and Gemini).

- `.claude/skills -> ../.agents/skills` for Claude Code, which only reads `.claude/skills`.
- `.codex/skills -> ../.agents/skills` for older Codex versions that use the legacy path.

Add a skill as `.agents/skills/<id>/SKILL.md` with `name` and `description` frontmatter.
