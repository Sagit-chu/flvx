# Floating Save Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a floating save button (FAB) to the config page that appears when configuration changes are detected.

**Architecture:** Inline FAB implementation using framer-motion AnimatePresence for enter/exit animations. Fixed-position circular button with slide-up animation, reusing existing hasChanges state and handleSave function.

**Tech Stack:** React, framer-motion (v11.18.2), shadcn-bridge/heroui Button, Tailwind CSS

---

## File Structure

| File | Action | Purpose |
|------|--------|---------|
| `vite-frontend/src/pages/config.tsx` | Modify | Add FAB imports and component at page bottom |

---

### Task 1: Add framer-motion Imports

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx:1-5`

- [ ] **Step 1: Add AnimatePresence and motion imports**

Add import statement after existing framer-motion imports (or at top if none exist).

Current imports at line 1-2:
```typescript
import { useState, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
```

Add new import after line 2:
```typescript
import { AnimatePresence, motion } from "framer-motion";
```

- [ ] **Step 2: Commit import addition**

```bash
git add vite-frontend/src/pages/config.tsx
git commit -m "feat(config): add framer-motion imports for FAB animation"
```

---

### Task 2: Add FAB Component

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx:1220-1224` (end of component)

- [ ] **Step 1: Add FAB at end of component (before closing div)**

Locate the end of `ConfigPage` component (line ~1223, the closing `</div>` after all modals).

Insert FAB component before the closing `</div>`:

```tsx
      {/* Floating Save Button (FAB) */}
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
    </div>
  );
}
```

- [ ] **Step 2: Run dev server to verify**

```bash
cd vite-frontend && npm run dev
```

Manual verification checklist:
- Open config page at http://localhost:3000/config
- Modify any config field
- Verify FAB appears with slide-up animation
- Click FAB to save
- Verify FAB disappears with slide-down animation after save
- Scroll page and verify FAB stays fixed in viewport corner
- Test on mobile viewport (resize browser or use dev tools)

- [ ] **Step 3: Commit FAB implementation**

```bash
git add vite-frontend/src/pages/config.tsx
git commit -m "feat(config): add floating save button (FAB) for issue #266"
```

---

## Verification Summary

| Requirement | Verification Method |
|-------------|---------------------|
| FAB hidden by default | Visual: no FAB on page load with no changes |
| FAB appears on change | Visual: modify field → FAB slides up |
| Fixed position | Visual: scroll page → FAB stays in corner |
| Slide-up animation | Visual: observe animation timing/bounce |
| Slide-down on save | Visual: click save → FAB slides down |
| Loading state | Visual: click save → spinner shown during save |
| Mobile compatibility | Visual: resize to mobile viewport → same behavior |

---

## Self-Review Checklist

- [x] Spec coverage: All requirements from design doc covered (imports + FAB component, animation params, button style, interaction behavior)
- [x] No placeholders: All code shown, no TBD/TODO
- [x] Type consistency: SaveIcon (line 45-59), handleSave (line 372-434), hasChanges (line 214), saving (line 213) all exist in config.tsx