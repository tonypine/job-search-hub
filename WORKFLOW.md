---
# Symphony workflow for job-search-hub. Operator settings (Linear scope, agent
# command, workspace root) live in the operator's symphony.yml, not here.
# Preview the assembled prompt with `symphony workflow preview --file WORKFLOW.md --agent claude`.
hooks:
  after_create: |
    git config core.hooksPath .githooks
    cd server && go mod download
# How Symphony's QA pass builds and opens the apps, over symphony.yml's
# defaults. make-app.sh signs ad hoc in the QA VM, which has no Apple identity.
# The Android build needs a writable ANDROID_USER_HOME for the debug keystore,
# a temp dir for the Kotlin daemon, and no Gradle daemon in the sandbox.
auto_review:
  playbooks:
    macos_app:
      build: cd macos && ./Scripts/make-app.sh
      app: macos/build/JobSearchHub.app
    android_app:
      build: cd android && ANDROID_HOME="$HOME/Library/Android/sdk" ANDROID_USER_HOME="$TMPDIR/android-home" JAVA_TOOL_OPTIONS="-Djava.io.tmpdir=$TMPDIR" ./gradlew --no-daemon :app:assembleDebug
      apk_path: android/app/build/outputs/apk/debug/app-debug.apk
      application_ids: ["com.tonypine.jobsearchhub"]
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
native macOS client (`macos/`, Swift) and an Android client (`android/`, Kotlin). Read `CLAUDE.md`
and `README.md` before planning.

- The repo is meant to be public. Never commit tokens, `.env`, or real company, person or
  recruiter data.
- Commits are `type: message` (`feat`, `fix`, `refactor`, `chore`, `test`, `docs`, `style`, `perf`),
  one small delivery per commit.

## Validation commands

CI is the full gate. `.github/workflows/ci.yml` runs on every pull request and reports one required
check, `ci`, which passes when none of its jobs failed: `server-static` (gofmt, go vet, staticcheck,
go mod tidy), `server-test` (the whole Go suite against Postgres 18), `macos` and `android`. Jobs
for parts of the repo a PR doesn't touch skip. Locally, run only the cheap, targeted checks below,
and leave the rest to CI.

For `server/` changes, run these. The sandbox can write only to the workspace and `/tmp`, so point
the Go build cache there:

```bash
cd server
export GOCACHE=/private/tmp/job-search-hub-go-build
gofmt -l .        # must print nothing
go vet ./...
go test ./internal/<package> ...   # only the changed packages that don't need the database
```

Store and tool tests need a fresh database through `HUB_TEST_DATABASE_URL`, and fail rather than
skip without it. Agent sessions do not get the repo's `.env`, so that variable is normally unset.
A package needs the database when its tests use `internal/testdatabase`:

- Run `go test` on the changed packages that don't need the database. Don't run `go test ./...`.
- Record each changed package that needs the database as `verified by CI (server-test)` in the
  workpad `Validation` section and in the PR's testing evidence. Do not fake, stub out, or delete
  those tests to get a green run.
- Never read, copy, or create `.env` files.

For `macos/` changes, don't run `swift build` or `swift test` locally: the agent sandbox blocks
SwiftPM's own sandbox, and blocks the dependency checkout, which writes `.git/hooks`. CI's `macos`
job is the gate; record it as `verified by CI (macos)`.

For `android/` changes, run `cd android && ./gradlew :core:test` when `android/core` changed. CI's
`android` job runs the full set (`:core:test :data:testDebugUnitTest :app:lintDebug
:app:assembleDebug`); record the rest as `verified by CI (android)`.

Every push runs `.githooks/pre-push`, which `after_create` turns on with `core.hooksPath`. When a
push changes Go under `server/`, the hook fails on a changed `.go` file that `gofmt -l` lists, or
on a `go vet ./...` finding. Never push with `git push --no-verify`. When the hook fails, fix what
it reports, commit, and push again.

A red `ci` check comes back to you through the CI failure triage protocol below. Symphony's QA
pass in `Auto Review` tests the PR as a user and runs no tests, so CI is the only test gate.

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
   and results, including the packages left as `verified by CI`), and follow-ups.
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
- Every package touched is either green under the local validation commands or listed as
  `verified by CI (<job>)`, and the `ci` check is green.

{% render "guardrails" %}
- Nothing personal or secret enters a commit, PR body, or Linear comment.

{% render "out_of_scope_backlog" %}

{% render "dependency_guardrail", lockfile: "server/go.sum" %}

{% render "workpad_template", agent: agent %}
