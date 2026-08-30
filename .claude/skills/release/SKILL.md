---
name: release
description: Cut a zen-linear release end to end. Resolves the version from the commit range, checks the docs that are about to represent a shipped version, curates docs/release-notes/vX.Y.Z.md into copy for the person downloading, runs the gate, hands over the tag command, then verifies what actually published. Manual invocation only, never auto-run: use it when Drew asks to cut a release, not because main looks ready.
---

# Release (zen-linear)

`.github/workflows/release.yml` already handles the mechanical half: it runs the
tests, cross-compiles six targets, writes the checksums, and cuts the release
from the notes in the tagged commit. This skill does not reimplement any of it.

What it does not do is judgement. Which commits a stranger cares about, what the
version should be, whether a doc about to become the documentation for a shipped
version is still true, and how the notes read. That is this skill.

**Say almost nothing.** Apply these rules silently. Never restate a rule or
justify a step you are about to take. When a rule stops you, name what is blocked
in one line and what you need to continue.

**Drew pushes the tag.** Phases 1 to 5 stop at handing over the command. A
published tag is permanent and cannot be renumbered. Phase 6 resumes once he
reports it pushed: a release is not done until the artifacts are verified.

## 1. Scope the release

- `git fetch --tags`, then confirm `main` is checked out, clean, and even with
  origin. Anything else stops here. Local `main` silently carrying commits that
  never reached origin is a real state this has been in, and tagging it cuts a
  release from a tree nobody reviewed.
- `git log <last-tag>..HEAD --no-merges` for the range. History is one squash
  commit per PR, so the commit titles are the unit.
- Sort every commit into **user-facing** or **internal**, and show the split.
  Internal work is most of a typical range: CI, agent config, refactors,
  test-only changes. It does not appear in the notes.
- A commit is user-facing if someone who only runs the tool would notice, or if
  it changes what they download, install, or read.

**Before the first release there is no last tag.** The range is the whole
history, and most of it is the fork's. Scope that one off what the tool does
today rather than off the log.

Then propose the bump. `docs/CONTRIBUTING.md` under "Version numbers" governs
which, and **get Drew's confirmation before going further.**

## 2. Check the docs the release makes public

Derive the set rather than reading all six. `git diff --name-only
<last-tag>..HEAD` gives the surfaces the range touched, and "Docs that describe
the code" in `docs/CONTRIBUTING.md` maps each one to the documents that describe
it. Read those.

This matters more here than at merge time. A doc that is wrong at this moment
becomes the documentation for a shipped version, and the release page links into
it.

**Then work the other way, from phase 1's user-facing list.** Every entry on it
has to be findable in a document that says what the code now does. That is the
check the map cannot make: the map catches a doc whose subject moved, and this
catches a change that arrived with no doc at all. A change nothing describes is
either a gap to fill now or work no user sees, and phase 1 already decided it was
user-facing, so it is usually the first.

`docs/install.md` earns the most attention of any of them. It names the platforms
the release carries, the requirements, the auth paths, where the binary lands,
and what `--version` reports. All of those are claims a release makes true or
false.

`docs/keys.md` is the one most likely to have drifted, because a key changes in
a PR that had no reason to think about it. `?` renders the same keys the code
holds, so the app is the reference, not the last version of the file.

Fix what the range made wrong before writing the notes, and report which
documents were read and which changed. Phase 5 commits them.

## 3. Curate the notes

Write `docs/release-notes/vX.Y.Z.md`, for the person downloading rather than the
person who wrote the patch. Drew's voice: terse, considerate, stoic, no strong
adverbs, no em-dashes.

Structure:

- No H1. `gh` titles the release from the tag, so one in the notes repeats it.
- One line saying what this release is.
- `## New` / `## Fixed` / `## Changed`, only the ones that apply. Bold lead-in
  naming the thing, then plain sentences about what changed. A first release has
  no before to differ from, so it says `## What it does` instead of all three.
- `## Install`.
- `## Known gaps`, where there are any worth naming.

Check `## Install` against what `docs/install.md` currently says rather than
copying the previous file forward. It goes stale the moment a platform or an
install path changes, and it is the section a reader acts on.

There is no card block and no appcast. Nothing reads these but a person on the
release page.

Run the mechanical checks before showing anything. They are cheap and catch what
rereading your own copy does not:

```sh
grep -c '—' docs/release-notes/vX.Y.Z.md    # must be 0
grep -nEi 'seamless|powerful|beautiful|just works|simply|easily|quickly' docs/release-notes/vX.Y.Z.md
```

Then show Drew the notes and get approval. Copy is the deliverable here.

## 4. Gate

`make all`, fully green: gofmt, go.mod tidiness, golangci-lint, the race suite,
and the build. Run it directly, never through a pipe that swallows the exit code.

## 5. Hand over

Commit the notes **and phase 2's doc fixes** to `main`. Doc-only changes go
straight to `main` here, and the pre-push hook rejects pushes to it, so the
commit is yours and the push is his.

Then confirm the tree is clean again before handing anything over. Phase 1's
check does not run twice, and a doc fix left uncommitted means the release links
to the stale page phase 2 existed to correct.

Then give Drew both commands, in this order, with the version named explicitly:

```sh
git push origin main
git tag -a vX.Y.Z -m "zen-linear vX.Y.Z" && git push origin vX.Y.Z
```

**`main` first, and it is not a style preference.** The release job checks out
the tag and reads `docs/release-notes/${GITHUB_REF_NAME}.md` out of it. A tag cut
before the notes commit is on `main` fails the run, and the tag is already
published by then.

The hook guards `refs/heads/main` only, so the tag pushes without `--no-verify`.
Never suggest `--no-verify` for either.

Stop here. Do not run them.

**If the tag goes first anyway**, the run fails on the missing notes and re-running
it fails identically, the tag being fixed to a commit that does not carry them.
Do not move a published tag. Push the notes to `main`, then cut the release
against the tag by hand, from the archives the failed run left as artifacts:

```sh
gh release create vX.Y.Z dist/* --notes-file docs/release-notes/vX.Y.Z.md --verify-tag
```

## 6. Verify what published

Resume once Drew reports the tag pushed, and check what landed rather than
trusting the workflow's exit code.

- `gh release view vX.Y.Z --json assets` shows **seven** assets: four tarballs,
  two windows zips (amd64 and arm64), and `checksums.txt`. A missing archive is a
  matrix leg that failed after the others published.
- Download one archive and match it against the published checksum. The
  checksums are generated in a different job from the builds.
- **Run the installer against the live release**, with `INSTALL_DIR` at a scratch
  path. Nothing before a release exists can exercise its download path, so this
  is the only time it is ever tested. `install.ps1` is covered by the
  `install-script` job in CI on every PR, so it needs no hand check here; it
  cannot be run from a mac anyway.
- The installed binary reports the version rather than `dev`. A `dev` here means
  the ldflags stamp broke, and the binary is otherwise fine, which is why nothing
  else catches it.
- Run it against a real Linear workspace, not just `--version`. Load a list,
  open an issue, save a field.

No Linear status work. Shipped tickets are already Done from their merges.

## Hand back

```text
## Released vX.Y.Z

**Release** <link>

**Shipped**
- <what a user gets>

**Verification**
- Docs: <n> read, <n> updated
- Assets: <n> of 6
- Checksum: <matched or not>
- Installer: <result against the live release>
- Version: <what the installed binary reports>
- Live run: <what was exercised against a real workspace>

**Known gaps**
- <what shipped unfinished, and where it is tracked>
```

The run is not complete while an asset is missing, a checksum is unmatched, the
installer is untested against the live release, the binary reports `dev`, or a
user-facing change in the notes has no document describing it.
