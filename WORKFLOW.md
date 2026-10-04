---
# Symphony workflow for job-search-hub. Operator settings (Linear scope, agent
# command, workspace root) live in the operator's symphony.yml, not here.
# Preview the assembled prompt with `symphony workflow preview --file WORKFLOW.md --agent claude`.
hooks:
  # The throwaway test Postgres (compose.test.yaml) is shared by every
  # workspace and restarts with Docker. A Docker that isn't running only
  # leaves the database-backed tests unverified.
  after_create: |
    docker compose -f compose.test.yaml up -d --wait ||
      echo "the test Postgres did not start; database-backed tests will fail" >&2
    cd server && go mod download
prompts:
  pr: |
    You are working on an existing GitHub pull request.

    PR: {{ pr.url }}
    Number: {{ pr.number }}
    Title: {{ pr.title }}
    Base: {{ pr.base_ref }}
    Head: {{ pr.head_ref }}
    Intent: {{ pr.intent }}

    Description:
    <github_pr_body>
    {{ pr.body }}
    </github_pr_body>

    Follow the managed Symphony PR runtime context, complete the requested PR
    intent in this repository, and validate before handoff.
---

You are working on a Linear ticket `{{ issue.identifier }}`

{% render "continuation_context", attempt: attempt %}

{% render "issue_context", issue: issue %}

{% render "default_posture" %}

{% render "scoped_tools" %}

## Repository

A local job-search hub: a Go server (`server/`) with Postgres, MCP tools and a `hub` CLI, plus a
native macOS client (`macos/`, Swift). Read `CLAUDE.md` and `README.md` before planning.

- The repo is meant to be public. Never commit tokens, `.env`, or real company, person or
  recruiter data.
- Commits are `type: message` (`feat`, `fix`, `refactor`, `chore`, `test`, `docs`, `style`, `perf`),
  one small delivery per commit.

## Validation commands

The sandbox can write only to the workspace and its temp folder, so point the Go build cache there:

```bash
cd server
export GOCACHE="${TMPDIR:-/tmp}/job-search-hub-go-build"
export HUB_TEST_DATABASE_URL=postgres://hub:hub-test@localhost:5435/postgres
gofmt -l .        # must print nothing
go vet ./...
go test ./...
```

Store and tool tests each create a fresh database through `HUB_TEST_DATABASE_URL`. The URL above
points at the throwaway test Postgres from `compose.test.yaml`, which the workspace hook starts. It
holds nothing but test databases, so the URL is no secret, and the whole suite runs in every session:

- Run the full `go test ./...` before handoff, not only the packages you changed.
- If tests fail with `connect to the test Postgres`, the test Postgres is not running and you cannot
  start it from the sandbox. Record the database-backed packages as
  `not verified: test Postgres on localhost:5435 is not running` in the workpad `Validation` section
  and in the PR's testing evidence. CI still runs them. Do not fake, stub out, or delete those tests
  to get a green run.
- Never read, copy, or create `.env` files, and never point the tests at the hub's own database on
  port 5434.

For `macos/` changes: `cd macos && swift build && swift test`.

CI (`.github/workflows/server.yml`) runs the same `gofmt`, `go vet` and `go test` against its own
throwaway Postgres on every pull request. Its `server` check must be green before `In Review`.

## Related skills

Symphony provides these skills in every session:

- `linear`: interact with Linear.
- `commit`: produce clean, logical commits during implementation.
- `pull`: sync with the latest `origin/main` before handoff.

Publish with the scoped `github_push_branch` tool and merge only with `github_merge_pull_request`.

{% render "status_map" %}

## Step 0: Determine current ticket state and route

1. Fetch the issue by its explicit ticket ID and read the current state.
2. Route per the Status map above:
   - `Backlog` -> do not modify the issue; stop.
   - `Todo` -> move to `In Progress` as the first tool call of the run, then bootstrap the workpad and
     start Step 1. If a PR is already attached, run the PR feedback sweep first.
   - `In Progress` -> continue Step 1 / Step 2 from the workpad.
   - `In Review` -> follow Step 3.
   - `Merging` -> follow Step 3, item 3.
   - `Rework` -> follow Step 4.
   - `Done` -> do nothing and stop.
3. If the branch's PR is `CLOSED` or `MERGED`, treat earlier branch work as non-reusable and start a
   fresh branch from `origin/main`.

## Step 1: Start or continue execution

{% render "workpad_bootstrap", agent: agent %}

{% render "reproduce_and_blast_radius" %}

## Step 2: Execution phase

1. Implement against the workpad plan, checking items off as they land.
2. Run the validation commands above for every package you touched; fix failures before pushing.
3. Before pushing, review `git diff origin/main..HEAD` for debug output, stray files, and anything
   personal (names, emails, tokens, real company data).
4. Commit with the `commit` skill, push with `github_push_branch`, and open the PR with
   `github_create_pull_request`. The PR body covers what changed and why, testing evidence (commands
   and results, including any `not verified` packages), and follow-ups.
5. Attach the PR URL to the issue with `linear_attach_url`.
6. Run the PR feedback sweep, refresh the workpad so `Plan`, `Acceptance Criteria`, and `Validation`
   match the finished work, then move the issue to `In Review` and end the turn.

{% render "pr_feedback_sweep" %}

{% render "ci_triage" %}

{% render "escape_hatches" %}

## Step 3: In Review and Merging

1. In `In Review`, do not code or change ticket content; a human reviews the PR.
2. If review feedback needs changes, move the issue to `Rework` and follow Step 4.
3. In `Merging`, the human has approved. Sync with `origin/main` if the PR conflicts, then call
   `github_merge_pull_request`. If it reports pending checks, wait and retry. Once it reports the PR
   merged, move the issue to `Done`.

## Step 4: Rework

1. Re-read the issue and every human comment; write down what changes this attempt.
2. Close the existing PR and start a fresh branch from `origin/main`.
3. Keep the single workpad: move the old `Plan`, `Acceptance Criteria`, and `Validation` under
   `### Superseded — attempt <n>`, write fresh ones, and run Steps 1-2 again.

{% render "completion_bar" %}
- Every package touched is either green under the validation commands or listed as `not verified`
  with the reason.

{% render "guardrails" %}
- Nothing personal or secret enters a commit, PR body, or Linear comment.

{% render "out_of_scope_backlog" %}

{% render "dependency_guardrail", lockfile: "server/go.sum" %}

{% render "workpad_template", agent: agent %}
