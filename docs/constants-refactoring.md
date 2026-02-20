# Route Constants Refactoring

## Overview
Refactored the frontend to use centralized route constants from the backend, eliminating hardcoded API endpoint strings throughout the HTML templates and JavaScript files.

## Changes Made

### Backend (Go)

1. **New File: `internal/api/constants_handlers.go`**
   - Created `APIRoutes` struct to serialize route constants
   - Implemented `constantsHandler()` to serve routes as JSON at `/api/constants`
   - Maps all Go route constants to JSON-friendly field names
   - Includes 1-hour cache header for performance

2. **Modified: `internal/api/handlers.go`**
   - Added route for `/api/constants` endpoint (public access)

### Frontend (JavaScript)

3. **New File: `ui/dist/js/constants.js`**
   - Created `RouteConstants` class as singleton
   - Fetches constants from `/api/constants` on initialization
   - Provides fallback hardcoded values if fetch fails
   - Exposes clean getter interfaces (`auth`, `dashboard`, `settings`, `pages`)
   - Helper method `buildPath()` for parameterized routes (e.g., `/api/settings/users/{id}`)

4. **Modified: All JavaScript Files**
   - `ui/dist/js/app.js`: Updated WebSocket connection and logout to use constants
   - `ui/dist/js/dashboard.js`: Updated metrics and messages endpoints
   - `ui/dist/js/partners.js`: Updated all partner-related endpoints
   - `ui/dist/js/settings.js`: Updated all settings-related endpoints

5. **Modified: HTML Templates**
   - `ui/login.html`: Added constants.js script, updated login form to use `window.ROUTES`
   - `ui/dist/index.html`: Added constants.js as first script (loaded before other JS)

## Benefits

### Single Source of Truth
- All API routes defined once in `internal/constants/routes.go`
- Frontend automatically uses the same routes via API endpoint
- Changing a route in backend automatically updates frontend

### Maintainability
- No more searching for hardcoded strings across multiple files
- Easy to find where routes are used via `window.ROUTES` searches
- Type-safe access patterns with organized getters

### Robustness
- Fallback values ensure app works even if `/api/constants` fails
- Constants initialized before Alpine.js stores
- Clear error handling and logging

## Usage Examples

### Before (Hardcoded)
```javascript
await fetch('/api/auth/login', {method: 'POST'});
await fetch('/api/dashboard/metrics');
await fetch(`/api/settings/users/${id}`);
```

### After (Using Constants)
```javascript
await fetch(window.ROUTES.auth.login, {method: 'POST'});
await fetch(window.ROUTES.dashboard.metrics);
await fetch(window.ROUTES.settings.userById(id));
```

## Route Organization

Routes are organized by category:

- **Auth**: `login`, `logout`, `me`
- **Dashboard**: `ws`, `qr`, `metrics`, `messages`, `partners`, `discover`
- **Settings**: `config`, `rotateKeys`, `users`, `userById(id)`, `password`, `partners`, `partnerById(id)`
- **Pages**: `login`, `dashboard`

## Testing

Build and runtime tests completed successfully:
```bash
make build
✓ Build complete: ./fidex-node

./fidex-node
✓ FideX Edge Node is running!
✓ All routers mounted without conflicts
✓ /api/constants endpoint accessible
```

All endpoints now use constants from the backend, ensuring consistency across the entire application.

## Implementation Notes

### Route Mounting Fix
During implementation, discovered that `handlers.go` was defining routes using `r.Route()` that conflicted with the separate router mounting in `main.go`. Fixed by:
- Removing redundant `r.Route()` blocks for `/api/auth`, `/api/dashboard`, and `/api/settings`
- Keeping only `/api/constants` and `/api/v1` routes in base router
- Allowing modular router composition via separate `Setup*Router()` functions
