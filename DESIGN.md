---
name: AssetLoop 物迹
description: Existing themed asset-management interface
colors:
  paper-light: "#f7f8f5"
  paper-dark: "#111714"
  card-light: "#fff"
  card-dark: "#19211d"
  ink-light: "#17211d"
  ink-dark: "#e7eee9"
  muted-light: "#64706a"
  muted-dark: "#aab8b0"
  emerald-light: "#166b4f"
  emerald-dark: "#65d0a4"
typography:
  body:
    fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif'
    fontSize: "15px"
    lineHeight: 1.55
  section:
    fontSize: "18px"
  label:
    fontWeight: 700
rounded:
  control: "10px"
  tag: "6px"
  fieldset: "12px"
  surface: "16px"
spacing:
  control-gap: "12px"
  section-gap: "20px"
  surface-padding: "24px"
---

# Design System: AssetLoop 物迹

## Overview

Record of the existing implementation, not a new brand proposal. Ground truth is
`internal/web/static/app.css` and the server templates. This is an operational
interface: clear product identity, compact actions, readable records and explicit
editing. The user requested the existing theme and drawers be preserved.

**Key Characteristics:**

- Semantic colors adapt to theme and the user's accent.
- Native controls, restrained tables and direct labels carry the task.
- Secondary editing lives in accessible drawers or dedicated management screens.

## Colors

Neutral surfaces have a slight green cast. Emerald is the default accent; blue,
violet, amber and rose are existing user choices, each with light/dark variants.
The current user's choice is not the system default.

**The Semantic Theme Rule.** Application surfaces use `--paper`, `--card`, `--field`,
`--ink`, `--muted`, `--line`, `--accent`, `--accent-soft` and `--accent-contrast`.
Never hard-code a user's current palette into a feature.

## Typography

The body stack uses installed Inter when available and platform UI fallbacks;
there is no bundled display font. Section headings use the section size above.
Financial amounts use tabular numerals. Labels are visibly distinct from values.
The dictionary keeps the existing page heading scale with restrained tracking.
Long names and translations wrap instead of widening a mobile page.

## Layout

The primary shell is capped at 1120px and otherwise follows 90vw. At 760px and
below, headings and filter groups stack. At 520px, management tables become
labeled record rows. Related controls group tightly; editing regions remain
separate from read-only summaries.

## Elevation & Depth

Ordinary surfaces use borders and tonal separation. Menus and drawers use the
existing offset, blurred popup shadows. The 3D stage inherits the accent-soft
surface; a canvas must not impose an opaque competing background.

## Shapes

Controls, fieldsets, tags and surfaces use the recurring radii recorded above.
Tags are compact descriptors, not pill-shaped primary actions. Borders remain
subtle separators rather than colored decoration.

## Components

### Buttons and fields

Primary buttons use accent/contrast colors and explicit action labels. Secondary
buttons use field backgrounds. Disabled controls remain visibly disabled. Inputs
use the field surface and strong border; focus changes to accent with a soft outline.
Icon-only actions retain an accessible name and title.

### Navigation and tables

Dictionary tabs use `aria-current="page"`; the active tab has ink color and an
underline. Reference counts are links. Mobile table labels come from the same
localized column vocabulary as desktop headers.

### Tag selections and drawers

Each dimension is a fieldset with a legend. Single-choice and multiple-choice
behavior follows the dimension, but every dimension can be cleared. Model changes
confirm removal of incompatible values. Shared-name changes require explicit
acknowledgement. Drawers preserve keyboard focus and close through their existing
cancel, close and Escape behavior.

### Motion

Time-line details expand inside their record, not in a detached card. Reduced
motion removes displacement. The model viewer pauses when hidden, offscreen or
being manipulated; reduced-motion preference disables autoplay.

## Do's and Don'ts

- Do reuse semantic colors and the current server-rendered controls.
- Do preserve keyboard access, localized error recovery and responsive labels.
- Don't add a second specification picker beside the typed tags.
- Don't treat descriptive resource tags as automatic model-binding instructions.
- Don't canonize inherited eyebrow decoration or overly tight global heading
  tracking into new surfaces; those are incumbent debt outside this feature.
