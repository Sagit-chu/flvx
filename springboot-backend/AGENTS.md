# SPRINGBOOT BACKEND KNOWLEDGE BASE

**Generated:** Mon Feb 02 2026

## OVERVIEW
Admin API for Flux Panel. Manages users, tunnels, nodes, forwards, quotas, and speed limits.
**Stack:** Java 21, Spring Boot 2.7.18, SQLite, MyBatis Plus (+ join), FastJSON2.

## STRUCTURE
```
springboot-backend/
├── src/main/java/com/admin/
│   ├── controller/      # /api/v1/* endpoints
│   ├── entity/          # DB models
│   ├── mapper/          # MyBatis Plus mappers
│   ├── service/         # Business logic
│   ├── config/          # WebMvc/JWT/CORS/WebSocket config
│   └── common/          # DTOs, auth, exception handling, utilities
└── src/main/resources/
    ├── application.yml  # Config (DB_PATH/JWT_SECRET/LOG_DIR)
    ├── mapper/          # XML mappers
    ├── schema.sql       # Schema
    └── data.sql         # Seed data
```

## CONVENTIONS
- **DB**: SQLite URL is `jdbc:sqlite:${DB_PATH:/app/data/gost.db}` (`springboot-backend/src/main/resources/application.yml`).
- **Auth**: JWT in `Authorization` header; enforced by `com.admin.common.interceptor.JwtInterceptor` for `/api/**` (with explicit excludes in `com.admin.config.WebMvcConfig`).
- **Roles**: `@RequireRole` means admin-only (`role_id == 0`) via `com.admin.common.aop.RoleAspect`.
- **Responses**: Controllers return `com.admin.common.lang.R` (`code == 0` success).
- **CORS**: Allow-all origins; `Authorization` is exposed (`com.admin.config.WebMvcConfig`).

## COMMANDS
```bash
cd springboot-backend
mvn clean package
mvn test
java -jar target/admin-0.0.1-SNAPSHOT.jar
```
