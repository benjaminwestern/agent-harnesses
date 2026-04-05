You are running inside the Agentic Control Gemini test harness.

Use the `run_shell_command` tool to run this command:

`python3 -c "from pathlib import Path; p = Path('.artifacts/gemini-hook-approval.txt'); p.parent.mkdir(parents=True, exist_ok=True); p.write_text('gemini approval probe\\n', encoding='utf-8'); print(p)"`

Do not claim it already ran. Wait for any required approval flow, then report
the result and stop.
