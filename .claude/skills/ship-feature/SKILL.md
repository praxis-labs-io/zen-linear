---
name: ship-feature
description: Run zen-linear's feature-complete PR process. Full local check via make all, push, open a draft PR, gather Copilot + /code-review findings, triage them (fix / mitigate / ignore, no tech debt), apply, push again, then mark the PR ready for review. Invoke when a feature build is complete and ready to ship.
---

# Ship Feature (zen-linear)

Go/tview adaptation of the feature-complete process. This is a downstream copy
of `~/.claude/skills/ship-feature/SKILL.md`; edit the source and copy out, never
edit this file directly.

Applies to fork feature work shipping to `origin` (zen-linear/zen-linear) `main`.
Upstream `feature/*` branches follow CLAUDE.md's upstream PR discipline instead,
not this skill.

## 1. Full local check

Run `make all` (lint + test + build) **directly, never through a pipe** —
`make lint | tail` reports success on failure and has let broken commits
through before.

- CI pins golangci-lint v2.8.0; a newer local binary false-positives on code CI
  accepts. Replicate CI with the pinned binary plus `GOTOOLCHAIN=go1.24.4`, or
  scope a newer binary with `--new-from-rev=upstream/main`.
- If anything fails, fix it and re-run until green. Do not push until it is.
- Rebuild the installed binary so Drew isn't running stale code during review:
  `go build -o ~/.local/bin/linear-tui ./cmd/linear-tui`.

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

**`gh pr edit --add-reviewer` cannot do this.** It lowercases the login and fails
with "Could not resolve user with login 'copilot'", and
`copilot-pull-request-reviewer[bot]` is the login Copilot _reviews as_, not a
requestable one. Both failures read like Copilot is unavailable; it isn't. Use
the GraphQL `requestReviews` mutation with the Bot's node id:

```bash
PR_ID=$(gh api graphql -f query='query { repository(owner:"zen-linear",name:"zen-linear"){ pullRequest(number:NNN){ id } } }' --jq '.data.repository.pullRequest.id')
BOT_ID=$(gh api graphql -f query='query { repository(owner:"zen-linear",name:"zen-linear"){ suggestedActors(capabilities:[CAN_BE_ASSIGNED],first:20){ nodes{ ... on Bot { id login } } } } }' --jq '.data.repository.suggestedActors.nodes[] | select(.login=="copilot-swe-agent") | .id')
gh api graphql -f query='mutation($pr:ID!,$bot:ID!){ requestReviews(input:{pullRequestId:$pr, botIds:[$bot], union:true}){ pullRequest{ reviewRequests(first:10){ nodes{ requestedReviewer{ ... on Bot { login } } } } } } }' -f pr="$PR_ID" -f bot="$BOT_ID"
```

The requestable actor is `copilot-swe-agent`; it comes back as
`copilot-pull-request-reviewer` in the confirmation, which is expected. Resolve
the id from `suggestedActors` rather than hardcoding it, and if that query
returns no Bot, say the request failed rather than that Copilot is unavailable.

**A repo with automatic Copilot review enabled produces a review whether or not
the request succeeded**, so a review appearing is not evidence the request
worked. Confirm from the mutation's own response.

It runs async; continue and re-check later.

**Never `@copilot`, in a comment or anywhere else.** The mutation above is the
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
