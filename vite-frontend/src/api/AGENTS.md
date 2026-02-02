# VITE FRONTEND (src/api) KNOWLEDGE BASE

## OVERVIEW
API client layer. Wraps axios and normalizes backend responses (`{ code, msg, data }`).

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Axios wrapper | `vite-frontend/src/api/network.ts` | Sets `axios.defaults.baseURL`; adds `Authorization` header |
| BaseURL init (WebView vs web) | `vite-frontend/src/api/network.ts` | WebView mode calls `getPanelAddresses()` |
| Endpoint functions | `vite-frontend/src/api/index.ts` | Mostly `Network.post("/…")` |

## CONVENTIONS
- Default baseURL is `/api/v1/` (or `${VITE_API_BASE}/api/v1/`).
- In WebView mode, baseURL is derived from the selected panel address; if unset, requests return `code: -1` with a “set panel address” message.
- 401 responses clear localStorage and redirect to `/`.
