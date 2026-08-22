# Agents

zen-linear can hand an issue to a terminal coding agent without you copying
anything out of it. The issue's title, description and comments go in as
context, your prompt goes on top, and the agent's output streams into a modal
while it runs.

It drives a CLI that lives on your machine. zen-linear does not talk to a model
itself and holds no key for one.

## Running one

Open the palette with `:` and pick **Ask agent about selected issue**
(`ask_agent`). It is scoped to an issue, so it answers from the issues and
details panes.

The prompt modal has three fields:

- **Template** picks one of your saved prompts and fills the prompt with it.
- **Prompt** is what the agent is asked. Edit it freely.
- **Workspace** is the directory the agent runs in. Blank uses the current
  working directory.

`⌃⏎` runs it, Esc cancels.

The output modal streams while the run goes:

    ↑↓ / j k    scroll          Tab     switch between the stream and the answer
    c           copy the answer r       copy a command to resume the run
    Esc         cancel the run and close

## Providers

Two, chosen with `agent_provider` in the config.

| `agent_provider` | Runs |
| --- | --- |
| `cursor` (default) | `cursor-agent` |
| `claude` | `claude` |

The binary has to be on your PATH. When it is not, the status bar says so and
nothing runs.

`agent_model` passes a model through to the provider's `--model` flag. Leave it
unset to take the CLI's own default.

`agent_sandbox` is a portable intent, not a flag, because the two CLIs express
it differently:

| `agent_sandbox` | claude | cursor |
| --- | --- | --- |
| `enabled` (default) | `--permission-mode` asks before acting | asks before running commands |
| `disabled` | permissive mode | `--force` |

`agent_workspace` sets a default for the modal's Workspace field. It is the
process's working directory, and for claude it is also passed as `--add-dir`.

## Prompt templates

Saved prompts live in `~/.zen-linear/prompts.json`, created on first use with
three built in:

- **Create a plan**. Outline approach, steps, risks and tests.
- **Explore and research**. Summarize the relevant files, behaviors and open
  questions.
- **Implement**. Make focused changes and outline the tests to run.

Edit them from the palette with **Edit agent prompt templates**
(`edit_prompt_templates`), or edit the file:

```json
[
  { "name": "Create a plan", "prompt": "Create a plan for the selected Linear issue…" }
]
```

A template with an empty name or an empty prompt is dropped.

## What the agent is sent

Your prompt, then the issue rendered as plain text:

```
Title: <the issue title>

Description:
<the description, or "(none)">

Comments:
- <author> at <time>
<body>
```

Nothing else. No credentials, and no write access back to Linear: whatever the
agent does happens in its workspace, and the issue is only ever read.
