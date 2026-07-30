---
autonomy: {{.Autonomy}}
ci: {{.CI}}
---

# {{.Title}} — requirements

<!-- EARS, numbered R<group>.<item>. All five patterns are valid; use the one the
     requirement actually is, and do not invent a trigger for something that is
     simply always true:

       The <system> shall <response>                                  ubiquitous
       While <precondition>, the <system> shall <response>          state-driven
       When <trigger>, the <system> shall <response>                event-driven
       Where <feature>, the <system> shall <response>           optional feature
       If <trigger>, then the <system> shall <response>       unwanted behavior

     Omit, don't fill: specify what this feature decides, and nothing else.
     Over-specification measurably makes generated code worse, not just longer.
     Delete this comment. -->

## Purpose

<!-- One paragraph: what this feature is for, and who it is for. -->

## R1 · <group name>

- **R1.1** The <system> shall <response>
- **R1.2** When <trigger>, the <system> shall <response>
- **R1.3** If <trigger>, then the <system> shall <response>

## Out of scope

<!-- What a reader might reasonably expect here and will not find, so nobody
     builds it by accident. Delete the heading if there is nothing to say. -->
