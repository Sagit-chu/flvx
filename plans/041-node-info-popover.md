# Node Card Info Popover

## Goal

Restore node card's remark and renewal info display to the 2.1.8-beta9 style: an info button (ℹ️) in the CardHeader that shows a hover popover with the info, instead of the current inline display in the CardBody.

## Status: Completed

PR #327 merged

## Tasks

- [x] Add `infoPopoverPlacement` state and `updateInfoPopoverPlacement` callback
- [x] Add info button with popover to CardHeader
- [x] Restore drag handle with touch support
- [x] Remove inline info display from CardBody
- [x] Fix build errors (JSX structure and unused variables)
- [x] Create PR and merge