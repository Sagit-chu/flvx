# Flvx iOS 26 Liquid Glass UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the Flvx frontend interface entirely into an "Apple iOS 26 Liquid Glass" visual style by utilizing high-radius squircles, heavy background blurs, mesh gradients, and highly semantic translucent containers.

**Architecture:** We will approach this from the ground up: first defining the global TailwindCSS design tokens and the base mesh-gradient layout, then systematically replacing the structural styling inside each React page component (`vite-frontend/src/pages/*.tsx`).

**Tech Stack:** React DOM, TailwindCSS (v4), shadcn-bridge (HeroUI), Vite

---

### Task 1: Setup Global CSS Variables and App Shell

**Files:**
- Modify: `vite-frontend/src/styles/globals.css` (or `index.css`)
- Modify: `vite-frontend/tailwind.config.js`
- Modify: `vite-frontend/src/App.tsx` (or `main.tsx` / `layouts` depending on structural entry point)

- [ ] **Step 1: Inject Liquid Glass theme variables**
  Open the main CSS file and add variables for the new blur radius, box-shadows, and background gradients.
  ```css
  :root {
    --glass-bg: rgba(255, 255, 255, 0.6);
    --glass-border: rgba(255, 255, 255, 0.8);
    --glass-card: rgba(255, 255, 255, 0.7);
    --glass-overlay: rgba(0, 0, 0, 0.3);
  }
  .dark {
    --glass-bg: rgba(30, 30, 30, 0.6);
    --glass-border: rgba(255, 255, 255, 0.15);
    --glass-card: rgba(40, 40, 40, 0.6);
  }
  .bg-mesh-gradient {
    background: radial-gradient(at 0% 0%, #ff9a9e 0%, transparent 50%),
                radial-gradient(at 100% 0%, #fecfef 0%, transparent 50%),
                radial-gradient(at 100% 100%, #c2e9fb 0%, transparent 50%),
                radial-gradient(at 0% 100%, #a1c4fd 0%, transparent 50%);
    background-color: #f2f2f7;
  }
  ```

- [ ] **Step 2: Update App Layout**
  Modify the root app container to use `.bg-mesh-gradient` and ensure the main container occupies `min-h-screen`.

### Task 2: Refactor Global Components (Card & Modal)

**Files:**
- Modify: `vite-frontend/src/shadcn-bridge/heroui/card.tsx`
- Modify: `vite-frontend/src/shadcn-bridge/heroui/modal.tsx`

- [ ] **Step 1: Liquid Card Base**
  Update the default className string for `Card` to incorporate: `backdrop-blur-3xl bg-white/60 dark:bg-zinc-900/60 border border-white/80 dark:border-white/10 rounded-2xl shadow-[0_10px_30px_rgba(0,0,0,0.1)]`.

- [ ] **Step 2: Modal Overlay Base**
  Update the default overlay className for `Modal` to use `bg-black/30 backdrop-blur-sm`, and its content panel to use the same `glass_card` classes as the Card component but with `rounded-3xl`.

### Task 3: Redesign Dashboard Page

**Files:**
- Modify: `vite-frontend/src/pages/dashboard.tsx`

- [ ] **Step 1: Replace hardcoded borders/bg with glass semantics**
  Find hardcoded `bg-white`, `border-gray-200`, `shadow-md` inside `DashboardPage` and `MetricCard`, replace with `bg-white/60 backdrop-blur-3xl shadow-[0_10px_30px_rgba(0,0,0,0.1)] border-white/80 rounded-2xl`.
- [ ] **Step 2: Adjust spacing**
  Ensure all metric cards have uniform `h-48` equivalent height and are strictly `rounded-2xl` with `p-6` padding.
- [ ] **Step 3: Update Flow Chart Card**
  Replace standard grid backgrounds in the flow chart with transparent spacing and vibrant `bg-blue-500` squircle bars without harsh borders.

### Task 4: Redesign Node Management Page

**Files:**
- Modify: `vite-frontend/src/pages/node.tsx`

- [ ] **Step 1: Replace standard List/Table view with Grid Cards**
  Update the node rendering map to output `glass_card` containers (`rounded-2xl`, blur, padding `p-6`).
- [ ] **Step 2: Apply semantic status highlights**
  Refactor the Online/Offline badges into pill-shapes (`rounded-full`) using the defined semantic colors (e.g. `bg-green-500/20 text-green-600` with a 6px inner dot `bg-green-500`).
- [ ] **Step 3: Embed Micro-charts**
  For CPU/RAM data inside the node card, switch standard progress bars to ultra-thin (height 4px) continuous lines utilizing standard brand colors.

### Task 5: Redesign Tunnels & Rules Configuration

**Files:**
- Modify: `vite-frontend/src/pages/tunnel.tsx`
- Modify: `vite-frontend/src/pages/forward.tsx`

- [ ] **Step 1: Update Tunnel lists into nested Glass Panels**
  Encapsulate each tunnel configuration into a wide `glass_card`.
- [ ] **Step 2: Create Visual Rule Tags**
  For the Forwarding rules, wrap the target IP/Port logic into visual badges: `bg-green-500/20` for Entry and `bg-blue-500/20` for Target.
- [ ] **Step 3: Refactor the "Add Rule" Floating action**
  Ensure the plus button follows the squircle format (`rounded-full`) with a prominent diffused shadow (`shadow-[0_4px_12px_rgba(0,122,255,0.3)]`).

### Task 6: Redesign Monitor Page

**Files:**
- Modify: `vite-frontend/src/pages/monitor.tsx`

- [ ] **Step 1: Style the Top Hero Metrics**
  Replace flat stat boxes with high-contrast, large typography inside `glass_card` backgrounds.
- [ ] **Step 2: Refactor Latency Indicators**
  Format the connection list rows as `bg-white/50 dark:bg-black/30` strips with pill-shaped status tags (`Healthy`, `Warning`, `Offline`) mapping exactly to the green/orange/red semantics from the design spec.

### Task 7: Redesign Group & Sharing Pages

**Files:**
- Modify: `vite-frontend/src/pages/group.tsx`
- Modify: `vite-frontend/src/pages/panel-sharing.tsx`

- [ ] **Step 1: Update Tab Switchers**
  Refactor the internal navigation tabs (e.g., "Tunnel Groups" vs "User Groups") into an encapsulated `p-1 rounded-xl bg-white/40 backdrop-blur-lg` container with animated active states (`shadow-sm bg-white`).
- [ ] **Step 2: Style Share Cards**
  Transform flat panel sharing list items into rich `glass_card` entities. Highlight expiration dates with the accent text color.

### Task 8: Redesign Settings, Config, and User Management

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx`
- Modify: `vite-frontend/src/pages/settings.tsx`
- Modify: `vite-frontend/src/pages/user.tsx`
- Modify: `vite-frontend/src/pages/limit.tsx`

- [ ] **Step 1: Flatten Forms**
  Convert traditional input groups into `rounded-xl bg-white/50 border border-white/60` containers. Remove outer boxing for standard `label + input` pairs.
- [ ] **Step 2: iOS Toggle Switches**
  Ensure that any `<Switch>` or `<Checkbox>` components use the new Accent brand color (`#007aff`) with full `rounded-full` geometry.
- [ ] **Step 3: Refactor User Badges**
  In `user.tsx`, replace text-based role columns with circular Avatar badges (e.g., `w-10 h-10 rounded-full bg-blue-500 text-white` with the first two letters of the username).

### Task 9: Profile & Password Modal Restyling

**Files:**
- Modify: `vite-frontend/src/pages/profile.tsx`
- Modify: `vite-frontend/src/pages/change-password.tsx`

- [ ] **Step 1: Apply Profile Card Structure**
  Create a split view on desktop using flex: Left side (Avatar + User Info + Admin Shortcuts), Right side (Password Form).
- [ ] **Step 2: Restyle Auth Inputs**
  Ensure all password inputs use `bg-white/50 backdrop-blur-md border border-white/60` and the update button has heavy shadow-glow.
