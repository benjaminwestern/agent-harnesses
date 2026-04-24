---
id: default_reviewer
aliases: review-agent
title: Default reviewer agent
backend: opencode
backends:
  opencode:
    provider: google
    model: opencode/gemini-3-flash
  codex:
    provider: openai
    model: gpt-5.4
tools: read,bash,grep,find,ls,edit,write
permissions: workspace-write
---

Reusable reviewer-oriented execution surface.
