---
date: 2024-03-14
project: Rocket.Chat.Electron
tags:
  - electron
  - ipc
  - macos
  - debugging
topic: macOS Electron IPC hangs when main process blocks on sync I/O
kind: lesson
scope: user-personal
confidence: high
---

# macOS Electron IPC hangs when main process blocks on sync I/O

## What happened
Shipped a feature that did synchronous file reads in the Electron main
process. Worked fine on Linux in CI. On macOS, the renderer hung for
several seconds whenever the feature ran.

## Why
macOS schedules the main thread differently than Linux.

## Lesson
Never do sync I/O in the Electron main process.
