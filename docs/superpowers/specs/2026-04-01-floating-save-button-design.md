# Floating Save Button Design

**Date:** 2026-04-01
**Issue:** https://github.com/Sagit-chu/flvx/issues/266
**Status:** Approved

## Overview

Add a Floating Action Button (FAB) to the config page (`vite-frontend/src/pages/config.tsx`) that appears when configuration changes are detected, allowing users to save without scrolling to the top.

## Requirements

From Issue #266:

1. **Default hidden**: FAB not visible when no config changes
2. **Show on change**: Auto-display when `hasChanges` becomes true
3. **Fixed position**: Suspended at bottom-right corner, does not scroll with page
4. **Mobile compatible**: Same behavior on desktop and mobile devices

## Design Decisions

### 1. Implementation Approach

**Inline FAB in config.tsx** (not a reusable component)

- Rationale: Current need is limited to config page only
- State management (`hasChanges`, `saving`) already exists in the page
- framer-motion patterns already established in project
- Avoids over-abstraction (YAGNI)

### 2. UI Structure

Position: `fixed bottom-6 right-6` (24px from viewport edges)

Visual layout:
```
┌──────────────────────────────────────┐
│  [页面内容，可滚动]                    │
│                                      │
│                                [●]   │ ← FAB (fixed position)
└──────────────────────────────────────┘
```

### 3. Button Appearance

- Shape: Circular (`w-12 h-12 rounded-full`)
- Color: Primary (matches existing save button)
- Icon: SaveIcon (already defined in config.tsx)
- Shadow: `shadow-lg` for visual hierarchy
- Style: Icon-only (no text label)

### 4. Animation

Using framer-motion with `AnimatePresence`:

| Phase | Properties |
|-------|------------|
| `initial` | `{ y: 100, opacity: 0 }` - starts below viewport |
| `animate` | `{ y: 0, opacity: 1 }` - slides up to position |
| `exit` | `{ y: 100, opacity: 0 }` - slides back down on hide |

Transition config:
```typescript
transition={{ type: "spring", damping: 20, stiffness: 300 }}
```

Spring parameters produce Material Design-like feel: smooth entrance, slight bounce settle.

### 5. Interaction Details

- **Click**: Calls existing `handleSave()` function
- **Loading state**: Button shows Spinner when `saving === true`
- **Hover**: Inherits Button component's primary color hover behavior
- **z-index**: `z-50` (above page content, below modals)
- **Prevent duplicate click**: Button disabled when `saving === true`

## Technical Implementation

### Code Location

File: `vite-frontend/src/pages/config.tsx`

### Required Imports

```typescript
import { AnimatePresence, motion } from "framer-motion";
```

### FAB Component Structure

```tsx
<AnimatePresence>
  {hasChanges && (
    <motion.div
      initial={{ y: 100, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      exit={{ y: 100, opacity: 0 }}
      transition={{ type: "spring", damping: 20, stiffness: 300 }}
      className="fixed bottom-6 right-6 z-50"
    >
      <Button
        isIconOnly
        color="primary"
        size="lg"
        className="w-12 h-12 rounded-full shadow-lg"
        isLoading={saving}
        onPress={handleSave}
      >
        {!saving && <SaveIcon className="w-5 h-5" />}
      </Button>
    </motion.div>
  )}
</AnimatePresence>
```

### Placement

Insert FAB at the end of the component, before the closing `</div>` (after all Cards and Modals).

### Dependencies

- framer-motion: Already installed (v11.18.2)
- Button: Already imported from `@/shadcn-bridge/heroui/button`
- SaveIcon: Already defined in config.tsx

## Behavior Matrix

| State | FAB Visibility | Button Enabled |
|-------|----------------|----------------|
| `hasChanges = false` | Hidden (not rendered) | N/A |
| `hasChanges = true, saving = false` | Visible, animating in | Yes |
| `hasChanges = true, saving = true` | Visible | No (loading) |
| Save success | Hidden (animating out) | N/A |

## Responsive Behavior

No special handling needed. `fixed bottom-6 right-6` works identically on:
- Desktop browsers
- Mobile browsers
- H5/WebView mode

The FAB maintains consistent 24px margin from viewport edges regardless of screen size.

## Edge Cases

1. **Multiple rapid toggles**: AnimatePresence handles gracefully - exit animation completes before new enter animation
2. **Page unload with unsaved changes**: Not addressed in this design (separate concern)
3. **FAB covers existing warning banner**: z-50 places FAB above the warning banner at line 1004-1013

## Testing Checklist

After implementation, verify:

- [ ] FAB appears when any config field is modified
- [ ] FAB slides up from bottom on appearance
- [ ] FAB slides down to bottom on disappearance
- [ ] FAB fixed position during page scroll
- [ ] FAB triggers save on click
- [ ] FAB shows spinner during save
- [ ] FAB disappears after successful save
- [ ] FAB works on mobile viewport
- [ ] FAB does not interfere with Modal dialogs