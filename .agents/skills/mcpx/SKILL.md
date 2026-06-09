---
name: "mcpx"
servers: ["slack"]
description: "Use project-approved MCP tools through mcpx. Trigger when the user asks to inspect or operate services backed by these MCP servers: slack."
---

# MCPX

Use this skill when the task needs one of these MCP servers:

- slack

## Discover

Inspect the available tool surface before calling tools:

```bash
mcpx --schema=".slack"
```

Use schema selectors to narrow large MCP surfaces before choosing a tool:

- `.server` shows one server, for example `mcpx --schema=.posthog`
- `.server.tool` shows one tool, for example `mcpx --schema=.posthog.projects-get`
- `.{a,b}` selects multiple keys at the current level
- `.server.{tool-a,tool-b,tool-c}` shows a short list of candidate tools

Normal workflow: inspect the project-approved servers first, identify likely
tool names from the outline, then run a narrower selector such as
`mcpx --schema=.posthog.{projects-get,alerts-list,alert-create}` before
calling a tool.

## Call

Call MCP tools through root server commands and pass tool input only through `--input`.
`--input` accepts inline JSON/JSON5, `@file`, and `@-` stdin values through argc.

```bash
mcpx <server> <tool> --input '{ }'
```

For larger payloads, prefer file or heredoc input:

```bash
mcpx <server> <tool> --input @payload.json

mcpx <server> <tool> --input @- <<'JSON'
{
  "example": true
}
JSON
```

## Notifications

Most tool calls emit no notifications and this section never applies. When an
MCP server pushes events during a call (progress, schema changes, custom
events), mcpx merges them into default structured output under `@notifications`:

```
count: 1
@notifications[1]{method,params}:
  notifications/progress,{progressToken:"...",progress:3,total:4,message:"step 3"}
```

For non-JSON text, binary, or mixed content, mcpx falls back to a trailing
sentinel line:

```
<tool result lines>
@notification: [{"method":"notifications/progress","params":{...}}]
```

Each entry has `method` plus method-specific `params`. Special cases:

- `notifications/progress` may carry `aggregatedCount` on the last entry per progress token, meaning intermediate progress was collapsed (first and last preserved verbatim).
- `notifications/tools/list_changed` is handled by mcpx automatically; no agent action required.
- `$oversize` appears in raw mode when the buffer cap was reached; default output renders it as `notifications oversize, saved to <path>`.

In `--raw` mode with a structured result and non-empty notifications, the
sentinel line is replaced by a JSON envelope:

```json
{ "result": <tool-result>, "notifications": [ ... ] }
```

Ignore notifications unless the task specifically depends on progress or
server events. Parse only when `@notifications`, the sentinel line, or the raw
envelope is present.

Do not hand-edit MCP configuration in this project. Servers are registered in the user's global mcpx registry.
