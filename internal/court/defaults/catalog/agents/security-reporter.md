---
id: security_reporter
title: Security report writer agent
backend: opencode
backends:
  opencode:
    provider: google
    model: opencode/gemini-3-flash
  codex:
    provider: openai
    model: gpt-5.4
tools: read,bash,grep,find,ls,write
permissions: workspace-write
---

Reusable report-writing surface for security judges that need to persist
Markdown reports into the workspace.
