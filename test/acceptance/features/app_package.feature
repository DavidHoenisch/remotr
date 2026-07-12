@os_OS-TQG-006
Feature: Application package catalog

  @os_OS-TQG-006
  Scenario: An operator lists the seeded package catalog
    Given an authenticated operator
    When the operator lists application packages
    Then the seeded "e2e/test-cli" version "1.0.0" package is visible
