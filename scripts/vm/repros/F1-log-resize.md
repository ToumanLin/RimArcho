# F1: log virtual rows can trigger a ResizeObserver loop

In an isolated test home with a populated game log, open Log, scroll the virtual
rows and resize the log pane/window. The original console error was
`ResizeObserver loop completed with undelivered notifications`.

The observer updated layout synchronously inside resize delivery. Commit
`3a7f505` moves the work to the next animation frame. Later host fixture walks and
the VM log controls exercised this fix without that error. This is a manual
recipe; no claim of a minimized F1 input trace is made.
