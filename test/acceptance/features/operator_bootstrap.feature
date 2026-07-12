@os_OS-TQG-006
Feature: Operator bootstrap

  @os_OS-TQG-006
  Scenario: Bootstrap credentials can list endpoints only once
    Given an available one-time operator bootstrap token
    When the operator bootstraps credentials
    Then endpoint listing succeeds with those credentials
    When the operator reuses the bootstrap token
    Then bootstrap is rejected
