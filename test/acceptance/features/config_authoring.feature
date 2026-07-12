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
