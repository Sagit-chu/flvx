# SPRINGBOOT BACKEND (com.admin) KNOWLEDGE BASE

## OVERVIEW
Primary Java code for the admin API. Controllers expose `/api/v1/*` endpoints and return `R` response envelopes.

## STRUCTURE
```
springboot-backend/src/main/java/com/admin/
├── controller/  # REST controllers (e.g., /api/v1/user)
├── service/     # Business logic interfaces + impl/
├── mapper/      # MyBatis Plus mappers
├── entity/      # DB entities
├── config/      # WebMvc/JWT/CORS/WebSocket config
└── common/      # DTOs, auth, exception handling, utilities
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| User/login endpoints | `springboot-backend/src/main/java/com/admin/controller/UserController.java` | `/api/v1/user/*` |
| Auth enforcement | `springboot-backend/src/main/java/com/admin/config/WebMvcConfig.java` | Intercepts `/api/**`, excludes login/config/captcha |
| JWT validation | `springboot-backend/src/main/java/com/admin/common/interceptor/JwtInterceptor.java` | Requires `Authorization` header |
| Admin-only ops | `springboot-backend/src/main/java/com/admin/common/annotation/RequireRole.java` | Enforced by `RoleAspect` |
| Response envelope | `springboot-backend/src/main/java/com/admin/common/lang/R.java` | `code == 0` success |
| Global error handling | `springboot-backend/src/main/java/com/admin/common/exception/GlobalExceptionHandler.java` | Maps exceptions -> `R.err(...)` |

## CONVENTIONS
- Controllers are mostly `@PostMapping` (even for list/get/delete) and use `/api/v1/*` prefixes.
- JWT is custom (no 3p lib) and includes `role_id` in payload (`springboot-backend/src/main/java/com/admin/common/utils/JwtUtil.java`).

## ANTI-PATTERNS
- Do not change auth header format lightly: frontend expects `Authorization: <token>` (no `Bearer`).
