# Feature: <short feature name>

## Objective

One paragraph describing the business outcome and success criteria at a high level.

## Background

Context the agent needs: existing behavior, pain points, and constraints.

## Requirements

Authoritative functional requirements. Use **one bullet per capability** — do not bundle multiple behaviors into a single line.

- <!-- req: R001 --> User can view their profile details
- <!-- req: R002 --> User can update profile name and email
- <!-- req: R003 --> User can add a new shipping address
- <!-- req: R004 --> User can edit an existing shipping address
- <!-- req: R005 --> User can set a primary shipping address
- <!-- req: R006 --> User can switch the primary address to another saved address

### user-service

Repository-scoped requirements (optional subsection):

- <!-- req: R010 --> Expose profile REST endpoints under `/api/v1/profile`

### order-service

- <!-- req: R020 --> Persist address changes and emit domain events

## Additional Business Requirements

Cross-cutting or product rules not tied to a single repo:

- <!-- req: R030 --> Primary address must be unique per user account
- <!-- req: R031 --> Address deletion must be blocked when it is the only saved address

## Acceptance Criteria

Testable outcomes. Each bullet should map to at least one task with `requirement_ids` and verification.

- <!-- req: AC001 --> Given a user with two addresses, when they set address B as primary, then checkout uses address B
- <!-- req: AC002 --> Given invalid postal code input, when user saves address, then API returns 400 with field errors
- <!-- req: AC003 --> Given concurrent profile updates, when two requests race, then last-write-wins is documented and tested

## Repositories

- user-service
- order-service
- workspace (for cross-repo integration scripts only)

## Cross-Repository Dependencies

- order-service depends on user-service address events for checkout
- user-service publishes `AddressUpdated` consumed by order-service

## Constraints

- No breaking API changes without version bump
- All new endpoints require OpenAPI documentation
- Verification commands run from repository root (do not prefix with `cd <repo> &&`)

## Testing Requirements

- Unit tests for each service change
- Integration test for primary-address switch end-to-end (workspace-level if needed)

## Demo Flow

Steps to manually validate the feature after implementation:

1. Create user with two addresses
2. Set second address as primary
3. Place order and confirm shipping address matches primary
4. Edit primary address and confirm checkout reflects change

## Risks

- Event ordering between user-service and order-service may cause stale checkout state
- Cross-repo integration environment (Consul, MySQL, Redis) may be unavailable locally

## Non-Goals

- International address format validation beyond existing library
- Bulk address import

## Notes

### Agent mapping checklist

When filling `.feature/plans/draft/requirements.json` and `.feature/tasks/tasks.json`:

1. Copy each `<!-- req: Rxxx -->` ID into `requirements.json` with matching `id`, `description`, `source_section`, `source_ref`
2. Create **at least one task per requirement** (or per repo slice) with `requirement_ids`
3. Set `verification` per task — commands only, no redundant `cd repo &&` prefix
4. Run `feature-dev plan coverage --from PLAN.md --json` until all gates pass:
   - `completeness` — every PLAN bullet → requirement
   - `decomposition` — every requirement → task
   - `traceability` — every task `requirement_id` → known requirement
   - `acceptance_coverage` — every AC bullet → task
