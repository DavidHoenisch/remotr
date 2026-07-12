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
