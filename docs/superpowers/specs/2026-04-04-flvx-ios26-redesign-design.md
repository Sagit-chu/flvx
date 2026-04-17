# Flvx iOS 26 Liquid Glass UI Redesign Spec

## 1. Overview
This document specifies the comprehensive UI/UX redesign of the Flvx frontend using an "Apple iOS 26 Liquid Glass" design language. The goal is to elevate the visual quality of the entire application, making it modern, spatially aware, and highly legible through extensive use of blur, translucency, squircle borders, and semantic contrast.

## 2. Scope
The redesign covers 100% of the frontend routing pages and overlay components under `vite-frontend/src/pages/` and global UI modules:
- Dashboard (`dashboard.tsx`)
- Node Management (`node.tsx`)
- Tunnel & Rule Configurations (`tunnel.tsx`, `forward.tsx`)
- System Monitor (`monitor.tsx`)
- User Management (`user.tsx`)
- Speed Limit Management (`limit.tsx`)
- Group Management (`group.tsx`)
- Panel Sharing (`panel-sharing.tsx`)
- Global Settings & Config (`config.tsx`, `settings.tsx`)
- Profile & Change Password (`profile.tsx`, `change-password.tsx`)
- All related Modals, Drawers, and floating UI (e.g., "Create Node", "Add Rule" forms).

## 3. Design System & Tokens
The new UI replaces traditional solid-color borders and flat surfaces with the following spatial design tokens:

### 3.1. Corner Radii (Squircles)
- **Outer Shell / Viewports**: 32px (`rounded-3xl` equivalent)
- **Cards / Containers**: 24px (`rounded-2xl`)
- **Buttons / Inputs**: 16px (`rounded-xl` or `rounded-full`)
- **Badges / Tags**: 6px or fully rounded.

### 3.2. Backgrounds & Blurs
- **Global Background**: A mesh gradient blending soft pinks and blues (`#ff9a9e`, `#fecfef`, `#a1c4fd`, `#c2e9fb`).
- **Glass Base (Primary Containers)**: `backdrop-blur-3xl` with an ultra-thin white overlay (`rgba(255, 255, 255, 0.6)` or `#ffffff99`).
- **Glass Inner Glow (Borders)**: 1px solid `rgba(255, 255, 255, 0.8)` (`#ffffffcc`).
- **Modals Background Overlay**: 30% black overlay (`#0000004d`).

### 3.3. Semantic Colors
- **Brand / Active / Primary / TCP**: Blue `#007aff`
- **Healthy / Success / Online**: Green `#34c759`
- **Warning / Wait / UDP**: Orange `#ff9500`
- **Danger / Offline / Delete**: Red `#ff3b30`
- **Secondary / Purple / Data**: Purple `#af52de`
- **Text**: Primary (`#1d1d1f`), Secondary (`#86868b`).

### 3.4. Elevation & Shadow
- Soft, highly diffused drop shadows rather than sharp lines: e.g., `box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1)`.

## 4. Implementation Strategy
We will implement the redesign systematically across the React + TailwindCSS + shadcn/HeroUI stack:
1. **CSS Variables / Tailwind Config**: Inject the new Liquid Glass design tokens (colors, extended radiuses, customized backdrop blurs, box shadows) into `tailwind.config.js` and `globals.css`.
2. **Global App Shell**: Update the root layout (`index.tsx` or main `App` layout) to host the dynamic mesh gradient background and the new translucent sidebar.
3. **Component Re-styling**: 
   - Override HeroUI default card, input, and modal styles using custom `classNames`.
   - Update `MetricCard`, `PageEmptyState`, `PageLoadingState`, and other base components to support the `glass_card` spec.
4. **Page-by-Page Integration**: Rewrite the JSX of each page to utilize the new layout structure, ensuring all existing interactive state and API logic is seamlessly preserved.
5. **Modal System Update**: Apply the transparent `#0000004d` overlay and 480px width glassy card style to all global dialogs.

## 5. Success Criteria
- [ ] No regression in business logic; all forms, interactions, and data rendering operate exactly as before.
- [ ] The visual system consistently employs the `glass_bg`, `glass_card`, and corresponding squircle radiuses across 100% of the UI.
- [ ] Modals and Overlays correctly blur the background mesh gradient.
- [ ] All responsive layouts appropriately wrap the card components on smaller displays.
