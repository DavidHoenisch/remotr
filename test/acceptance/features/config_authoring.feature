@os_OS-TQG-006
Feature: Configuration authoring

  @os_OS-TQG-006
  Scenario: Invalid configuration is rejected
    Given an invalid configuration repository
    When the operator validates the repository
    Then validation is rejected

  @os_OS-TQG-006
  Scenario: Rendered output is deterministic
    Given the Compose configuration repository
    When the operator renders fleet "test-fleet" twice
    Then both rendered artifacts are identical

  @os_OS-AEC-004
  Scenario: Unknown canonical resource field is rejected precisely
    Given a canonical configuration with an unknown resource field
    When the operator validates the repository
    Then validation identifies resource "base/curl" and field "presnt"

  @os_OS-AEC-014
  Scenario: Cross-kind duplicate resource name is rejected
    Given a canonical configuration with a cross-kind duplicate name
    When the operator validates the repository
    Then validation rejects ambiguous resource "base/shared"

  @os_OS-AEC-016
  Scenario: Deferred provider is rejected by capability matrix
    Given a canonical configuration selecting deferred DNF
    When the operator validates the repository
    Then validation reports the RPM-family roadmap for resource "base/curl"

  @os_OS-AEC-005
  Scenario: Legacy repository tooling exposes canonical migration information
    Given a legacy configuration repository
    When the operator discovers validates and renders fleet "test-fleet"
    Then tooling reports resource kind "package" and capability "provider:package/apt"
    And validation emits the schema zero deprecation diagnostic
    And no composed artifacts are written to the source repository

  @os_OS-FOM-003 @os_OS-FOM-012 @os_OS-FOM-014 @os_OS-LIA-002 @os_OS-NFM-001 @os_OS-NFM-002
  Scenario: M1 advertised applicator fields are accepted and unsupported intent is rejected
    Given a canonical M1 applicator repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised M1 field
    Given a canonical user resource with an invalid shell field
    When the operator validates the repository
    Then validation identifies resource "base/alice" and field "shell"

  @os_OS-MSM-001 @os_OS-MSM-006
  Scenario: M3 host baseline is expressed without generic commands
    Given a canonical M3 host-baseline repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised M3 field

  @os_OS-ESM-002 @os_OS-ESM-005
  Scenario: Endpoint schedule backend fields are validated at the configuration seam
    Given a cron endpoint schedule with a systemd-only field
    When the operator validates the repository
    Then validation identifies resource "base/nightly-backup" and field "persistent"

  @os_OS-ESM-004 @os_OS-ESM-006
  Scenario: Cron endpoint schedules survive validation and canonical composition
    Given a canonical cron endpoint schedule repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised cron schedule field

  @os_OS-ESM-007
  Scenario: Systemd timer schedules survive validation and canonical composition
    Given a canonical systemd timer schedule repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised systemd timer field

  @os_OS-SRM-001
  Scenario: Provider-neutral services preserve supported systemd intent and reject unsupported masking
    Given a canonical provider-neutral systemd service repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised service field
    Given an OpenRC service requesting masked state
    When the operator validates the repository
    Then validation identifies resource "base/ssh" and field "masked"

  @os_OS-SRM-003 @os_OS-SRM-004
  Scenario: First-class systemd units and drop-ins survive canonical composition
    Given a canonical systemd unit and drop-in repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised systemd unit field

  @os_OS-SRM-008 @os_OS-SRM-010 @os_OS-SRM-011
  Scenario: Coordinated reboot intent survives validation and canonical composition
    Given a canonical coordinated reboot repository
    When the operator validates the repository
    Then validation is accepted
    And rendering preserves every advertised reboot field
