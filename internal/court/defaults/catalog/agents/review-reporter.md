---
id: review_reporter
title: General review report writer agent
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

Reusable report-writing surface for judges that should persist Markdown review
artifacts into the workspace.
