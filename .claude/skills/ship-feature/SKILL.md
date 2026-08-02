---
name: ship-feature
description: Run zen-linear's feature-complete PR process. Full local check via make all, push, open a draft PR, gather Copilot + /code-review findings, triage them (fix / mitigate / ignore, no tech debt), apply, push again, then mark the PR ready for review. Invoke when a feature build is complete and ready to ship.
---

# Ship Feature (zen-linear)

Go/tview adaptation of the feature-complete process, downstream of
`~/.claude/skills/ship-feature/SKILL.md`. Not a verbatim copy: the commands,
the lint pin, and the ticket conventions below are this repo's and are
maintained here. Generic process changes belong in the source first, then come
across by hand. Copying the source over this file wholesale deletes the Go
specifics.

Applies to fork feature work shipping to `origin` (zen-linear/zen-linear) `main`.
Upstream `feature/*` branches follow CLAUDE.md's upstream PR discipline instead,
not this skill.

## 1. Full local check

Run `make all` (lint + test + build) **directly, never through a pipe** —
`make lint | tail` reports success on failure and has let broken commits
through before.

- CI pins golangci-lint v2.12.2 (`.github/workflows/ci.yml`), matching what brew
  ships. `make lint` needs no `GOTOOLCHAIN` override and reports what CI reports.
  Keep the pin current with the local version: the old v2.8.0 pin drifted four
  versions behind, so local runs and CI disagreed.
- If anything fails, fix it and re-run until green. Do not push until it is.
- Rebuild the installed binary so Drew isn't running stale code during review:
  `go build -o ~/.local/bin/zen-linear ./cmd/zen-linear`.

## 2. Push the branch

Commit and push (`git push -u origin <branch>`). Use Linear's generated branch
name (`gitBranchName` from the MCP); don't invent one.

This is its own step deliberately. Buried inside the PR step it stops reading
as a discrete action and becomes something to chain onto another command.
Step 9 is where that chaining has already caused a silent CI failure.

## 3. Open the PR as a draft

- Open the PR **as a draft**, so review happens before a full CI run is spent.
- Title and body reference every `ZNL-###` ticket the PR closes, so Linear
  auto-links them.
- The PR body is published under Drew's name: show it to him word for word
  before pushing it up.

## 4. Request a Copilot review

One REST call, with the reviewer login `Copilot`:

```bash
gh api -X POST repos/zen-linear/zen-linear/pulls/NNN/requested_reviewers -f 'reviewers[]=Copilot'
```

Confirm it registered rather than assuming. The response carries
`requested_reviewers`, and a live request shows the Bot there:

```bash
gh api repos/zen-linear/zen-linear/pulls/NNN --jq '.requested_reviewers[].login'
```

**`gh pr edit --add-reviewer` cannot do this.** It lowercases the login and
fails with "Could not resolve user with login 'copilot'". That reads like
Copilot is unavailable; it isn't.

**Do not resolve a bot id from `suggestedActors` and call the GraphQL
`requestReviews` mutation.** `suggestedActors` returns `copilot-swe-agent`,
which is the coding agent, not the reviewer. The mutation accepts its id,
returns success, and requests nothing: `reviewRequests` comes back empty and no
review ever runs. The reviewer is a different bot, `Copilot`, which reviews as
`copilot-pull-request-reviewer`. Reach it through the REST call above. This
cost two PRs' worth of false "Copilot is broken" diagnosis on 2026-08-02.

**Never confirm with `gh pr view --json reviewRequests`.** It omits Bot
reviewers and returns `[]` while the request is live. Use the REST field above,
or GraphQL with a `... on Bot` fragment.

**A repo with automatic Copilot review enabled produces a review whether or not
the request succeeded**, so a review appearing is not evidence the request
worked. Confirm from the request's own response.

It runs async; continue and re-check later.

**Never `@copilot`, in a comment or anywhere else.** The REST call above is the
only way to request a review. An `@`-mention summons it out of band, and it
re-fires on every edit of the comment that carries it. Read its findings from
the review comments and write your triage as ordinary prose that does not
address it. This holds for every comment on the PR, the description included.

## 5. Run /code-review — Drew runs this, not you

**Stop here and ask.** `/code-review` is user-triggered and billed; a session
cannot invoke it — not via the Skill tool, not via Bash, not via a Workflow.
Don't try, and don't treat the failure as a bug to route around.

Say plainly that you're blocked on this and what to run:

- `/code-review` for the working diff
- `/code-review ultra` for a multi-agent cloud review of the branch, or
  `/code-review ultra <PR#>` for the PR opened in step 3

Then **wait** for the findings before starting step 7. Step 6 is Copilot's,
runs on its own clock, and should proceed while you wait — fetch its comments
as soon as they land. It is steps 7–10 that block, because the triage in step 8
needs both sources in front of it.

If Drew declines or says to skip it, continue with Copilot's review as the only
source and **say so in the step 7 output**. What you must never do is run your
own review pass and present it as `/code-review`'s findings — name which review
actually produced each finding.

## 6. Pull down Copilot's comments

Once Copilot has finished, fetch its review comments from the PR. Re-check if
they aren't posted yet.

## 7. Review the combined findings

Merge Copilot's comments and `/code-review`'s findings into a single list.
De-duplicate where both flag the same thing.

## 8. Triage each finding

For every finding, recommend one of: **fix**, **mitigate**, or **ignore** — each
with an explicit one-line reason.

- **Default to fixing.** Do not leave tech debt.
- **Mitigate** only when a full fix is out of scope for this PR — capture the
  residual as a **Linear ticket** (Zen Linear team, right bucket), never a
  silent gap or an in-code `TODO`.
- **Ignore** only when the finding is wrong or genuinely not worth it — say why.

Present the triage table, apply the agreed fixes, and re-run step 1 until green.

## 9. Push the fixes, then mark ready — as two separate actions

**Push, let the push register, and only then mark the PR ready. Never chain
them** (`git push && gh pr ready`).

Both emit a webhook — `synchronize` and `ready_for_review`. Fired in the same
instant they land in the same CI concurrency group, so one cancels the other,
and the survivor is often the `synchronize` run, whose payload still says
`draft: true` and therefore skips every job. The PR then shows skipped checks,
which look like passes at a glance, with no CI having run at all. It is a
failure that reports success.

Keying the concurrency group on `github.event.action` fixes it repo-side, but
don't assume that's configured — separate the two actions regardless.

After marking ready, **confirm CI actually started** (`gh pr checks` or the run
list). "Skipping" is not "passing". If nothing ran, close and reopen the PR to
fire a clean `reopened` event rather than pushing an empty commit.

## 10. Close out

- **Let Linear move the ticket.** Marking the PR ready moves it to **In Review**
  on its own via the ticket id in the branch/PR. Don't write that status by
  hand — a manual move is a second copy of a transition the integration owns.
- **Move it yourself only if the automation didn't fire.** Check first, then
  `save_issue` with the status **by name, never a UUID**.
- **Ticket:** update it with any scope changes uncovered during the build. If no
  ticket exists, note that and skip.
- **Never merge.** Shipping ends at "ready for review"; merge only on an
  explicit instruction to merge.

Report the final state: PR link, CI status (from an actual check, not an
assumption), ticket status, and the triage summary. Say plainly what was
verified and what wasn't — a green CI is not a substitute for TUI behavior that
needed eyes on a screen. For visual or interactive changes no test covers,
offer the `runbook`/`interactive-runbook` flow before declaring the run
complete.
