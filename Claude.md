# WMS Project Instructions

## Project Overview

This project is a Warehouse Management System (WMS).

The frontend application is already completed using Vue.js.

The backend does not exist yet and must be developed using Go.

The application database is Microsoft SQL Server.

RabbitMQ is used for asynchronous/background processing where the HTTP client does not need to wait for the operation to complete.

Swagger/OpenAPI documentation must be provided for backend APIs.

The complete application must run using Docker and Docker Compose.

The current project does NOT require integration with:

* AGV
* AMR
* PLC
* MQTT
* ROS
* Fleet Management System
* Traffic Management System

Do not add these integrations unless explicitly requested.

---

# Technology Stack

## Frontend

Existing frontend:

* Vue 3
* Vite
* JavaScript or TypeScript depending on the existing project
* Vue Router
* Pinia if already used
* Axios or Fetch depending on the existing implementation

IMPORTANT:

* Do not replace Vue.
* Do not redesign existing UI unless explicitly requested.
* Do not rewrite working Vue components unnecessarily.
* Reuse the existing frontend structure.
* Preserve existing routes and workflows.

---

# Backend

Backend must use:

* Go
* Gin Web Framework
* Microsoft SQL Server
* GORM
* REST API
* JSON
* JWT Authentication
* bcrypt
* RabbitMQ
* Swagger / OpenAPI

Preferred packages:

Database:

gorm.io/gorm
gorm.io/driver/sqlserver

RabbitMQ:

github.com/rabbitmq/amqp091-go

Swagger:

github.com/swaggo/swag
github.com/swaggo/gin-swagger
github.com/swaggo/files

Use currently compatible stable versions.

Do not introduce another backend framework unless explicitly requested.

---

# High-Level Architecture

Use a modular monolith architecture with background workers.

Architecture:

Browser
|
v
Vue Frontend
|
| REST / JSON
v
Go API Server
|
+--------------------+
|                    |
v                    v
SQL Server          RabbitMQ
|
v
Go Worker
|
v
SQL Server

The API server and workers may share internal business packages.

Do NOT create separate microservice repositories.

Keep the application as a modular monolith unless explicitly requested otherwise.

---

# Synchronous vs Asynchronous Processing

Not every operation should use RabbitMQ.

Use synchronous REST processing when the frontend needs an immediate result.

Examples:

* Login
* Get user
* Get product
* Get inventory
* Search inventory
* Create simple master data
* Update warehouse
* Validate barcode
* Check stock
* View order
* CRUD operations that complete quickly

Flow:

Vue
|
v
Go API
|
v
SQL Server
|
v
HTTP Response

---

# RabbitMQ Usage

Use RabbitMQ only for asynchronous/background operations where the caller does not need to wait for final processing.

Examples may include:

* report generation
* large import processing
* large export processing
* batch operations
* background notifications
* audit processing
* long-running calculations
* large inventory reconciliation
* integration processing added in the future
* non-critical post-processing
* background cleanup
* asynchronous business workflows

Example:

Vue
|
| POST
v
Go API
|
| Validate request
|
| Create job record
|
| Publish message
v
RabbitMQ
|
v
Worker
|
v
Process Job
|
v
SQL Server

API may return:

HTTP 202 Accepted

Example:

{
"success": true,
"data": {
"job_id": "..."
},
"message": "Job accepted for processing"
}

---

# RabbitMQ Important Rule

Do NOT send every database operation through RabbitMQ.

Do NOT use RabbitMQ simply because it exists.

Before using RabbitMQ, determine whether:

1. The operation is long-running.
2. The frontend needs the final result immediately.
3. The operation can safely run asynchronously.
4. Retry is useful.
5. Duplicate processing can be handled safely.
6. Processing order matters.

If normal synchronous processing is simpler and appropriate, use REST + SQL Server directly.

---

# RabbitMQ Architecture

Preferred package structure:

backend/
internal/
messaging/
rabbitmq/
connection.go
publisher.go
consumer.go
topology.go

```
jobs/
  model.go
  service.go
  repository.go

workers/
  report_worker.go
  import_worker.go
  export_worker.go
```

Do not publish RabbitMQ messages directly from HTTP handlers.

Preferred flow:

Handler
|
v
Service
|
+------ Database
|
+------ Message Publisher

RabbitMQ infrastructure must remain separated from business logic.

---

# RabbitMQ Connection

Configure RabbitMQ using environment variables.

Example:

RABBITMQ_HOST=rabbitmq
RABBITMQ_PORT=5672
RABBITMQ_USER=wms
RABBITMQ_PASSWORD=password
RABBITMQ_VHOST=/

Do not hardcode credentials.

Inside Docker Compose the RabbitMQ hostname must be:

rabbitmq

not:

localhost

---

# RabbitMQ Management UI

For development, enable the RabbitMQ Management UI.

Recommended Docker image:

rabbitmq:management

Default internal ports:

5672  - AMQP
15672 - Management UI

Development access may be exposed as:

http://localhost:15672

Credentials must come from environment variables.

---

# RabbitMQ Queue Design

Use clear queue names.

Example naming:

wms.report.generate
wms.import.process
wms.export.process
wms.notification.process

Avoid generic queue names such as:

queue1
task
message

Queue names must describe their purpose.

---

# Message Format

Use JSON messages.

Every asynchronous message should include enough metadata for traceability.

Recommended structure:

{
"message_id": "uuid",
"event_type": "report.generate",
"created_at": "UTC timestamp",
"correlation_id": "uuid",
"user_id": "uuid",
"payload": {}
}

Do not place sensitive information such as passwords or JWT tokens inside RabbitMQ messages.

---

# Idempotency

RabbitMQ messages may occasionally be delivered more than once.

Consumers must be designed to safely handle duplicate messages.

For important operations:

* use message_id
* maintain processing status where appropriate
* check whether the message has already been processed
* avoid duplicate database changes

Never assume exactly-once delivery.

---

# RabbitMQ Acknowledgement

Workers must acknowledge messages only after successful processing.

On success:

ACK

On temporary failure:

retry according to defined retry strategy

On permanent failure:

send to Dead Letter Queue when appropriate.

Do not ACK a message before its required business operation successfully completes.

---

# Retry and Dead Letter Queue

For important asynchronous jobs, support:

Main Queue
|
v
Worker
|
+---- Success -> ACK
|
+---- Temporary Error -> Retry
|
+---- Repeated Failure -> DLQ

Do not create infinite retry loops.

Define a maximum retry count.

Log failed message ID and correlation ID.

---

# RabbitMQ Durability

Production queues should normally be durable.

Important messages should normally be persistent.

RabbitMQ must use a Docker volume so queue state is not unnecessarily lost when containers restart.

---

# SQL Server and RabbitMQ Consistency

Be careful when an operation requires both:

1. SQL Server update
2. RabbitMQ publish

Do not assume SQL Server transactions include RabbitMQ.

They are separate systems.

For simple non-critical events, publishing after a successful database transaction may be acceptable.

For important business events where message loss would cause inconsistent business state, prefer the Transactional Outbox Pattern.

---

# Transactional Outbox Pattern

For critical asynchronous business events:

HTTP Request
|
v
Service
|
v
SQL Transaction
|
+--- Business Data
|
+--- Outbox Record
|
v
COMMIT
|
v
Outbox Publisher
|
v
RabbitMQ

This prevents situations such as:

Database COMMIT succeeds
but
RabbitMQ publish fails

Do not implement the Outbox Pattern everywhere.

Use it when message delivery is important for business consistency.

---

# Background Job Status

For long-running jobs, maintain job status in SQL Server.

Suggested statuses:

PENDING
PROCESSING
COMPLETED
FAILED

Possible fields:

id
job_type
status
progress
created_by
created_at
started_at
completed_at
error_message

Frontend may query:

GET /api/v1/jobs/:id

Example:

{
"success": true,
"data": {
"id": "...",
"status": "PROCESSING",
"progress": 45
}
}

Do not keep critical job status only in memory.

---

# Swagger / OpenAPI

All REST APIs must be documented using Swagger/OpenAPI.

Use Swagger annotations compatible with swaggo.

Swagger documentation must be generated from backend API definitions.

Recommended packages:

github.com/swaggo/swag

github.com/swaggo/gin-swagger

github.com/swaggo/files

---

# Swagger Endpoint

Development Swagger UI should be available at:

/swagger/index.html

Example:

http://localhost:8080/swagger/index.html

Swagger should allow developers to:

* inspect API endpoints
* inspect HTTP methods
* inspect request parameters
* inspect request bodies
* inspect response models
* inspect HTTP status codes
* test API calls

---

# Swagger Documentation Rules

Every externally used REST endpoint should document:

* Summary
* Description where useful
* Tags
* Parameters
* Request body
* Success response
* Error responses
* Authentication requirement

Example annotation style:

// GetProducts godoc
// @Summary Get product list
// @Tags Products
// @Produce json
// @Param page query int false "Page"
// @Param limit query int false "Limit"
// @Success 200 {object} ProductListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /products [get]

Keep Swagger documentation synchronized with the actual API.

Do not document endpoints that do not exist.

---

# Swagger Authentication

Protected APIs must support JWT Bearer authentication in Swagger.

Swagger security definition should support:

Authorization: Bearer <JWT>

Developers must be able to authorize once through Swagger UI and test protected endpoints.

Never hardcode a valid production JWT into Swagger configuration.

---

# Swagger Generation

Provide a repeatable command for generating Swagger documentation.

Example:

swag init -g cmd/server/main.go

If project structure requires another command, document the correct command in README.

Swagger generation must not require manually editing generated Swagger files.

---

# REST API Standard

All application APIs must start with:

/api/v1

Examples:

POST   /api/v1/auth/login

GET    /api/v1/products
GET    /api/v1/products/:id
POST   /api/v1/products
PUT    /api/v1/products/:id
DELETE /api/v1/products/:id

GET    /api/v1/warehouses
GET    /api/v1/locations
GET    /api/v1/inventory

GET    /api/v1/jobs/:id

Swagger itself does not need to be under /api/v1.

---

# Standard API Response

Success:

{
"success": true,
"data": {},
"message": ""
}

List:

{
"success": true,
"data": [],
"meta": {
"page": 1,
"limit": 20,
"total": 100,
"total_pages": 5
}
}

Accepted asynchronous job:

{
"success": true,
"data": {
"job_id": "..."
},
"message": "Job accepted for processing"
}

Error:

{
"success": false,
"message": "error description",
"error": "ERROR_CODE"
}

---

# HTTP Status Codes

Use appropriate status codes.

200 OK

201 Created

202 Accepted

204 No Content

400 Bad Request

401 Unauthorized

403 Forbidden

404 Not Found

409 Conflict

422 Unprocessable Entity

500 Internal Server Error

Use 202 Accepted when an asynchronous operation was accepted but has not finished processing.

---

# Microsoft SQL Server

Microsoft SQL Server is the only application database unless explicitly changed.

Use:

gorm.io/driver/sqlserver

Database configuration must come from environment variables.

Example:

DB_HOST=sqlserver
DB_PORT=1433
DB_USER=sa
DB_PASSWORD=password
DB_NAME=wms
DB_ENCRYPT=false
DB_TRUST_SERVER_CERTIFICATE=true

Never hardcode credentials.

---

# SQL Server Connection From Docker

Backend container must connect to:

sqlserver:1433

Do not use localhost from inside the backend container.

Remember:

localhost inside a container means that same container.

---

# Database Design

Do not create tables blindly.

Before creating tables:

1. Inspect corresponding frontend module.
2. Identify required fields.
3. Identify relationships.
4. Identify unique constraints.
5. Identify indexes.
6. Identify nullability.
7. Identify business rules.

---

# Database Migrations

Database schema changes must be reproducible.

Keep migration files inside:

backend/migrations/

Do not rely solely on manually editing SQL Server.

GORM AutoMigrate may be used during early development when appropriate.

Do not treat AutoMigrate as the complete production migration strategy.

---

# Backend Architecture

Use:

HTTP Request
|
v
Handler
|
v
Service
|
v
Repository
|
v
SQL Server

For asynchronous operations:

HTTP Request
|
v
Handler
|
v
Service
|
+---- Repository
|
+---- RabbitMQ Publisher
|
v
RabbitMQ
|
v
Worker
|
v
Service
|
v
Repository
|
v
SQL Server

Handlers must never contain database or RabbitMQ implementation logic directly.

---

# Recommended Project Structure

backend/

cmd/
server/
main.go

```
worker/
  main.go
```

internal/

```
auth/
  handler.go
  service.go
  repository.go
  model.go

user/

warehouse/

zone/

location/

product/

inventory/

inbound/

outbound/

order/

picking/

jobs/

messaging/
  rabbitmq/
    connection.go
    publisher.go
    consumer.go
    topology.go

workers/
```

config/

migrations/

docs/

pkg/

go.mod
go.sum
Dockerfile
.env.example

The API server and worker should share business packages rather than duplicate business logic.

---

# Existing Frontend Is Source Of Truth

Before implementing backend functionality:

1. Inspect Vue views.
2. Inspect components.
3. Inspect router.
4. Inspect stores.
5. Inspect services.
6. Inspect mock data.
7. Inspect localStorage/sessionStorage.
8. Inspect TypeScript models/interfaces.
9. Inspect forms.
10. Inspect tables.
11. Inspect filters.
12. Inspect search.
13. Inspect sorting.
14. Inspect pagination.
15. Inspect authentication.
16. Inspect roles/permissions.

Do not guess requirements that can be discovered from source code.

---

# Mock Data Migration

Do not immediately remove mock data.

For each module:

1. Analyze current mock structure.
2. Determine required API.
3. Determine SQL Server schema.
4. Determine whether operation should be synchronous or asynchronous.
5. Implement backend.
6. Test via Swagger.
7. Integrate Vue.
8. Verify UI.
9. Remove obsolete mock data.

---

# Authentication

Use JWT.

Public example:

POST /api/v1/auth/login

Protected endpoints use centralized authentication middleware.

Use bcrypt for passwords.

Never store plain-text passwords.

---

# Frontend API Integration

Centralize frontend API configuration.

Preferred:

frontend/src/services/api.ts

Do not hardcode backend URLs in Vue components.

Example:

VITE_API_BASE_URL=/api/v1

When accessed through Nginx, prefer reverse proxying:

/api/*
|
v
backend:8080

This avoids unnecessary production CORS complexity.

---

# Docker Requirements

The complete application must run with Docker Compose.

Required services:

* frontend
* backend
* worker
* sqlserver
* rabbitmq

Concept:

docker compose
|
+-- frontend
|
+-- backend
|
+-- worker
|
+-- sqlserver
|
+-- rabbitmq

---

# Docker Compose Architecture

Browser
|
v
Frontend / Nginx
|
+--- /api ----------> backend:8080
|
v

Backend
|
+------------------> sqlserver:1433
|
+------------------> rabbitmq:5672

Worker
|
+------------------> rabbitmq:5672
|
+------------------> sqlserver:1433

RabbitMQ Management:

localhost:15672

Swagger:

localhost:8080/swagger/index.html

---

# Docker Persistence

SQL Server must use a persistent Docker volume.

RabbitMQ must use a persistent Docker volume.

Example logical volumes:

sqlserver_data

rabbitmq_data

Do not rely on temporary container filesystem for persistent database or queue data.

---

# Docker Health Checks

Where practical provide health checks for:

* SQL Server
* RabbitMQ
* Backend

Backend should not assume dependencies are immediately ready merely because containers started.

Implement connection retry/backoff where appropriate.

---

# Environment Configuration

Use environment variables.

Example:

APP_PORT=8080
APP_ENV=development

DB_HOST=sqlserver
DB_PORT=1433
DB_USER=sa
DB_PASSWORD=
DB_NAME=wms

JWT_SECRET=

RABBITMQ_HOST=rabbitmq
RABBITMQ_PORT=5672
RABBITMQ_USER=wms
RABBITMQ_PASSWORD=
RABBITMQ_VHOST=/

Do not commit production credentials.

Provide:

.env.example

---

# Health Endpoint

Provide:

GET /api/v1/health

Health information should allow development verification of application dependencies.

Example:

{
"success": true,
"data": {
"status": "ok",
"database": "connected",
"rabbitmq": "connected"
}
}

Do not expose credentials or internal connection strings.

---

# Logging

Use structured logs.

Important fields may include:

request_id
correlation_id
message_id
user_id
method
endpoint
status
duration
job_id

For workers also log:

queue
message_id
attempt
processing_duration

Never log passwords, JWT secrets or database credentials.

---

# Correlation IDs

Use correlation IDs for asynchronous workflows where useful.

Example:

HTTP Request
correlation_id=ABC
|
v
RabbitMQ Message
correlation_id=ABC
|
v
Worker Log
correlation_id=ABC

This makes tracing asynchronous operations easier.

---

# Inventory Design

Inventory should be location based.

Warehouse
|
v
Zone
|
v
Location
|
v
Inventory

Maintain inventory transaction history.

Recommended transaction types:

RECEIVE
PUTAWAY
MOVE
PICK
SHIP
ADJUST
RETURN

Important inventory changes must be traceable.

---

# Database Transactions

Use SQL Server transactions when multiple database changes form one business operation.

Examples:

Order + Order Items

Receiving + Inventory + Inventory Transaction

Picking + Inventory Deduction + Inventory Transaction

If required operations fail, rollback.

Remember:

SQL Server transactions do not include RabbitMQ transactions.

Use appropriate consistency patterns when both systems are involved.

---

# Validation

Backend validation is mandatory.

Validate:

* required fields
* ranges
* status
* IDs
* quantity
* duplicate codes
* relationships
* business rules

Never rely only on Vue validation.

---

# Search / Filter / Pagination

If frontend provides:

* search
* filter
* sorting
* pagination

support these in backend where appropriate.

Example:

GET /api/v1/products?page=1&limit=20&search=ABC

Do not load an entire large SQL Server table to the frontend only to filter it locally.

---

# No AGV Integration

Do not implement:

* AGV
* AMR
* MQTT
* PLC
* ROS
* Traffic Management
* Fleet Management

unless explicitly requested later.

RabbitMQ in this project is for WMS asynchronous/background processing.

It does not imply AGV integration.

---

# Coding Rules

Write idiomatic Go.

Prefer:

* simple architecture
* explicit error handling
* focused functions
* maintainable code

Avoid:

* premature microservices
* unnecessary abstractions
* unnecessary dependencies
* excessive interfaces
* duplicated business logic

Run:

go fmt ./...

before completing backend changes.

---

# Claude Development Workflow

For each module:

## STEP 1

Inspect existing Vue frontend.

## STEP 2

Identify data requirements.

## STEP 3

Determine:

SYNCHRONOUS

or

ASYNCHRONOUS

Do not automatically choose RabbitMQ.

Explain why asynchronous processing is required when choosing it.

## STEP 4

Design REST API.

## STEP 5

Update Swagger/API contract.

## STEP 6

Design SQL Server schema.

## STEP 7

Implement:

Model
Repository
Service
Handler
Route

If asynchronous:

Publisher
Queue
Consumer/Worker
Job tracking

## STEP 8

Generate Swagger documentation.

## STEP 9

Test API via automated tests and Swagger.

## STEP 10

Integrate Vue.

## STEP 11

Remove obsolete mock data.

## STEP 12

Verify Docker environment.

---

# Before Major Changes

For major architecture/database/messaging changes explain:

1. Current implementation
2. Problem
3. Proposed solution
4. Files affected
5. SQL Server impact
6. RabbitMQ impact
7. Frontend impact
8. Docker impact
9. API/Swagger impact

Then implement.

Small isolated fixes can be implemented directly.

---

# Backend Verification

After backend changes run:

go fmt ./...

go vet ./...

go test ./...

Generate Swagger:

swag init -g cmd/server/main.go

or use the correct project-specific command.

Verify backend builds successfully.

---

# Frontend Verification

After frontend changes:

npm run build

Fix compilation errors before considering the task complete.

---

# Docker Verification

After Docker changes:

docker compose config

docker compose build

docker compose up

Verify:

1. SQL Server starts and becomes healthy.
2. RabbitMQ starts and becomes healthy.
3. RabbitMQ Management UI is reachable.
4. Backend connects to SQL Server.
5. Backend connects to RabbitMQ.
6. Worker connects to RabbitMQ.
7. GET /api/v1/health works.
8. Swagger UI works.
9. Vue loads.
10. Vue communicates with Go backend.
11. Worker can consume a test message.
12. SQL Server data persists after restart.
13. RabbitMQ configuration/data persists as expected.

---

# Definition Of Done

A synchronous API feature is complete when:

* Go compiles
* API works
* SQL Server works
* validation works
* Swagger is updated
* Swagger endpoint can be tested
* Vue successfully uses API
* UI remains correct
* Docker environment works

An asynchronous feature is complete when:

* API accepts job correctly
* HTTP 202 is returned when appropriate
* RabbitMQ publish works
* Worker consumes message
* ACK behavior is correct
* retry/error handling works
* duplicate processing is handled safely where required
* job status is stored when required
* SQL Server changes are correct
* Swagger documents the request/status API
* logs contain message/correlation IDs
* Docker environment works

---

# Critical Instructions For Claude

Do not create a new frontend.

Do not redesign completed UI.

Do not replace SQL Server.

Do not add PostgreSQL.

Do not add SQLite.

Do not add AGV integration.

Do not add MQTT.

Do not create microservices.

Do not route every operation through RabbitMQ.

Do not put RabbitMQ logic directly in HTTP handlers.

Do not expose RabbitMQ directly to the Vue frontend.

Do not allow frontend code to publish or consume RabbitMQ messages.

Do not create undocumented APIs.

Do not manually edit generated Swagger files when they can be regenerated.

Always inspect existing frontend implementation before designing backend APIs.

Keep the architecture simple:

Vue
+
Go REST API
+
SQL Server
+
RabbitMQ Worker
+
Swagger
+
Docker Compose
