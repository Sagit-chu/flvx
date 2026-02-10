# FLVX - Modification Notice

This project is a derivative work based on **flux-panel** (https://github.com/bqlpfy/flux-panel).

## Original Project
- **Name**: flux-panel
- **Source**: https://github.com/bqlpfy/flux-panel
- **License**: Apache License 2.0

## Modifications
The following major changes and additions have been made in this fork (FLVX):

### 1. Backend Architecture (Replaced)
- **Removed**: The original `springboot-backend/` (Java/Spring Boot) has been entirely removed.
- **Added**: A new `go-backend/` (Go/SQLite) implementation replaces the original backend.

### 2. Forwarding Agent (Modified)
- **Modified**: `go-gost/` - Modified forwarding agent wrapper.
- **Modified**: `go-gost/x/` - Modified local fork of the `gost` extensions library.

### 3. Frontend (Modified)
- **Modified**: `vite-frontend/` - Significant updates to the React/Vite dashboard to compatible with the new Go backend, including UI/UX improvements (HeroUI + Tailwind).

### 4. Mobile Applications (Removed)
- **Removed**: `android-app/` - Source code for the Android client.
- **Removed**: `ios-app/` - Source code for the iOS client.

### 5. Infrastructure & Scripts
- **Modified**: `docker-compose-v4.yml`, `docker-compose-v6.yml` (Updated for Go backend).
- **Modified**: `install.sh`, `panel_install.sh` (Updated installation logic).
- **Added**: `AGENTS.md` (Project documentation).

## License for Modifications
All new files, directories, and modifications introduced in this fork are licensed under the **GNU General Public License v3.0 (GPLv3)**.

> This program is free software: you can redistribute it and/or modify
> it under the terms of the GNU General Public License as published by
> the Free Software Foundation, either version 3 of the License, or
> (at your option) any later version.

For the full text of the GPLv3, see <https://www.gnu.org/licenses/gpl-3.0.html>.
