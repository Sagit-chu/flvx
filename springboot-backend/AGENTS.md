# SPRINGBOOT BACKEND KNOWLEDGE BASE

## OVERVIEW
Admin API for Flux Panel. Manages users, licenses, and traffic rules.
**Stack:** Java 21, Spring Boot 2.7.18, SQLite, MyBatis Plus.

## STRUCTURE
```
springboot-backend/
├── src/main/java/com/admin/
│   ├── controller/      # API Endpoints
│   ├── entity/          # DB Models (MyBatis Plus)
│   ├── mapper/          # Data Access
│   ├── service/         # Business Logic
│   └── common/          # Utils, DTOs
└── src/main/resources/
    ├── application.yml  # Config
    ├── mapper/          # XML Mappers
    ├── data.sql         # Init data
    └── bgimages/        # Static resources
```

## CONVENTIONS
- **DB**: SQLite used via `sqlite-jdbc`.
- **ORM**: MyBatis Plus + MyBatis Plus Join.
- **JSON**: FastJSON2 used for serialization.
- **Utils**: Hutool used extensively.
- **Auth**: Likely custom or token-based (see `controller` logic).

## COMMANDS
```bash
# Build
mvn clean package

# Run
java -jar target/admin-0.0.1-SNAPSHOT.jar
```
