# Specification UI finish review

Inline substitute for the finish reviewer; no independent subagent review.
Scope: the existing Operate interface, especially the tag dictionary. No approved
visual comp or new identity was requested. PRODUCT.md records previously approved
product facts; architecture and product-plan documents remain authoritative.

disposition: fix

## persistence

Product record and updated surface contract exist. DESIGN.md and its sidecar are
the final documenter deliverables. Captures: `.impeccable/review/desktop.png`
(1366×900) and `mobile.png` (390×844), both taken at document top on schema 14.
Mobile capture is a viewport, not a claim to show the whole scrollable document.

## fidelity

| Element | Finding |
|---|---|
| TYPE | Incumbent system UI face matches the Operate contract. The new dictionary inherits overly tight global heading tracking; scope its correction to this page. |
| MATERIAL | Flat themed surfaces and existing controls match; no simulated material or added raster assets. |
| GROUND | Dark green-neutral ground and user violet accent match app.css and saved preferences. |
| Primary task | Create, search, type/value navigation and reference access are visible. |
| Narrow layout | Filters stack and rows use labeled fields; measured document width stays within viewport. |
| Active navigation | Contradicted: undefined `--text` token and no underline style weaken the active state. |
| Model identity | Category/model remain separate; no legacy specification creation step. |

## ceiling

Preserve native controls, compact actions and existing drawers. A new visual world
or decorative interaction would exceed this refinement's scope.

## material_fixes

1. Use the existing ink token and visible underline for the current dictionary tab.
2. Give the dictionary heading tracking no tighter than -0.03em, without restyling unrelated pages.

## keep

Keep optional typed selections, references, shared-name confirmation, themes and
keyboard-accessible drawers. Do not introduce a second specification flow.

## Mechanical evidence

The one detector run returned no regex findings but reported missing HTML parser
modules. It did not evaluate selectors, custom properties or computed contrast.
49 Node behavior checks and the Go suite passed before the two findings above.
PostgreSQL live verification is explicitly deferred, not counted as a pass.

## Verdict pass

1. Active navigation: resolved. Both replacement captures show the underline;
   browser computed style reports `underline`, using the ink token.
2. Dictionary heading: resolved. Scoped CSS uses -0.03em; at 38px the browser
   reports -1.14px. Desktop and mobile captures retain readable layout.

## Remaining

Clear for the two listed fixes; no new visual hunt was performed. Documentation
records the incumbent system rather than a new identity. No raster was added or
replaced by this feature. The existing phone poster and GLB are unchanged inputs.

disposition: ship

This verdict covers the listed fixes, not a comprehensive accessibility or
production-readiness certification. The inline review is not an independent review.
