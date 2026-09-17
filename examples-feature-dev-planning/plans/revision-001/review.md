# Feature Plan Review

## Objective

Feature implementation plan

## Relevant Repositories

- user-service
- order-service
- product-service
- shared-token-validation-lib
- api-gateway

## Requirements

- **R001**: Authenticated cart and order operations must derive identity from validated JWT claims instead of accepting arbitrary usernames or user ids.
- **R002**: Authenticated users must be able to fetch the current cart, including cart items, product display data, and a computed total.
- **R003**: Users must be able to retrieve only their own order history; order access must be scoped to the current authenticated user.
- **R004**: Order creation must validate the cart, load products, reject empty carts, reserve or decrement stock, persist order items, and clear only the purchased entries after success.
- **R005**: The system must define and document a partial-stock failure strategy for synchronous reservation and rollback behavior in the demo scope.
- **R006**: Order and cart persistence must be transaction-safe, with reliable order item persistence and a clear local transaction boundary.
- **R007**: Standardized HTTP status codes and error payloads must be implemented across the business services to replace generic exception behavior.
- **R008**: Input validation and authorization enforcement must be added for product mutation, user mutation, cart operations, and order status transitions using Bean Validation and role/ownership checks.
- **R009**: Gateway routing and authentication metadata must be explicit and consistent with the Consul basePath contract, including route discovery and auth behavior.
- **R010**: Required external configuration and runtime dependencies must be documented and validated without exposing secret values.
- **R011**: The demo must include focused tests for cart logic, order creation, authentication, and an end-to-end purchase flow covering persistence and cart clearing.

## Proposed Tasks

- **T001** [user-service]: Define JWT identity and auth contract for cart/order endpoints
- **T002** [order-service]: Implement authenticated cart retrieval and total calculation (depends on: T001)
- **T003** [order-service]: Implement user-order history and authenticated order access (depends on: T001, T002)
- **T004** [order-service]: Complete order creation with stock validation, persistence, and cart clearing (depends on: T002, T003)
- **T005** [product-service]: Align product stock and downstream integration behavior for purchase flow (depends on: T001)
- **T006** [shared-token-validation-lib]: Standardize validation, error handling, and authorization across services (depends on: T001, T002, T003, T004, T005)
- **T007** [api-gateway]: Document runtime dependencies and gateway routing contract (depends on: T001)
- **T008** [order-service]: Add purchase-flow and business-rule test coverage (depends on: T004, T005, T006)

## Dependency Graph

```text
T001 [user-service] Define JWT identity and auth contract for cart/order endpoints
T001 -> T002 [order-service]
T001 -> T003 [order-service]
T002 -> T003 [order-service]
T002 -> T004 [order-service]
T003 -> T004 [order-service]
T001 -> T005 [product-service]
T001 -> T006 [shared-token-validation-lib]
T002 -> T006 [shared-token-validation-lib]
T003 -> T006 [shared-token-validation-lib]
T004 -> T006 [shared-token-validation-lib]
T005 -> T006 [shared-token-validation-lib]
T001 -> T007 [api-gateway]
T004 -> T008 [order-service]
T005 -> T008 [order-service]
T006 -> T008 [order-service]
```

## Cross-Repository Dependencies

- T001 (user-service) -> T002 (order-service)
- T001 (user-service) -> T003 (order-service)
- T001 (user-service) -> T005 (product-service)
- T001 (user-service) -> T006 (shared-token-validation-lib)
- T002 (order-service) -> T006 (shared-token-validation-lib)
- T003 (order-service) -> T006 (shared-token-validation-lib)
- T004 (order-service) -> T006 (shared-token-validation-lib)
- T005 (product-service) -> T006 (shared-token-validation-lib)
- T001 (user-service) -> T007 (api-gateway)
- T005 (product-service) -> T008 (order-service)
- T006 (shared-token-validation-lib) -> T008 (order-service)

## Assumptions

- **A001** (HIGH, impact HIGH): The existing Spring Security JWT flow and shared token validation library remain the canonical authentication mechanism for protected order and cart endpoints.
- **A002** (HIGH, impact HIGH): The cart and order flows will stay within the existing service ownership model, with product availability checked through the product service and local persistence owned by order-service.
- **A003** (MEDIUM, impact MEDIUM): Consul metadata-based discovery and external configuration remain the runtime source of truth for service base paths during the demo implementation.

## Risks

- **RK001** [HIGH/CROSS_REPOSITORY]: Order creation depends on product availability and cart state across service boundaries, so inconsistent stock or cart updates can create purchase inconsistencies.
- **RK002** [HIGH/SECURITY]: The current API surfaces accept arbitrary usernames and generic exception handling, making authorization and identity enforcement brittle.
- **RK003** [MEDIUM/DATA_INTEGRITY]: Order items may not be persisted reliably because save operations are commented or transaction boundaries are incomplete.
- **RK004** [MEDIUM/OPS]: Gateway behavior and external configuration are runtime-dependent on Consul and Config Server, which can fail or be misconfigured in local/demo environments.

## Expected Change Areas

- **shared-token-validation-lib**: shared-token-validation-lib/src/main/java/com/rajugowda/jwt/validator, shared-token-validation-lib/src/test/java
- **api-gateway**: api-gateway/src/main/java/com/example/api_gateway, api-gateway/src/main/resources
- **config-server**: config-server/src/main/java/com/example/config, config-server/src/main/resources
- **order-service**: order-service/src/main/java/com/example/order/api, order-service/src/main/java/com/example/order/application, order-service/src/main/java/com/example/order/infrastructure, order-service/src/test/java
- **product-service**: product-service/src/main/java/com/example/product/api/controller, product-service/src/main/java/com/example/product/application/service, product-service/src/test/java
- **user-service**: user-service/src/main/java/com/example/user/api, user-service/src/main/java/com/example/user/application, user-service/src/main/java/com/example/user/security, user-service/src/test/java

## Verification Plan

Strategy: repository-aware

- **api-gateway**: cd api-gateway && mvn test -DskipTests=false, cd config-server && mvn test -DskipTests=false
- **user-service**: cd user-service && mvn test -DskipTests=false, cd shared-token-validation-lib && mvn test -DskipTests=false
- **order-service**: cd order-service && mvn test -DskipTests=false, cd product-service && mvn test -DskipTests=false && cd ../order-service && mvn test -DskipTests=false
- **product-service**: cd product-service && mvn test -DskipTests=false
- **shared-token-validation-lib**: cd shared-token-validation-lib && mvn test -DskipTests=false

## Open Questions

- None

## Plan Changes Since Previous Revision

- Initial plan revision

## Approval Status

Revision: 1

Status: REVIEW_PENDING

**USER APPROVAL REQUIRED**

### Validation Warnings

- cross-repository dependency: T001 (user-service) -> T002 (order-service)
- cross-repository dependency: T001 (user-service) -> T003 (order-service)
- cross-repository dependency: T001 (user-service) -> T005 (product-service)
- cross-repository dependency: T001 (user-service) -> T006 (shared-token-validation-lib)
- cross-repository dependency: T002 (order-service) -> T006 (shared-token-validation-lib)
- cross-repository dependency: T003 (order-service) -> T006 (shared-token-validation-lib)
- cross-repository dependency: T004 (order-service) -> T006 (shared-token-validation-lib)
- cross-repository dependency: T005 (product-service) -> T006 (shared-token-validation-lib)
- cross-repository dependency: T001 (user-service) -> T007 (api-gateway)
- cross-repository dependency: T005 (product-service) -> T008 (order-service)
- cross-repository dependency: T006 (shared-token-validation-lib) -> T008 (order-service)
