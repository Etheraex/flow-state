---
description: Check current work against BUILDPLAN.md — which phase, is "Done when" met, any drift
---

You are auditing the user's progress against the phased plan in `BUILDPLAN.md`.

Do this:

1. Read `BUILDPLAN.md` for the phase definitions and their **"Done when"** gates.
2. Inspect the current state of the repo — source files, git log/diff, any configs,
   test backends, load-gen setup — to infer what actually exists and works.
   (Read only. Do not edit anything.)
3. Determine **which phase the user is currently in** and report, concisely:
   - **Phase:** the phase they're working in, and why you think so.
   - **Done-when status:** for that phase's "Done when" criteria, which are met,
     which aren't, and what's missing. Be concrete — name the file or the missing
     behavior, not vague praise.
   - **Drift / reaching ahead:** anything that jumps ahead of the current phase
     (e.g. Redis before Phase 6, service discovery before the health-check pain of
     Phase 3) or skips a gate the plan wants felt first. The phase ordering is
     pedagogical — each phase exists to motivate the next. Flag violations plainly.
   - **Concepts to make sure you actually understand** before moving on — pull the
     phase's "Concepts" bullets and ask the user 1–2 pointed questions to check
     comprehension, in the Socratic spirit. Don't just answer them.
   - **Suggested next step** toward closing the current phase.

Keep it tight and honest. If they've genuinely met the gate, say so and point at the
next phase — and nudge them to tag it (`git tag phase-N`) so they can diff their own
architecture across phases later. If not, don't wave it through.

Remember the working agreement in `CLAUDE.md`: do not write code into files, and
prefer questions over answers. If you propose code, put it in the chat only.

$ARGUMENTS
