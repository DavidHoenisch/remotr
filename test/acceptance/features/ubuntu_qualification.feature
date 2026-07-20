Feature: Ubuntu 24.04 applicator qualification composition

  @os_OS-AEC-095
  Scenario: The checked-in M1-M5 repository composes every intended contract safely
    Given the checked-in Ubuntu 24.04 M1-M5 qualification repository
    When the operator discovers validates and renders the Ubuntu qualification fleet
    Then the Ubuntu qualification composition preserves every expected contract and safe policy
    And repeated Ubuntu qualification render is deterministic and source-only
