# Announcement Popup Notification Design

## Overview
This feature implements a popup notification modal for important dashboard announcements to ensure users see them immediately, addressing GitHub Issue #169.

## Requirements
1. Automatic display of a popup modal when opening the dashboard page if a new/updated announcement exists.
2. Includes a "Don't show again" option to remember the user's choice to dismiss it.
3. Smart triggering: Only pops up for *new* or *updated* announcements.
4. Support Markdown formatting for the announcement content.
5. Retain the existing permanent top banner as a fallback.

## Backend Changes (Go)
The `/api/v1/announcement/get` API currently only returns `content` and `enabled`. It must be updated to return the timestamp of the last update to enable the frontend to detect changes.

1.  **Repository (`internal/store/repo/repository.go`)**: Ensure `GetAnnouncement` retrieves `UpdatedTime` (or falls back to `CreatedTime`).
2.  **Handler (`internal/http/handler/handler.go`)**: Modify `getAnnouncement` to include an `update_time` (int64) field in its JSON response.

## Frontend Changes (Vite/React/Tailwind)
1.  **API Interface (`src/api/index.ts`)**:
    *   Update `AnnouncementData` to include `update_time: number`.
2.  **Storage Mechanism**:
    *   Use browser `localStorage` to persist the user's view state. Key: `flvx_announcement_seen_time`.
3.  **UI Component (`AnnouncementModal`)**:
    *   Create a new modal component for the dashboard.
    *   The modal content will render the markdown of the announcement.
    *   It will feature two primary actions:
        *   **"Close"**: Closes the modal temporarily for this session (does NOT update `localStorage`). It will pop up again on the next page load.
        *   **"Don't show again"**: Closes the modal AND sets `localStorage.setItem('flvx_announcement_seen_time', announcement.update_time)`.
4.  **Integration (`src/pages/dashboard.tsx` & `use-dashboard-data.ts`)**:
    *   Add state to manage the modal visibility (e.g., `isAnnouncementModalOpen`).
    *   On data load, compare the fetched `update_time` with the stored `flvx_announcement_seen_time`. If the fetched time is greater (or if no stored time exists), set `isAnnouncementModalOpen(true)`.

## Error Handling and Edge Cases
*   If `localStorage` is unavailable or throws an error (e.g., Private Browsing mode restrictions), the modal may show repeatedly. The code should safely catch `localStorage` access errors.
*   If `update_time` is missing from an old database record, the backend should gracefully fall back to the creation time or a safe default (like 0) to ensure the logic doesn't break.
