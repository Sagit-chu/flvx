# Announcement Popup Notification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a popup modal for announcements that automatically shows to users when a new or updated announcement is published.

**Architecture:** We will modify the Go backend to return `update_time` along with the announcement data. In the Vite frontend, we will store the user's `flvx_announcement_seen_time` in `localStorage`. If the fetched `update_time` is greater than the stored timestamp, we trigger a NextUI Modal displaying the announcement content.

**Tech Stack:** Go, Vite, React, TailwindCSS, NextUI.

---

### Task 1: Update API Response in Go Backend

**Files:**
- Modify: `go-backend/internal/http/handler/handler.go`

- [ ] **Step 1: Write the minimal implementation**

Modify the `getAnnouncement` function in `go-backend/internal/http/handler/handler.go`. 
Find the response map inside `getAnnouncement` and add the `update_time` key:

```go
	if ann == nil {
		response.WriteJSON(w, response.OK(map[string]interface{}{
			"content":     "",
			"enabled":     0,
			"update_time": 0,
		}))
		return
	}

	updateTime := ann.CreatedTime
	if ann.UpdatedTime.Valid {
		updateTime = ann.UpdatedTime.Int64
	}

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"content":     ann.Content,
		"enabled":     ann.Enabled,
		"update_time": updateTime,
	}))
```

- [ ] **Step 2: Commit**

```bash
git add go-backend/internal/http/handler/handler.go
git commit -m "feat(api): include update_time in announcement response"
```

---

### Task 2: Update Frontend API Interface

**Files:**
- Modify: `vite-frontend/src/api/index.ts`

- [ ] **Step 1: Write the minimal implementation**

Modify the `AnnouncementData` interface in `vite-frontend/src/api/index.ts` to include `update_time`.

```typescript
export interface AnnouncementData {
  content: string;
  enabled: number;
  update_time?: number;
}
```

- [ ] **Step 2: Commit**

```bash
git add vite-frontend/src/api/index.ts
git commit -m "feat(ui): add update_time to AnnouncementData interface"
```

---

### Task 3: Create AnnouncementModal Component

**Files:**
- Create: `vite-frontend/src/pages/dashboard/components/announcement-modal.tsx`

- [ ] **Step 1: Write the minimal implementation**

Create `vite-frontend/src/pages/dashboard/components/announcement-modal.tsx` with the following content:

```tsx
import type { AnnouncementData } from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface AnnouncementModalProps {
  announcement: AnnouncementData;
  isOpen: boolean;
  onClose: () => void;
  onDontShowAgain: () => void;
}

export const AnnouncementModal = ({
  announcement,
  isOpen,
  onClose,
  onDontShowAgain,
}: AnnouncementModalProps) => {
  return (
    <Modal isOpen={isOpen} onOpenChange={(open) => !open && onClose()} size="2xl">
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">平台公告</ModalHeader>
        <ModalBody>
          <div className="prose prose-sm dark:prose-invert max-w-none max-h-[60vh] overflow-y-auto">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {announcement.content}
            </ReactMarkdown>
          </div>
        </ModalBody>
        <ModalFooter>
          <Button variant="flat" onPress={onDontShowAgain}>
            不再提示
          </Button>
          <Button color="primary" onPress={onClose}>
            关闭
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
```

- [ ] **Step 2: Commit**

```bash
git add vite-frontend/src/pages/dashboard/components/announcement-modal.tsx
git commit -m "feat(ui): create AnnouncementModal component"
```

---

### Task 4: Integrate Modal State in Dashboard Custom Hook

**Files:**
- Modify: `vite-frontend/src/pages/dashboard/use-dashboard-data.ts`

- [ ] **Step 1: Update the hook return type interface**

At the top of `vite-frontend/src/pages/dashboard/use-dashboard-data.ts` where `DashboardData` is or similar, add the new properties (if it uses an explicit return type). If it's inferred, skip this. Wait, let's check the code:

```typescript
  isAnnouncementModalOpen: boolean;
  setIsAnnouncementModalOpen: (isOpen: boolean) => void;
  dismissAnnouncementModal: () => void;
```
Ensure they are added to the returned object at the bottom of the `useDashboardData` hook.

Find the `const loadAnnouncement` function. 

- [ ] **Step 2: Write the minimal implementation**

First, add state at the top of the hook:
```typescript
  const [isAnnouncementModalOpen, setIsAnnouncementModalOpen] = useState(false);
```

Then, modify the `loadAnnouncement` logic inside `useDashboardData`:
```typescript
      if (res.code === 0 && res.data && res.data.enabled === 1) {
        setAnnouncement(res.data);
        
        try {
          const storedTimeStr = localStorage.getItem("flvx_announcement_seen_time");
          const storedTime = storedTimeStr ? parseInt(storedTimeStr, 10) : 0;
          const updateTime = res.data.update_time || 0;
          
          if (updateTime > storedTime) {
            setIsAnnouncementModalOpen(true);
          }
        } catch (err) {
          console.warn("Failed to read localStorage for announcement state", err);
          setIsAnnouncementModalOpen(true);
        }
      } else {
        setAnnouncement(null);
      }
```

Add the dismiss handler inside the hook:
```typescript
  const dismissAnnouncementModal = useCallback(() => {
    setIsAnnouncementModalOpen(false);
    if (announcement && announcement.update_time) {
      try {
        localStorage.setItem("flvx_announcement_seen_time", announcement.update_time.toString());
      } catch (err) {
        console.warn("Failed to set localStorage for announcement state", err);
      }
    }
  }, [announcement]);
```

Ensure these are included in the return object of the hook:
```typescript
    isAnnouncementModalOpen,
    setIsAnnouncementModalOpen,
    dismissAnnouncementModal,
```

- [ ] **Step 3: Commit**

```bash
git add vite-frontend/src/pages/dashboard/use-dashboard-data.ts
git commit -m "feat(ui): manage announcement modal state in dashboard hook"
```

---

### Task 5: Add Modal to Dashboard Layout

**Files:**
- Modify: `vite-frontend/src/pages/dashboard.tsx`

- [ ] **Step 1: Write the minimal implementation**

Import the modal component at the top:
```tsx
import { AnnouncementModal } from "@/pages/dashboard/components/announcement-modal";
```

Add the new properties to the destructured `useDashboardData` object:
```tsx
    isAnnouncementModalOpen,
    setIsAnnouncementModalOpen,
    dismissAnnouncementModal,
```

Add the modal instance near the end of the dashboard rendering (just below `{announcement && <AnnouncementBanner ... />}` or inside the main `<div>`):
```tsx
      {announcement && (
        <AnnouncementModal
          announcement={announcement}
          isOpen={isAnnouncementModalOpen}
          onClose={() => setIsAnnouncementModalOpen(false)}
          onDontShowAgain={dismissAnnouncementModal}
        />
      )}
```

- [ ] **Step 2: Commit**

```bash
git add vite-frontend/src/pages/dashboard.tsx
git commit -m "feat(ui): add announcement modal to dashboard layout"
```
