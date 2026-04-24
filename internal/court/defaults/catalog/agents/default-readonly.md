---
id: default_readonly
title: Default read-only agent
backend: opencode
backends:
  opencode:
    provider: google
    model: opencode/gemini-3-flash
  codex:
    provider: openai
    model: gpt-5.4
tools: read,bash,grep,find,ls
permissions: read-only
---

Reusable read-only execution surface for clerks and judges.
