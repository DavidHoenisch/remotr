# Remotr Desktop visual theme

The first-release theme is a refined industrial operations ledger: warm
near-white work surfaces, cool stone navigation, charcoal typography, cobalt
actions, and restrained mint, amber, and red evidence states. It is deliberately
light, dense, and quiet. Decoration must never compete with Fleet state or
action consequences.

`theme.css` is the single frontend token contract. Components consume semantic
variables rather than introducing page-specific colors, spacing, shadows, or
motion values.

## Typography

IBM Plex Sans is the interface face. IBM Plex Mono is reserved for Endpoint
IDs, Release refs, digests, versions, timestamps, counts, and keyboard hints.
The five committed font files are bundled through `@fontsource`; release styles
must not reference remote fonts or other runtime assets. Numeric operational
evidence uses tabular figures.

## Layout and spacing

Spacing follows a 4-pixel base rhythm. Controls are 32 pixels high, dense table
rows are 36 pixels, the connection bar is 48 pixels, and the persistent
navigation rail is 224 pixels. The supported logical-window floor is 1100 by
720 pixels. One-pixel borders establish hierarchy; shadows are reserved for
raised controls and topmost overlays.

## Color semantics

- Cobalt is interactive emphasis, selection, and the only primary action color.
- Mint means compliant or successful evidence.
- Amber means drift, caution, deferred work, or stale evidence.
- Red means failed, destructive, or blocked evidence.
- Blue-teal is informational evidence that is neither success nor failure.
- Neutral stone represents absent, unsupported, or not-reported evidence.

Status must always include visible text and, where useful, an icon. Color is
never the only carrier of meaning. Muted text is for supporting metadata, not
for required instructions or errors.

## Focus, motion, and elevation

Every interactive control uses the global cobalt `:focus-visible` outline and
ring. Motion is brief and functional: 120 milliseconds for direct feedback,
180 milliseconds for normal transitions, and 240 milliseconds for overlays.
The reduced-motion query collapses all three durations to 1 millisecond and
disables smooth scrolling. Data access and keyboard focus never depend on an
animation completing.

Overlays use one subdued scrim and the documented overlay shadow. Do not add
decorative gradients, glass effects, oversized dashboard cards, or competing
elevation systems.
