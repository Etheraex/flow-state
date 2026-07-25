---
description: Spaced-recall quiz on concepts from earlier phases to fight forgetting
---

Quiz the user on concepts from phases they've already completed — spaced repetition
to counter the forgetting that creeps in over a multi-week project. The goal is
active recall: make them retrieve the answer, don't hand it over.

Do this:

1. Determine which phases are done (ask, or infer from `BUILDPLAN.md` + repo state).
   Draw questions from **completed** phases, weighted toward the *older* ones — those
   are the ones fading. If they name a topic in `$ARGUMENTS`, focus there.
2. Ask **one question at a time.** Wait for the answer before revealing anything.
   Prefer questions that probe the *why*, not trivia:
   - "Why does the Phase 6 check-then-act race produce a negative counter?"
   - "In Phase 3, why is recovery gated on N consecutive successes rather than one?"
   - "Phase 3.5: why refuse an empty membership update from the registry?"
   - "Why swap the backend set with atomic.Pointer instead of locking a slice?"
3. After each answer: say whether it's right, fill the gap concisely if not, and — in
   the Socratic spirit — follow up with a sharper "why" if they were surface-level.
4. Keep a light running read on where they're solid vs. shaky, and at the end point
   at 1–2 concepts worth re-reading (name the `BUILDPLAN.md` section or stdlib doc).

Do not write any files. Do not dump the whole answer key up front. 5–8 questions is
plenty for one session.

$ARGUMENTS
