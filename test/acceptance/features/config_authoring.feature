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
